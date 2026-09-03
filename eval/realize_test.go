package eval

import (
	"testing"

	"znkr.io/markst/value"
)

// TestMergeText pins what merging a run does with the whitespace markup
// composition duplicates at a seam, over runs longer than the two items a
// pairwise fold ever saw at once.
func TestMergeText(t *testing.T) {
	text := func(s string) value.Content { return &value.Text{Text: s} }
	labeled := func(s string) value.Content {
		return &value.Text{Text: s, Label: &value.Label{}}
	}

	tests := []struct {
		name  string
		items []value.Content
		want  []string
	}{
		{"pair", []value.Content{text("Hello"), text(" world")}, []string{"Hello world"}},
		{"seam", []value.Content{text("Hello "), text(" world")}, []string{"Hello world"}},
		{
			"run of words",
			[]value.Content{text("one"), text(" "), text("two"), text(" "), text("three")},
			[]string{"one two three"},
		},
		{
			"every seam doubled",
			[]value.Content{text("a "), text(" b "), text(" c")},
			[]string{"a b c"},
		},
		{
			"inner whitespace is untouched",
			[]value.Content{text("a    b"), text(" c")},
			[]string{"a    b c"},
		},
		{
			"a space that trims away leaves the run joined",
			[]value.Content{text("a "), text(" "), text("b")},
			[]string{"a b"},
		},
		{"single", []value.Content{text("alone")}, []string{"alone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeText(tt.items)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeText() = %v, want %v", texts(got), tt.want)
			}
			for i, want := range tt.want {
				if s := got[i].(*value.Text).Text; s != want {
					t.Errorf("mergeText()[%d] = %q, want %q", i, s, want)
				}
			}
		})
	}

	// A labeled text is named by its label, so it neither merges nor lets the
	// texts around it merge through it.
	t.Run("a label breaks the run", func(t *testing.T) {
		got := mergeText([]value.Content{text("a"), labeled("b"), text("c"), text("d")})
		if want := []string{"a", "b", "cd"}; len(got) != len(want) {
			t.Fatalf("mergeText() = %v, want %v", texts(got), want)
		}
	})

	// A run of one is the item itself: merging it would copy a node for nothing.
	t.Run("a run of one is not copied", func(t *testing.T) {
		only := text("alone")
		got := mergeText([]value.Content{only, &value.Parbreak{}})
		if got[0] != only {
			t.Errorf("mergeText() replaced a lone text with a copy")
		}
	})
}

func texts(items []value.Content) []string {
	out := make([]string, len(items))
	for i, it := range items {
		if t, ok := it.(*value.Text); ok {
			out[i] = t.Text
		} else {
			out[i] = it.Name()
		}
	}
	return out
}
