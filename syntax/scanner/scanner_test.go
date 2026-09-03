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

package scanner

import (
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"znkr.io/markst/syntax"
)

var nodeCmpOpts = cmp.Options{cmp.AllowUnexported(syntax.Leaf{}, syntax.Inner{}, syntax.Error{})}

// nodeBuilder tracks offsets to generate proper spans for test expectations.
type nodeBuilder struct {
	offset uint32
}

func (b *nodeBuilder) leaf(kind syntax.Kind, literal string) syntax.Node {
	start := b.offset
	b.offset += uint32(len(literal))
	return syntax.NewLeaf(kind, syntax.Span{Start: start, End: b.offset})
}

func (b *nodeBuilder) inner(kind syntax.Kind, children []syntax.Node) syntax.Node {
	return syntax.NewInner(kind, children)
}

func (b *nodeBuilder) err(msg string, literal string) syntax.Node {
	start := b.offset
	b.offset += uint32(len(literal))
	return syntax.NewError(syntax.Span{Start: start, End: b.offset}, msg)
}

func (b *nodeBuilder) errWithHints(msg string, literal string, hints []string) syntax.Node {
	start := b.offset
	b.offset += uint32(len(literal))
	return syntax.NewError(syntax.Span{Start: start, End: b.offset}, msg, hints...)
}

