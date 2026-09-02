package markst

import "znkr.io/markst/value"

// Section is a heading and the sections nested under it.
type Section struct {
	Heading  *value.Heading
	Children []*Section
}

// Outline returns doc's headings as a tree, nested by depth — a table of
// contents, in whatever form the caller renders one.
//
// Every heading carries a [value.Label], so there is always an anchor to link
// a section by; [value.Label.Auto] tells a generated one from a label the
// source wrote.
//
// A skipped level nests under the nearest shallower heading rather than
// inventing one to sit in: `=` followed by `===` gives a section holding a
// section, and nothing in between. A document that opens at `==` has that
// heading at the top level, because there is no `=` for it to belong to.
func Outline(doc *value.Document) []*Section {
	if doc == nil || doc.Body == nil {
		return nil
	}

	var top []*Section
	// path is the chain of sections currently open, outermost first. A heading
	// belongs to the deepest one shallower than itself.
	var path []*Section
	for c := range value.Preorder(doc.Body, value.SetOf(value.KindHeading)) {
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
