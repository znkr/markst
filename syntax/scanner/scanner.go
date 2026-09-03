// Package scanner turns Markst source into tokens.
//
// [Scanner.Next] hands back one token at a time. Which characters make a token
// depends on the mode — markup, math, or code — and the parser switches the
// mode with [Scanner.SetMode] as it enters and leaves each construct, entering
// code mode after a #, for instance.
//
// Invalid input becomes a [syntax.KindError] token carrying a [syntax.Error]
// with the diagnostic.
//
// Raw text between backticks comes out as one composite node rather than
// several tokens, because dedenting and trimming it needs the whole block.
package scanner

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/scanner/internal/reader"
)

// Scanner reads tokens off Markst source. Make one with [New] and call
// [Scanner.Next] until it returns [syntax.KindEnd].
//
// A Scanner is not safe for concurrent use.
type Scanner struct {
	r *reader.Reader

	// a allocates the nodes this scanner produces, and the ones the parser
	// builds over them: a syntax tree's nodes live and die together, so they
	// come out of one arena. Reachable through [Scanner.Arena].
	a syntax.Arena

	// state
	mode    syntax.Mode
	newline bool
	err     *protoerr

	// node holds a composite node produced by the current scan (used by math
	// mode to emit a FieldAccess node for `a.b`). It is consumed and cleared by
	// [Next].
	node syntax.Node
}

type protoerr struct {
	message string
	hints   []string
}

// New returns a scanner over src, starting in markup mode.
func New(src []byte) *Scanner {
	return &Scanner{r: reader.New(src)}
}

// Arena returns the arena the scanner allocates its nodes in, so that a caller
// building nodes over them — the parser — can put them in the same blocks.
func (s *Scanner) Arena() *syntax.Arena { return &s.a }

// Mode returns the mode the scanner is reading in.
func (s *Scanner) Mode() syntax.Mode {
	return s.mode
}

// SetMode switches the mode the scanner reads in, changing how the same
// characters are tokenized. The parser calls it on entering and leaving each
// construct.
func (s *Scanner) SetMode(mode syntax.Mode) {
	s.mode = mode
}

// Source returns a [syntax.Source] converting between byte offsets and
// line/column positions in the text being scanned. Call it once scanning is
// done: it knows only about the newlines seen so far.
func (s *Scanner) Source() syntax.Source {
	content := s.r.Source()
	newlines := s.r.Newlines()
	// The reader records a line start when it consumes EOF, which keeps its
	// own column tracking right but invents a line that does not exist unless
	// the input actually ends in a line terminator. Trim it here rather than
	// in the reader, so positions never name a line past the end of the
	// source. For input that does end in a terminator the reader's dedupe
	// guard has already collapsed the EOF entry into the real line start, so
	// there is nothing to trim.
	if n := len(newlines); n > 0 && newlines[n-1] == uint32(len(content)) && !endsWithLineBreak(content) {
		newlines = newlines[:n-1]
	}
	return &source{
		content:  content,
		newlines: newlines,
	}
}

// Offset returns where in the source the scanner has read to.
func (s *Scanner) Offset() int {
	return s.r.Offset()
}

// Seek moves the scanner to a byte offset, which is how the parser backtracks.
func (s *Scanner) Seek(offset int) {
	s.r.Seek(offset)
}

// Next returns the next token: its kind, and a [syntax.Leaf] node, or a
// [syntax.Error] node if the input was invalid. At the end of the source it
// returns [syntax.KindEnd].
//
// Raw text between backticks is the exception. It comes back as a
// [syntax.Inner] node of kind [syntax.KindRaw] holding the whole block.
func (s *Scanner) Next() (syntax.Kind, syntax.Node) {
	start := s.r.Offset()
	ch := s.r.Next()
	s.newline = false

	if ch == '`' && s.mode != syntax.ModeMath {
		return s.scanRaw()
	}

	kind := s.scan(ch, start)
	span := s.spanFrom(start)
	if err := s.err; err != nil {
		s.err = nil
		s.node = nil
		return syntax.KindError, s.a.Error(span, err.message, err.hints...)
	} else if node := s.node; node != nil {
		s.node = nil
		return kind, node
	} else {
		return kind, s.a.Leaf(kind, span)
	}
}

