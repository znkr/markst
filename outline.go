package markst

import "znkr.io/markst/value"

// Section is a heading and the sections nested under it.
type Section struct {
	Heading  *value.Heading
	Children []*Section
}

// Outline returns a document's headings as a tree, nested by depth: a table of
// contents, for the caller to render however it likes.
//
// Pass the [value.Document] that [Compile] returned, or a [value.Index] of one
// ([WithIndex]) to read the headings from the index instead of traversing the
// document.
//
// Every heading has a [value.Label] to link the section by, and
// [value.Label.Auto] distinguishes a generated label from one the source wrote.
//
// A skipped level nests under the nearest shallower heading; no intermediate
// section is invented. So `=` followed by `===` gives one section containing
// another, with nothing between them, and a document that opens at `==` has
// that heading at the top level, there being no `=` to nest it under.
func Outline(t value.Tree) []*Section {
	if t == nil {
		return nil
	}

	var top []*Section
	// path holds the sections currently open, outermost first. A heading
	// belongs to the deepest one shallower than itself.
	var path []*Section
	for c := range t.Preorder(value.SetOf(value.KindHeading)) {
		h := c.Node().(*value.Heading)
		for len(path) > 0 && path[len(path)-1].Heading.Depth >= h.Depth {
			path = path[:len(path)-1]
		}
		s := &Section{Heading: h}
		if len(path) == 0 {
			top = append(top, s)
		} else {
			parent := path[len(path)-1]
			parent.Children = append(parent.Children, s)
		}
		path = append(path, s)
	}
	return top
}
