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

package markst_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"znkr.io/markst"
	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// articleLib is the shape this feature exists for: a helper written in markst,
// compiled once, and used by documents that never mention it.
const articleLib = `#let article(title: none, published: none, summary: none) = {
    if type(published) == str {
        published = datetime.parse_date(published)
    }
    set document(title: title, date: published)
    [
        #metadata((
            title: title,
            published: published,
            summary: summary,
        )) <doc-meta>
    ]
}
`

// compileLibrary compiles src and fails the test if it did not compile,
// returning the library and any warnings.
func compileLibrary(t *testing.T, name string, src string) (*markst.Library, []markst.Diagnostic) {
	t.Helper()
	lib, warns, err := markst.CompileLibrary(t.Context(), name, []byte(src))
	if err != nil {
		t.Fatalf("CompileLibrary(%q) = %v", name, err)
	}
	return lib, warns
}

// compile compiles src and fails the test if it did not compile.
func compile(t *testing.T, src string, opts ...markst.Option) *value.Document {
	t.Helper()
	doc, warns, err := markst.Compile(t.Context(), []byte(src), opts...)
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warns) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warns)
	}
	return doc
}

// diagnostics compiles src expecting failure and returns the error list.
func diagnostics(t *testing.T, src string, opts ...markst.Option) markst.DiagnosticList {
	t.Helper()
	_, _, err := markst.Compile(t.Context(), []byte(src), opts...)
	var diags markst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T (%v), want markst.DiagnosticList", err, err)
	}
	return diags
}

// TestLibraryFunctionNeedsNoImport is the headline case: a document uses a
// function it never imported, and the metadata that function emits comes back
// out through Query.
func TestLibraryFunctionNeedsNoImport(t *testing.T) {
	lib, warns := compileLibrary(t, "lib.mst", articleLib)
	if len(warns) > 0 {
		t.Errorf("CompileLibrary() warnings = %v, want none", warns)
	}
	if _, ok := lib.Lookup(name.Make("article")); !ok {
		t.Fatalf("Lookup(article) not found; library exports nothing usable")
	}
	if lib.Name() != "lib.mst" {
		t.Errorf("Name() = %q, want %q", lib.Name(), "lib.mst")
	}

	doc := compile(t, `#article(title: "Routing", published: "2024-07-06")

= Background

Text.
`, markst.WithName("index.mst"), markst.WithLibrary(lib))

	meta, ok := markst.Query(doc, name.Make("doc-meta"))
	if !ok {
		t.Fatalf("Query(doc-meta) not found")
	}
	d, ok := meta.(*value.Dict)
	if !ok {
		t.Fatalf("doc-meta is %T, want *value.Dict", meta)
	}
	title, _ := d.Elems.Get("title")
	if want := value.Str("Routing"); title != want {
		t.Errorf("doc-meta.title = %v, want %v", title, want)
	}
}

// TestLibrarySetRuleAppliesToDocument is the regression test for the
// session/module split. `set document` runs inside the library's function but
// must land on the document being compiled — if the closure kept the session
// it was built in, both fields would stay empty.
func TestLibrarySetRuleAppliesToDocument(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", articleLib)
	doc := compile(t, `#article(title: "Routing", published: "2024-07-06")`,
		markst.WithLibrary(lib))

	if doc.Title != "Routing" {
		t.Errorf("doc.Title = %q, want %q — the library's set rule did not reach this document", doc.Title, "Routing")
	}
	if doc.Date == nil {
		t.Fatalf("doc.Date = nil, want 2024-07-06")
	}
	if got := doc.Date.T.Format("2006-01-02"); got != "2024-07-06" {
		t.Errorf("doc.Date = %v, want 2024-07-06", got)
	}
}

// TestLibraryLabelsBelongToDocument checks the other piece of session state a
// library function touches: the label set. A label used once from the library
// and once from the document is a reuse the *document's* session must catch.
func TestLibraryLabelsBelongToDocument(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", articleLib)
	_, warns, err := markst.Compile(t.Context(),
		[]byte(`#article(title: "Routing", published: "2024-07-06")

#metadata("again") <doc-meta>
`),
		markst.WithName("index.mst"), markst.WithLibrary(lib))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want one label-reuse warning", warns)
	}
	if !strings.Contains(warns[0].Msg, "label") {
		t.Errorf("warning = %q, want it to be about a label", warns[0].Msg)
	}
}