// Column returns which column the scanner has read to, counting from 0.
func (s *Scanner) Column() int {
	return s.r.Column()
}

// Newline reports whether the token just scanned contained a newline, which is
// how the parser finds paragraph breaks and the ends of headings.
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
			kind := s.error("unexpected end of block comment", "consider escaping the `*` with a backslash or opening the block comment with `/*`")
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
		return s.scanMath(start, ch)
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

	// `` is two delimiters with nothing between them, so build the node here:
	// there is no content for scanBlockyRaw or scanInlineRaw to work on.
	if backticks == 2 {
		span := s.spanFrom(start)
		delims := s.a.Nodes(2)
		delims[0] = s.a.Leaf(syntax.KindRawDelim, syntax.Span{Start: span.Start, End: span.Start + 1})
		delims[1] = s.a.Leaf(syntax.KindRawDelim, syntax.Span{Start: span.End - 1, End: span.End})
		return syntax.KindRaw, s.a.Inner(syntax.KindRaw, delims)
	}

	// Find the end of the raw text.
	found := 0
	for found < backticks {
		switch s.r.Next() {
		case reader.EOF:
			return syntax.KindError, s.a.Error(s.spanFrom(start), "unclosed raw text")
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
		nodes = append(nodes, s.a.Leaf(kind, s.spanFrom(prevStart)))
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

	return syntax.KindRaw, s.a.Inner(syntax.KindRaw, s.a.CloneNodes(nodes))
}

// scanBlockyRaw parses a language tag, has smart behavior for trimming whitespace in the start/end
// lines, and trims common leading whitespace from all other lines as the "dedent". The exact
// behavior is described below.
//
// # The initial line
//
//   - Text up to the first whitespace or backtick is the language tag.
//   - The rest of the line is trimmed entirely if it is all whitespace, and
//     otherwise loses a single leading space. Characters trimmed on later
//     lines merge into the same trimmed element.
//   - Whatever is left of the line is kept as text.
//
// # Inner lines
//
//   - The dedent is the fewest leading whitespace characters — characters,
//     not bytes — on any line that has something other than whitespace on
//     it. The opening delimiter's line does not count towards it; the
//     closing delimiter's line does, even when it is nothing but whitespace
//     up to the delimiter.
//   - Each line then loses its newline and its dedent, and what remains
//     becomes a text element, possibly an empty one.
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
	if len(tag) != 0 {
		push(syntax.KindRawLang)
	}

	// The rest of the function operates on the lines between the backticks.
	start := s.r.Offset()
	prevStart := start
	var lines []string
	for s.r.Offset() < rawEnd {
		if ch := s.r.Peek(); isNewline(ch) {
			lines = append(lines, string(s.r.From(prevStart)))
			s.consumeNewline()
			prevStart = s.r.Offset()
			continue
		}
		s.r.Next()
	}
	// Capture the final segment (from last newline to rawEnd)
	s.r.Seek(rawEnd)
	lines = append(lines, string(s.r.From(prevStart)))
	s.r.Seek(start)

	// Determine dedent level. Whitespace-only lines (except the last) are
	// ignored since they don't contribute to the common indentation.
	dedent := math.MaxInt
	for i, line := range lines {
		if i == 0 {
			continue
		}
		indent, allWhite := measureIndent(line)
		if allWhite && i < len(lines)-1 {
			continue
		}
		dedent = min(dedent, indent)
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
			// A last line ending in a backtick loses a single space. This has
			// to happen before the first line is added, because the first and
			// last line may be the same line.
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
			// All whitespace. This is the one place the scanner advances
			// without a matching `push`; the rest of the function always
			// folds this text into a later `KindRawTrimmed`.
			s.r.Seek(s.r.Offset() + len(lines[0]))
		} else {
			lineEnd := s.r.Offset() + len(lines[0])
			if s.r.Peek() == ' ' {
				// Trim a single space after the lang tag on the first line.
				s.r.Next()
				push(syntax.KindRawTrimmed)
			}
			// The rest of the line is known to be non-empty here.
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
			goto BackupAndFinish
		case '/':
			if ch := s.r.Peek(); ch == '/' || ch == '*' {
				goto BackupAndFinish
			}
		case '-':
			if ch := s.r.Peek(); ch == '-' || ch == '?' {
				goto BackupAndFinish
			}
		case '.':
			if s.r.ContinuesWith("..") {
				goto BackupAndFinish
			}
		case 'h':
			if s.r.ContinuesWith("ttp://") || s.r.ContinuesWith("ttps://") {
				goto BackupAndFinish
			}
		case '@':
			if ch := s.r.Peek(); isValidInLabelLiteral(ch) {
				goto BackupAndFinish
			}
		case '\t', '\n', '\r', '\\', '[', ']', '~', '\'', '"', '*',
			'_', ':', '`', '$', '<', '>', '#':
			goto BackupAndFinish
		default:
			if unicode.IsSpace(ch) {
				goto BackupAndFinish
			}
		}
	}
