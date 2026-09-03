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
	doc, _, err := markst.Compile(b.Context(), benchSource(b))
	if err != nil {
		b.Fatal(err)
	}
	return doc
}

func benchIndex(b *testing.B) *value.Index {
	b.Helper()
	var idx value.Index
	if _, _, err := markst.Compile(b.Context(), benchSource(b), markst.WithIndex(&idx)); err != nil {
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

// The context is cancellable, as a host's would be, so the number includes the
// one cancellation registration each run makes. Evaluation itself checks the
// resulting flag only at loop back edges and function entry.
func BenchmarkEval(b *testing.B) {
	mod := analyzer.Analyze(parser.Parse(benchSource(b)))
	b.ReportAllocs()
	for b.Loop() {
		_, diags, _ := eval.Eval(b.Context(), mod)
		if len(diags.Errors) > 0 {
			b.Fatalf("Eval() = %v", diags.Errors)
		}
	}
}

// BenchmarkLoop is a hot loop and nothing else: 200k iterations over an integer
// add, where per-iteration overhead is the whole number. Every loop carries a
// check that leaves it when its body records an error, and this is what says
// what that check costs.
func BenchmarkLoop(b *testing.B) {
	src := "#{\n  let i = 0\n  let s = 0\n  while i < 200000 {\n    s += i\n    i += 1\n  }\n  s\n}\n"
	mod := analyzer.Analyze(parser.Parse([]byte(src)))
	b.ReportAllocs()
	for b.Loop() {
		_, diags, _ := eval.Eval(b.Context(), mod)
		if len(diags.Errors) > 0 {
			b.Fatalf("Eval() = %v", diags.Errors)
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
		if _, _, err := markst.Compile(b.Context(), src); err != nil {
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

// BenchmarkQuery is one labeled metadata out of a whole document — the walk a
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
		if _, _, err := markst.Compile(b.Context(), src, markst.WithIndex(&idx)); err != nil {
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
