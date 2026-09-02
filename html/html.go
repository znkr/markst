// Package html renders a realized markst document as HTML.
//
// [Render] writes an HTML *fragment*: the document's content, with no
// <html>, <head>, or <body> around it. Wrapping it in a page is the caller's
// job, because the title, the stylesheet, and the rest of the page belong to
// whatever is publishing the document, not to the document.
//
//	err := html.Render(os.Stdout, doc)
//
// # Extending it
//
// Some of what a document names, markst cannot resolve on its own: where an
// image path points, what a [value.Custom] element means, whether raw text
// should be syntax-highlighted. [WithElement] is the way in. A hook is
// consulted before the built-in rendering of every element and says whether it
// handled it:
//
//	html.Render(w, doc, html.WithElement(
//		func(e *html.Encoder, c value.Content) (bool, error) {
//			raw, ok := c.(*value.Raw)
//			if !ok {
//				return false, nil // not mine; render it the usual way
//			}
//			e.HTML(highlight(raw.Text, raw.Lang))
//			return true, nil
//		}))
//
// Hooks compose: each is tried in the order it was added, so two extensions
// that care about different elements can be passed together. Inside a hook,
// [Encoder.Content] renders nested content through the same pipeline and
// [Encoder.Default] renders one element the built-in way, which is how a hook
// decorates an element rather than replacing it.
//
// # URLs
//
// An #image path, a #link destination, and the anchor an @ref resolves to are
// escaped and otherwise written as the document wrote them — a javascript:
// URL included. A markst document is authored content, and a renderer that
// quietly rewrote what it said would be lying about it. [WithLinkURL] and its
// siblings are the place to impose a policy on input that is not trusted.
//
// # Whitespace
//
// Output is not indented. A newline follows each block-level element, and
// nothing else is added, so the markup stays diffable without any of it being
// significant.
package html

import (
	"bufio"
	"io"

	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// Option configures [Render].
type Option func(*config)

type config struct {
	hooks          []Element
	headingLevel   int
	imageURL       func(string) (string, error)
	linkURL        func(string) (string, error)
	labelURL       func(name.Name) (string, error)
	footnotes      []*value.Footnote
	noFootnoteList bool
}

// Element renders one content element. It reports whether it handled c; when
// it did not, the next hook is tried, and finally the built-in rendering.
type Element func(e *Encoder, c value.Content) (handled bool, err error)

// WithElement adds a rendering hook, consulted before the built-in rendering of
// every element. Hooks are tried in the order they were added, so two
// extensions that handle different elements can be passed side by side.
func WithElement(f Element) Option {
	return func(c *config) { c.hooks = append(c.hooks, f) }
}

// WithHeadingLevel sets the HTML heading level a top-level markst heading gets.
// The default is 1. A page whose <h1> is its title passes 2, so that `= Intro`
// becomes <h2>. Levels past <h6> are clamped, HTML having no more.
func WithHeadingLevel(n int) Option {
	return func(c *config) { c.headingLevel = n }
}

// WithImageURL rewrites an #image path into the URL it is served from.
func WithImageURL(f func(path string) (string, error)) Option {
	return func(c *config) { c.imageURL = f }
}

// WithLinkURL rewrites a #link destination into the URL it points at.
func WithLinkURL(f func(dest string) (string, error)) Option {
	return func(c *config) { c.linkURL = f }
}

// WithLabelURL rewrites the label an @ref names into the URL it links to. The
// default is a fragment link to an element in this document, "#" + the label.
// A reference to a footnote never reaches it: that one repeats the footnote's
// number instead of linking to the citation.
func WithLabelURL(f func(l name.Name) (string, error)) Option {
	return func(c *config) { c.labelURL = f }
}

// WithoutFootnoteList renders footnote citations but not the list of endnotes
// they point at — for a fragment whose footnotes belong to a document that is
// rendered somewhere else.
func WithoutFootnoteList() Option {
	return func(c *config) { c.noFootnoteList = true }
}

// WithFootnotes fixes the numbering, rather than taking it from the content
// being rendered. Pass what [Footnotes] returned for the whole document to
// render a piece of it — a heading in a table of contents, say — whose
// citations carry the numbers they have in the document rather than starting
// again from 1. Pair it with [WithoutFootnoteList], or the piece gets the
// whole document's endnotes appended to it.
func WithFootnotes(notes []*value.Footnote) Option {
	return func(c *config) { c.footnotes = notes }
}

func newConfig(opts []Option) *config {
	c := &config{headingLevel: 1}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Render writes c to w as an HTML fragment.
//
// Pass the [value.Document] that [znkr.io/markst.Compile] returned to render a
// whole document, or any content to render a piece of one — the summary a
// document carries in its metadata, say. Either way the footnotes found in
// what was passed are numbered from 1 and listed at the end; see
// [WithoutFootnoteList].
//
// The first failure stops the render. There are three: a hook that returned an
// error, a write to w that did not succeed, and a [value.Custom] element with
// no hook to render it — markst carries a host's element through the pipeline
// but has no idea what it means, so there is nothing honest to write for one.
// Every element markst does define has an HTML form and cannot fail.
func Render(w io.Writer, c value.Content, opts ...Option) error {
	cfg := newConfig(opts)

	var flush func() error
	sw, ok := w.(stringWriter)
	if !ok {
		bw := bufio.NewWriter(w)
		sw, flush = bw, bw.Flush
	}

	notes := cfg.footnotes
	if notes == nil {
		notes = collect(c)
	}

	e := &Encoder{w: sw, cfg: cfg, notes: index(notes)}
	e.Content(c)
	if !cfg.noFootnoteList {
		e.footnoteList()
	}
	if flush != nil {
		e.fail(flush())
	}
	return e.err
}

// stringWriter is the writer the encoder wants: one that takes a string
// without turning it into a fresh []byte first. bytes.Buffer, strings.Builder,
// and bufio.Writer all qualify, so the common cases are never double-buffered.
type stringWriter interface {
	io.Writer
	io.StringWriter
}
