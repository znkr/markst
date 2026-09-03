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
func benchBody(sections int) *value.Document {
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
	return &value.Document{Body: &value.Sequence{Children: children}}
}

// BenchmarkAssignHeadingLabels covers the walk that labels every unlabeled
// heading: one pass for the headings, and one over each heading's own body for
// its text, pruned at footnotes.
func BenchmarkAssignHeadingLabels(b *testing.B) {
	doc := benchBody(32)
	b.ReportAllocs()
	for b.Loop() {
		unlabel(doc)
		s := &session{labels: map[name.Name]struct{}{}}
		s.assignHeadingLabels(doc)
	}
}

// BenchmarkAssignHeadingLabelsIndexed is the same pass read off an index, which
// is what a caller who asked for one gets: the index is built by then, and the
// headings are found without walking the document again.
func BenchmarkAssignHeadingLabelsIndexed(b *testing.B) {
	doc := benchBody(32)
	idx := value.NewIndex(doc)
	b.ReportAllocs()
	for b.Loop() {
		unlabel(doc)
		s := &session{labels: map[name.Name]struct{}{}}
		s.assignHeadingLabels(&idx)
	}
}

// unlabel strips the labels of a previous run, so each iteration does the same
// work as the first.
func unlabel(doc *value.Document) {
	for c := range value.Preorder(doc, value.SetOf(value.KindHeading)) {
		c.Node().(*value.Heading).Label = nil
	}
}
