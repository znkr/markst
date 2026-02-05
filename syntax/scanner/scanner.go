package scanner

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/scanner/internal/reader"
)

type Scanner struct {
	r *reader.Reader

	// state
	mode    syntax.Mode
	newline bool
	err     *protoerr
}

type protoerr struct {
	message string
	hints   []string
}

func New(src string) *Scanner {
	return &Scanner{r: reader.New(src)}
}

func (s *Scanner) Mode() syntax.Mode {
	return s.mode
}

func (s *Scanner) SetMode(mode syntax.Mode) {
	s.mode = mode
}

func (s *Scanner) Source() syntax.Source {
	return &source{
		content:  s.r.Source(),
		newlines: s.r.Newlines(),
	}
}

func (s *Scanner) Offset() int {
	return s.r.Offset()
}

func (s *Scanner) Seek(offset int) {
	s.r.Seek(offset)
}

func (s *Scanner) Next() (syntax.Kind, syntax.Node) {
	start := s.r.Offset()
	ch := s.r.Next()
	s.newline = false

	if ch == '`' && s.mode != syntax.ModeMath {
		return s.scanRaw()
	}

	kind := s.scan(ch, start)
	text := s.r.From(start)
	span := s.spanFrom(start)
	if err := s.err; err != nil {
		s.err = nil
		return syntax.KindError, syntax.NewError(span, err.message, text, err.hints...)
	} else {
		return kind, syntax.NewLeaf(kind, span, text)
	}
}

func (s *Scanner) Column() int {
	return s.r.Column()
}

func (s *Scanner) Newline() bool {
	return s.newline
}

func (s *Scanner) error(msg string, hints ...string) syntax.Kind {
	s.err = &protoerr{message: msg, hints: hints}
	return syntax.KindError
}

func (s *Scanner) errorf(format string, args ...any) syntax.Kind {
	s.err = &protoerr{message: fmt.Sprintf(format, args...)}
	return syntax.KindError
}

func (s *Scanner) spanFrom(start int) syntax.Span {
	if start > math.MaxUint32 || s.r.Offset() > math.MaxUint32 {
		panic("span offset out of uint32 range")
	}
	return syntax.Span{Start: uint32(start), End: uint32(s.r.Offset())}
}

func (s *Scanner) scan(ch rune, start int) syntax.Kind {
	switch ch {
	case reader.EOF:
		return syntax.KindEnd
	case '/':
		if ch := s.r.Peek(); ch == '/' {
			s.r.Next()
			return s.scanLineComment()
		} else if ch == '*' {
			s.r.Next()
			return s.scanBlockComment()
		}
	case '*':
		if ch := s.r.Peek(); ch == '/' {
			s.r.Next()
			kind := s.error("unmatched end of multiline comment", "consider escaping the `*` with a backslash or opening the block comment with `/*`")
			return kind
		}
	}

	if isSpace(s.mode, ch) {
		return s.scanWhitespace(ch)
	}
	switch s.mode {
	case syntax.ModeMarkup:
		return s.scanMarkup(start, ch)
	case syntax.ModeMath:
		panic("math mode not implemented")
	case syntax.ModeCode:
		return s.scanCode(start, ch)
	default:
		panic(fmt.Sprintf("unknown mode: %d", s.mode))
	}
}

