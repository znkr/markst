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
	"fmt"
	"slices"
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/value"
)

func TestOutline(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "flat",
			src:  "= One\n\n= Two\n",
			want: "one\ntwo\n",
		},
		{
			name: "nested",
			src:  "= One\n\n== A\n\n== B\n\n= Two\n",
			want: "one\n  a\n  b\ntwo\n",
		},
		{
			name: "deeper-then-back",
			src:  "= One\n\n== A\n\n=== A1\n\n== B\n",
			want: "one\n  a\n    a1\n  b\n",
		},
		{
			name: "skipped-level",
			src:  "= One\n\n=== Deep\n",
			want: "one\n  deep\n",
		},
		{
			name: "starts-deep",
			src:  "== A\n\n== B\n",
			want: "a\nb\n",
		},
		{
			name: "shallower-than-the-first",
			src:  "== A\n\n= One\n",
			want: "a\none\n",
		},
		{
			name: "no-headings",
			src:  "Just a paragraph.\n",
			want: "",
		},
		{
			name: "heading-inside-a-list-item",
			src:  "- #[= Nested]\n",
			want: "nested\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, _, err := markst.Compile(t.Context(), []byte(tc.src))
			if err != nil {
				t.Fatalf("compiling: %v", err)
			}
			var b strings.Builder
			writeOutline(&b, markst.Outline(doc), 0)
			if got := b.String(); got != tc.want {
				t.Errorf("Outline() =\n%q\nwant\n%q", got, tc.want)
			}

			// An index of the document is the same outline, read off the
			// record instead of the document.
			idx := value.NewIndex(doc)
			var indexed strings.Builder
			writeOutline(&indexed, markst.Outline(&idx), 0)
			if got := indexed.String(); got != tc.want {
				t.Errorf("Outline(index) =\n%q\nwant\n%q", got, tc.want)
			}
		})
	}
}

func writeOutline(b *strings.Builder, sections []*markst.Section, depth int) {
	for _, s := range sections {
		fmt.Fprintf(b, "%s%s\n", strings.Repeat("  ", depth), s.Heading.GetLabel().Name)
		writeOutline(b, s.Children, depth+1)
	}
}

func TestOutlineNilDocument(t *testing.T) {
	if got := markst.Outline(nil); got != nil {
		t.Errorf("Outline(nil) = %v, want nil", got)
	}
	var doc *value.Document
	if got := markst.Outline(doc); got != nil {
		t.Errorf("Outline((*value.Document)(nil)) = %v, want nil", got)
	}
	var idx value.Index
	if got := markst.Outline(&idx); got != nil {
		t.Errorf("Outline(&value.Index{}) = %v, want nil", got)
	}
}

// TestCompileWithIndex checks the option end to end: the index Compile fills
// walks the document it was compiled from.
func TestCompileWithIndex(t *testing.T) {
	var idx value.Index
	doc, _, err := markst.Compile(t.Context(), []byte("= One\n\nSome text.\n\n== Two\n"), markst.WithIndex(&idx))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if idx.Root() != value.Content(doc) {
		t.Errorf("index is rooted at %v, want the compiled document", idx.Root())
	}
	var want, got []string
	for c := range value.Preorder(doc, value.AnyKind) {
		want = append(want, c.Node().Name())
	}
	for c := range idx.Preorder(value.AnyKind) {
		got = append(got, c.Node().Name())
	}
	if !slices.Equal(got, want) {
		t.Errorf("the index walks %v, want %v", got, want)
	}
}

// TestCompileWithIndexOnFailure checks that the index describes what Compile
// returned, which after a failure is the document it got to rather than none.
func TestCompileWithIndexOnFailure(t *testing.T) {
	var idx value.Index
	doc, _, err := markst.Compile(t.Context(), []byte("#let x = "), markst.WithIndex(&idx))
	if err == nil {
		t.Fatal("Compile() of a broken document succeeded")
	}
	if idx.Root() != value.Content(doc) {
		t.Errorf("index is rooted at %v, want the document Compile returned", idx.Root())
	}
}
