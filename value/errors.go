package value

import (
	"fmt"

	"znkr.io/writst/name"
)

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