func (s *Scanner) scanMarkup(start int, ch rune) syntax.Kind {
	switch ch {
	case '\\':
		return s.scanBackslash()
	case 'h':
		if s.r.ConsumeIf("ttp://") || s.r.ConsumeIf("ttps://") {
			return s.scanLink()
		}
	case '<':
		if ch := s.r.Peek(); isIDContinue(ch) || ch == '>' {
			return s.scanLabel()
		}
	case '@':
		if isIDContinue(s.r.Peek()) {
			return s.scanRefMarker()
		}
	case '.':
		if s.r.ConsumeIf("..") {
			return syntax.KindShorthand
		}
	case '-':
		if s.r.ConsumeIf("--") {
			return syntax.KindShorthand
		}
		if ch := s.r.Peek(); ch == '-' || ch == '?' || unicode.IsNumber(ch) {
			return syntax.KindShorthand
		}
		if s.spaceOrEnd() {
			return syntax.KindListMarker
		}
	case '+':
		if s.spaceOrEnd() {
			return syntax.KindEnumMarker
		}
	case '/':
		if s.spaceOrEnd() {
			return syntax.KindTermMarker
		}
	case '*':
		if !s.inWord() {
			return syntax.KindStar
		}
	case '_':
		if !s.inWord() {
			return syntax.KindUnderscore
		}
	case '#':
		return syntax.KindHash
	case '[':
		return syntax.KindLeftBracket
	case ']':
		return syntax.KindRightBracket
	case '\'', '"':
		return syntax.KindSmartQuote
	case '$':
		return syntax.KindDollar
	case '~':
		return syntax.KindShorthand
	case ':':
		return syntax.KindColon
	case '=':
		s.r.ConsumeWhile(func(ch rune) bool { return ch == '=' })
		if s.spaceOrEnd() {
			return syntax.KindHeadingMarker
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return s.scanNumbering(start)
	}
	return s.scanText()
}

// scanRaw scans a raw text block. It parses an entire raw segment in the scanner to avoid going
// back and forth between the parser and the scanner for each raw text segment.
func (s *Scanner) scanRaw() (syntax.Kind, syntax.Node) {
	start := s.r.Offset() - 1 // include the initial backtick

	// Determine the number of opening backticks.
	backticks := 1
	for s.r.Peek() == '`' {
		s.r.Next()
		backticks++
	}

	// Special case for ``.
	if backticks == 2 {
		span := s.spanFrom(start)
		return syntax.KindRaw, syntax.NewInner(syntax.KindRaw, []syntax.Node{
			syntax.NewLeaf(syntax.KindRawDelim, syntax.Span{Start: span.Start, End: span.Start + 1}, "`"),
			syntax.NewLeaf(syntax.KindRawDelim, syntax.Span{Start: span.End - 1, End: span.End}, "`"),
		})
	}

	// Find the end of the raw text.
	found := 0
	for found < backticks {
		switch s.r.Next() {
		case reader.EOF:
			return syntax.KindError, syntax.NewError(s.spanFrom(start), "unclosed raw text", s.r.From(start))
		case '`':
			found++
		default:
			found = 0
		}
	}
	end := s.r.Offset()

	var nodes []syntax.Node
	prevStart := start
	push := func(kind syntax.Kind) {
		nodes = append(nodes, syntax.NewLeaf(kind, s.spanFrom(prevStart), s.r.From(prevStart)))
		prevStart = s.r.Offset()
	}

	// Opening delimiter
	s.r.Seek(start + backticks)
	push(syntax.KindRawDelim)

	if backticks >= 3 {
		s.scanBlockyRaw(end-backticks, push)
	} else {
		s.scanInlineRaw(end-backticks, push)
	}

	// Closing delimiter
	s.r.Seek(end)
	push(syntax.KindRawDelim)

	return syntax.KindRaw, syntax.NewInner(syntax.KindRaw, nodes)
}

// scanBlockyRaw parses a language tag, has smart behavior for trimming whitespace in the start/end
// lines, and trims common leading whitespace from all other lines as the "dedent". The exact
// behavior is described below.
//
// # The initial line
//
//   - Text until the first whitespace or backtick is parsed as the language tag.
//   - We check the rest of the line and if all characters are whitespace,
//     trim it. Otherwise we trim a single leading space if present.
//     If more trimmed characters follow on future lines, they will be
//     merged into the same trimmed element.
//   - If we didn't trim the entire line, the rest is kept as text.
//
// # Inner lines
//
//   - We determine the "dedent" by iterating over the lines. The dedent is
//     the minimum number of leading whitespace characters (not bytes) before
//     each line that has any non-whitespace characters.
//     The opening delimiter's line does not contribute to the dedent, but
//     the closing delimiter's line does (even if that line is entirely
//     whitespace up to the delimiter).
//   - We then trim the newline and dedent characters of each line, and add a
//     (potentially empty) text element of all remaining characters.
//
// # The final line
//
//   - If the last line is entirely whitespace, it is trimmed.
//   - Otherwise its text is kept like an inner line. However, if the last
//     non-whitespace character of the final line is a backtick, then one
//     ascii space (if present) is trimmed from the end.
func (s *Scanner) scanBlockyRaw(rawEnd int, push func(syntax.Kind)) {
	// Language tag
	tag := s.r.ConsumeWhile(func(ch rune) bool { return !unicode.IsSpace(ch) && ch != '`' })
	if tag != "" {
		push(syntax.KindRawLang)
	}

	// The rest of the function operates on the lines between the backticks.
	start := s.r.Offset()
	prevStart := start
	var lines []string
	for s.r.Offset() < rawEnd {
		if ch := s.r.Peek(); isNewline(ch) {
			lines = append(lines, s.r.From(prevStart))
			s.consumeNewline()
			prevStart = s.r.Offset()
			continue
		}
		s.r.Next()
	}
	// Capture the final segment (from last newline to rawEnd)
	s.r.Seek(rawEnd)
	lines = append(lines, s.r.From(prevStart))
	s.r.Seek(start)

	// Determine dedent level.
	dedent := math.MaxInt
	for i, line := range lines {
		if i == 0 {
			continue
		}
		ldedent := 0
		for _, r := range line {
			if !unicode.IsSpace(r) {
				goto NotAllWhitespace
			}
			ldedent++
		}
		if i < len(lines)-1 {
			// Ignore all whitespace only lines except for the last one.
			continue
		}
	NotAllWhitespace:
		dedent = min(dedent, ldedent)
	}
	if dedent == math.MaxInt {
		dedent = 0
	}

	// Trim whitespace from the last line. Will be added as a `KindRawTrimmed`
	// kind below.
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		idx := strings.LastIndexFunc(lastLine, func(r rune) bool { return !unicode.IsSpace(r) })
		if idx == -1 {
			// All white space
			lines = lines[:len(lines)-1]
		} else {
			// If last line ends in a backtick, try to trim a single space. This check must happen
			// before we add the first line since the last and first lines might be the same.
			if lastLine[idx] == '`' {
				lines[len(lines)-1] = strings.TrimSuffix(lastLine, " ")
			}
		}
	}

	// Handle the first line: trim if all whitespace, or trim a single space at the start. Note that
	// the first line does not affect the dedent value.
	if len(lines) > 0 {
		idx := strings.IndexFunc(lines[0], func(r rune) bool { return !unicode.IsSpace(r) })
		if idx == -1 {
			// All white space. This is the only spot we advance the scanner, but don't immediately
			// call `push`. But the rest of the function ensures we will always add this text to
			// a `KindRawTrimmed` later.
			s.r.Seek(s.r.Offset() + len(lines[0]))
		} else {
			lineEnd := s.r.Offset() + len(lines[0])
			if s.r.Peek() == ' ' {
				// Trim a single space after the lang tag on the first line.
				s.r.Next()
				push(syntax.KindRawTrimmed)
			}
			// We know here that the rest of the line is non-empty.
			s.r.Seek(lineEnd)
			push(syntax.KindText)
		}
		lines = lines[1:]
	}

	// Add lines.
	for _, line := range lines {
		s.consumeNewline()
		dedentBytes := 0
		for range dedent {
			_, n := utf8.DecodeRuneInString(line[dedentBytes:])
			dedentBytes += n
		}
		s.r.Seek(s.r.Offset() + dedentBytes)
		push(syntax.KindRawTrimmed)
		s.r.Seek(s.r.Offset() + len(line) - dedentBytes)
		push(syntax.KindText)
	}

	// Add final trimmed.
	if s.r.Offset() < rawEnd {
		s.r.Seek(rawEnd)
		push(syntax.KindRawTrimmed)
	}
}

