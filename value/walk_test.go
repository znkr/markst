package value

import (
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

func TestAll(t *testing.T) {
	c := doc(2)
	var got []string
	for e := range All(c) {
		got = append(got, e.Name())
	}
	want := []string{
		"sequence",
		"par", "sequence", "text", "strong", "emph", "text", "metadata",
		"par", "sequence", "text", "strong", "emph", "text", "metadata",
	}
	if len(got) != len(want) {
		t.Fatalf("All() visited %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("All() visited %v, want %v", got, want)
		}
	}
}

// TestAllStopsEarly checks that breaking out of the range stops the walk rather
// than running it to the end — what makes a query for the first match cheap.
func TestAllStopsEarly(t *testing.T) {
	visited := 0
	for range All(doc(100)) {
		visited++
		break
	}
	if visited != 1 {
		t.Errorf("All() visited %d elements after a break, want 1", visited)
	}
}

// TestAllDoesNotAllocatePerElement pins what the generated walk buys: the yield
// function is handed down untouched, so the cost of a walk does not grow with
// the document. Whatever a range-over-func costs to set up, it is paid once.
func TestAllDoesNotAllocatePerElement(t *testing.T) {
	count := func(c Content) func() {
		return func() {
			n := 0
			for range All(c) {
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
		t.Errorf("All() allocated %v for a document of 1 paragraph and %v for 100; "+
			"the walk should allocate nothing per element", small, large)
	}
}
