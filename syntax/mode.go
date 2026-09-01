package syntax

// Mode represents the current lexical mode of the scanner. Markst source code
// is lexed differently depending on whether the scanner is in markup, math,
// or code context. The parser switches modes by calling [scanner.Scanner.SetMode]
// as it enters and exits different syntactic constructs.
type Mode int

const (
	// ModeMarkup is the default mode for text and markup constructs.
	// Whitespace in markup mode is limited to space (U+0020), tab (U+0009),
	// and newlines (LF, CR).
	ModeMarkup Mode = iota
	// ModeMath is active inside math equations ($ ... $).
	ModeMath
	// ModeCode is active inside code blocks and after #.
	// Whitespace in code mode includes all Unicode whitespace characters
	// (as defined by unicode.IsSpace).
	ModeCode
)
