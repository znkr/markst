package value

import (
	"fmt"

	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/types"
)

// Error is a failed computation, as a value. It is a [Value], so it flows
// through evaluation like any other, and a Go error, so callers can report it.
//
// Evaluation does not stop at an Error. An instruction handed one produces it
// as its own result and records nothing new, so a single failure is reported
// once no matter how far its value travels.
type Error struct {
	Span syntax.Span

	// Origin is the source Span points into. An error carries it because more
	// than one source can be in play: a failure inside a library function
	// happens while a document is being compiled, but its span indexes the
	// library's bytes, not the document's.
	Origin syntax.Origin

	Msg   string
	Hints []Hint

	// Trace is the chain of call sites that led here, outermost first, holding
	// only the calls that crossed from one [syntax.Origin] into another. A
	// failure entirely within one source has an empty Trace.
	Trace []syntax.Frame
}

// Hint is a suggestion attached to a diagnostic. Span points at what the hint
// is about, which need not be where the diagnostic points: "`phi` is not a
// function" belongs on the callee, while the error it explains belongs on the
// argument. A hint with nothing of its own to point at uses [syntax.NoSpan]
// and is taken to be about the diagnostic's own span.
type Hint struct {
	Span syntax.Span
	Msg  string
}

// Hints turns plain messages into hints with no span of their own, so each one
// points at the diagnostic it is attached to. That is what most hints want.
func Hints(msgs ...string) []Hint {
	if len(msgs) == 0 {
		return nil
	}
	hints := make([]Hint, len(msgs))
	for i, m := range msgs {
		hints[i] = Hint{Span: syntax.NoSpan, Msg: m}
	}
	return hints
}

func (e *Error) Type() types.Type { return types.Error }
func (e *Error) aValue()          {}

func (e *Error) Equal(other Value) bool {
	o, ok := other.(*Error)
	return ok && e == o
}

func (e *Error) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Printf("<error: %s>", e.Msg)
}

// Error returns the diagnostic message, so that an *Error is a Go error.
func (e *Error) Error() string { return e.Msg }

// IsError returns v as an [Error] and reports whether it is one.
func IsError(v Value) (*Error, bool) {
	e, ok := v.(*Error)
	return e, ok
}

// FunctionCallError is what a builtin returns when a call fails. Location says
// which argument is at fault, so the evaluator can point the diagnostic at that
// argument in the source rather than at the whole call.
type FunctionCallError struct {
	Msg      string
	Hints    []string
	Location ArgLoc
}

// Error returns the message, so that a *FunctionCallError is a Go error.
func (e *FunctionCallError) Error() string { return e.Msg }

// Hint adds a hint to the diagnostic, shown alongside the message.
func (e *FunctionCallError) Hint(hint string) { e.Hints = append(e.Hints, hint) }

// ArgLoc names the argument a [FunctionCallError] is about. It is one of
// [ArgLocPositional], [ArgLocNamed], or [ArgLocNamedPair].
type ArgLoc interface {
	aArgLoc()
}

// ArgLocPositional is the positional argument at this index. Index 0 is the
// receiver of a method call.
type ArgLocPositional int

// ArgLocNamed is the value of the named argument with this name.
type ArgLocNamed name.Name

// ArgLocNamedPair is a named argument's name and value together, which is what
// an unexpected argument should be reported on.
type ArgLocNamedPair name.Name

func (ArgLocPositional) aArgLoc() {}
func (ArgLocNamed) aArgLoc()      {}
func (ArgLocNamedPair) aArgLoc()  {}

// ArgErrorPosf returns an error about positional argument idx. Index 0 is the
// receiver of a method call, so the first explicit argument is index 1.
func ArgErrorPosf(idx int, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocPositional(idx),
	}
}

// ArgErrorNamedf returns an error about the value of the named argument name.
func ArgErrorNamedf(name name.Name, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocNamed(name),
	}
}

// ArgErrorNamedPairf returns an error about a named argument's name and value
// together. Use it when the argument itself is the problem, as for one the
// function does not accept.
func ArgErrorNamedPairf(name name.Name, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocNamedPair(name),
	}
}
