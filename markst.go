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
}

// WithName sets the display name diagnostics about this source are reported
// under — the file it was read from, conventionally. Without it a diagnostic
// still says which line and column it is about, just not which file.
//
// [CompileLibrary] takes its name as an argument instead: a library's failures
// surface while some other file is being compiled, where an unnamed location
// would be unreadable.
func WithName(name string) Option {
	return func(c *config) { c.name = name }
}

// WithLibrary makes every binding of each library resolve as if it were part
// of the built-in universe, so a document can use what a library defines
// without importing anything.
//
// Precedence runs outward: a document's own `#let` shadows a library binding,
// a library binding shadows a built-in, and a later library shadows an earlier
// one.
func WithLibrary(libs ...*Library) Option {
	return func(c *config) {
		for _, lib := range libs {
			for n, v := range lib.exports {
				c.bind(n, v)
			}
		}
	}
}

// WithBindings does for host-defined Go values what [WithLibrary] does for
// markst-defined ones: each name resolves as if it were a member of the
// built-in universe. Values must be non-nil; pass a [value.Error] for
// "declared, but using it must fail".
func WithBindings(bindings map[name.Name]value.Value) Option {
	return func(c *config) {
		for n, v := range bindings {
			c.bind(n, v)
		}
	}
}

// WithNow fixes the instant the document is rendered at — what
// `datetime.today` reads the current date off. Without it the system clock is
// used; passing a fixed instant makes a render reproducible.
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
	if c.now.IsZero() {
		return nil
	}
	return []eval.Option{eval.WithNow(c.now)}
}

// Compile turns markst source into a realized document, running the whole
// pipeline over it: parse, analyze, evaluate, realize. Turning the result into
// output — HTML, or anything else — is the presenter's job; see
// [znkr.io/markst/smartquote] for the one part of that a presenter cannot do
// on its own.
//
// Every heading in the returned document carries a [value.Label], so there is
// always an anchor to link a section by. A heading the source left unlabelled
// gets one derived from its text — after show rules, so it describes the
// heading a reader sees — marked [value.Label.Auto] and made unique against
// every label the document already uses. Only the labels the source wrote are
// part of the document's namespace: `@ref` does not resolve a generated one.
//
// Warnings are returned separately from err, because they describe a document
// that compiled: a label used twice, content discarded where it can have no
// effect. A caller that folded them into failure would reject documents that
// are fine. err is a [DiagnosticList] when it is non-nil, so the individual
// diagnostics are reachable through errors.As or its Unwrap, and its Error
// method names all of them rather than only the first.
//
// Every returned [Diagnostic] carries a resolved [syntax.Location] — byte
// offsets and the line/column they correspond to — and the name of the source
// it belongs to, so a caller holding only these return values can report where
// a problem is without going back to the source bytes. That matters once
// [WithLibrary] is in play: a failure inside a library function is reported
// against the library's own text, with the call site that reached it.
// [FormatDiagnostics] renders them.
func Compile(src []byte, opts ...Option) (*value.Document, []Diagnostic, error) {
	c := newConfig(opts)
	root := parser.Parse(src)
	mod := analyzer.Analyze(root, c.analyzerOpts()...)
	doc, warnings, errs := eval.Eval(mod, c.evalOpts()...)
	warns := diagnose(Warning, warnings)
	if len(errs) > 0 {
		return doc, warns, DiagnosticList(diagnose(Error, errs))
	}
	return doc, warns, nil
}

// Library is a compiled markst file held for the bindings it defines rather
// than the document it produces — the way a set of helpers written in markst
// is shared between documents.
//
// Compile it once and hand it to as many [Compile] calls as you like via
// [WithLibrary]. The functions it defines are ordinary values: they carry the
// library along for their own code and source positions, but run against
// whichever document is being compiled, so what they do — content, `set`
// rules, labels, failures — belongs to that document.
type Library struct {
	name    string
	exports map[name.Name]value.Value
}

// Name returns the library's display name.
func (l *Library) Name() string { return l.name }

// Lookup returns the value the library binds to n.
func (l *Library) Lookup(n name.Name) (value.Value, bool) {
	v, ok := l.exports[n]
	return v, ok
}

// CompileLibrary compiles src for its top-level bindings. name is the
// library's display name, used to locate every diagnostic that arises in it —
// both here, and later, when a document calls one of its functions.
//
// Every top-level binding is exported; there is no export list. Names bound
// inside a block are local to it and never escape.
//
// A library's markup body is discarded — a library is compiled for what it
// defines, not what it renders — so a body that is not empty is reported as a
// warning. Errors and warnings are returned on the same terms as [Compile],
// and a library that failed to compile is not returned.
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
// is a function of its own only so that the name package stays reachable —
// CompileLibrary's `name` parameter shadows it.
func newLibrary(libname string, exports *value.Dict) *Library {
	lib := &Library{name: libname, exports: make(map[name.Name]value.Value, exports.Elems.Len())}
	for k, v := range exports.Elems.All() {
		lib.exports[name.Make(string(k))] = v
	}
	return lib
}

// isEmptyBody reports whether a library's markup body renders nothing, so that
// only a library that actually tried to produce output is warned about.
//
// Whitespace does not count. A file of nothing but `let` bindings still has
// blank lines between them, and those lower to parbreaks and spaces — warning
// about those would fire on every library there is.
func isEmptyBody(body value.Value) bool {
	if body == nil {
		return true
	}
	c := value.ToContent(body)
	if c == nil {
		return true
	}
	for n := range value.All(c) {
		switch n := n.(type) {
		case *value.Sequence, *value.Parbreak, *value.Linebreak:
			// Structure and blank lines, carrying nothing of their own.
		case *value.Text:
			if strings.TrimSpace(n.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// Query returns the value carried by the [value.Metadata] element in doc
// labelled label — the way a document hands data to the program presenting it:
//
//	#metadata("2024-02-29") <published>
//
// Metadata without a label is never returned: the label is what identifies it.
// A label shared by two metadata elements is reported as a warning by
// [Compile]; Query answers with the first in document order.
func Query(doc *value.Document, label name.Name) (value.Value, bool) {
	if doc == nil || doc.Body == nil {
		return nil, false
	}
	for c := range value.All(doc.Body) {
		m, ok := c.(*value.Metadata)
		if !ok || m.Label == nil || m.Label.Name != label {
			continue
		}
		return m.Value, true
	}
	return nil, false
}
