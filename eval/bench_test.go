package eval

import (
	"os"
	"testing"

	"znkr.io/markst/expr"
	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
	"znkr.io/markst/value"
)

// benchModule is the corpus article, analyzed and ready to run. Eval is two
// halves — running the module, then realizing what it produced — and the
// benchmarks below time them apart, which the exported [Eval] cannot.
func benchModule(b *testing.B) *expr.Module {
	b.Helper()
	src, err := os.ReadFile("../testdata/bench/article.mst")
	if err != nil {
		b.Fatal(err)
	}
	return analyzer.Analyze(parser.Parse(src))
}

// BenchmarkEvaluate is the module run to a content tree: everything Eval does
// before realization.
func BenchmarkEvaluate(b *testing.B) {
	mod := benchModule(b)
	b.ReportAllocs()
	for b.Loop() {
		s, v := runTop(mod, nil)
		if len(s.errors) > 0 {
			b.Fatalf("evaluating: %v", s.errors)
		}
		if v == nil {
			b.Fatal("evaluated to nothing")
		}
	}
}

// BenchmarkRealize is the second half: paragraphs formed, items grouped, show
// rules applied, headings labeled.
func BenchmarkRealize(b *testing.B) {
	mod := benchModule(b)
	s, v := runTop(mod, nil)
	if len(s.errors) > 0 {
		b.Fatalf("evaluating: %v", s.errors)
	}
	c := value.ToContent(foldStyles(v))
	b.ReportAllocs()
	for b.Loop() {
		// A fresh session each time: realization records document properties
		// and footnotes on it, and a reused one would find them already there.
		fresh := &session{labels: s.labels}
		if doc := fresh.realizeDocument(c); doc.Body == nil {
			b.Fatal("realized nothing")
		}
	}
}

// BenchmarkRealizeWithIndex is realization for a caller who asked for an index:
// the walk that labels headings builds one on the way through.
func BenchmarkRealizeWithIndex(b *testing.B) {
	mod := benchModule(b)
	s, v := runTop(mod, nil)
	if len(s.errors) > 0 {
		b.Fatalf("evaluating: %v", s.errors)
	}
	c := value.ToContent(foldStyles(v))
	var idx value.Index
	b.ReportAllocs()
	for b.Loop() {
		fresh := &session{labels: s.labels, index: &idx}
		if doc := fresh.realizeDocument(c); doc.Body == nil {
			b.Fatal("realized nothing")
		}
	}
}
