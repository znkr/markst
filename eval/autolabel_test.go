package eval

import (
	"testing"

	"znkr.io/markst/name"
)

// TestSlug pins the label name derived from a heading's text. Every case whose
// want is marked "= goldmark" was checked against goldmark v1.7.17 with
// parser.WithAutoHeadingID(): the same text as an ATX heading produces that id.
func TestSlug(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"Hello World", "hello-world"},                             // = goldmark
		{"Hello, World!", "hello-world"},                           // = goldmark
		{"Hello,  World!", "hello--world"},                         // = goldmark, runs are not collapsed
		{"snake_case and kebab-case", "snake-case-and-kebab-case"}, // = goldmark
		{"3 Little Pigs", "3-little-pigs"},                         // = goldmark
		{"What's new?", "whats-new"},                               // = goldmark
		{"C++ / Go", "c--go"},                                      // = goldmark
		{"A -- B", "a----b"},                                       // = goldmark
		{"---", "---"},                                             // = goldmark, separators alone are a name
		{"!!!", "heading"},                                         // = goldmark, nothing survives
		{"", "heading"},                                            // = goldmark
		{"   Spaced out   ", "spaced-out"},                         // = goldmark, edges trimmed first
		{"Tabs\tand\nnewlines", "tabs-and-newlines"},
		// goldmark drops multi-byte characters outright — `ber-uns` and
		// `heading`. Keeping them is the one deliberate divergence.
		{"Über uns", "über-uns"},
		{"日本語", "日本語"},
		{"Ünicode ÄÖÜ", "ünicode-äöü"},
	}
	for _, tt := range tests {
		if got := slug(tt.text); got != tt.want {
			t.Errorf("slug(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestIDPoolClaim(t *testing.T) {
	reserved := map[name.Name]struct{}{
		name.Make("taken"):   {},
		name.Make("intro-1"): {},
	}
	p := newIDPool(reserved)
	tests := []struct {
		id, want string
	}{
		{"intro", "intro"},
		{"intro", "intro-2"}, // "intro-1" is reserved
		{"intro", "intro-3"},
		{"taken", "taken-1"},
		{"heading", "heading"},
		{"heading", "heading-1"},
	}
	for _, tt := range tests {
		if got := p.claim(tt.id); got != tt.want {
			t.Errorf("claim(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}
