package value

import (
	"slices"
	"testing"
)

// indexNames returns the element names an index walk with kinds visits.
func indexNames(c Content, kinds KindSet) []string {
	x := NewIndex(c)
	var got []string
	for cur := range x.Preorder(kinds) {
		got = append(got, cur.Node().Name())
	}
	return got
}

// TestIndexWalksLikeTheTree is the contract that makes an index substitutable
// for the content it was built over: for every mask, the same elements in the
// same order.
func TestIndexWalksLikeTheTree(t *testing.T) {
	c := doc(3)
	masks := []KindSet{
		AnyKind,
		SetOf(KindText),
		SetOf(KindMetadata),
		SetOf(KindPar, KindEmph),
		AnyKind.Remove(KindSequence, KindText),
		SetOf(KindTable), // nothing matches
	}
	for _, kinds := range masks {
		want := elemNames(c, kinds)
		if got := indexNames(c, kinds); !slices.Equal(got, want) {
			t.Errorf("Index.Preorder(%#x) visited %v, want %v", uint64(kinds), got, want)
		}
	}
}

func TestIndexRootAndLen(t *testing.T) {
	c := doc(2)
	x := NewIndex(c)
	if x.Root() != c {
		t.Errorf("Root() = %v, want the element the index was built over", x.Root())
	}
	if want := len(elemNames(c, AnyKind)); x.Len() != want {
		t.Errorf("Len() = %d, want %d", x.Len(), want)
	}
	var empty Index
	if empty.Root() != nil || empty.Len() != 0 {
		t.Errorf("the zero Index holds %d elements rooted at %v, want none", empty.Len(), empty.Root())
	}
	for range empty.Preorder(AnyKind) {
		t.Fatal("the zero Index walked something")
	}
}

func TestIndexSkipChildren(t *testing.T) {
	x := NewIndex(doc(2))
	var got []string
	for c := range x.Preorder(SetOf(KindText, KindStrong)) {
		got = append(got, c.Node().Name())
		if c.Kind() == KindStrong {
			c.SkipChildren()
		}
	}
	want := []string{"text", "strong", "text", "strong"}
	if !slices.Equal(got, want) {
		t.Errorf("Index.Preorder() with SkipChildren visited %v, want %v", got, want)
	}
}

func TestIndexStopsEarly(t *testing.T) {
	x := NewIndex(doc(100))
	visited := 0
	for range x.Preorder(AnyKind) {
		visited++
		break
	}
	if visited != 1 {
		t.Errorf("Index.Preorder() visited %d elements after a break, want 1", visited)
	}
}

// TestIndexCursorAncestors checks that a cursor from an index reports the same
// chain as one from a walk, including across a subtree the index stepped over.
func TestIndexCursorAncestors(t *testing.T) {
	text := &Text{Text: "nested"}
	heading := &Heading{Depth: 1, Body: &Strong{Body: &Emph{Body: text}}}
	body := &Sequence{Children: []Content{
		&Par{Body: &Sequence{Children: []Content{&Text{Text: "skipped"}}}},
		heading,
	}}
	x := NewIndex(body)

	var found bool
	for c := range x.Preorder(SetOf(KindEmph)) {
		found = true
		if c.Depth() != 3 {
			t.Errorf("Depth() = %d, want 3", c.Depth())
		}
		want := []Content{body, heading, heading.Body, c.Node()}
		if got := c.Path(); !slices.Equal(got, want) {
			t.Errorf("Path() = %v, want %v", got, want)
		}
		var enclosing []Content
		for e := range c.Enclosing(SetOf(KindHeading)) {
			enclosing = append(enclosing, e.Node())
		}
		if want := []Content{heading}; !slices.Equal(enclosing, want) {
			t.Errorf("Enclosing(heading) = %v, want %v", enclosing, want)
		}
	}
	if !found {
		t.Fatal("Index.Preorder() found no emph")
	}
}

// TestIndexSkipsSubtrees pins the thing only an index can do: a subtree that
// holds none of the kinds asked for is never touched. It is observable through
// the elements a walk over the same tree would have had to look at.
func TestIndexSkipsSubtrees(t *testing.T) {
	x := NewIndex(doc(50))
	// Nothing in the document is a table, so the whole tree is one skip.
	for range x.Preorder(SetOf(KindTable)) {
		t.Fatal("Index.Preorder(table) visited something")
	}
	if got := len(indexNames(doc(50), SetOf(KindMetadata))); got != 50 {
		t.Errorf("Index.Preorder(metadata) visited %d elements, want 50", got)
	}
}
