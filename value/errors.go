package value

import (
	"fmt"

	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
)

// Error is a runtime value representing a failed computation. Unlike
// [FunctionCallError] (a Go error type used inside builtin call sites), Error
// is both a [Value] (so it can flow through the SSA value table like any
// other runtime value) and a Go `error` (so it can satisfy the diagnostic
// interfaces consumed by callers of the evaluator). Evaluation continues
// past Error values rather than tearing down the whole program.
//
// Instructions whose operands include an Error propagate it as their result
// without re-recording on the session error list; operations that genuinely
// require a non-error operand skip silently because the upstream Error is
// already in the list.
type Error struct {
	Span  syntax.Span
	Msg   string
	Hints []string
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

// Error returns the diagnostic message. It exists so *Error satisfies the
// standard library `error` interface; the underlying data lives in [Msg].
func (e *Error) Error() string { return e.Msg }

// IsError reports whether v is a [*Error] value.
func IsError(v Value) (*Error, bool) {
	e, ok := v.(*Error)
	return e, ok
}

// FunctionCallError represents an error that occurred during a function call.
type FunctionCallError struct {
	Msg      string
	Hints    []string
	Location ArgLoc
}

func (e *FunctionCallError) Error() string    { return e.Msg }
func (e *FunctionCallError) Hint(hint string) { e.Hints = append(e.Hints, hint) }

type ArgLoc interface {
	aArgLoc()
}

type ArgLocPositional int
type ArgLocNamed name.Name
type ArgLocNamedPair name.Name

func (ArgLocPositional) aArgLoc() {}
func (ArgLocNamed) aArgLoc()      {}
func (ArgLocNamedPair) aArgLoc()  {}

// ArgErrorPosf creates an [ArgError] pointing at positional argument idx.
// Index 0 is "self" for method calls; index 1 is the first explicit argument.
func ArgErrorPosf(idx int, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocPositional(idx),
	}
}

// ArgErrorNamedf creates an [ArgError] pointing at the value of the named
// argument with the given name.
func ArgErrorNamedf(name name.Name, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocNamed(name),
	}
}

// ArgErrorNamedPairf creates an ArgError that points at the entire named pair
// (key + value), used for unknown/unexpected named arguments.
func ArgErrorNamedPairf(name name.Name, format string, args ...any) *FunctionCallError {
	return &FunctionCallError{
		Msg:      fmt.Sprintf(format, args...),
		Location: ArgLocNamedPair(name),
	}
}
