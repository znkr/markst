package builtin

import "testing"

// TestSingleRune covers the inputs that no equation can produce but a string
// value can carry into `accent(x, s)`: an empty string and invalid UTF-8, both
// of which decode to RuneError and must not be mistaken for a single character.
func TestSingleRune(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want rune
		ok   bool
	}{
		{"ascii", "a", 'a', true},
		{"combining", "̂", '̂', true},
		{"text presentation stripped", "↔︎", '↔', true},
		{"replacement character", "�", '�', true},
		{"empty", "", 0, false},
		{"two runes", "ab", 0, false},
		{"emoji presentation kept", "↔️", 0, false},
		{"invalid utf-8", "\xff", 0, false},
		{"truncated utf-8", "\xe2\x86", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := singleRune(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Errorf("singleRune(%q) = %q, %v, want %q, %v", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}