BackupAndFinish:
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
		_, err := strconv.ParseInt(string(number), 10, 64)
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
		x, err := strconv.ParseInt(string(seq), 16, 64)
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

// scanMath tokenizes a single token in math mode. It mirrors Typst's math
// lexer: shorthands, single-character operators, primes, delimiters, and math
// identifiers/text. Delimiters are lexed as `{Left,Right}{Brace,Paren}` and
// converted back to text/shorthand by the parser.
func (s *Scanner) scanMath(start int, ch rune) syntax.Kind {
	switch ch {
	case '\\':
		return s.scanBackslash()
	case '"':
		return s.scanString()

	// Multi-character (and lone `* - ~`) shorthands.
	case '-':
		_ = s.r.ConsumeIf(">>") || s.r.ConsumeIf(">") || s.r.ConsumeIf("->")
		return syntax.KindMathShorthand
	case '~':
		_ = s.r.ConsumeIf("~>") || s.r.ConsumeIf(">")
		return syntax.KindMathShorthand
	case '*':
		return syntax.KindMathShorthand
	case ':':
		if s.r.ConsumeIf("=") || s.r.ConsumeIf(":=") {
			return syntax.KindMathShorthand
		}
		return s.scanMathText(start, ch)
	case '<':
		if s.r.ConsumeIf("==>") || s.r.ConsumeIf("-->") || s.r.ConsumeIf("--") ||
			s.r.ConsumeIf("-<") || s.r.ConsumeIf("->") || s.r.ConsumeIf("<-") ||
			s.r.ConsumeIf("<<") || s.r.ConsumeIf("=>") || s.r.ConsumeIf("==") ||
			s.r.ConsumeIf("~~") || s.r.ConsumeIf("=") || s.r.ConsumeIf("<") ||
			s.r.ConsumeIf("-") || s.r.ConsumeIf("~") {
			return syntax.KindMathShorthand
		}
		return s.scanMathText(start, ch)
	case '>':
		if s.r.ConsumeIf("->") || s.r.ConsumeIf(">>") || s.r.ConsumeIf("=") || s.r.ConsumeIf(">") {
			return syntax.KindMathShorthand
		}
		return s.scanMathText(start, ch)
	case '=':
		if s.r.ConsumeIf("=>") || s.r.ConsumeIf(">") || s.r.ConsumeIf(":") {
			return syntax.KindMathShorthand
		}
		return s.scanMathText(start, ch)
	case '|':
		if s.r.ConsumeIf("->") || s.r.ConsumeIf("=>") || s.r.ConsumeIf("|") {
			return syntax.KindMathShorthand
		}
		if s.r.ConsumeIf("]") {
			return syntax.KindRightBrace
		}
		return s.scanMathText(start, ch)

	// Single-character tokens.
	case '.':
		if s.r.ConsumeIf("..") {
			return syntax.KindMathShorthand
		}
		return syntax.KindDot
	case ',':
		return syntax.KindComma
	case ';':
		return syntax.KindSemicolon
	case '#':
		return syntax.KindHash
	case '_':
		return syntax.KindUnderscore
	case '$':
		return syntax.KindDollar
	case '/':
		return syntax.KindSlash
	case '^':
		return syntax.KindHat
	case '&':
		return syntax.KindMathAlignPoint
	case '√', '∛', '∜':
		return syntax.KindRoot
	case '!':
		if s.r.ConsumeIf("=") {
			return syntax.KindMathShorthand
		}
		return syntax.KindBang

	case '\'':
		s.r.ConsumeWhile(func(c rune) bool { return c == '\'' })
		return syntax.KindMathPrimes

	// Delimiters: lexed as braces/parens, converted back by the parser.
	case '(':
		return syntax.KindLeftParen
	case ')':
		return syntax.KindRightParen
	case '[':
		// `[|` is the double-bracket shorthand; a lone `[` opens on its own.
		s.r.ConsumeIf("|")
		return syntax.KindLeftBrace
	case '{':
		return syntax.KindLeftBrace
	case ']', '}':
		return syntax.KindRightBrace

	default:
		// Identifiers require an id-start followed by at least one id-continue;
		// a lone letter is math text (rendered italic).
		if isMathIDStart(ch) && isMathIDContinue(s.r.Peek()) {
			s.r.ConsumeWhile(isMathIDContinue)
			return s.scanMathIdentOrField(start)
		}
		return s.scanMathText(start, ch)
	}
}

