package syntax

// Mode is one of the three ways Markst source can be read: as markup, as math,
// or as code. The same characters tokenize differently in each, so the parser
// switches the scanner's mode as it enters and leaves each construct.
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