// scanInlineRaw splits text on lines with non-newlines as `Text` kinds and newlines as
// `RawTrimmed`. Inline raw text does not dedent the text, all non-newline whitespace is kept.
func (s *Scanner) scanInlineRaw(rawEnd int, push func(syntax.Kind)) {
	for s.r.Offset() < rawEnd {
		if ch := s.r.Peek(); isNewline(ch) {
			push(syntax.KindText)
			s.consumeNewline()
			push(syntax.KindRawTrimmed)
			continue
		}
		s.r.Next()
	}
	push(syntax.KindText)
}

func (s *Scanner) scanText() syntax.Kind {
	for {
		ch := s.r.Next()
		switch ch {
		case reader.EOF:
			goto Finish
		case ' ':
			if ch := s.r.Peek(); isAlphanumeric(ch) {
				continue
			}
			goto BachupAndFinish
		case '/':
			if ch := s.r.Peek(); ch == '/' || ch == '*' {
				goto BachupAndFinish
			}
		case '-':
			if ch := s.r.Peek(); ch == '-' || ch == '?' {
				goto BachupAndFinish
			}
		case '.':
			if s.r.ContinuesWith("..") {
				goto BachupAndFinish
			}
		case 'h':
			if s.r.ContinuesWith("ttp://") || s.r.ContinuesWith("ttps://") {
				goto BachupAndFinish
			}
		case '@':
			if ch := s.r.Peek(); isValidInLabelLiteral(ch) {
				goto BachupAndFinish
			}
		case '\t', '\n', '\r', '\\', '[', ']', '~', '\'', '"', '*',
			'_', ':', '`', '$', '<', '>', '#':
			goto BachupAndFinish
		default:
			if unicode.IsSpace(ch) {
				goto BachupAndFinish
			}
		}
	}
BachupAndFinish:
	s.r.Backup()
Finish:
	return syntax.KindText
}