// scanMathIdentOrField returns a MathIdent, or a FieldAccess node (stashed in
// s.node) if the identifier is followed by one or more `.field` accesses.
func (s *Scanner) scanMathIdentOrField(start int) syntax.Kind {
	kind := syntax.KindMathIdent
	var node syntax.Node = s.a.Leaf(kind, s.spanFrom(start))
	for {
		identStart, ok := s.maybeDotIdent()
		if !ok {
			break
		}
		identEnd := s.r.Offset()
		dot := s.a.Leaf(syntax.KindDot, syntax.Span{Start: uint32(identStart - 1), End: uint32(identStart)})
		ident := s.a.Leaf(syntax.KindIdent, syntax.Span{Start: uint32(identStart), End: uint32(identEnd)})
		kind = syntax.KindFieldAccess
		parts := s.a.Nodes(3)
		parts[0], parts[1], parts[2] = node, dot, ident
		node = s.a.Inner(kind, parts)
	}
	if kind == syntax.KindFieldAccess {
		s.node = node
	}
	return kind
}

// maybeDotIdent, when positioned at a `.` directly followed by a math
// identifier, consumes `.ident` and returns the byte offset where the
// identifier begins. Otherwise it consumes nothing.
func (s *Scanner) maybeDotIdent() (int, bool) {
	if next, ok := s.r.Scout(1); ok && isMathIDStart(next) && s.r.ConsumeIf(".") {
		identStart := s.r.Offset()
		s.r.Next()
		s.r.ConsumeWhile(isMathIDContinue)
		return identStart, true
	}
	return 0, false
}

// scanMathText consumes a math text atom: a run of digits (optionally with a
// fractional part), or a single character.
func (s *Scanner) scanMathText(start int, ch rune) syntax.Kind {
	if unicode.IsNumber(ch) {
		s.r.ConsumeWhile(unicode.IsNumber)
		if s.r.Peek() == '.' {
			if next, ok := s.r.Scout(1); ok && unicode.IsNumber(next) {
				s.r.Next()
				s.r.ConsumeWhile(unicode.IsNumber)
			}
		}
	}
	// Otherwise the single rune ch is already consumed.
	return syntax.KindMathText
}

// MaybeMathNamedArg looks for a named argument, `name:`, at byte offset start.
// On a match it consumes the name and returns its node, leaving the scanner on
// the `:`. On no match it returns nil, having scanned nothing.
func (s *Scanner) MaybeMathNamedArg(start int) syntax.Node {
	cursor := s.r.Offset()
	s.r.Seek(start)
	if isIDStart(s.r.Peek()) {
		s.r.Next()
		s.r.ConsumeWhile(isIDContinue)
		// A colon must directly follow, and not the `:=`/`::=` shorthands.
		if s.r.Peek() == ':' && !s.r.ContinuesWith(":=") && !s.r.ContinuesWith("::=") {
			text := s.r.From(start)
			if !bytes.Equal(text, []byte("_")) {
				return s.a.Leaf(syntax.KindIdent, s.spanFrom(start))
			}
			return s.a.Error(s.spanFrom(start), "expected identifier, found underscore")
		}
	}
	s.r.Seek(cursor)
	return nil
}