// TestLibraryErrorIsLocatedInTheLibrary checks that a failure inside library
// code is reported against the library's own text, with the call site in the
// document that reached it — not resolved against the document's bytes, where
// the offset means something else entirely.
func TestLibraryErrorIsLocatedInTheLibrary(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", articleLib)
	diags := diagnostics(t, `#article(title: "Routing", published: "2024-13-01")`,
		markst.WithName("index.mst"), markst.WithLibrary(lib))

	if len(diags) != 1 {
		t.Fatalf("diagnostics = %v, want one", diags)
	}
	d := diags[0]
	if d.Origin != "lib.mst" {
		t.Errorf("Origin = %q, want %q", d.Origin, "lib.mst")
	}
	// Line 3 of articleLib is the datetime.parse_date call.
	if d.Loc.Start.Line != 3 {
		t.Errorf("Loc.Start = %v, want line 3 of the library", d.Loc.Start)
	}
	if len(d.Trace) != 1 {
		t.Fatalf("Trace = %v, want the one call site that crossed into the library", d.Trace)
	}
	f := d.Trace[0]
	if f.Origin != "index.mst" || f.Loc.Start.Line != 1 || f.Callee != "article" {
		t.Errorf("Trace[0] = %+v, want index.mst line 1 in article", f)
	}
}

// TestDocumentErrorOrigin checks the other half: an ordinary failure carries
// the document's own name, and stays locatable when the host gave none.
func TestDocumentErrorOrigin(t *testing.T) {
	t.Run("named", func(t *testing.T) {
		diags := diagnostics(t, "#nope\n", markst.WithName("index.mst"))
		if diags[0].Origin != "index.mst" {
			t.Errorf("Origin = %q, want %q", diags[0].Origin, "index.mst")
		}
		if len(diags[0].Trace) != 0 {
			t.Errorf("Trace = %v, want empty for a failure in the document itself", diags[0].Trace)
		}
	})
	t.Run("unnamed", func(t *testing.T) {
		diags := diagnostics(t, "#nope\n")
		if diags[0].Origin != "" {
			t.Errorf("Origin = %q, want empty", diags[0].Origin)
		}
		if !diags[0].Loc.IsValid() {
			t.Errorf("Loc = %v, want a valid location even without a name", diags[0].Loc)
		}
	})
}