func (s *Scanner) scanLineComment() syntax.Kind {
	s.r.ConsumeWhile(func(ch rune) bool { return !isNewline(ch) })
	return syntax.KindLineComment
}

func (s *Scanner) scanBlockComment() syntax.Kind {
	d := 1
Loop:
	for {
		switch s.r.Next() {
		case '/':
			switch s.r.Next() {
			case '*':
				d++
			}
		case '*':
			switch s.r.Next() {
			case '/':
				d--
				if d == 0 {
					break Loop
				}
			}
		case reader.EOF:
			return s.error("unterminated multiline comment")
		}
	}
	return syntax.KindBlockComment
}

func (s *Scanner) scanWhitespace(ch rune) syntax.Kind {
	newlines := 0
	for {
		if s.consumeIfNewline(ch) {
			newlines++
		}
		ch = s.r.Peek()
		if !isSpace(s.mode, ch) {
			break
		}
		s.r.Next()
	}
	s.newline = newlines > 0
	if s.mode == syntax.ModeMarkup && newlines >= 2 {
		return syntax.KindParbreak
	}
	return syntax.KindSpace
}

func (s *Scanner) consumeIfNewline(ch rune) bool {
	if !isNewline(ch) {
		return false
	}
	if ch == '\r' && s.r.Peek() == '\n' {
		s.r.Next()
	}
	return true
}

func (s *Scanner) consumeNewline() {
	ch := s.r.Next()
	if !s.consumeIfNewline(ch) {
		panic("consumeNewline: not a newline")
	}
}

func (s *Scanner) scanNumbering(start int) syntax.Kind {
	s.r.ConsumeWhile(isASCIIDigit)
	number := s.r.From(start)
	if s.r.ConsumeIf(".") && s.spaceOrEnd() {
		_, err := strconv.ParseInt(number, 10, 64)
		if err != nil {
			return s.errorf("invalid list numbering: %s", s.r.From(start))
		}
		return syntax.KindEnumMarker
	}
	return s.scanText()
}

