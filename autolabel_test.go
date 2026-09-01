package markst_test

import (
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/value"
)

// headingLabel is what a presenter reads off a heading to anchor it with.
type headingLabel struct {
	name string
	auto bool
}

// headingLabels compiles src and returns the label of every heading in
// document order. A heading without one is a failure: the point of the feature
// is that there is always something to link to.
func headingLabels(t *testing.T, src string) []headingLabel {
	t.Helper()
	doc, warnings, err := markst.Compile([]byte(src))
	if err != nil {
		t.Fatalf("Compile() = %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Compile() warnings = %v, want none", warnings)
	}
	var got []headingLabel
	for c := range value.All(doc.Body) {
		h, ok := c.(*value.Heading)
		if !ok {
			continue
		}
		if h.Label == nil {
			t.Errorf("heading %s has no label", value.FormatContent(h))
			continue
		}
		got = append(got, headingLabel{h.Label.Name.String(), h.Label.Auto})
	}
	return got
}

func TestAutoHeadingLabels(t *testing.T) {
	auto := func(names ...string) []headingLabel {
		out := make([]headingLabel, len(names))
		for i, n := range names {
			out[i] = headingLabel{n, true}
		}
		return out
	}

	tests := []struct {
		name string
		src  string
		want []headingLabel
	}{{
		name: "plain",
		src:  "= Hello World\n\n== Hello, World!\n",
		want: auto("hello-world", "hello-world-1"),
	}, {
		name: "markup in the heading contributes its text",
		src:  "= Hello *world*\n\n= Hello `code`\n",
		want: auto("hello-world", "hello-code"),
	}, {
		// goldmark slugs the source line, so the URL lands in its id
		// (`a-linkhttpx`). Realized text gives the better answer.
		name: "link contributes only its text",
		src:  "= A #link(\"http://x\")[link]\n",
		want: auto("a-link"),
	}, {
		name: "footnote text is not part of the title",
		src:  "= Note#footnote[Consider the counterexample.]\n",
		want: auto("note"),
	}, {
		name: "nothing to slug",
		src:  "= !!!\n\n= ???\n",
		want: auto("heading", "heading-1"),
	}, {
		name: "duplicates are numbered from one",
		src:  "= Intro\n\n= Intro\n\n= Intro\n",
		want: auto("intro", "intro-1", "intro-2"),
	}, {
		name: "non-ascii is kept",
		src:  "= Über uns\n\n= 日本語\n",
		want: auto("über-uns", "日本語"),
	}, {
		name: "a label in the source is left alone",
		src:  "= Introduction <intro>\n\n= Conclusion\n",
		want: []headingLabel{{"intro", false}, {"conclusion", true}},
	}, {
		// The explicit label is reserved even though it sits further down the
		// document than the heading that would have taken the name.
		name: "a name the source uses elsewhere is not taken",
		src:  "= Intro\n\nSome prose. <intro>\n",
		want: auto("intro-1"),
	}, {
		// The label is derived after show rules run, so it describes the
		// heading the reader ends up with.
		name: "show rules are applied first",
		src:  "#show heading: it => [= Renamed]\n= Original\n",
		want: auto("renamed"),
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headingLabels(t, tt.src)
			if len(got) != len(tt.want) {
				t.Fatalf("labels = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("label %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestAutoHeadingLabelIsNotAReference pins the one thing a generated label is
// not: part of the document's namespace. `@ref` resolves what the source wrote,
// and nothing else — a document that means to reference a section says so by
// labelling it.
func TestAutoHeadingLabelIsNotAReference(t *testing.T) {
	_, _, err := markst.Compile([]byte("= Hello World\n\nSee @hello-world.\n"))
	if err == nil {
		t.Fatalf("Compile() = nil error, want one about a missing label")
	}
	if msg := err.Error(); !strings.Contains(msg, "does not exist in the document") {
		t.Errorf("Compile() error = %q, want one about a missing label", msg)
	}
}
