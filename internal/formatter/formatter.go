package formatter

import (
	"fmt"
	"strings"

	"znkr.io/writst/syntax"
)

type Formattable interface {
	Format(f *Formatter)
}

// Formatter provides a simple DSL for formatting IR nodes.
type Formatter struct {
	sb         *strings.Builder
	mode       syntax.Mode
	indent     int
	inlineCode bool // true when formatting inline code like { expr }
	// forceMultiline requests that the next content block ([BracketedList])
	// use its multi-line layout even if it would otherwise fit inline. It is
	// consumed (reset) by that call so it does not propagate to nested content.
	// [FuncCall] sets it across a run of trailing blocks so a short block never
	// sits inline next to a multi-line sibling.
	forceMultiline bool
}

func New(mode syntax.Mode) *Formatter {
	return &Formatter{sb: new(strings.Builder), mode: mode}
}

func (f *Formatter) Len() int       { return f.sb.Len() }
func (f *Formatter) String() string { return f.sb.String() }

func (f *Formatter) Mode() syntax.Mode        { return f.mode }
func (f *Formatter) SetMode(mode syntax.Mode) { f.mode = mode }

func (f *Formatter) IncreaseIndent() { f.indent++ }
func (f *Formatter) DecreaseIndent() { f.indent-- }

func (f *Formatter) Inline(fn func(f *Formatter)) {
	prev := f.inlineCode
	f.inlineCode = true
	fn(f)
	f.inlineCode = prev
}

func (f *Formatter) Str(s string)                   { f.sb.WriteString(s) }
func (f *Formatter) Linebreak()                     { f.Str("\n"); f.Indent() }
func (f *Formatter) Indent()                        { f.Str(strings.Repeat("  ", f.indent)) }
func (f *Formatter) Print(v any)                    { fmt.Fprint(f.sb, v) }
func (f *Formatter) Printf(format string, a ...any) { fmt.Fprintf(f.sb, format, a...) }

// Prefix writes # when in markup mode (transitioning into code)
func (f *Formatter) Prefix() {
	if f.mode == syntax.ModeMarkup {
		f.Str("#")
	}
}

// Keyword writes a keyword followed by a space
func (f *Formatter) Keyword(s string) {
	f.Str(s)
	f.Str(" ")
}

type Arg struct {
	name       string
	positional bool
	value      any
}

func NamedArg(name string, v any) Arg { return Arg{name: name, value: v} }
func PositionalArg(v any) Arg         { return Arg{positional: true, value: v} }

// format writes the argument (with its `name: ` prefix, if named) to f.
func (a Arg) format(f *Formatter) {
	if !a.positional {
		f.Str(a.name)
		f.Str(": ")
	}
	switch v := a.value.(type) {
	case Formattable:
		v.Format(f)
	case string:
		f.Printf("%q", v)
	default:
		f.Print(v)
	}
}

// maxInlineWidth is the column budget for keeping a call, argument list, or
// content block on a single line. Anything whose single-line rendering would
// exceed it (accounting for the current indent) is broken across lines instead.
const maxInlineWidth = 40

// FuncCall formats #name(args)[blocks...] for content nodes
func (f *Formatter) FuncCall(name string, args []Arg, blocks ...Formattable) {
	f.Prefix()
	f.Str(name)
	if len(args) > 0 {
		// Pre-render each argument at the nested indent. Layout is either fully
		// inline or fully broken (one argument per line): a call goes multi-line
		// when any argument is itself multi-line, or when the single-line form
		// would overflow the width budget.
		rendered := make([]string, len(args))
		multiline := false
		for i, a := range args {
			sub := &Formatter{sb: new(strings.Builder), mode: f.mode, indent: f.indent + 1, inlineCode: f.inlineCode}
			a.format(sub)
			rendered[i] = sub.String()
			if strings.Contains(rendered[i], "\n") {
				multiline = true
			}
		}
		joined := strings.Join(rendered, ", ")
		// A single argument stays inline regardless of length (breaking one
		// argument onto its own line buys nothing); a multi-argument call goes
		// inline only when its single-line form fits the width budget.
		inline := !multiline && (len(args) == 1 ||
			f.indent*2+len(name)+len(joined)+3 <= maxInlineWidth)
		f.Str("(")
		if inline {
			f.Str(joined)
		} else {
			f.indent++
			for _, s := range rendered {
				f.Linebreak()
				f.Str(s)
				f.Str(",")
			}
			f.indent--
			f.Linebreak()
		}
		f.Str(")")
	}
	if len(blocks) > 0 {
		// Trailing content blocks must stay adjacent (`][`) to remain a single
		// call, so they cannot be separated onto their own lines. To avoid a
		// multi-line block sitting next to an inline one, render them all
		// multi-line as soon as any single one breaks.
		anyMultiline := false
		for _, block := range blocks {
			sub := &Formatter{sb: new(strings.Builder), mode: f.mode, indent: f.indent, inlineCode: f.inlineCode}
			block.Format(sub)
			if strings.Contains(sub.String(), "\n") {
				anyMultiline = true
				break
			}
		}
		for _, block := range blocks {
			f.forceMultiline = anyMultiline
			block.Format(f)
		}
		f.forceMultiline = false
	}
	if len(args) == 0 && len(blocks) == 0 {
		f.Str("()")
	}
}

// BracketedList renders a content block `[items…]`. The items are concatenated
// inline when the result fits the width budget and has no line breaks;
// otherwise each item is placed on its own indented line. Use it for content
// whose parts are worth breaking apart when long (e.g. an equation body),
// unlike ordinary inline runs which stay concatenated via Bracketed.
func (f *Formatter) BracketedList(items []Formattable) {
	// Consume any force-multi-line request so it applies to this block only,
	// not to content nested inside it.
	force := f.forceMultiline
	f.forceMultiline = false

	sub := &Formatter{sb: new(strings.Builder), mode: f.mode, indent: f.indent + 1, inlineCode: f.inlineCode}
	for _, it := range items {
		it.Format(sub)
	}
	s := sub.String()
	if !force && !strings.Contains(s, "\n") && f.indent*2+len(s)+2 <= maxInlineWidth {
		f.Str("[")
		f.Str(s)
		f.Str("]")
		return
	}
	f.Str("[")
	f.indent++
	for _, it := range items {
		f.Linebreak()
		it.Format(f)
	}
	f.indent--
	f.Linebreak()
	f.Str("]")
}