// MaybeMathSpreadArg looks for a spread argument, `..`, at byte offset start.
// On a match it consumes it and returns a Dots node. On no match it returns
// nil, having scanned nothing.
func (s *Scanner) MaybeMathSpreadArg(start int) syntax.Node {
	cursor := s.r.Offset()
	s.r.Seek(start)
	if s.r.ConsumeIf("..") {
		// Don't infer a spread before trivia/end, a dot (`...` shorthand), or an
		// argument terminator (spreads nothing).
		if ch := s.r.Peek(); !s.spaceOrEnd() && ch != '.' && ch != ',' && ch != ';' && ch != ')' && ch != '$' {
			return s.a.Leaf(syntax.KindDots, s.spanFrom(start))
		}
	}
	s.r.Seek(cursor)
	return nil
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
	ident := string(s.r.From(start))

	prev := s.r.Upto(start)
	if (!bytes.HasSuffix(prev, []byte(".")) || bytes.HasSuffix(prev, []byte(".."))) && !bytes.HasSuffix(prev, []byte("@")) {
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
				seq := s.r.ConsumeWhile(isASCIIAlphanumeric)
				if !s.r.ConsumeIf("}") {
					if err == "" {
						err = "invalid unicode escape sequence"
						hints = []string{"expected '}'"}
					}
					continue
				}
				// Validate the code point itself, not just the shape, so the
				// analyzer never has to decode an escape that isn't a
				// character (mirrors [Scanner.scanBackslash] for markup).
				if x, perr := strconv.ParseInt(string(seq), 16, 64); perr != nil || x > unicode.MaxRune || (0xD800 <= x && x < 0xE000) {
					if err == "" {
						err = "invalid unicode escape sequence"
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

	// Handle numbers with a trailing incomplete exponent like `1e` as an invalid floating point
	// number rather than an invalid suffix.
	if bytes.Equal(suffix, []byte("e")) || bytes.Equal(suffix, []byte("E")) {
		number = s.r.From(start)
		suffix = []byte{}
		isFloat = true
	}

	var suffixErr string
	if len(suffix) != 0 {
		if _, ok := numberSuffixes[string(suffix)]; !ok {
			suffixErr = fmt.Sprintf("invalid number suffix: %s", string(suffix))
		}
	}

	var numberErr string
	switch {
	case isFloat:
		if _, err := strconv.ParseFloat(string(number), 64); err != nil && !errors.Is(err, strconv.ErrRange) {
			numberErr = fmt.Sprintf("invalid floating point number: %s", string(number))
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
		value, err := strconv.ParseInt(string(number[prefix:]), base, 64)
		if err != nil {
			numberErr = fmt.Sprintf("invalid %s number: %s", name, string(number))
			break
		}
		if len(suffix) != 0 && base != 10 {
			if suffixErr == "" {
				suffixErr = fmt.Sprintf("try using a decimal number %d%s", value, string(suffix))
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
	case len(suffix) != 0:
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

// endsWithLineBreak reports whether content ends in a line terminator. A lone
// CR counts, just as [isNewline] accepts it.
func endsWithLineBreak(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	return isNewline(rune(content[len(content)-1]))
}
func isValidInLabelLiteral(ch rune) bool {
	return isIDContinue(ch) || ch == ':' || ch == '.'
}

func isIDStart(ch rune) bool {
	return syntax.IsIdentStart(ch)
}

func isIDContinue(ch rune) bool {
	return syntax.IsIdentContinue(ch)
}

// isMathIDStart reports whether ch can start a math identifier. Unlike code
// identifiers, `_` is excluded (it is the subscript operator in math).
func isMathIDStart(ch rune) bool {
	return unicode.IsLetter(ch)
}

// isMathIDContinue reports whether ch can continue a math identifier. Unlike
// code identifiers, `_` and `-` are excluded (they are math operators).
func isMathIDContinue(ch rune) bool {
	return isAlphanumeric(ch)
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

// measureIndent returns the number of leading whitespace runes in s
// and whether the entire string is whitespace.
func measureIndent(s string) (indent int, allWhite bool) {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return indent, false
		}
		indent++
	}
	return indent, true
}
