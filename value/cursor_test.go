package value

import (
	"slices"
	"testing"

	"znkr.io/markst/name"
)

// doc builds a document n paragraphs deep in nesting and n wide, so a walk over
// it visits a number of elements that grows with n.
func doc(n int) Content {
	children := make([]Content, 0, n)
	for i := 0; i < n; i++ {
		children = append(children, &Par{Body: &Sequence{Children: []Content{
			&Text{Text: "text"},
			&Strong{Body: &Emph{Body: &Text{Text: "nested"}}},
			&Metadata{Value: Int(i), Label: &Label{Name: name.Make("m")}},
		}}})
	}
	return &Sequence{Children: children}
}

// elemNames returns the element names a walk over c with kinds visits, in order.
func elemNames(c Content, kinds KindSet) []string {
	var got []string
	for cur := range Preorder(c, kinds) {
		got = append(got, cur.Node().Name())
	}
	return got
}

func TestKindSetFits(t *testing.T) {
	if numElemKinds > 64 {
		t.Errorf("numElemKinds = %d, more than the 64 bits of a KindSet", numElemKinds)
	}
}

func TestKindSet(t *testing.T) {
	s := SetOf(KindHeading, KindText)
	if !s.Contains(KindHeading) || !s.Contains(KindText) {
		t.Errorf("SetOf(heading, text) is missing one of them")
	}
	if s.Contains(KindPar) {
		t.Errorf("SetOf(heading, text) contains par")
	}
	if got := s.Add(KindPar); !got.Contains(KindPar) {
		t.Errorf("Add(par) did not add par")
	}
	if got := AnyKind.Remove(KindText); got.Contains(KindText) {
		t.Errorf("Remove(text) did not remove text")
	}
	for k := ElemKind(0); k < numElemKinds; k++ {
		if !AnyKind.Contains(k) {
			t.Errorf("AnyKind is missing kind %d", k)
		}
	}
}

// TestPreorder pins document order: an element, then its children left to
// right, which is what every consumer reads a document in.
func TestPreorder(t *testing.T) {
	want := []string{
		"sequence",
		"par", "sequence", "text", "strong", "emph", "text", "metadata",
		"par", "sequence", "text", "strong", "emph", "text", "metadata",
	}
	if got := elemNames(doc(2), AnyKind); !slices.Equal(got, want) {
		t.Errorf("Preorder() visited %v, want %v", got, want)
	}
}

// TestPreorderFilters checks that the mask decides what is yielded and not what
// is walked: a text sits under elements the mask leaves out, and is still found.
func TestPreorderFilters(t *testing.T) {
	want := []string{"text", "text", "text", "text"}
	if got := elemNames(doc(2), SetOf(KindText)); !slices.Equal(got, want) {
		t.Errorf("Preorder(text) visited %v, want %v", got, want)
	}
	if got := elemNames(doc(2), SetOf(KindTable)); got != nil {
		t.Errorf("Preorder(table) visited %v, want nothing", got)
	}
}

// TestPreorderSkipChildren checks that pruning skips exactly one subtree: the
// strong's own text is gone, the paragraph's is not.
func TestPreorderSkipChildren(t *testing.T) {
	var got []string
	for c := range Preorder(doc(2), SetOf(KindText, KindStrong)) {
		got = append(got, c.Node().Name())
		if c.Kind() == KindStrong {
			c.SkipChildren()
		}
	}
	want := []string{"text", "strong", "text", "strong"}
	if !slices.Equal(got, want) {
		t.Errorf("Preorder() with SkipChildren visited %v, want %v", got, want)
	}
}

// TestPreorderStopsEarly checks that breaking out of the range stops the whole
// walk, outer levels included, rather than running it to the end — what makes a
// query for the first match cheap.
func TestPreorderStopsEarly(t *testing.T) {
	visited := 0
	for range Preorder(doc(100), AnyKind) {
		visited++
		break
	}
	if visited != 1 {
		t.Errorf("Preorder() visited %d elements after a break, want 1", visited)
	}
}

// TestPreorderSkipsMetadataValue pins the one thing below an element that a
// walk does not reach: metadata's value is data the document carries, not part
// of the document.
func TestPreorderSkipsMetadataValue(t *testing.T) {
	inner := &Text{Text: "inner"}
	c := &Metadata{Value: inner}
	for cur := range Preorder(c, AnyKind) {
		if cur.Node() == Content(inner) {
			t.Fatalf("Preorder() descended into Metadata.Value")
		}
	}
}

// TestCursorAncestors checks the chain a cursor reports against the nesting it
// was built from — the question document order alone cannot answer.
func TestCursorAncestors(t *testing.T) {
	text := &Text{Text: "nested"}
	emph := &Emph{Body: text}
	heading := &Heading{Depth: 1, Body: &Strong{Body: emph}}
	body := &Sequence{Children: []Content{heading}}

	var found bool
	for c := range Preorder(body, SetOf(KindText)) {
		found = true
		if c.Depth() != 4 {
			t.Errorf("Depth() = %d, want 4", c.Depth())
		}
		parent, ok := c.Parent()
		if !ok || parent.Node() != Content(emph) {
			t.Errorf("Parent() = %v, %v, want the emph", parent.Node(), ok)
		}
		want := []Content{body, heading, heading.Body, emph, text}
		if got := c.Path(); !slices.Equal(got, want) {
			t.Errorf("Path() = %v, want %v", got, want)
		}
		var enclosing []Content
		for e := range c.Enclosing(SetOf(KindHeading, KindEmph)) {
			enclosing = append(enclosing, e.Node())
		}
		if want := []Content{emph, heading}; !slices.Equal(enclosing, want) {
			t.Errorf("Enclosing() = %v, want %v", enclosing, want)
		}
	}
	if !found {
		t.Fatal("Preorder() found no text")
	}
}

// TestCursorRootHasNoParent checks the one place the chain ends: the element the
// walk started at, whatever it is nested in elsewhere.
func TestCursorRootHasNoParent(t *testing.T) {
	for c := range Preorder(&Text{Text: "x"}, AnyKind) {
		if _, ok := c.Parent(); ok {
			t.Errorf("Parent() at the root returned a cursor, want none")
		}
		if c.Depth() != 0 {
			t.Errorf("Depth() at the root = %d, want 0", c.Depth())
		}
	}
}

// TestCursorPathOutlivesTheWalk checks what Path is for: the chain a cursor
// reports lives on the walk's stack, and a snapshot of it does not.
func TestCursorPathOutlivesTheWalk(t *testing.T) {
	var path []Content
	for c := range Preorder(doc(2), SetOf(KindText)) {
		path = c.Path()
		break
	}
	if len(path) != 4 || path[len(path)-1].Kind() != KindText {
		t.Errorf("Path() = %v, want the chain down to a text", path)
	}
}

// TestPreorderDoesNotAllocatePerElement pins what the generated descent buys:
// the walker is handed down untouched, so the cost of a walk does not grow with
// the document. Whatever a range-over-func costs to set up, it is paid once.
func TestPreorderDoesNotAllocatePerElement(t *testing.T) {
	count := func(c Content) func() {
		return func() {
			n := 0
			for range Preorder(c, AnyKind) {
				n++
			}
			if n == 0 {
				t.Fatal("walked nothing")
			}
		}
	}
	small := testing.AllocsPerRun(100, count(doc(1)))
	large := testing.AllocsPerRun(100, count(doc(100)))
	if large != small {
		t.Errorf("Preorder() allocated %v for a document of 1 paragraph and %v for 100; "+
			"the walk should allocate nothing per element", small, large)
	}
}
