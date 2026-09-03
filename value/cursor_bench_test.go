package value

import (
	"strings"
	"testing"

	"znkr.io/markst/name"
)

// article builds a document shaped like prose rather than the uniform nesting
// of [doc]: sections of paragraphs, each with a footnote whose body is content
// no query about the article itself wants to descend into.
func article(sections int) Content {
	var children []Content
	for i := 0; i < sections; i++ {
		children = append(children, &Heading{Depth: 1, Body: &Sequence{Children: []Content{
			&Text{Text: "Section"},
			&Footnote{Body: &Par{Body: &Sequence{Children: []Content{
				&Text{Text: "a note that is not part of the title"},
				&Emph{Body: &Text{Text: "emphasized"}},
			}}}},
		}}})
		for j := 0; j < 4; j++ {
			children = append(children, &Par{Body: &Sequence{Children: []Content{
				&Text{Text: strings.Repeat("word ", 8)},
				&Strong{Body: &Emph{Body: &Text{Text: "nested"}}},
				&Link{Dest: "https://example.com", Body: &Text{Text: "a link"}},
			}}})
		}
		children = append(children, &Metadata{Value: Int(i), Label: &Label{Name: name.Make("m")}})
	}
	return &Sequence{Children: children}
}

// BenchmarkWalkAll is the whole-document walk: every element yielded, nothing
// filtered.
func BenchmarkWalkAll(b *testing.B) {
	c := article(64)
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for range Preorder(c, AnyKind) {
			n++
		}
		if n == 0 {
			b.Fatal("walked nothing")
		}
	}
}

// BenchmarkWalkFiltered is the Query shape: one element type out of a whole
// document.
func BenchmarkWalkFiltered(b *testing.B) {
	c := article(64)
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for range Preorder(c, SetOf(KindMetadata)) {
			n++
		}
		if n == 0 {
			b.Fatal("found nothing")
		}
	}
}

// BenchmarkWalkPruned is the headingText shape: a filtered walk that steps over
// the subtrees of one element type.
func BenchmarkWalkPruned(b *testing.B) {
	c := article(64)
	kinds := SetOf(KindText, KindFootnote)
	b.ReportAllocs()
	for b.Loop() {
		var sb strings.Builder
		for e := range Preorder(c, kinds) {
			switch n := e.Node().(type) {
			case *Text:
				sb.WriteString(n.Text)
			case *Footnote:
				e.SkipChildren()
			}
		}
		if sb.Len() == 0 {
			b.Fatal("collected nothing")
		}
	}
}

// BenchmarkWalkFirstMatch is the early-exit shape: stop at the first element of
// a kind, which for a document of any size should not depend on its size.
func BenchmarkWalkFirstMatch(b *testing.B) {
	c := article(64)
	b.ReportAllocs()
	for b.Loop() {
		found := false
		for range Preorder(c, SetOf(KindMetadata)) {
			found = true
			break
		}
		if !found {
			b.Fatal("found nothing")
		}
	}
}

// BenchmarkIndexInit is what an index costs to build: one full walk, plus a
// record per element.
func BenchmarkIndexInit(b *testing.B) {
	c := article(64)
	var x Index
	b.ReportAllocs()
	for b.Loop() {
		x.Init(c)
	}
	b.ReportMetric(float64(x.Len()), "elems")
}

// BenchmarkIndexWalkFiltered is the same filtered walk as BenchmarkWalkFiltered,
// read off an index instead of the tree — the comparison that says how many
// walks it takes to pay the index back.
func BenchmarkIndexWalkFiltered(b *testing.B) {
	x := NewIndex(article(64))
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for range x.Preorder(SetOf(KindMetadata)) {
			n++
		}
		if n == 0 {
			b.Fatal("found nothing")
		}
	}
}

// BenchmarkIndexWalkAll is the unfiltered walk, where the index can skip
// nothing and is pure overhead against the tree.
func BenchmarkIndexWalkAll(b *testing.B) {
	x := NewIndex(article(64))
	b.ReportAllocs()
	for b.Loop() {
		n := 0
		for range x.Preorder(AnyKind) {
			n++
		}
		if n == 0 {
			b.Fatal("walked nothing")
		}
	}
}
