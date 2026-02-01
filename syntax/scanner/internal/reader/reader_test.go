package reader

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

func TestEmpty(t *testing.T) {
	r := New("")

	if ch := r.Next(); ch != EOF {
		t.Errorf("r.Next() = %q, want EOF", ch)
	}

	if b := r.Peek(); b != EOF {
		t.Errorf("r.Peek() = %q, want '\\0'", b)
	}
}

func TestNext(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"hello", "Hello, 世界"},
		{"large text", strings.Repeat("Hello, 世界", 2000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.in)
			in := tt.in

			var runes []rune
			var offsets = []int{r.Offset()}
			for ch := r.Next(); ch != EOF; ch = r.Next() {
				runes = append(runes, ch)
				offsets = append(offsets, r.Offset())
			}

			if diff := cmp.Diff(runes, []rune(in)); diff != "" {
				t.Errorf("Runes read from Reader are different from input [-got,+want]:\n%s", diff)
			}

			var offsetsWant []int
			for i := range in {
				offsetsWant = append(offsetsWant, i)
			}
			offsetsWant = append(offsetsWant, len(in))

			if diff := cmp.Diff(offsets, offsetsWant); diff != "" {
				t.Errorf("Offsets from Reader is different from input [-got,+want]:\n%s", diff)
			}
		})
	}
}

func TestPeek(t *testing.T) {
	r := New("世界")
	if got, want := r.Peek(), '世'; got != want {
		t.Errorf("(first rune) r.Peek() = %v, want %v", got, want)
	}
	r.Next()
	if got, want := r.Peek(), '界'; got != want {
		t.Errorf("(second rune) r.Peek() = %v, want %v", got, want)
	}
	r.Next()
	if got, want := r.Peek(), EOF; got != want {
		t.Errorf("(at end) r.Peek() = %v, want %v", got, want)
	}
}

func TestBackup(t *testing.T) {
	r := New("Hello")

	if ch := r.Next(); ch != 'H' {
		t.Errorf("1st r.Next() = %q, want 'H'", ch)
	}
	if ch := r.Next(); ch != 'e' {
		t.Errorf("2nd r.Next() = %q, want 'e'", ch)
	}

	r.Backup()

	if ch := r.Next(); ch != 'e' {
		t.Errorf("after Backup, r.Next() = %q, want 'e'", ch)
	}

	if ch := r.Next(); ch != 'l' {
		t.Errorf("3rd r.Next() = %q, want 'l'", ch)
	}
}

func TestBackup_Newlines(t *testing.T) {
	// Test the column recalculation logic
	input := "a\nb\nc"
	r := New(input)

	// Read 'a', '\n', 'b'
	r.Next() // a
	r.Next() // \n
	r.Next() // b

	if col := r.Column(); col != 1 { // at 'b', col is 1 (after 'b')
		t.Errorf("at 'b', Column() = %d, want 1", col)
	}

	// Backup 'b' -> at '\n'
	r.Backup()
	if peek := r.Peek(); peek != 'b' {
		t.Errorf("after backup 'b', Peek() = %q, want 'b'", peek)
	}

	if col := r.Column(); col != 0 {
		t.Errorf("after backup 'b', Column() = %d, want 0", col)
	}

	// Backup '\n' -> at 'a'
	r.Backup()
	if peek := r.Peek(); peek != '\n' {
		t.Errorf("after backup 'b', Peek() = %q, want 'b'", peek)
	}
	if col := r.Column(); col != 1 {
		t.Errorf("after backup '\\n', Column() = %d, want 1", col)
	}
}

func TestContinuesWith(t *testing.T) {
	r := New("Hello World")

	if !r.ContinuesWith("Hello") {
		t.Error("HasPrefix('Hello') failed")
	}

	if r.ContinuesWith("World") {
		t.Error("HasPrefix('World') succeeded, want false")
	}

	// Verify state hasn't changed
	if r.Peek() != 'H' {
		t.Errorf("Peek() = %q, want 'H'", r.Peek())
	}
}

