package scanner

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"znkr.io/writst/syntax"
)

func TestScanner_MarkupMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []syntax.Node
	}{
		{
			name:  "simple text",
			input: "Hello",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "Hello"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "whitespace",
			input: "Hello world\n\nParagraph\n  Next",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "Hello world"),
				syntax.Leaf(syntax.KindParbreak, "\n\n"),
				syntax.Leaf(syntax.KindText, "Paragraph"),
				syntax.Leaf(syntax.KindSpace, "\n  "),
				syntax.Leaf(syntax.KindText, "Next"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "text with inline link",
			input: "Check https://example.com/ foo",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "Check "),
				syntax.Leaf(syntax.KindLink, "https://example.com/"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "foo"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "backslash_newline",
			input: "\\\n",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLinebreak, "\\"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "backslash_escape_char",
			input: "\\*word",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEscape, "\\*"),
				syntax.Leaf(syntax.KindText, "word"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Unicode escapes
		{
			name:  "unicode_escape",
			input: "\\u{41}",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEscape, "\\u{41}"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unicode_escape_followed_by_text",
			input: "\\u{41}B",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEscape, "\\u{41}"),
				syntax.Leaf(syntax.KindText, "B"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unicode_escape_invalid_hex",
			input: "\\u{ZZ}",
			expected: []syntax.Node{
				syntax.Error("invalid Unicode escape sequence", "\\u{ZZ}"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unicode_escape_unclosed",
			input: "\\u{41",
			expected: []syntax.Node{
				syntax.Error("unclosed Unicode escape sequence", "\\u{41"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "line_comment",
			input: "// comment\ntext",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLineComment, "// comment"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindText, "text"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "block_comment",
			input: "/* comment */",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindBlockComment, "/* comment */"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "nested_block_comment",
			input: "/* outer /* inner */ */",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindBlockComment, "/* outer /* inner */ */"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unmatched_end_block_comment",
			input: "*/",
			expected: []syntax.Node{
				{
					Kind: syntax.KindError, Value: &syntax.ErrorValue{
						Message: "unmatched end of multiline comment",
						Hints:   []string{"consider escaping the `*` with a backslash or opening the block comment with `/*`"},
						Literal: "*/",
					},
				},
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unterminated_block_comment",
			input: "/* foo",
			expected: []syntax.Node{
				syntax.Error("unterminated multiline comment", "/* foo"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "formatted_text",
			input: "*bold* _italic_",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStar, "*"),
				syntax.Leaf(syntax.KindText, "bold"),
				syntax.Leaf(syntax.KindStar, "*"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindUnderscore, "_"),
				syntax.Leaf(syntax.KindText, "italic"),
				syntax.Leaf(syntax.KindUnderscore, "_"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "markers",
			input: "- item\n+ enum\n/ term\n= header",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindListMarker, "-"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "item"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindEnumMarker, "+"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "enum"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindTermMarker, "/"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "term"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindHeadingMarker, "="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "header"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "punctuation",
			input: "[]'\"$: #",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLeftBracket, "["),
				syntax.Leaf(syntax.KindRightBracket, "]"),
				syntax.Leaf(syntax.KindSmartQuote, "'"),
				syntax.Leaf(syntax.KindSmartQuote, "\""),
				syntax.Leaf(syntax.KindDollar, "$"),
				syntax.Leaf(syntax.KindColon, ":"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindHash, "#"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "links",
			input: "http://example.com https://example.org",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLink, "http://example.com"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindLink, "https://example.org"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "link_unbalanced",
			input: "http://(com",
			expected: []syntax.Node{
				syntax.Error("automatic links cannot contain unbalanced brackets, use 'link' function instead", "http://(com"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "labels_refs",
			input: "<lbl> @ref",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLabel, "<lbl>"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindRefMarker, "@ref"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "label_unclosed",
			input: "<lbl",
			expected: []syntax.Node{
				syntax.Error("unclosed label, expected '>'", "<lbl"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "label_empty",
			input: "<>",
			expected: []syntax.Node{
				syntax.Error("label cannot be empty", "<>"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "shorthands",
			input: "... -- --- -? ~ ..",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindShorthand, "..."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindShorthand, "-"),
				syntax.Leaf(syntax.KindListMarker, "-"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindShorthand, "---"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindShorthand, "-"),
				syntax.Leaf(syntax.KindText, "?"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindShorthand, "~"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, ".."),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "text_interruption",
			input: "word*word",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "word"),
				syntax.Leaf(syntax.KindText, "*word"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Numbering tests
		{
			name:  "numbering_simple",
			input: "1. item",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "1."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "item"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_multi_digit",
			input: "123. item",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "123."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "item"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_at_end",
			input: "42.",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "42."),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_followed_by_newline",
			input: "5.\ntext",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "5."),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindText, "text"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_no_space_is_text",
			input: "1.23",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "1.23"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_no_dot_is_text",
			input: "42 items",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "42 items"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_multiple",
			input: "1. first\n2. second",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "1."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "first"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindEnumMarker, "2."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "second"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_single_digit_at_end",
			input: "1.",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "1."),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numbering_zero",
			input: "0. item",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEnumMarker, "0."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "item"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "link_trailing_punctuation",
			input: "http://example.com.",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLink, "http://example.com"),
				syntax.Leaf(syntax.KindText, "."),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "shorthand_hyphen_number",
			input: "-1",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindShorthand, "-"),
				syntax.Leaf(syntax.KindText, "1"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "backslash_eof",
			input: "\\",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLinebreak, "\\"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "backslash_space",
			input: "\\ ",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLinebreak, "\\"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Raw text (inline)
		{
			name:  "raw_inline_simple",
			input: "`code`",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "`"),
					syntax.Leaf(syntax.KindText, "code"),
					syntax.Leaf(syntax.KindRawDelim, "`"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_inline_with_spaces",
			input: "`hello world`",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "`"),
					syntax.Leaf(syntax.KindText, "hello world"),
					syntax.Leaf(syntax.KindRawDelim, "`"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_inline_empty",
			input: "``",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "`"),
					syntax.Leaf(syntax.KindRawDelim, "`"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_inline_with_newline",
			input: "`line1\nline2`",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "`"),
					syntax.Leaf(syntax.KindText, "line1"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "line2"),
					syntax.Leaf(syntax.KindRawDelim, "`"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_in_text",
			input: "hello `code` world",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindText, "hello"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "`"),
					syntax.Leaf(syntax.KindText, "code"),
					syntax.Leaf(syntax.KindRawDelim, "`"),
				}),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindText, "world"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_unclosed",
			input: "`code",
			expected: []syntax.Node{
				syntax.Error("unclosed raw text", "`code"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Raw text (block - 3+ backticks)
		// Note: in block raw, text immediately after opening backticks is the language tag
		{
			name:  "raw_block_lang_only",
			input: "```code```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawLang, "code"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_with_lang",
			input: "```rust\nfn main() {}\n```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawLang, "rust"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "fn main() {}"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_leading_space_trimmed",
			input: "```typ let x = 1```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawLang, "typ"),
					syntax.Leaf(syntax.KindRawTrimmed, " "),
					syntax.Leaf(syntax.KindText, "let x = 1"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// When closing delimiter has no indent, dedent is 0
		{
			name:  "raw_block_no_dedent",
			input: "```\n  line1\n  line2\n```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "  line1"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "  line2"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// When closing delimiter is indented, dedent is calculated
		{
			name:  "raw_block_dedent",
			input: "```\n  line1\n  line2\n  ```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindText, "line1"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindText, "line2"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_4_backticks",
			input: "````code````",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "````"),
					syntax.Leaf(syntax.KindRawLang, "code"),
					syntax.Leaf(syntax.KindRawDelim, "````"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_nested_backticks",
			input: "````\n```code```\n````",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "````"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "```code```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "````"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_unclosed",
			input: "```code",
			expected: []syntax.Node{
				syntax.Error("unclosed raw text", "```code"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_trailing_space_before_backtick",
			input: "``` x` ```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, " "),
					syntax.Leaf(syntax.KindText, "x`"),
					syntax.Leaf(syntax.KindRawTrimmed, " "), // trailing space before backtick is also trimmed
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Mixed indent with no closing indent = no dedent
		{
			name:  "raw_block_mixed_indent_no_dedent",
			input: "```\n    line1\n  line2\n```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "    line1"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindText, "  line2"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Mixed indent with closing indent = minimum dedent
		{
			name:  "raw_block_mixed_indent",
			input: "```\n    line1\n  line2\n  ```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindText, "  line1"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindText, "line2"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n  "),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// 6 backticks opens a raw block that needs 6 backticks to close (not 3+3)
		{
			name:  "raw_block_6_backticks_unclosed",
			input: "``````",
			expected: []syntax.Node{
				syntax.Error("unclosed raw text", "``````"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_empty",
			input: "```\n```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "raw_block_whitespace_first_line",
			input: "```   \ncode\n```",
			expected: []syntax.Node{
				syntax.Inner(syntax.KindRaw, []syntax.Node{
					syntax.Leaf(syntax.KindRawDelim, "```"),
					syntax.Leaf(syntax.KindRawTrimmed, "   \n"),
					syntax.Leaf(syntax.KindText, "code"),
					syntax.Leaf(syntax.KindRawTrimmed, "\n"),
					syntax.Leaf(syntax.KindRawDelim, "```"),
				}),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.input)
			var got []syntax.Node
			for {
				kind, node := s.Next()
				got = append(got, node)
				if kind == syntax.KindEnd {
					break
				}
			}

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Scan() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanner_Newline(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []bool
	}{
		{
			name:     "no_newlines",
			input:    "hello",
			expected: []bool{false, false},
		},
		{
			name:     "single_newline",
			input:    "hello\nworld",
			expected: []bool{false, true, false, false},
		},
		{
			name:     "parbreak",
			input:    "hello\n\nworld",
			expected: []bool{false, true, false, false},
		},
		{
			name:     "space_without_newline",
			input:    "a  ",
			expected: []bool{false, false, false},
		},
		{
			name:     "carriage_return",
			input:    "a\rb",
			expected: []bool{false, true, false, false},
		},
		{
			name:     "crlf",
			input:    "a\r\nb",
			expected: []bool{false, true, false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.input)
			var got []bool
			for {
				kind, _ := s.Next()
				got = append(got, s.Newline())
				if kind == syntax.KindEnd {
					break
				}
			}

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Newline() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanner_CodeMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []syntax.Node
	}{
		// Delimiters
		{
			name:  "delimiters",
			input: "{}[](),;:",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLeftBrace, "{"),
				syntax.Leaf(syntax.KindRightBrace, "}"),
				syntax.Leaf(syntax.KindLeftBracket, "["),
				syntax.Leaf(syntax.KindRightBracket, "]"),
				syntax.Leaf(syntax.KindLeftParen, "("),
				syntax.Leaf(syntax.KindRightParen, ")"),
				syntax.Leaf(syntax.KindComma, ","),
				syntax.Leaf(syntax.KindSemicolon, ";"),
				syntax.Leaf(syntax.KindColon, ":"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Operators
		{
			name:  "arithmetic_operators",
			input: "+ - * /",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindPlus, "+"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindMinus, "-"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindStar, "*"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindSlash, "/"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "comparison_operators",
			input: "== != < <= > >=",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEqEq, "=="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindExclEq, "!="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindLt, "<"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindLtEq, "<="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindGt, ">"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindGtEq, ">="),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "assignment_operators",
			input: "= += -= *= /=",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindEq, "="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindPlusEq, "+="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindHyphEq, "-="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindStarEq, "*="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindSlashEq, "/="),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "special_operators",
			input: "=> .. .",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindArrow, "=>"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindDots, ".."),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindDot, "."),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Keywords
		{
			name:  "keywords",
			input: "let set show if else for in while",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLet, "let"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindSet, "set"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindShow, "show"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindIf, "if"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindElse, "else"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindFor, "for"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindIn, "in"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindWhile, "while"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "more_keywords",
			input: "break continue return import include as context",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindBreak, "break"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindContinue, "continue"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindReturn, "return"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindImport, "import"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInclude, "include"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindAs, "as"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindContext, "context"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "boolean_operators",
			input: "not and or",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindNot, "not"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindAnd, "and"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindOr, "or"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Literals
		{
			name:  "literals",
			input: "none auto true false",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindNone, "none"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindAuto, "auto"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindBool, "true"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindBool, "false"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Identifiers
		{
			name:  "identifiers",
			input: "foo bar_baz _private",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindIdent, "foo"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindIdent, "bar_baz"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindIdent, "_private"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "underscore_alone",
			input: "_ + _",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindUnderscore, "_"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindPlus, "+"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindUnderscore, "_"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Numbers
		{
			name:  "integers",
			input: "0 42 123",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindInt, "0"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "42"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "123"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "floats",
			input: "3.14 0.5 .25",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindFloat, "3.14"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindFloat, "0.5"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindFloat, ".25"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "scientific_notation",
			input: "1.2e3 1e-4 1E+5",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindFloat, "1.2e3"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindFloat, "1e-4"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindFloat, "1E+5"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "scientific_notation_invalid_suffix",
			input: "1.2e",
			expected: []syntax.Node{
				syntax.Error("invalid number suffix: \"e\"", "1.2e"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "hex_octal_binary",
			input: "0xff 0o77 0b101",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindInt, "0xff"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "0o77"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "0b101"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "numeric_with_units",
			input: "12pt 3.5em 90deg 50%",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindNumeric, "12pt"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindNumeric, "3.5em"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindNumeric, "90deg"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindNumeric, "50%"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "number_followed_by_dot",
			input: "1.foo",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindInt, "1"),
				syntax.Leaf(syntax.KindDot, "."),
				syntax.Leaf(syntax.KindIdent, "foo"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Strings
		{
			name:  "string_simple",
			input: `"hello"`,
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStr, `"hello"`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_with_escapes",
			input: `"hello\nworld"`,
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStr, `"hello\nworld"`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_unicode",
			input: `"\u{1F600}"`,
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStr, `"\u{1F600}"`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_escaped_quote",
			input: `"\""`,
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStr, `"\""`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_escaped_backslash",
			input: `"\\"`,
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindStr, `"\\"`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_invalid_escape",
			input: `"\z"`,
			expected: []syntax.Node{
				syntax.Error("invalid escape sequence", `"\z"`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "string_unclosed",
			input: `"hello`,
			expected: []syntax.Node{
				syntax.Error("unclosed string", `"hello`),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Labels in code mode
		{
			name:  "label_in_code",
			input: "<my-label>",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLabel, "<my-label>"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Comments work in code mode too
		{
			name:  "comments_in_code",
			input: "x // comment\ny",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindIdent, "x"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindLineComment, "// comment"),
				syntax.Leaf(syntax.KindSpace, "\n"),
				syntax.Leaf(syntax.KindIdent, "y"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		// Mixed expression
		{
			name:  "expression",
			input: "let x = 1 + 2",
			expected: []syntax.Node{
				syntax.Leaf(syntax.KindLet, "let"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindIdent, "x"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindEq, "="),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "1"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindPlus, "+"),
				syntax.Leaf(syntax.KindSpace, " "),
				syntax.Leaf(syntax.KindInt, "2"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "invalid_number_suffix",
			input: "12invalid",
			expected: []syntax.Node{
				syntax.Error("invalid number suffix: \"invalid\"", "12invalid"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "hex_with_suffix",
			input: "0x12pt",
			expected: []syntax.Node{
				syntax.Error("invalid hexadecimal number", "0x12pt"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "invalid_hex",
			input: "0xG",
			expected: []syntax.Node{
				syntax.Error("invalid hexadecimal number", "0xG"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
		{
			name:  "unexpected_char",
			input: "%",
			expected: []syntax.Node{
				syntax.Error("unexpected character", "%"),
				syntax.Leaf(syntax.KindEnd, ""),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.input)
			s.SetMode(syntax.ModeCode)
			var got []syntax.Node
			for {
				kind, node := s.Next()
				got = append(got, node)
				if kind == syntax.KindEnd {
					break
				}
			}

			if diff := cmp.Diff(tt.expected, got); diff != "" {
				t.Errorf("Scan() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanner_Seek(t *testing.T) {
	inputs := []string{
		"hello\nworld",
		"a\n\nb",
		"one\ntwo\nthree",
		"*bold* _italic_",
		"// comment\ntext",
		"",
	}

	type record struct {
		offset  int
		kind    syntax.Kind
		newline bool
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// First pass: scan sequentially and record state before each token
			s := New(input)
			var records []record
			for {
				offset := s.Offset()
				kind, _ := s.Next()
				records = append(records, record{offset, kind, s.Newline()})
				if kind == syntax.KindEnd {
					break
				}
			}

			// Second pass: seek forward through all offsets and verify
			s2 := New(input)
			for _, rec := range records {
				s2.Seek(rec.offset)
				kind, _ := s2.Next()
				if kind != rec.kind {
					t.Errorf("forward Seek(%d): kind = %v, want %v", rec.offset, kind, rec.kind)
				}
				if s2.Newline() != rec.newline {
					t.Errorf("forward Seek(%d): Newline() = %v, want %v", rec.offset, s2.Newline(), rec.newline)
				}
			}

			// Third pass: seek backward through all offsets and verify
			s3 := New(input)
			for _, rec := range slices.Backward(records) {
				s3.Seek(rec.offset)
				kind, _ := s3.Next()
				if kind != rec.kind {
					t.Errorf("backward Seek(%d): kind = %v, want %v", rec.offset, kind, rec.kind)
				}
				if s3.Newline() != rec.newline {
					t.Errorf("backward Seek(%d): Newline() = %v, want %v", rec.offset, s3.Newline(), rec.newline)
				}
			}
		})
	}
}

func TestScanner_Column(t *testing.T) {
	code := "h\nello"
	s := New(code)

	if c := s.Column(); c != 0 {
		t.Errorf("initial Column() = %d, want 0", c)
	}

	// Scan 'h'
	kind, val := s.Next()
	if kind != syntax.KindText || val.Value.Text() != "h" {
		t.Fatalf("expected 'h', got %v %q", kind, val.Value.Text())
	}
	if c := s.Column(); c != 1 {
		t.Errorf("after 'h' Column() = %d, want 1", c)
	}

	// Scan '\n'
	kind, val = s.Next()
	if kind != syntax.KindSpace || val.Value.Text() != "\n" {
		t.Fatalf("expected '\\n', got %v %q", kind, val.Value.Text())
	}
	if c := s.Column(); c != 0 {
		t.Errorf("after '\\n' Column() = %d, want 0", c)
	}

	// Scan 'e' (part of "ello" text)
	// scanText scans the whole text block "ello".
	kind, val = s.Next()
	if kind != syntax.KindText || val.Value.Text() != "ello" {
		t.Fatalf("expected 'ello', got %v %q", kind, val.Value.Text())
	}
	// 'ello' length 4. Start at 0. 0+4 = 4. But EOF resets col to 0.
	if c := s.Column(); c != 0 {
		t.Errorf("after 'ello' Column() = %d, want 0", c)
	}
}
