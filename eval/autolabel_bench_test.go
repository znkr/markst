package eval

import (
	"fmt"
	"testing"

	"znkr.io/markst/name"
	"znkr.io/markst/value"
)

// benchBody builds a realized body of the shape assignHeadingLabels is run over:
// headings whose titles carry the markup a heading usually does, footnotes among
// them, and paragraphs between.
func benchBody(sections int) value.Content {
	var children []value.Content
	for i := range sections {
		children = append(children,
			&value.Heading{Depth: 1, Body: &value.Sequence{Children: []value.Content{
				&value.Text{Text: fmt.Sprintf("Section %d — ", i)},
				&value.Emph{Body: &value.Text{Text: "on walking"}},
				&value.Footnote{Body: &value.Par{Body: &value.Sequence{Children: []value.Content{
					&value.Text{Text: "a note whose text is not part of the title"},
					&value.Strong{Body: &value.Text{Text: "and is emphatic about it"}},
				}}}},
			}}},
			&value.Par{Body: &value.Sequence{Children: []value.Content{
				&value.Text{Text: "Some prose under the heading."},
				&value.Strong{Body: &value.Text{Text: "bold"}},
			}}},
		)
	}
	return &value.Sequence{Children: children}
}

// BenchmarkAssignHeadingLabels covers the walk that labels every unlabelled
// heading: one pass for the headings, and one over each heading's own body for
// its text, pruned at footnotes.
func BenchmarkAssignHeadingLabels(b *testing.B) {
	body := benchBody(32)
	b.ReportAllocs()
	for b.Loop() {
		for c := range value.Preorder(body, value.SetOf(value.KindHeading)) {
			c.Node().(*value.Heading).Label = nil
		}
		s := &session{labels: map[name.Name]struct{}{}}
		s.assignHeadingLabels(body)
	}
}
