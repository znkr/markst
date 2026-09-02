package markst_test

import (
	"io"
	"os"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/eval"
	"znkr.io/markst/html"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
	"znkr.io/markst/syntax/scanner"
	"znkr.io/markst/value"
)

// benchSource is an article of the kind markst exists to render: prose with
// headings, lists, a table, footnotes, math, raw blocks and metadata. Every
// benchmark here runs over it, so a stage's cost can be read against the whole
// pipeline's rather than on its own.
func benchSource(b *testing.B) []byte {
	b.Helper()
	src, err := os.ReadFile("testdata/bench/article.mst")
	if err != nil {
		b.Fatal(err)
	}
	return src
}

func benchDocument(b *testing.B) *value.Document {
	b.Helper()
	doc, _, err := markst.Compile(benchSource(b))
	if err != nil {
		b.Fatal(err)
	}
	return doc
}

func benchIndex(b *testing.B) *value.Index {
	b.Helper()
	var idx value.Index
	if _, _, err := markst.Compile(benchSource(b), markst.WithIndex(&idx)); err != nil {
		b.Fatal(err)
	}
	return &idx
}

// BenchmarkScan is the tokenizer alone, over the same source [BenchmarkParse]
// parses: what the parser spends before it builds a single node. The parser
// drives the scanner's mode, so a markup-mode scan of the whole file is an
// approximation — close enough to say what share of parsing is lexing.
func BenchmarkScan(b *testing.B) {
	src := benchSource(b)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		s := scanner.New(src)
		for {
			kind, _ := s.Next()
			if kind == syntax.KindEnd {
				break
			}
		}
	}
}

func BenchmarkParse(b *testing.B) {
	src := benchSource(b)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		parser.Parse(src)
	}
}

func BenchmarkAnalyze(b *testing.B) {
	root := parser.Parse(benchSource(b))
	b.ReportAllocs()
	for b.Loop() {
		analyzer.Analyze(root)
	}
}

func BenchmarkEval(b *testing.B) {
	mod := analyzer.Analyze(parser.Parse(benchSource(b)))
	b.ReportAllocs()
	for b.Loop() {
		_, _, errs := eval.Eval(mod)
		if len(errs) > 0 {
			b.Fatalf("Eval() = %v", errs)
		}
	}
}

// BenchmarkCompile is the whole pipeline, and the denominator for every other
// number here.
func BenchmarkCompile(b *testing.B) {
	src := benchSource(b)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := markst.Compile(src); err != nil {
			b.Fatalf("Compile() = %v", err)
		}
	}
}

func BenchmarkRender(b *testing.B) {
	doc := benchDocument(b)
	b.ReportAllocs()
	for b.Loop() {
		if err := html.Render(io.Discard, doc); err != nil {
			b.Fatalf("Render() = %v", err)
		}
	}
}

// BenchmarkQuery is one labelled metadata out of a whole document — the walk a
// host doing front matter runs once per field.
func BenchmarkQuery(b *testing.B) {
	doc := benchDocument(b)
	label := name.Make("published")
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := markst.Query(doc, label); !ok {
			b.Fatal("Query() found nothing")
		}
	}
}

func BenchmarkQueryIndexed(b *testing.B) {
	idx := benchIndex(b)
	label := name.Make("published")
	b.ReportAllocs()
	for b.Loop() {
		if _, ok := markst.Query(idx, label); !ok {
			b.Fatal("Query() found nothing")
		}
	}
}

func BenchmarkOutline(b *testing.B) {
	doc := benchDocument(b)
	b.ReportAllocs()
	for b.Loop() {
		if len(markst.Outline(doc)) == 0 {
			b.Fatal("Outline() found nothing")
		}
	}
}

// BenchmarkCompileWithIndex is what asking for an index adds to a compile, and
// the three benchmarks after it are what spending one saves.
func BenchmarkCompileWithIndex(b *testing.B) {
	src := benchSource(b)
	var idx value.Index
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := markst.Compile(src, markst.WithIndex(&idx)); err != nil {
			b.Fatalf("Compile() = %v", err)
		}
	}
}

func BenchmarkOutlineIndexed(b *testing.B) {
	idx := benchIndex(b)
	b.ReportAllocs()
	for b.Loop() {
		if len(markst.Outline(idx)) == 0 {
			b.Fatal("Outline() found nothing")
		}
	}
}

func BenchmarkRenderIndexed(b *testing.B) {
	doc := benchDocument(b)
	idx := benchIndex(b)
	b.ReportAllocs()
	for b.Loop() {
		if err := html.Render(io.Discard, doc, html.WithIndex(idx)); err != nil {
			b.Fatalf("Render() = %v", err)
		}
	}
}

func BenchmarkFootnotesIndexed(b *testing.B) {
	idx := benchIndex(b)
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for range idx.Preorder(value.SetOf(value.KindFootnote)) {
			n++
		}
		if n == 0 {
			b.Fatal("found no footnotes")
		}
	}
}

func BenchmarkFootnotes(b *testing.B) {
	doc := benchDocument(b)
	b.ReportAllocs()
	for b.Loop() {
		if len(html.Footnotes(doc)) == 0 {
			b.Fatal("Footnotes() found nothing")
		}
	}
}