func (s *Scanner) scanBackslash() syntax.Kind {
	if s.r.ConsumeIf("u{") {
		seq := s.r.ConsumeWhile(isASCIIAlphanumeric)
		if !s.r.ConsumeIf("}") {
			return s.errorf("unclosed Unicode escape sequence")
		}
		x, err := strconv.ParseInt(seq, 16, 64)
		if err != nil || x > unicode.MaxRune || (0xD800 <= x && x < 0xE000) {
			return s.error("invalid Unicode escape sequence")
		}
		return syntax.KindEscape
	}

	if ch := s.r.Peek(); ch == reader.EOF || unicode.IsSpace(ch) {
		return syntax.KindLinebreak
	} else {
		s.r.Next()
		return syntax.KindEscape
	}
}

func (s *Scanner) scanLink() syntax.Kind {
	var brackets []byte
	s.r.ConsumeWhile(func(ch rune) bool {
		if '0' <= ch && ch <= '9' || 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' {
			return true
		}
		switch ch {
		case '!', '#', '$', '%', '&', '*', '+', ',', '-', '.', '/', ':', ';', '?', '@', '_', '~', '\'':
			return true
		case '[':
			brackets = append(brackets, '[')
			return true
		case ']':
			if len(brackets) == 0 || brackets[len(brackets)-1] != '[' {
				return false
			}
			brackets = brackets[:len(brackets)-1]
			return true
		case '(':
			brackets = append(brackets, '(')
			return true
		case ')':
			if len(brackets) == 0 || brackets[len(brackets)-1] != '(' {
				return false
			}
			brackets = brackets[:len(brackets)-1]
			return true
		default:
			return false
		}
	})

	// Don't include trailing punctuation.
	s.r.BackupWhile(func(ch rune) bool {
		switch ch {
		case '!', ',', '.', ':', ';', '?', '\'':
			return true
		default:
			return false
		}
	})

	if balanced := len(brackets) == 0; !balanced {
		return s.error("automatic links cannot contain unbalanced brackets, use 'link' function instead")
	}

	return syntax.KindLink
}

func (s *Scanner) scanLabel() syntax.Kind {
	label := s.r.ConsumeWhile(isValidInLabelLiteral)
	if !s.r.ConsumeIf(">") {
		return s.error("unclosed label, expected '>'")
	}
	if len(label) == 0 {
		return s.error("label cannot be empty")
	}
	return syntax.KindLabel
}

func (s *Scanner) scanRefMarker() syntax.Kind {
	s.r.ConsumeWhile(isValidInLabelLiteral)

	// Don't include trailing punctuation.
	s.r.BackupWhile(func(ch rune) bool {
		switch ch {
		case '.', ':':
			return true
		default:
			return false
		}
	})
	return syntax.KindRefMarker
}

func (s *Scanner) scanCode(start int, ch rune) syntax.Kind {
	switch ch {
	case '{':
		return syntax.KindLeftBrace
	case '}':
		return syntax.KindRightBrace
	case '[':
		return syntax.KindLeftBracket
	case ']':
		return syntax.KindRightBracket
	case '(':
		return syntax.KindLeftParen
	case ')':
		return syntax.KindRightParen
	case ',':
		return syntax.KindComma
	case ';':
		return syntax.KindSemicolon
	case ':':
		return syntax.KindColon
	case '$':
		return syntax.KindDollar
	case '+':
		if s.r.ConsumeIf("=") {
			return syntax.KindPlusEq
		}
		return syntax.KindPlus
	case '-', '\u2212':
		if s.r.ConsumeIf("=") {
			return syntax.KindHyphEq
		}
		return syntax.KindMinus
	case '*':
		if s.r.ConsumeIf("=") {
			return syntax.KindStarEq
		}
		return syntax.KindStar
	case '/':
		if s.r.ConsumeIf("=") {
			return syntax.KindSlashEq
		}
		return syntax.KindSlash
	case '=':
		if s.r.ConsumeIf("=") {
			return syntax.KindEqEq
		}
		if s.r.ConsumeIf(">") {
			return syntax.KindArrow
		}
		return syntax.KindEq
	case '!':
		if s.r.ConsumeIf("=") {
			return syntax.KindExclEq
		}
		goto Unexpected
	case '<':
		if s.r.ConsumeIf("=") {
			return syntax.KindLtEq
		}
		if ch := s.r.Peek(); isIDContinue(ch) || ch == '>' {
			return s.scanLabel()
		}
		return syntax.KindLt
	case '>':
		if s.r.ConsumeIf("=") {
			return syntax.KindGtEq
		}
		return syntax.KindGt
	case '.':
		if ch := s.r.Peek(); isASCIIDigit(ch) {
			return s.scanNumber(start, '.')
		}
		if s.r.ConsumeIf(".") {
			return syntax.KindDots
		}
		return syntax.KindDot
	case '"':
		return s.scanString()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return s.scanNumber(start, ch)
	}
	if isIDStart(ch) {
		return s.scanIdent(start)
	}
Unexpected:
	return s.errorf("unexpected character: %s", string(ch))
}

