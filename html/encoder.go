package html

import (
	"io"

	"znkr.io/markst/smartquote"
	"znkr.io/markst/value"
)

// Attr is one HTML attribute. An Attr with an empty Name is dropped, which is
// what makes an optional attribute a value rather than a branch.
type Attr struct {
	Name  string
	Value string
}

// Encoder writes the HTML a render produces. A [WithElement] hook is handed one
// and writes its own markup through it.
//
// It is also an [io.Writer], and bytes written that way go out verbatim, which
// is how an html/template fragment lands in the document.
//
// Like a [bufio.Writer], an Encoder records the first failure and does nothing
// afterwards, so a hook can write a run of markup and check once at the end.
// [Encoder.Err] reports it, and [Render] returns it.
type Encoder struct {
	w     stringWriter
	cfg   *config
	notes *notes
	err   error

	// last is the byte most recently written, 0 before anything has been. It
	// is used for one thing: to tell whether a block just ended, and so
	// whether a separator is needed.
	last byte

	q smartquote.Quoter
	// quote holds what the quoter made of the element being rendered: the glyph
	// for a smart quote, and "" for everything else.
	quote string
	// mathStretch reports whether the delimiters of the innermost delimited
	// group stretch, so that a mid inside it can match them.
	mathStretch bool

	// mathBlock reports whether the equation being rendered is a block one.
	// Attachments on an operator go above and below it only there; inline they
	// would push the line apart.
	mathBlock bool
}

// Err returns the first failure, or nil while there has been none.
func (e *Encoder) Err() error { return e.err }

// fail records err as the first failure, if there was none before and err is
// not nil.
func (e *Encoder) fail(err error) {
	if e.err == nil && err != nil {
		e.err = err
	}
}

// Write writes p verbatim, so an [Encoder] can be handed to anything that
// writes HTML of its own, an html/template above all.
func (e *Encoder) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	n, err := e.w.Write(p)
	if n > 0 {
		e.last = p[n-1]
	}
	e.fail(err)
	return n, err
}

// HTML writes s verbatim. Whatever it contains reaches the output as markup,
// so the caller is the one who must know it is safe there.
func (e *Encoder) HTML(s string) {
	if e.err != nil || s == "" {
		return
	}
	_, err := e.w.WriteString(s)
	e.last = s[len(s)-1]
	e.fail(err)
}

// Text writes s as text, escaped so that it reads as itself.
func (e *Encoder) Text(s string) { e.escape(s, false) }

// Start writes an opening tag with attrs, in the order given. An Attr with an
// empty Name is skipped.
func (e *Encoder) Start(tag string, attrs ...Attr) {
	e.HTML("<")
	e.HTML(tag)
	for _, a := range attrs {
		if a.Name == "" {
			continue
		}
		e.HTML(" ")
		e.HTML(a.Name)
		e.HTML(`="`)
		e.escape(a.Value, true)
		e.HTML(`"`)
	}
	e.HTML(">")
}

// End writes a closing tag.
func (e *Encoder) End(tag string) {
	e.HTML("</")
	e.HTML(tag)
	e.HTML(">")
}

// Newline writes the separator that follows a block-level element.
func (e *Encoder) Newline() { e.HTML("\n") }

// Content renders c, hooks and all: each [WithElement] hook is offered c in
// turn, and the built-in rendering runs when none of them took it. This is how
// a hook descends into the content it was handed.
func (e *Encoder) Content(c value.Content) {
	if e.err != nil || c == nil {
		return
	}
	e.quote = e.q.Advance(c)
	for _, h := range e.cfg.hooks {
		handled, err := h(e, c)
		if err != nil {
			e.fail(err)
			return
		}
		if handled {
			return
		}
	}
	e.render(c)
}

// Default renders c the built-in way, skipping the hooks for c itself but not
// for what is nested in it. A hook calls it on the element it was given, to
// decorate that element rather than replace it.
func (e *Encoder) Default(c value.Content) {
	if e.err != nil || c == nil {
		return
	}
	e.render(c)
}

// escape writes s with the characters that would otherwise be markup replaced
// by their entities. In an attribute value the quote closing it counts; in text
// it does not, and leaving it alone keeps the output readable.
func (e *Encoder) escape(s string, attr bool) {
	if e.err != nil {
		return
	}
	last := 0
	for i := 0; i < len(s); i++ {
		var repl string
		switch s[i] {
		case '&':
			repl = "&amp;"
		case '<':
			repl = "&lt;"
		case '>':
			repl = "&gt;"
		case '"':
			if !attr {
				continue
			}
			repl = "&quot;"
		default:
			continue
		}
		e.HTML(s[last:i])
		e.HTML(repl)
		last = i + 1
	}
	e.HTML(s[last:])
}

// atLineStart reports whether the last thing written ended a line, which is
// what a block element does.
func (e *Encoder) atLineStart() bool { return e.last == '\n' || e.last == 0 }

// idAttr returns the id an element's label gives it, or the empty Attr when it
// carries none. A label is how the rest of the document points at an element,
// which in HTML is the id.
func idAttr(c value.Content) Attr {
	l := c.GetLabel()
	if l == nil {
		return Attr{}
	}
	return Attr{"id", l.Name.String()}
}

var _ io.Writer = (*Encoder)(nil)
