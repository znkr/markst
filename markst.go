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

// Package markst compiles Markst markup to a document tree, which
// [znkr.io/markst/html] renders as HTML.
//
// [Compile] runs the whole pipeline over a source file. [CompileLibrary] does
// the same for a file compiled for the bindings it defines rather than the
// document it produces. [Outline] and [Query] read a compiled document.
package markst

import (
	"strings"
	"time"

	"znkr.io/markst/eval"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
	"znkr.io/markst/value"
)

// Option configures [Compile] and [CompileLibrary].
type Option func(*config)

type config struct {
	name     string
	bindings map[name.Name]value.Value
	now      time.Time
	index    *value.Index
}

// WithName sets the name diagnostics about this source are reported under,
// conventionally the file it was read from. Without it a diagnostic still says
// which line and column it is about, just not which file.
//
// [CompileLibrary] takes its name as an argument instead, since a library's
// failures surface while some other file is being compiled.
func WithName(name string) Option {
	return func(c *config) { c.name = name }
}

// WithLibrary makes each library's bindings resolve as if they were built in,
// so a document can use what a library defines without importing anything.
//
// A document's own `#let` shadows a library binding, a library binding shadows
// a built-in, and a later library shadows an earlier one.
func WithLibrary(libs ...*Library) Option {
	return func(c *config) {
		for _, lib := range libs {
			for n, v := range lib.exports {
				c.bind(n, v)
			}
		}
	}
}

// WithBindings makes host-defined Go values resolve as if they were built in,
// as [WithLibrary] does for values defined in markst. Values must be non-nil;
// to declare a name whose use must fail, bind a [value.Error].
func WithBindings(bindings map[name.Name]value.Value) Option {
	return func(c *config) {
		for n, v := range bindings {
			c.bind(n, v)
		}
	}
}

// WithIndex fills x with a [value.Index] of the compiled document, a flat
// record of its content that makes repeated traversals cheaper. Pass it to
// [Outline] or [znkr.io/markst/html.WithIndex] instead of traversing the
// document again:
//
//	var idx value.Index
//	doc, warns, err := markst.Compile(src, markst.WithIndex(&idx))
//	toc := markst.Outline(&idx)
//	err = html.Render(w, doc, html.WithIndex(&idx))
//
// Compile writes x whether or not the compile succeeded, replacing anything
// already there. The index becomes invalid as soon as the document's content
// changes; see [value.Index]. [CompileLibrary] ignores it, since a library
// produces no document to index.
func WithIndex(x *value.Index) Option {
	return func(c *config) { c.index = x }
}

// WithNow fixes the instant the document is compiled at, which is what
// `datetime.today` reads. Without it the system clock is used, so passing a
// fixed instant is what makes a compile reproducible.
func WithNow(t time.Time) Option {
	return func(c *config) { c.now = t }
}

func (c *config) bind(n name.Name, v value.Value) {
	if c.bindings == nil {
		c.bindings = make(map[name.Name]value.Value)
	}
	c.bindings[n] = v
}