var keywords = map[string]syntax.Kind{
	"none":     syntax.KindNone,
	"auto":     syntax.KindAuto,
	"true":     syntax.KindBool,
	"false":    syntax.KindBool,
	"not":      syntax.KindNot,
	"and":      syntax.KindAnd,
	"or":       syntax.KindOr,
	"let":      syntax.KindLet,
	"set":      syntax.KindSet,
	"show":     syntax.KindShow,
	"context":  syntax.KindContext,
	"if":       syntax.KindIf,
	"else":     syntax.KindElse,
	"for":      syntax.KindFor,
	"in":       syntax.KindIn,
	"while":    syntax.KindWhile,
	"break":    syntax.KindBreak,
	"continue": syntax.KindContinue,
	"return":   syntax.KindReturn,
	"import":   syntax.KindImport,
	"include":  syntax.KindInclude,
	"as":       syntax.KindAs,
}

func (s *Scanner) scanIdent(start int) syntax.Kind {
	s.r.ConsumeWhile(isIDContinue)
	ident := s.r.From(start)

	prev := s.r.Upto(start)
	if (!strings.HasSuffix(prev, ".") || strings.HasSuffix(prev, "..")) && !strings.HasSuffix(prev, "@") {
		if kind, ok := keywords[ident]; ok {
			return kind
		}
	}

	if ident == "_" {
		return syntax.KindUnderscore
	}

	return syntax.KindIdent
}

func (s *Scanner) scanString() syntax.Kind {
	var err string
	var hints []string
	for {
		switch s.r.Next() {
		case reader.EOF:
			if err != "" {
				return s.error(err, hints...)
			}
			return s.error("unclosed string")
		case '"':
			if err != "" {
				return s.error(err, hints...)
			}
			return syntax.KindStr
		case '\\':
			if s.r.Peek() == reader.EOF {
				continue
			}
			switch ch := s.r.Next(); ch {
			case 'u':
				if !s.r.ConsumeIf("{") {
					if err == "" {
						err = "invalid unicode escape sequence"
						hints = []string{"expected '{'"}
					}
					continue
				}
				s.r.ConsumeWhile(isASCIIAlphanumeric)
				if !s.r.ConsumeIf("}") {
					if err == "" {
						err = "invalid unicode escape sequence"
						hints = []string{"expected '}'"}
					}
					continue
				}
			case 'n', 'r', 't', '\\', '"', '\'':
				// Valid escape sequences
			default:
				if err == "" {
					err = "invalid escape sequence"
				}
			}
		}
	}
}

var numberSuffixes = map[string]struct{}{
	"pt":  {},
	"mm":  {},
	"cm":  {},
	"in":  {},
	"deg": {},
	"rad": {},
	"em":  {},
	"fr":  {},
	"%":   {},
}

