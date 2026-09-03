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
	"testing"

	"znkr.io/markst/syntax"
)

func TestSource_Position(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		newlines []uint32 // line offsets (start of each line, excluding the first line)
		offset   uint32
		expected syntax.Position
	}{
		{
			name:     "start_of_file",
			content:  "hello",
			newlines: nil,
			offset:   0,
			expected: syntax.Position{Line: 1, Column: 1},
		},
		{
			name:     "middle_of_first_line",
			content:  "hello",
			newlines: nil,
			offset:   2,
			expected: syntax.Position{Line: 1, Column: 3},
		},
		{
			name:     "start_of_second_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			offset:   6,
			expected: syntax.Position{Line: 2, Column: 1},
		},
		{
			name:     "middle_of_second_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			offset:   8,
			expected: syntax.Position{Line: 2, Column: 3},
		},
		{
			name:     "start_of_third_line",
			content:  "a\nb\nc",
			newlines: []uint32{2, 4},
			offset:   4,
			expected: syntax.Position{Line: 3, Column: 1},
		},
		{
			name:     "end_of_first_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			offset:   5,
			expected: syntax.Position{Line: 1, Column: 6},
		},
		{
			name:     "end_of_file_single_line",
			content:  "hello",
			newlines: nil,
			offset:   5,
			expected: syntax.Position{Line: 1, Column: 6},
		},
		{
			name:     "end_of_file_multi_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			offset:   11,
			expected: syntax.Position{Line: 2, Column: 6},
		},
		// Offset at newline character
		{
			name:     "at_newline_character",
			content:  "hello\nworld",
			newlines: []uint32{6},
			offset:   5, // the '\n' itself
			expected: syntax.Position{Line: 1, Column: 6},
		},
		{
			name:     "at_first_newline_multi_line",
			content:  "a\nb\nc",
			newlines: []uint32{2, 4},
			offset:   1, // first '\n'
			expected: syntax.Position{Line: 1, Column: 2},
		},
		{
			name:     "at_second_newline_multi_line",
			content:  "a\nb\nc",
			newlines: []uint32{2, 4},
			offset:   3, // second '\n'
			expected: syntax.Position{Line: 2, Column: 2},
		},
		// Multibyte UTF-8 tests
		{
			name:     "multibyte_start_of_file",
			content:  "日本語", // 3 chars, 9 bytes
			newlines: nil,
			offset:   0,
			expected: syntax.Position{Line: 1, Column: 1},
		},
		{
			name:     "multibyte_second_char",
			content:  "日本語", // offset 3 = start of 本
			newlines: nil,
			offset:   3,
			expected: syntax.Position{Line: 1, Column: 2},
		},
		{
			name:     "multibyte_third_char",
			content:  "日本語", // offset 6 = start of 語
			newlines: nil,
			offset:   6,
			expected: syntax.Position{Line: 1, Column: 3},
		},
		{
			name:     "multibyte_end_of_file",
			content:  "日本語", // offset 9 = end
			newlines: nil,
			offset:   9,
			expected: syntax.Position{Line: 1, Column: 4},
		},
		{
			name:     "mixed_ascii_and_multibyte",
			content:  "a日b", // a=1byte, 日=3bytes, b=1byte
			newlines: nil,
			offset:   4, // start of 'b'
			expected: syntax.Position{Line: 1, Column: 3},
		},
		{
			name:     "multibyte_second_line",
			content:  "hello\n世界", // 世界 = 2 chars, 6 bytes
			newlines: []uint32{6},
			offset:   9, // start of 界
			expected: syntax.Position{Line: 2, Column: 2},
		},
		{
			name:     "emoji",
			content:  "a🎉b", // 🎉 = 4 bytes
			newlines: nil,
			offset:   5, // start of 'b'
			expected: syntax.Position{Line: 1, Column: 3},
		},
		// Edge cases
		{
			name:     "empty_file",
			content:  "",
			newlines: nil,
			offset:   0,
			expected: syntax.Position{Line: 1, Column: 1},
		},
		{
			name:     "just_newline",
			content:  "\n",
			newlines: []uint32{1},
			offset:   0,
			expected: syntax.Position{Line: 1, Column: 1},
		},
		{
			name:     "just_newline_at_newline",
			content:  "\n",
			newlines: []uint32{1},
			offset:   1,
			expected: syntax.Position{Line: 2, Column: 1},
		},
		{
			name:     "empty_line_between",
			content:  "a\n\nb",
			newlines: []uint32{2, 3},
			offset:   2, // start of empty line
			expected: syntax.Position{Line: 2, Column: 1},
		},
		{
			name:     "empty_line_at_newline",
			content:  "a\n\nb",
			newlines: []uint32{2, 3},
			offset:   3, // start of 'b'
			expected: syntax.Position{Line: 3, Column: 1},
		},
		{
			name:     "single_char_file",
			content:  "x",
			newlines: nil,
			offset:   0,
			expected: syntax.Position{Line: 1, Column: 1},
		},
		{
			name:     "single_char_file_end",
			content:  "x",
			newlines: nil,
			offset:   1,
			expected: syntax.Position{Line: 1, Column: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &source{content: []byte(tt.content), newlines: tt.newlines}
			got := f.Position(tt.offset)
			if got != tt.expected {
				t.Errorf("Position(%d) = %+v, want %+v", tt.offset, got, tt.expected)
			}
			// Round-trip: Offset(Position(offset)) should return the original offset
			roundTrip := f.Offset(got)
			if roundTrip != tt.offset {
				t.Errorf("Offset(Position(%d)) = %d, want %d", tt.offset, roundTrip, tt.offset)
			}
		})
	}
}