func TestConsumeIf(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		setup    func(*testing.T, *Reader)
		s        string
		want     bool
		wantPeek rune
	}{
		{
			name:     "match",
			input:    "Hello World",
			s:        "Hello",
			want:     true,
			wantPeek: ' ',
		},
		{
			name:     "no_match",
			input:    "Hello World",
			s:        "World",
			want:     false,
			wantPeek: 'H',
		},
		{
			name:     "partial_match_fail",
			input:    "Hell",
			s:        "Hello",
			want:     false,
			wantPeek: 'H',
		},
		{
			name:  "match_mid_stream",
			input: "Hello World",
			setup: func(t *testing.T, r *Reader) {
				if !r.ConsumeIf("Hello") {
					t.Fatal("setup failed")
				}
			},
			s:        " ",
			want:     true,
			wantPeek: 'W',
		},
		{
			name:  "no_match_mid_stream",
			input: "Hello World",
			setup: func(t *testing.T, r *Reader) {
				if !r.ConsumeIf("Hello") {
					t.Fatal("setup failed")
				}
				if !r.ConsumeIf(" ") {
					t.Fatal("setup failed 2")
				}
			},
			s:        "World!",
			want:     false,
			wantPeek: 'W',
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.input)
			if tt.setup != nil {
				tt.setup(t, r)
			}
			if got := r.ConsumeIf(tt.s); got != tt.want {
				t.Errorf("ConsumeIf(%q) = %v, want %v", tt.s, got, tt.want)
			}
			if got := r.Peek(); got != tt.wantPeek {
				t.Errorf("Peek() = %q, want %q", got, tt.wantPeek)
			}
		})
	}
}

func TestConsumeWhile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		setup    func(*testing.T, *Reader)
		cond     func(rune) bool
		want     string
		wantPeek rune
	}{
		{
			name:     "consume_word",
			input:    "Hello World",
			cond:     func(r rune) bool { return r != ' ' && r != EOF },
			want:     "Hello",
			wantPeek: ' ',
		},
		{
			name:     "consume_none",
			input:    "Hello World",
			cond:     func(r rune) bool { return r == ' ' },
			want:     "",
			wantPeek: 'H',
		},
		{
			name:     "consume_all",
			input:    "Hello",
			cond:     func(r rune) bool { return true },
			want:     "Hello",
			wantPeek: EOF,
		},
		{
			name:  "consume_rest",
			input: "Hello World",
			setup: func(t *testing.T, r *Reader) {
				if !r.ConsumeIf("Hello") {
					t.Fatal("setup failed")
				}
			},
			cond:     func(r rune) bool { return true },
			want:     " World",
			wantPeek: EOF,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.input)
			if tt.setup != nil {
				tt.setup(t, r)
			}
			got := r.ConsumeWhile(tt.cond)
			if got != tt.want {
				t.Errorf("ConsumeWhile() = %q, want %q", got, tt.want)
			}
			if got := r.Peek(); got != tt.wantPeek {
				t.Errorf("Peek() = %q, want %q", got, tt.wantPeek)
			}
		})
	}
}

func TestBackupWhile(t *testing.T) {
	r := New("Hello World")
	// Consume everything
	r.ConsumeWhile(func(rune) bool { return true })

	// Backup "World"
	r.BackupWhile(func(r rune) bool {
		return r != ' '
	})

	if got := r.Peek(); got != 'W' {
		t.Errorf("After BackupWhile, Peek() = %q, want 'W'", got)
	}

	// Backup space
	r.BackupWhile(func(r rune) bool {
		return r == ' '
	})

	if got := r.Peek(); got != ' ' {
		t.Errorf("After BackupWhile space, Peek() = %q, want ' '", got)
	}

	// Backup "Hello"
	r.BackupWhile(func(r rune) bool {
		return true
	})

	if got := r.Peek(); got != 'H' {
		t.Errorf("After BackupWhile all, Peek() = %q, want 'H'", got)
	}
}