func (s *Scanner) scanNumber(start int, first rune) syntax.Kind {
	base := 10
	prefix := 0
	if first == '0' {
		switch s.r.Peek() {
		case 'b':
			base = 2
			s.r.Next()
			prefix = 2
		case 'o':
			base = 8
			s.r.Next()
			prefix = 2
		case 'x':
			base = 16
			s.r.Next()
			prefix = 2
		}
	}

	if base == 16 {
		s.r.ConsumeWhile(isASCIIAlphanumeric)
	} else {
		s.r.ConsumeWhile(isASCIIDigit)
	}

	isFloat := false
	if base == 10 {
		switch {
		case first == '.':
			isFloat = true
		case s.r.Peek() == '.':
			if s.r.ContinuesWith("..") {
				break
			}
			if ch, ok := s.r.Scout(1); ok && isIDStart(ch) {
				break
			}
			isFloat = true
			s.r.Next()
			s.r.ConsumeWhile(isASCIIDigit)
		}

		// Scientific notation
		if ch := s.r.Peek(); ch == 'e' || ch == 'E' {
			if next, ok := s.r.Scout(1); ok && (isASCIIDigit(next) || next == '+' || next == '-') {
				isFloat = true
				s.r.Next()
				if s.r.Peek() == '+' || s.r.Peek() == '-' {
					s.r.Next()
				}
				s.r.ConsumeWhile(isASCIIDigit)
			}
		}
	}

	number := s.r.From(start)
	suffix := s.r.ConsumeWhile(func(r rune) bool {
		return isASCIIAlphanumeric(r) || r == '%'
	})
	var suffixErr string
	if suffix != "" {
		if _, ok := numberSuffixes[suffix]; !ok {
			suffixErr = fmt.Sprintf("invalid number suffix: %q", suffix)
		}
	}

	var numberErr string
	switch {
	case isFloat:
		if _, err := strconv.ParseFloat(number, 64); err != nil {
			numberErr = fmt.Sprintf("invalid floating point number: %s", number)
		}
	default:
		var name string
		switch base {
		case 2:
			name = "binary"
		case 8:
			name = "octal"
		case 10:
			name = "decimal"
		case 16:
			name = "hexadecimal"
		}
		value, err := strconv.ParseInt(number[prefix:], base, 64)
		if err != nil {
			numberErr = fmt.Sprintf("invalid %s number: %s", name, number)
			break
		}
		if suffix != "" && base != 10 {
			if suffixErr == "" {
				suffixErr = fmt.Sprintf("try using a decimal number %d%s", value, suffix)
			}
			numberErr = fmt.Sprintf("%s numbers cannot have a unit suffix", name)
			break
		}
	}

	switch {
	case numberErr != "" && suffixErr != "":
		kind := s.error(numberErr, suffixErr)
		return kind
	case numberErr != "" && suffixErr == "":
		return s.error(numberErr)
	case numberErr == "" && suffixErr != "":
		return s.error(suffixErr)
	case suffix != "":
		return syntax.KindNumeric
	default:
		if isFloat {
			return syntax.KindFloat
		}
		return syntax.KindInt
	}
}

func (s *Scanner) inWord() bool {
	prev, ok := s.r.Scout(-2)
	next := s.r.Peek()
	return ok && isAlphanumeric(prev) && isAlphanumeric(next)
}

func (s *Scanner) spaceOrEnd() bool {
	ch := s.r.Peek()
	return ch == reader.EOF || unicode.IsSpace(ch) || s.r.ContinuesWith("//") || s.r.ContinuesWith("/*")
}

func isSpace(m syntax.Mode, ch rune) bool {
	switch m {
	case syntax.ModeMarkup:
		return ch == ' ' || ch == '\t' || isNewline(ch)
	default:
		return unicode.IsSpace(ch)
	}
}

func isNewline(ch rune) bool {
	return ch == '\n' || ch == '\r'
}
func isValidInLabelLiteral(ch rune) bool {
	return isIDContinue(ch) || ch == ':' || ch == '.'
}

func isIDStart(ch rune) bool {
	// TODO: Use unicode XID_Start property?
	return unicode.IsLetter(ch) || ch == '_'
}

func isIDContinue(ch rune) bool {
	// TODO: Use unicode XID_Continue property?
	return isAlphanumeric(ch) || ch == '-' || ch == '_'
}

func isASCIIAlphanumeric(ch rune) bool {
	return '0' <= ch && ch <= '9' || 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z'
}

func isASCIIDigit(ch rune) bool {
	return '0' <= ch && ch <= '9'
}

func isAlphanumeric(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsNumber(ch)
}