func TestSource_Offset(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		newlines []uint32
		pos      syntax.Position
		expected uint32
	}{
		{
			name:     "zero_line",
			content:  "hello",
			newlines: nil,
			pos:      syntax.Position{Line: 0, Column: 1},
			expected: 0,
		},
		{
			name:     "zero_column",
			content:  "hello",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 0},
			expected: 0,
		},
		{
			name:     "both_zero",
			content:  "hello",
			newlines: nil,
			pos:      syntax.Position{Line: 0, Column: 0},
			expected: 0,
		},
		{
			name:     "start_of_file",
			content:  "hello",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 1},
			expected: 0,
		},
		{
			name:     "middle_of_first_line",
			content:  "hello",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 3},
			expected: 2,
		},
		{
			name:     "start_of_second_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			pos:      syntax.Position{Line: 2, Column: 1},
			expected: 6,
		},
		{
			name:     "middle_of_second_line",
			content:  "hello\nworld",
			newlines: []uint32{6},
			pos:      syntax.Position{Line: 2, Column: 3},
			expected: 8,
		},
		{
			name:     "multibyte_second_char",
			content:  "日本語",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 2},
			expected: 3,
		},
		{
			name:     "multibyte_third_char",
			content:  "日本語",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 3},
			expected: 6,
		},
		{
			name:     "mixed_ascii_and_multibyte",
			content:  "a日b",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 3},
			expected: 4,
		},
		{
			name:     "emoji",
			content:  "a🎉b",
			newlines: nil,
			pos:      syntax.Position{Line: 1, Column: 3},
			expected: 5,
		},
		{
			name:     "multibyte_second_line",
			content:  "hello\n世界",
			newlines: []uint32{6},
			pos:      syntax.Position{Line: 2, Column: 2},
			expected: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &source{content: []byte(tt.content), newlines: tt.newlines}
			got := f.Offset(tt.pos)
			if got != tt.expected {
				t.Errorf("Offset(%+v) = %d, want %d", tt.pos, got, tt.expected)
			}
		})
	}
}

// TestScannerSource_Position exercises the [syntax.Source] a real [Scanner]
// hands out, rather than a hand-built source. The reader records a line start
// when it consumes EOF; [Scanner.Source] trims that entry unless the input
// genuinely ends in a line terminator, so a position never names a line past
// the end of the source.
func TestScannerSource_Position(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []syntax.Position // want[i] is the position of offset i
	}{
		{
			name:    "no_trailing_newline",
			content: "abc",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 1, Column: 3},
				{Line: 1, Column: 4}, // EOF stays on line 1
			},
		},
		{
			name:    "empty",
			content: "",
			want:    []syntax.Position{{Line: 1, Column: 1}},
		},
		{
			name:    "trailing_lf",
			content: "a\n",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 2, Column: 1}, // there really is an empty line 2
			},
		},
		{
			name:    "trailing_cr",
			content: "a\r",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 2, Column: 1},
			},
		},
		{
			name:    "trailing_crlf",
			content: "a\r\n",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 1, Column: 3}, // the pair is one break, so LF is still line 1
				{Line: 2, Column: 1},
			},
		},
		{
			name:    "multiple_lines_no_trailing_newline",
			content: "ab\ncd",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 1, Column: 3},
				{Line: 2, Column: 1},
				{Line: 2, Column: 2},
				{Line: 2, Column: 3},
			},
		},
		{
			name:    "lone_cr_is_a_line_break",
			content: "ab\rcd",
			want: []syntax.Position{
				{Line: 1, Column: 1},
				{Line: 1, Column: 2},
				{Line: 1, Column: 3},
				{Line: 2, Column: 1},
				{Line: 2, Column: 2},
				{Line: 2, Column: 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := scanAll(t, tt.content)
			lines := uint32(1)
			for _, p := range tt.want {
				lines = max(lines, p.Line)
			}
			for offset, want := range tt.want {
				got := src.Position(uint32(offset))
				if got != want {
					t.Errorf("Position(%d) = %v, want %v", offset, got, want)
				}
				if got.Line > lines {
					t.Errorf("Position(%d).Line = %d, past the last line %d", offset, got.Line, lines)
				}
			}
		})
	}
}

// scanAll runs the scanner over content to the end of input and returns the
// resulting [syntax.Source]. Source must be called after scanning, because
// line starts accumulate as the reader advances.
func scanAll(t *testing.T, content string) syntax.Source {
	t.Helper()
	s := New([]byte(content))
	for {
		kind, _ := s.Next()
		if kind == syntax.KindEnd {
			break
		}
	}
	return s.Source()
}
