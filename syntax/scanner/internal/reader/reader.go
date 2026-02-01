package reader

import (
	"unicode/utf8"
)

// EOF represents the end of input
const EOF rune = -1

// Reader provides reading functionality tailored to the specific needs of Scanner.
type Reader struct {
	src      string
	ch       rune
	chw      int
	col      int // Current column number (0-based). < 0 if invalid.
	cur      int
	newlines []uint32
}

// New returns a new reader.
func New(src string) *Reader {
	b := &Reader{
		src: src,
	}
	b.fill()
	return b
}

func (b *Reader) fill() {
	if b.cur >= len(b.src) {
		b.ch = EOF
		b.chw = 0
		return
	}
	r, n := utf8.DecodeRuneInString(b.src[b.cur:])
	b.ch = r
	b.chw = n
}

// Column returns the current column number (0-based).
func (b *Reader) Column() int {
	if b.col < 0 {
		// Invalid column, recompute by scanning backwards to newline or start.
		b.col = 0
		for cur := b.cur; cur > 0; {
			ch, chw := utf8.DecodeLastRuneInString(b.src[:cur])
			if ch == '\n' {
				break
			}
			b.col++
			cur -= chw
		}
	}
	return b.col
}

// Peek looks at the current rune without consuming it.
func (b *Reader) Peek() rune {
	return b.ch
}

// Next returns the current rune and advances the reader by one rune.
// Returns EOF if the end of the input was reached.
func (b *Reader) Next() rune {
	r := b.ch
	n := b.chw
	b.cur += n
	if r == '\n' || r == EOF {
		b.col = 0
		if len(b.newlines) == 0 || b.newlines[len(b.newlines)-1] < uint32(b.cur) {
			b.newlines = append(b.newlines, uint32(b.cur))
		}
	} else if b.col >= 0 {
		// Only update column if valid.
		b.col++
	}
	b.fill()
	return r
}

// Scout returns the rune at the given distance relative to the current position.
//
// A distance of 0 returns the current rune.
// A positive distance looks ahead.
// A negative distance looks behind.
//
// Returns utf8.RuneError and false if the scout moves past the end of the input or before the start
// of the input.
func (b *Reader) Scout(dist int) (rune, bool) {
	if dist == 0 {
		return b.ch, true
	}

	if dist < 0 {
		pos := b.cur
		for i := 0; i > dist; i-- {
			if pos == 0 {
				return utf8.RuneError, false
			}
			_, width := utf8.DecodeLastRuneInString(b.src[:pos])
			pos -= width
		}
		r, _ := utf8.DecodeRuneInString(b.src[pos:])
		return r, true
	}

	// dist > 0
	if b.ch == EOF {
		return utf8.RuneError, false
	}

	pos := b.cur + b.chw

	for i := 1; i < dist; i++ {
		if pos >= len(b.src) {
			return utf8.RuneError, false
		}
		_, width := utf8.DecodeRuneInString(b.src[pos:])
		pos += width
	}

	if pos >= len(b.src) {
		return utf8.RuneError, false
	}
	r, _ := utf8.DecodeRuneInString(b.src[pos:])
	return r, true
}

// Backup undoes the last call to [Next].
//
// Panics if called after [Release] or at the beginning of the stream.
func (b *Reader) Backup() {
	if b.cur == 0 {
		panic("backup: cannot backup past the beginning")
	}
	ch, n := utf8.DecodeLastRuneInString(b.src[:b.cur])
	b.cur -= n
	b.ch = ch
	b.chw = n
	// Move back one column, if we go back to < 0, it will be recomputed on next Column() call or
	// when Next() encounters a newline.
	b.col--
}

// ContinuesWith reports whether the remaining input starts with s.
func (b *Reader) ContinuesWith(s string) bool {
	if len(b.src)-b.cur < len(s) {
		return false
	}
	return b.src[b.cur:b.cur+len(s)] == s
}

// ConsumeIf consumes s if the remaining input starts with it and returns true.
// Returns false if the input does not start with s.
func (b *Reader) ConsumeIf(s string) bool {
	if b.ContinuesWith(s) {
		b.cur += len(s)
		b.fill()
		return true
	}
	return false
}

// ConsumeWhile consumes runes while cond returns true and returns the consumed string.
func (b *Reader) ConsumeWhile(cond func(rune) bool) string {
	start := b.cur
	for b.ch != EOF && cond(b.ch) {
		b.Next()
	}
	return b.src[start:b.cur]
}

// BackupWhile moves the reader backwards while cond returns true for the preceding rune.
func (b *Reader) BackupWhile(cond func(rune) bool) {
	for b.cur != 0 {
		ch, n := utf8.DecodeLastRuneInString(b.src[:b.cur])
		if !cond(ch) {
			break
		}
		b.cur -= n
		b.ch = ch
		b.chw = n
	}
}

// Seek moves the reader to the given byte offset. Panics if pos is out of bounds.
func (b *Reader) Seek(pos int) {
	if pos < 0 || pos > len(b.src) {
		panic("jump: position out of bounds")
	}
	b.cur = pos
	b.fill()
	// Invalidate column, will be recomputed on next Column() call if necessary.
	b.col = -1
}

// Offset returns the offset of the current reading cursor.
func (b *Reader) Offset() int {
	return b.cur
}

// From returns the substring from start to the current position.
func (b *Reader) From(start int) string {
	return b.src[start:b.cur]
}

// Upto returns the substring from the beginning of the input to end.
func (b *Reader) Upto(end int) string {
	return b.src[0:end]
}

func (b *Reader) Source() string {
	return b.src
}

// Newlines returns the offsets of all newlines encountered so far.
func (b *Reader) Newlines() []uint32 {
	return b.newlines
}