func newConfig(opts []Option) *config {
	c := &config{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *config) analyzerOpts() []analyzer.Option {
	opts := []analyzer.Option{analyzer.WithName(c.name)}
	if c.bindings != nil {
		opts = append(opts, analyzer.WithBindings(c.bindings))
	}
	return opts
}

func (c *config) evalOpts() []eval.Option {
	var opts []eval.Option
	if !c.now.IsZero() {
		opts = append(opts, eval.WithNow(c.now))
	}
	return opts
}

// Compile turns markst source into a realized document. Rendering it is a
// separate step. [znkr.io/markst/html] writes HTML; a presenter for any other
// format traverses the document with [value.Preorder], using
// [znkr.io/markst/smartquote] to resolve quotation marks.
//
// Every heading in the returned document has a [value.Label], so a section can
// always be linked to, and [Outline] returns them as a tree. A heading left
// unlabeled by the source gets a label derived from its text, taken after show
// rules have run so that it matches the heading a reader sees. Such a label is
// marked [value.Label.Auto] and made unique against the labels already in the
// document, but it is not part of the document's namespace: `@ref` resolves
// only labels the source wrote. A reference can name a label anywhere in the
// document, so `@conclusion` works in an introduction.
//
// Warnings are returned separately from err because they describe a document
// that did compile, such as a label used twice or content discarded where it
// has no effect. A non-nil err is a [DiagnosticList], so its Error method
// reports every diagnostic and errors.As can reach an individual one.
//
// Each [Diagnostic] carries a resolved [syntax.Location] and the name of the
// source it came from, so a caller can report the position without the source
// bytes. This matters with [WithLibrary]: a failure inside a library function
// is reported against the library's own text, together with the call site that
// reached it. [FormatDiagnostics] renders them.
func Compile(src []byte, opts ...Option) (*value.Document, []Diagnostic, error) {
	c := newConfig(opts)
	root := parser.Parse(src)
	mod := analyzer.Analyze(root, c.analyzerOpts()...)

	evalOpts := c.evalOpts()
	if c.index != nil {
		evalOpts = append(evalOpts, eval.WithIndex(c.index))
	}

	doc, warnings, errs := eval.Eval(mod, evalOpts...)
	warns := diagnose(Warning, warnings)
	if len(errs) > 0 {
		return doc, warns, DiagnosticList(diagnose(Error, errs))
	}
	return doc, warns, nil
}

// Library is a compiled markst file kept for the bindings it defines rather
// than the document it produces. It is how helpers written in markst are shared
// between documents.
//
// Compile it once and pass it to any number of [Compile] calls through
// [WithLibrary]. The functions it defines are ordinary values, and each runs
// against whichever document is being compiled, so the content, rules, labels
// and errors it produces belong to that document rather than to the library.
type Library struct {
	name    string
	exports map[name.Name]value.Value
}

// Name returns the library's display name.
func (l *Library) Name() string { return l.name }

// Lookup returns the value the library binds to n, and reports whether it
// binds n at all.
func (l *Library) Lookup(n name.Name) (value.Value, bool) {
	v, ok := l.exports[n]
	return v, ok
}

// CompileLibrary compiles src for its top-level bindings. name locates every
// diagnostic arising in the library, both here and later, when a document
// calls one of its functions.
//
// Every top-level binding is exported; there is no export list. Names bound
// inside a block stay local to it.
//
// A library's markup body is discarded, so a body that is not empty is reported
// as a warning. Errors and warnings are returned on the same terms as
// [Compile], and a library that failed to compile is not returned.
func CompileLibrary(name string, src []byte, opts ...Option) (*Library, []Diagnostic, error) {
	c := newConfig(append(opts, WithName(name)))
	root := parser.Parse(src)
	mod := analyzer.Analyze(root, append(c.analyzerOpts(), analyzer.WithExports())...)
	body, exports, warnings, errs := eval.EvalExports(mod, c.evalOpts()...)
	warns := diagnose(Warning, warnings)
	if len(errs) > 0 {
		return nil, warns, DiagnosticList(diagnose(Error, errs))
	}
	if !isEmptyBody(body) {
		warns = append(warns, Diagnostic{
			Severity: Warning,
			Origin:   name,
			Msg:      "content in a library is discarded",
			Hints:    []Hint{{Msg: "a library is compiled for the bindings it defines; move the content into a document"}},
		})
	}

	return newLibrary(name, exports), warns, nil
}

// newLibrary interns the export dict's string keys back into name handles. It
// is a separate function only so the name package stays reachable:
// CompileLibrary's `name` parameter shadows it.
func newLibrary(libname string, exports *value.Dict) *Library {
	lib := &Library{name: libname, exports: make(map[name.Name]value.Value, exports.Elems.Len())}
	for k, v := range exports.Elems.All() {
		lib.exports[name.Make(string(k))] = v
	}
	return lib
}

// isEmptyBody reports whether a library's markup body renders nothing, so that
// only a library that did try to produce output is warned about.
//
// Whitespace does not count. A file of only `let` bindings still has blank
// lines between them, which lower to parbreaks and spaces; counting those would
// warn about every library.
// rendersSomething is every element except the three that produce no output of
// their own: a sequence is structure, and a parbreak or linebreak between two
// `let` bindings separates nothing. Leaving them out stops them being yielded,
// not descended into.
var rendersSomething = value.AnyKind.Remove(
	value.KindSequence,
	value.KindParbreak,
	value.KindLinebreak,
)

func isEmptyBody(body value.Value) bool {
	if body == nil {
		return true
	}
	c := value.ToContent(body)
	if c == nil {
		return true
	}
	for cur := range value.Preorder(c, rendersSomething) {
		if t, ok := cur.Node().(*value.Text); ok && strings.TrimSpace(t.Text) == "" {
			continue
		}
		return false
	}
	return true
}

// Query returns the value of the [value.Metadata] element carrying the given
// label. It is how a document passes data to the program presenting it:
//
//	#metadata("2024-02-29") <published>
//
// Pass the [value.Document] [Compile] returned, or a [value.Index] of one. Each
// query traverses what it is given, so an index is worth building when a host
// reads several fields.
//
// Unlabeled metadata is never returned, since the label identifies it. A label
// on two metadata elements is reported as a warning by [Compile]; Query returns
// the first in document order.
func Query(t value.Tree, label name.Name) (value.Value, bool) {
	if t == nil {
		return nil, false
	}
	for c := range t.Preorder(value.SetOf(value.KindMetadata)) {
		m := c.Node().(*value.Metadata)
		if m.Label == nil || m.Label.Name != label {
			continue
		}
		return m.Value, true
	}
	return nil, false
}
