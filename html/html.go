// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package html renders a realized markst document as HTML.
//
// [Render] writes an HTML fragment: the document's content, with no
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
// should be syntax-highlighted. [WithElement] adds a hook for these. It is
// consulted before the built-in rendering of every element and reports whether
// it handled the element:
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
// handling different elements can be passed together. Inside a hook,
// [Encoder.Content] renders nested content through the same pipeline, and
// [Encoder.Default] renders one element the built-in way, which lets a hook
// decorate an element rather than replace it.
//
// # URLs
//
// An #image path, a #link destination, and the anchor an @ref resolves to are
// escaped, but otherwise written as the document wrote them, a javascript: URL
// included. A markst document is authored content, so the renderer does not
// second-guess it. To impose a policy on input you do not trust, use
// [WithLinkURL] and its siblings.
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
	index          *value.Index
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

// WithFootnotes fixes the numbering instead of taking it from the content
// being rendered. To render a piece of a document — a heading in a table of
// contents, say — with the numbers that piece has in the whole document, pass
// what [Footnotes] returned for the document. Pair it with
// [WithoutFootnoteList], or the piece gets the whole document's endnotes.
func WithFootnotes(notes []*value.Footnote) Option {
	return func(c *config) { c.footnotes = notes }
}

// WithIndex hands the renderer a [value.Index] of the content being rendered,
// which it reads the footnotes off instead of walking that content to find
// them. Build one with [znkr.io/markst.WithIndex] and pass the same index to
// every render of that document.
//
// The index must be of the content passed to [Render]: it decides which
// footnotes are in what is being rendered, so an index of the whole document
// would number a fragment's citations as if the whole document were there. To
// render a piece of a document, fix the numbering with [WithFootnotes] instead.
// The index is ignored when that option is set.
func WithIndex(x *value.Index) Option {
	return func(c *config) { c.index = x }
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
// The first failure stops the render. Three things can fail: a hook returning
// an error, a write to w, and a [value.Custom] element with no hook to render
// it, since markst has no rendering for a host's own element. Every element
// markst defines has an HTML form and cannot fail.
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
		notes = collect(cfg.index, c)
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

// stringWriter is a writer that accepts a string without converting it to a
// fresh []byte first. bytes.Buffer, strings.Builder, and bufio.Writer all
// satisfy it, so the common cases are never double-buffered.
type stringWriter interface {
	io.Writer
	io.StringWriter
}