// TestLibraryReadsCallersNow checks that per-render state other than
// diagnostics also comes from the caller: `datetime.today` in library code
// must read the instant the *document* is being rendered at.
//
// The callback case covers the builtin-side half of the same rule — a user
// closure invoked by array.map runs in whatever runtime the builtin was
// called in.
func TestLibraryReadsCallersNow(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", `#let today() = datetime.today().display()
#let today-via-callback() = (0,).map(_ => datetime.today().display()).at(0)
`)
	now := time.Date(1970, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, fn := range []string{"today", "today-via-callback"} {
		t.Run(fn, func(t *testing.T) {
			doc := compile(t, "#metadata("+fn+"()) <d>",
				markst.WithLibrary(lib), markst.WithNow(now))
			got, ok := markst.Query(doc, name.Make("d"))
			if !ok {
				t.Fatalf("Query(d) not found")
			}
			if want := value.Str("1970-01-01"); got != want {
				t.Errorf("%s() = %v, want %v — the library did not see the document's clock", fn, got, want)
			}
		})
	}
}

// TestLibraryPrecedence pins the resolution order: a library shadows a
// built-in, a later library shadows an earlier one, and the document's own
// binding shadows them all.
func TestLibraryPrecedence(t *testing.T) {
	first, _ := compileLibrary(t, "first.mst", `#let emph(x) = "first"
#let only-in-first() = "first"
`)
	second, _ := compileLibrary(t, "second.mst", `#let emph(x) = "second"`)

	tests := []struct {
		name string
		src  string
		opts []markst.Option
		want value.Str
	}{
		{
			name: "library shadows builtin",
			src:  `#metadata(emph("x")) <d>`,
			opts: []markst.Option{markst.WithLibrary(first)},
			want: "first",
		},
		{
			name: "later library shadows earlier",
			src:  `#metadata(emph("x")) <d>`,
			opts: []markst.Option{markst.WithLibrary(first, second)},
			want: "second",
		},
		{
			name: "document shadows library",
			src: `#let emph(x) = "document"
#metadata(emph("x")) <d>`,
			opts: []markst.Option{markst.WithLibrary(first)},
			want: "document",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := compile(t, tt.src, tt.opts...)
			got, _ := markst.Query(doc, name.Make("d"))
			if got != tt.want {
				t.Errorf("= %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLibraryComposition checks that a library may be built on another one,
// which is the same mechanism applied one level down.
func TestLibraryComposition(t *testing.T) {
	base, _ := compileLibrary(t, "base.mst", `#let greeting() = "hello"`)
	derived, _, err := markst.CompileLibrary(t.Context(), "derived.mst",
		[]byte(`#let greet(who) = greeting() + ", " + who`),
		markst.WithLibrary(base))
	if err != nil {
		t.Fatalf("CompileLibrary(derived) = %v", err)
	}

	// Only the derived library is passed to the document: base travels inside
	// greet as an ordinary captured value, not as a name the document can see.
	doc := compile(t, `#metadata(greet("world")) <d>`, markst.WithLibrary(derived))
	got, _ := markst.Query(doc, name.Make("d"))
	if want := value.Str("hello, world"); got != want {
		t.Errorf("greet() = %v, want %v", got, want)
	}
	if _, ok := derived.Lookup(name.Make("greeting")); ok {
		t.Errorf("derived library re-exports greeting; only its own bindings should be exported")
	}
}

// TestLibraryExportsFinalValue checks that what is exported is the binding as
// of the end of the file, not as of its first definition.
func TestLibraryExportsFinalValue(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", `#let x = "first"
#{ x = "second" }
`)
	doc := compile(t, `#metadata(x) <d>`, markst.WithLibrary(lib))
	got, _ := markst.Query(doc, name.Make("d"))
	if want := value.Str("second"); got != want {
		t.Errorf("x = %v, want %v", got, want)
	}
}

// TestLibraryScoping checks that a name bound inside a block is not exported:
// only the top level is the library's interface.
func TestLibraryScoping(t *testing.T) {
	lib, _ := compileLibrary(t, "lib.mst", `#{ let hidden = 1 }
#let shown = 2
`)
	if _, ok := lib.Lookup(name.Make("hidden")); ok {
		t.Errorf("Lookup(hidden) found; block-scoped bindings must not be exported")
	}
	if _, ok := lib.Lookup(name.Make("shown")); !ok {
		t.Errorf("Lookup(shown) not found")
	}
}

// TestLibraryWithContentWarns checks that a library which tries to render
// something is told its body goes nowhere.
func TestLibraryWithContentWarns(t *testing.T) {
	_, warns := compileLibrary(t, "lib.mst", `#let f() = 1

This text has nowhere to go.
`)
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want one", warns)
	}
	if warns[0].Origin != "lib.mst" {
		t.Errorf("Origin = %q, want %q", warns[0].Origin, "lib.mst")
	}
	if !strings.Contains(warns[0].Msg, "discarded") {
		t.Errorf("warning = %q, want it to say the content is discarded", warns[0].Msg)
	}
}

// TestLibraryCompileError checks that a library that does not compile is not
// handed back as if it had.
func TestLibraryCompileError(t *testing.T) {
	lib, _, err := markst.CompileLibrary(t.Context(), "lib.mst", []byte("#let f() = nope\n#f()\n"))
	var diags markst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("CompileLibrary() error is %T (%v), want markst.DiagnosticList", err, err)
	}
	if lib != nil {
		t.Errorf("CompileLibrary() = %v, want nil on failure", lib)
	}
	if diags[0].Origin != "lib.mst" {
		t.Errorf("Origin = %q, want %q", diags[0].Origin, "lib.mst")
	}
}

// TestImportAndIncludeAreDiagnostics guards the two constructs a host reaches
// for first on hearing that markst has libraries. The parser understands both
// and nothing below it does; until that changes they must say so rather than
// crash the compile.
func TestImportAndIncludeAreDiagnostics(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{`#import "lib.mst": *`, "imports are not supported"},
		{`#import "lib.mst": article`, "imports are not supported"},
		{`#import "lib.mst"`, "imports are not supported"},
		{`#include "lib.mst"`, "includes are not supported"},
	} {
		t.Run(tt.src, func(t *testing.T) {
			diags := diagnostics(t, tt.src, markst.WithName("index.mst"))
			if diags[0].Msg != tt.want {
				t.Errorf("Msg = %q, want %q", diags[0].Msg, tt.want)
			}
			if !diags[0].Loc.IsValid() {
				t.Errorf("Loc = %v, want a location", diags[0].Loc)
			}
		})
	}
}

// TestLibraryWhitespaceDoesNotWarn guards the content warning against the
// thing every library has: blank lines between its bindings, which lower to
// parbreaks and spaces. Warning about those would fire on every library.
func TestLibraryWhitespaceDoesNotWarn(t *testing.T) {
	for _, src := range []string{
		"#let f() = 1\n",
		"#let f() = 1\n\n#let g() = 2\n",
		"// a comment\n\n#let f() = 1\n\n\n",
	} {
		t.Run(src, func(t *testing.T) {
			if _, warns := compileLibrary(t, "lib.mst", src); len(warns) > 0 {
				t.Errorf("warnings = %v, want none", warns)
			}
		})
	}
}
