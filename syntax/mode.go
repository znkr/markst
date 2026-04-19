package syntax

// Mode represents the current lexical mode of the scanner. Writst source code
// is lexed differently depending on whether the scanner is in markup, math,
// or code context. The parser switches modes by calling [scanner.Scanner.SetMode]
// as it enters and exits different syntactic constructs.
type Mode int

const (
	ModeMarkup Mode = iota // ModeMarkup is the default mode for text and markup constructs.
	ModeMath               // ModeMath is active inside math equations ($ ... $).
	ModeCode               // ModeCode is active inside code blocks and after #.
)