func TestScanner_MarkupMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func(b *nodeBuilder) []syntax.Node
	}{
		{
			name:  "simple_text",
			input: "Hello",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "Hello"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "whitespace",
			input: "Hello world\n\nParagraph\n  Next",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "Hello world"),
					b.leaf(syntax.KindParbreak, "\n\n"),
					b.leaf(syntax.KindText, "Paragraph"),
					b.leaf(syntax.KindSpace, "\n  "),
					b.leaf(syntax.KindText, "Next"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "text_with_inline_link",
			input: "Check https://example.com/ foo",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "Check "),
					b.leaf(syntax.KindLink, "https://example.com/"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "foo"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "backslash_newline",
			input: "\\\n",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLinebreak, "\\"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "backslash_escape_char",
			input: "\\*word",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEscape, "\\*"),
					b.leaf(syntax.KindText, "word"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Unicode escapes
		{
			name:  "unicode_escape",
			input: "\\u{41}",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEscape, "\\u{41}"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unicode_escape_followed_by_text",
			input: "\\u{41}B",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEscape, "\\u{41}"),
					b.leaf(syntax.KindText, "B"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unicode_escape_invalid_hex",
			input: "\\u{ZZ}",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid Unicode escape sequence", "\\u{ZZ}"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unicode_escape_unclosed",
			input: "\\u{41",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed Unicode escape sequence", "\\u{41"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "line_comment",
			input: "// comment\ntext",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLineComment, "// comment"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindText, "text"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "block_comment",
			input: "/* comment */",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindBlockComment, "/* comment */"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "nested_block_comment",
			input: "/* outer /* inner */ */",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindBlockComment, "/* outer /* inner */ */"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unmatched_end_block_comment",
			input: "*/",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.errWithHints("unexpected end of block comment", "*/", []string{"consider escaping the `*` with a backslash or opening the block comment with `/*`"}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unterminated_block_comment",
			input: "/* foo",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unterminated multiline comment", "/* foo"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "formatted_text",
			input: "*bold* _italic_",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStar, "*"),
					b.leaf(syntax.KindText, "bold"),
					b.leaf(syntax.KindStar, "*"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindUnderscore, "_"),
					b.leaf(syntax.KindText, "italic"),
					b.leaf(syntax.KindUnderscore, "_"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "markers",
			input: "- item\n+ enum\n/ term\n= header",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindListMarker, "-"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "item"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindEnumMarker, "+"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "enum"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindTermMarker, "/"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "term"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindHeadingMarker, "="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "header"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "punctuation",
			input: "[]'\"$: #",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLeftBracket, "["),
					b.leaf(syntax.KindRightBracket, "]"),
					b.leaf(syntax.KindSmartQuote, "'"),
					b.leaf(syntax.KindSmartQuote, "\""),
					b.leaf(syntax.KindDollar, "$"),
					b.leaf(syntax.KindColon, ":"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindHash, "#"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "links",
			input: "http://example.com https://example.org",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLink, "http://example.com"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindLink, "https://example.org"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "link_unbalanced",
			input: "http://(com",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("automatic links cannot contain unbalanced brackets, use 'link' function instead", "http://(com"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "labels_refs",
			input: "<lbl> @ref",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLabel, "<lbl>"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindRefMarker, "@ref"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "label_unclosed",
			input: "<lbl",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed label, expected '>'", "<lbl"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "label_empty",
			input: "<>",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("label cannot be empty", "<>"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "shorthands",
			input: "... -- --- -? ~ ..",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindShorthand, "..."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindShorthand, "-"),
					b.leaf(syntax.KindListMarker, "-"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindShorthand, "---"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindShorthand, "-"),
					b.leaf(syntax.KindText, "?"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindShorthand, "~"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, ".."),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "text_interruption",
			input: "word*word",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "word"),
					b.leaf(syntax.KindText, "*word"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Numbering tests
		{
			name:  "numbering_simple",
			input: "1. item",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "1."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "item"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_multi_digit",
			input: "123. item",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "123."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "item"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_at_end",
			input: "42.",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "42."),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_followed_by_newline",
			input: "5.\ntext",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "5."),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindText, "text"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_no_space_is_text",
			input: "1.23",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "1.23"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_no_dot_is_text",
			input: "42 items",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "42 items"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_multiple",
			input: "1. first\n2. second",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "1."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "first"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindEnumMarker, "2."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "second"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_single_digit_at_end",
			input: "1.",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "1."),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numbering_zero",
			input: "0. item",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEnumMarker, "0."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "item"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "link_trailing_punctuation",
			input: "http://example.com.",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLink, "http://example.com"),
					b.leaf(syntax.KindText, "."),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "shorthand_hyphen_number",
			input: "-1",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindShorthand, "-"),
					b.leaf(syntax.KindText, "1"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "backslash_eof",
			input: "\\",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLinebreak, "\\"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "backslash_space",
			input: "\\ ",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLinebreak, "\\"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Raw text (inline)
		{
			name:  "raw_inline_simple",
			input: "`code`",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "`"),
						b.leaf(syntax.KindText, "code"),
						b.leaf(syntax.KindRawDelim, "`"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_inline_with_spaces",
			input: "`hello world`",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "`"),
						b.leaf(syntax.KindText, "hello world"),
						b.leaf(syntax.KindRawDelim, "`"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_inline_empty",
			input: "``",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "`"),
						b.leaf(syntax.KindRawDelim, "`"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_inline_with_newline",
			input: "`line1\nline2`",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "`"),
						b.leaf(syntax.KindText, "line1"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "line2"),
						b.leaf(syntax.KindRawDelim, "`"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_in_text",
			input: "hello `code` world",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindText, "hello"),
					b.leaf(syntax.KindSpace, " "),
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "`"),
						b.leaf(syntax.KindText, "code"),
						b.leaf(syntax.KindRawDelim, "`"),
					}),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindText, "world"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_unclosed",
			input: "`code",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed raw text", "`code"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Raw text (block - 3+ backticks)
		// Note: in block raw, text immediately after opening backticks is the language tag
		{
			name:  "raw_block_lang_only",
			input: "```code```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawLang, "code"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_with_lang",
			input: "```rust\nfn main() {}\n```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawLang, "rust"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "fn main() {}"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_leading_space_trimmed",
			input: "```typ let x = 1```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawLang, "typ"),
						b.leaf(syntax.KindRawTrimmed, " "),
						b.leaf(syntax.KindText, "let x = 1"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// When closing delimiter has no indent, dedent is 0
		{
			name:  "raw_block_no_dedent",
			input: "```\n  line1\n  line2\n```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "  line1"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "  line2"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// When closing delimiter is indented, dedent is calculated
		{
			name:  "raw_block_dedent",
			input: "```\n  line1\n  line2\n  ```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindText, "line1"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindText, "line2"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_4_backticks",
			input: "````code````",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "````"),
						b.leaf(syntax.KindRawLang, "code"),
						b.leaf(syntax.KindRawDelim, "````"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_nested_backticks",
			input: "````\n```code```\n````",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "````"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "```code```"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "````"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_unclosed",
			input: "```code",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed raw text", "```code"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_trailing_space_before_backtick",
			input: "``` x` ```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, " "),
						b.leaf(syntax.KindText, "x`"),
						b.leaf(syntax.KindRawTrimmed, " "), // trailing space before backtick is also trimmed
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Mixed indent with no closing indent = no dedent
		{
			name:  "raw_block_mixed_indent_no_dedent",
			input: "```\n    line1\n  line2\n```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "    line1"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindText, "  line2"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Mixed indent with closing indent = minimum dedent
		{
			name:  "raw_block_mixed_indent",
			input: "```\n    line1\n  line2\n  ```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindText, "  line1"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindText, "line2"),
						b.leaf(syntax.KindRawTrimmed, "\n  "),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// 6 backticks opens a raw block that needs 6 backticks to close (not 3+3)
		{
			name:  "raw_block_6_backticks_unclosed",
			input: "``````",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed raw text", "``````"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_empty",
			input: "```\n```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "raw_block_whitespace_first_line",
			input: "```   \ncode\n```",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.inner(syntax.KindRaw, []syntax.Node{
						b.leaf(syntax.KindRawDelim, "```"),
						b.leaf(syntax.KindRawTrimmed, "   \n"),
						b.leaf(syntax.KindText, "code"),
						b.leaf(syntax.KindRawTrimmed, "\n"),
						b.leaf(syntax.KindRawDelim, "```"),
					}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// interesting edge cases
		{
			name:  "array_bad_tokens",
			input: "#(1*/2)",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindHash, "#"),
					b.leaf(syntax.KindText, "(1"),
					b.errWithHints("unexpected end of block comment", "*/", []string{"consider escaping the `*` with a backslash or opening the block comment with `/*`"}),
					b.leaf(syntax.KindText, "2)"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New([]byte(tt.input))
			var got []syntax.Node
			for {
				kind, node := s.Next()
				got = append(got, node)
				if kind == syntax.KindEnd {
					break
				}
			}

			b := &nodeBuilder{}
			expected := tt.expected(b)

			if diff := cmp.Diff(expected, got, nodeCmpOpts); diff != "" {
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
			s := New([]byte(tt.input))
			var got []bool
			for {
				kind, _ := s.Next()
				got = append(got, s.Newline())
				if kind == syntax.KindEnd {
					break
				}
			}

			if diff := cmp.Diff(tt.expected, got, nodeCmpOpts); diff != "" {
				t.Errorf("Newline() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanner_CodeMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func(b *nodeBuilder) []syntax.Node
	}{
		// Delimiters
		{
			name:  "delimiters",
			input: "{}[](),;:",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLeftBrace, "{"),
					b.leaf(syntax.KindRightBrace, "}"),
					b.leaf(syntax.KindLeftBracket, "["),
					b.leaf(syntax.KindRightBracket, "]"),
					b.leaf(syntax.KindLeftParen, "("),
					b.leaf(syntax.KindRightParen, ")"),
					b.leaf(syntax.KindComma, ","),
					b.leaf(syntax.KindSemicolon, ";"),
					b.leaf(syntax.KindColon, ":"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Operators
		{
			name:  "arithmetic_operators",
			input: "+ - * /",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindPlus, "+"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindMinus, "-"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindStar, "*"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindSlash, "/"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "comparison_operators",
			input: "== != < <= > >=",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEqEq, "=="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindExclEq, "!="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindLt, "<"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindLtEq, "<="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindGt, ">"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindGtEq, ">="),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "assignment_operators",
			input: "= += -= *= /=",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindEq, "="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindPlusEq, "+="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindHyphEq, "-="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindStarEq, "*="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindSlashEq, "/="),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "special_operators",
			input: "=> .. .",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindArrow, "=>"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindDots, ".."),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindDot, "."),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Keywords
		{
			name:  "keywords",
			input: "let set show if else for in while",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLet, "let"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindSet, "set"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindShow, "show"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindIf, "if"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindElse, "else"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindFor, "for"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindIn, "in"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindWhile, "while"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "more_keywords",
			input: "break continue return import include as context",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindBreak, "break"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindContinue, "continue"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindReturn, "return"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindImport, "import"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInclude, "include"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindAs, "as"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindContext, "context"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "boolean_operators",
			input: "not and or",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindNot, "not"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindAnd, "and"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindOr, "or"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Literals
		{
			name:  "literals",
			input: "none auto true false",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindNone, "none"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindAuto, "auto"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindBool, "true"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindBool, "false"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Identifiers
		{
			name:  "identifiers",
			input: "foo bar_baz _private",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindIdent, "foo"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindIdent, "bar_baz"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindIdent, "_private"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "underscore_alone",
			input: "_ + _",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindUnderscore, "_"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindPlus, "+"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindUnderscore, "_"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Numbers
		{
			name:  "integers",
			input: "0 42 123",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindInt, "0"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "42"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "123"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "floats",
			input: "3.14 0.5 .25",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindFloat, "3.14"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindFloat, "0.5"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindFloat, ".25"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "scientific_notation",
			input: "1.2e3 1e-4 1E+5",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindFloat, "1.2e3"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindFloat, "1e-4"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindFloat, "1E+5"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "scientific_notation_invalid_suffix",
			input: "1.2e",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid floating point number: 1.2e", "1.2e"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "hex_octal_binary",
			input: "0xff 0o77 0b101",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindInt, "0xff"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "0o77"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "0b101"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "numeric_with_units",
			input: "12pt 3.5em 90deg 50%",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindNumeric, "12pt"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindNumeric, "3.5em"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindNumeric, "90deg"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindNumeric, "50%"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "number_followed_by_dot",
			input: "1.foo",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindInt, "1"),
					b.leaf(syntax.KindDot, "."),
					b.leaf(syntax.KindIdent, "foo"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Strings
		{
			name:  "string_simple",
			input: `"hello"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"hello"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_with_escapes",
			input: `"hello\nworld"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"hello\nworld"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unicode",
			input: `"\u{1F600}"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"\u{1F600}"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unicode_empty",
			input: `"\u{}"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid unicode escape sequence", `"\u{}"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unicode_not_hex",
			input: `"\u{zz}"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid unicode escape sequence", `"\u{zz}"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unicode_out_of_range",
			input: `"\u{110000}"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid unicode escape sequence", `"\u{110000}"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unicode_surrogate",
			input: `"\u{d800}"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid unicode escape sequence", `"\u{d800}"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_escaped_single_quote",
			input: `"\'"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"\'"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_escaped_quote",
			input: `"\""`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"\""`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_escaped_backslash",
			input: `"\\"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindStr, `"\\"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_invalid_escape",
			input: `"\z"`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid escape sequence", `"\z"`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "string_unclosed",
			input: `"hello`,
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unclosed string", `"hello`),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Labels in code mode
		{
			name:  "label_in_code",
			input: "<my-label>",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLabel, "<my-label>"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Comments work in code mode too
		{
			name:  "comments_in_code",
			input: "x // comment\ny",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindIdent, "x"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindLineComment, "// comment"),
					b.leaf(syntax.KindSpace, "\n"),
					b.leaf(syntax.KindIdent, "y"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		// Mixed expression
		{
			name:  "expression",
			input: "let x = 1 + 2",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLet, "let"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindIdent, "x"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindEq, "="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "1"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindPlus, "+"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindInt, "2"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "invalid_number_suffix",
			input: "12invalid",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid number suffix: invalid", "12invalid"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "hex_with_suffix",
			input: "0x12pt",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid hexadecimal number: 0x12pt", "0x12pt"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "invalid_hex",
			input: "0xG",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("invalid hexadecimal number: 0xG", "0xG"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "unexpected_char",
			input: "%",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.err("unexpected character: %", "%"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New([]byte(tt.input))
			s.SetMode(syntax.ModeCode)
			var got []syntax.Node
			for {
				kind, node := s.Next()
				got = append(got, node)
				if kind == syntax.KindEnd {
					break
				}
			}

			b := &nodeBuilder{}
			expected := tt.expected(b)

			if diff := cmp.Diff(expected, got, nodeCmpOpts); diff != "" {
				t.Errorf("Scan() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanner_MathMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected func(b *nodeBuilder) []syntax.Node
	}{
		{
			name:  "superscript",
			input: "x^2",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "x"),
					b.leaf(syntax.KindHat, "^"),
					b.leaf(syntax.KindMathText, "2"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "attach",
			input: "a_1^2",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "a"),
					b.leaf(syntax.KindUnderscore, "_"),
					b.leaf(syntax.KindMathText, "1"),
					b.leaf(syntax.KindHat, "^"),
					b.leaf(syntax.KindMathText, "2"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "ident",
			input: "pi",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathIdent, "pi"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "field_access",
			input: "arrow.r",
			expected: func(b *nodeBuilder) []syntax.Node {
				arrow := b.leaf(syntax.KindMathIdent, "arrow")
				dot := b.leaf(syntax.KindDot, ".")
				r := b.leaf(syntax.KindIdent, "r")
				return []syntax.Node{
					b.inner(syntax.KindFieldAccess, []syntax.Node{arrow, dot, r}),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "fraction",
			input: "x/2",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "x"),
					b.leaf(syntax.KindSlash, "/"),
					b.leaf(syntax.KindMathText, "2"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "primes",
			input: "a'''",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "a"),
					b.leaf(syntax.KindMathPrimes, "'''"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "shorthand_leq",
			input: "a <= b",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "a"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindMathShorthand, "<="),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindMathText, "b"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "number",
			input: "123.45",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindMathText, "123.45"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "root_and_align",
			input: "√x &",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindRoot, "√"),
					b.leaf(syntax.KindMathText, "x"),
					b.leaf(syntax.KindSpace, " "),
					b.leaf(syntax.KindMathAlignPoint, "&"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
		{
			name:  "lr_delimiters",
			input: "[|x|]",
			expected: func(b *nodeBuilder) []syntax.Node {
				return []syntax.Node{
					b.leaf(syntax.KindLeftBrace, "[|"),
					b.leaf(syntax.KindMathText, "x"),
					b.leaf(syntax.KindRightBrace, "|]"),
					b.leaf(syntax.KindEnd, ""),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New([]byte(tt.input))
			s.SetMode(syntax.ModeMath)
			var got []syntax.Node
			for {
				kind, node := s.Next()
				got = append(got, node)
				if kind == syntax.KindEnd {
					break
				}
			}

			b := &nodeBuilder{}
			expected := tt.expected(b)

			if diff := cmp.Diff(expected, got, nodeCmpOpts); diff != "" {
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
			s := New([]byte(input))
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
			s2 := New([]byte(input))
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
			s3 := New([]byte(input))
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
	src := []byte(code)
	s := New(src)
	text := func(n syntax.Node) string { return string(syntax.Text(src, n)) }

	if c := s.Column(); c != 0 {
		t.Errorf("initial Column() = %d, want 0", c)
	}

	// Scan 'h'
	kind, val := s.Next()
	if kind != syntax.KindText || text(val) != "h" {
		t.Fatalf("expected 'h', got %v %q", kind, text(val))
	}
	if c := s.Column(); c != 1 {
		t.Errorf("after 'h' Column() = %d, want 1", c)
	}

	// Scan '\n'
	kind, val = s.Next()
	if kind != syntax.KindSpace || text(val) != "\n" {
		t.Fatalf("expected '\\n', got %v %q", kind, text(val))
	}
	if c := s.Column(); c != 0 {
		t.Errorf("after '\\n' Column() = %d, want 0", c)
	}

	// Scan 'e' (part of "ello" text)
	// scanText scans the whole text block "ello".
	kind, val = s.Next()
	if kind != syntax.KindText || text(val) != "ello" {
		t.Fatalf("expected 'ello', got %v %q", kind, text(val))
	}
	// 'ello' length 4. Start at 0. 0+4 = 4. But EOF resets col to 0.
	if c := s.Column(); c != 0 {
		t.Errorf("after 'ello' Column() = %d, want 0", c)
	}
}
