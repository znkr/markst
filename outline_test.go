package markst_test

import (
	"fmt"
	"strings"
	"testing"

	"znkr.io/markst"
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
			doc, _, err := markst.Compile([]byte(tc.src))
			if err != nil {
				t.Fatalf("compiling: %v", err)
			}
			var b strings.Builder
			writeOutline(&b, markst.Outline(doc), 0)
			if got := b.String(); got != tc.want {
				t.Errorf("Outline() =\n%q\nwant\n%q", got, tc.want)
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
}