func TestScout(t *testing.T) {
	input := "abc"
	r := New(input)
	if ch, ok := r.Scout(1); ch != 'b' || !ok {
		t.Errorf("Scout(1) = %q, %v; want 'b', true", ch, ok)
	}
	if ch, ok := r.Scout(2); ch != 'c' || !ok {
		t.Errorf("Scout(2) = %q, %v; want 'c', true", ch, ok)
	}
	if ch, ok := r.Scout(3); ch != utf8.RuneError || ok {
		t.Errorf("Scout(3) = %q, %v; want RuneError, false", ch, ok)
	}
	if ch, ok := r.Scout(-1); ch != utf8.RuneError || ok {
		t.Errorf("Scout(-1) = %q, %v; want RuneError, false", ch, ok)
	}
	// Advance to 'b'
	r.Next()
	if ch, ok := r.Scout(0); ch != 'b' || !ok {
		t.Errorf("at b: Scout(0) = %q, %v; want 'b', true", ch, ok)
	}
	if ch, ok := r.Scout(1); ch != 'c' || !ok {
		t.Errorf("at b: Scout(1) = %q, %v; want 'c', true", ch, ok)
	}
	if ch, ok := r.Scout(-1); ch != 'a' || !ok {
		t.Errorf("at b: Scout(-1) = %q, %v; want 'a', true", ch, ok)
	}
	if ch, ok := r.Scout(-2); ch != utf8.RuneError || ok {
		t.Errorf("at b: Scout(-2) = %q, %v; want RuneError, false", ch, ok) // Before start
	}
}

func TestColumn(t *testing.T) {
	tests := []struct {
		name string
		in   string
		cols []int
	}{
		{
			"simple",
			"abc",
			[]int{0, 1, 2},
		},
		{
			"newline",
			"a\nb",
			[]int{0, 1, 0},
		},
		{
			"starts with newline",
			"\na",
			[]int{0, 0},
		},
		{
			"multiple newlines",
			"a\n\nb",
			[]int{0, 1, 0, 0},
		},
		{
			"chinese",
			"世界",
			[]int{0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.in)
			var cols []int
			for r.Peek() != EOF {
				cols = append(cols, r.Column())
				r.Next()
			}
			if diff := cmp.Diff(cols, tt.cols); diff != "" {
				t.Errorf("Column() mismatch [-got,+want]:\n%s", diff)
			}
		})
	}
}

func TestSeek(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		pos      int
		wantPeek rune
		wantCol  int
	}{
		{
			name:     "seek_to_start",
			input:    "Hello",
			pos:      0,
			wantPeek: 'H',
			wantCol:  0,
		},
		{
			name:     "seek_to_middle",
			input:    "Hello",
			pos:      2,
			wantPeek: 'l',
			wantCol:  2,
		},
		{
			name:     "seek_to_end",
			input:    "Hello",
			pos:      5,
			wantPeek: EOF,
			wantCol:  5,
		},
		{
			name:     "seek_after_newline",
			input:    "Hi\nWorld",
			pos:      3,
			wantPeek: 'W',
			wantCol:  0,
		},
		{
			name:     "seek_mid_line_after_newline",
			input:    "Hi\nWorld",
			pos:      5,
			wantPeek: 'r',
			wantCol:  2,
		},
		{
			name:     "seek_unicode",
			input:    "世界",
			pos:      3, // after first rune (3 bytes)
			wantPeek: '界',
			wantCol:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.input)
			r.Seek(tt.pos)
			if got := r.Peek(); got != tt.wantPeek {
				t.Errorf("Peek() = %q, want %q", got, tt.wantPeek)
			}
			if got := r.Column(); got != tt.wantCol {
				t.Errorf("Column() = %d, want %d", got, tt.wantCol)
			}
			if got := r.Offset(); got != tt.pos {
				t.Errorf("Offset() = %d, want %d", got, tt.pos)
			}
		})
	}
}

