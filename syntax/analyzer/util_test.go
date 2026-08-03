package analyzer

import (
	"testing"
)

func TestUnquote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"abc"`, "abc"},
		{`"a"`, "a"},
		{`""`, ""},
		{`"a\nb"`, "a\nb"},
		{`"\t"`, "\t"},
		{`"\r"`, "\r"},
		{`"\\\""`, `\"`},
		{`"\u{61}"`, "a"},
		{`"\u{1F600}"`, "😀"},
		{`"a\u{20}b"`, "a b"},
		{`"\'"`, "'"},
		// Escapes the scanner rejects never reach unquote; it keeps them as
		// written rather than panicking.
		{`"\z"`, `\z`},
		{`"\u{}"`, `\u{}`},
		{`"\u"`, `\u`},
	}

	for _, tt := range tests {
		got := unquote(tt.input)
		if got != tt.want {
			t.Errorf("unquote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