func TestSeek_MatchesSequentialScan(t *testing.T) {
	inputs := []string{
		"Hello World",
		"Hi\nWorld",
		"a\nb\nc\nd",
		"\n\n\n",
		"世界\n你好",
		"line1\nline2\nline3",
		"no newlines here",
		"\nstarts with newline",
		"ends with newline\n",
		"",
	}

	type record struct {
		offset, col int
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			// First pass: scan sequentially and record column at each offset
			r := New(input)
			var records []record
			for r.Peek() != EOF {
				records = append(records, record{r.Offset(), r.Column()})
				r.Next()
			}
			records = append(records, record{r.Offset(), r.Column()})

			// Second pass: seek forward through all offsets
			r2 := New(input)
			for _, rec := range records {
				r2.Seek(rec.offset)
				if got := r2.Column(); got != rec.col {
					t.Errorf("forward Seek(%d): Column() = %d, want %d", rec.offset, got, rec.col)
				}
			}

			// Third pass: seek backward through all offsets
			r3 := New(input)
			for _, rec := range slices.Backward(records) {
				r3.Seek(rec.offset)
				if got := r3.Column(); got != rec.col {
					t.Errorf("backward Seek(%d): Column() = %d, want %d", rec.offset, got, rec.col)
				}
			}
		})
	}
}

func TestFromUpto(t *testing.T) {
	input := "Hello World"
	r := New(input)

	r.ConsumeIf("Hello")
	if got := r.From(0); got != "Hello" {
		t.Errorf("From(0) = %q, want %q", got, "Hello")
	}

	if got := r.Upto(5); got != "Hello" {
		t.Errorf("Upto(5) = %q, want %q", got, "Hello")
	}

	r.ConsumeIf(" ")
	if got := r.From(5); got != " " {
		t.Errorf("From(5) = %q, want %q", got, " ")
	}
}

func TestNewlines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []uint32
	}{
		{
			name: "no_newlines",
			in:   "hello",
			want: []uint32{5}, // EOF position only
		},
		{
			name: "single_newline",
			in:   "a\nb",
			want: []uint32{2, 3}, // after '\n' at offset 2, EOF at 3
		},
		{
			name: "multiple_newlines",
			in:   "a\nb\nc",
			want: []uint32{2, 4, 5}, // '\n' at 2, '\n' at 4, EOF at 5
		},
		{
			name: "consecutive_newlines",
			in:   "a\n\nb",
			want: []uint32{2, 3, 4}, // '\n' at 2, '\n' at 3, EOF at 4
		},
		{
			name: "starts_with_newline",
			in:   "\na",
			want: []uint32{1, 2}, // '\n' at 1, EOF at 2
		},
		{
			name: "ends_with_newline",
			in:   "a\n",
			want: []uint32{2}, // '\n' at 2, EOF also at 2 (deduped)
		},
		{
			name: "only_newlines",
			in:   "\n\n",
			want: []uint32{1, 2}, // '\n' at 1, '\n' at 2, EOF at 2 (deduped)
		},
		{
			name: "empty",
			in:   "",
			want: []uint32{0}, // EOF at 0
		},
		{
			name: "unicode_with_newlines",
			in:   "世\n界",
			want: []uint32{4, 7}, // '\n' after 世 (3 bytes) at 4, EOF at 7
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.in)
			// Consume entire input to populate newlines
			for r.Next() != EOF {
			}
			got := r.Newlines()
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Newlines() mismatch [-got,+want]:\n%s", diff)
			}
		})
	}
}

func TestNewlines_NoDuplicates(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		action func(*Reader)
		want   []uint32
	}{
		{
			name: "multiple_eof",
			in:   "a",
			action: func(r *Reader) {
				r.Next() // 'a'
				r.Next() // EOF
				r.Next() // EOF again
				r.Next() // EOF again
			},
			want: []uint32{1},
		},
		{
			name: "backup_and_re_read_newline",
			in:   "a\nb",
			action: func(r *Reader) {
				r.Next()   // 'a'
				r.Next()   // '\n'
				r.Next()   // 'b'
				r.Backup() // back to '\n'
				r.Backup() // back to 'a'
				r.Next()   // 'a' again
				r.Next()   // '\n' again
			},
			want: []uint32{2},
		},
		{
			name: "seek_and_re_read_newline",
			in:   "a\nb\nc",
			action: func(r *Reader) {
				for r.Next() != EOF {
				}
				r.Seek(0)
				for r.Next() != EOF {
				}
			},
			want: []uint32{2, 4, 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New(tt.in)
			tt.action(r)
			if diff := cmp.Diff(r.Newlines(), tt.want); diff != "" {
				t.Errorf("Newlines() mismatch [-got,+want]:\n%s", diff)
			}
		})
	}
}
