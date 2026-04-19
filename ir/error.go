package ir

import (
	"fmt"
	"unique"

	"znkr.io/writst/syntax"
)

// Error is the interface for evaluation errors. Each error carries a source
// [syntax.Span] for location reporting, a message, and optional hints.
type Error interface {
	Span() syntax.Span
	Error() string
	Hints() []string

	aError()
}

// ValueError is an error produced during value evaluation, such as a type
// mismatch or out-of-range value.
type ValueError struct {
	span  syntax.Span
	msg   string
	hints []string
}

func (err *ValueError) Span() syntax.Span { return err.span }
func (err *ValueError) Error() string     { return err.msg }
func (err *ValueError) Hints() []string   { return err.hints }
func (err *ValueError) aError()           {}

// ErrorList is a collection of evaluation errors that implements the error
// interface. Its Error method returns the first error's message.
type ErrorList []Error

func (err ErrorList) Error() string { return err[0].Error() }
func (err ErrorList) Unwrap() []error {
	r := make([]error, 0, len(err))
	for _, e := range err {
		r = append(r, e)
	}
	return r
}

// ArgError ////////////////////////////////////////////////////////////////////

// ArgError is an error associated with a specific function argument.
type ArgError struct {
	// match is a function that checks whether a given argument matches the one
	// associated with this error, and if so returns its source span for error
	// reporting.
	match func(i int, arg Arg) (syntax.Span, bool)
	// msg is the error message to report for this argument.
	msg string
	// hints are optional hints to provide to the user for this error.
	hints []string
}

// ArgErrorPosf creates an [ArgError] pointing at positional argument idx.
// Index 0 is "self" for method calls; index 1 is the first explicit argument.
func ArgErrorPosf(idx int, format string, args ...any) *ArgError {
	return &ArgError{
		match: func(i int, arg Arg) (syntax.Span, bool) {
			earg, ok := arg.(*ExprArg)
			if ok && i == idx {
				return earg.expr.Span(), true
			}
			return syntax.Span{}, false
		},
		msg:   fmt.Sprintf(format, args...),
		hints: nil,
	}
}

// ArgErrorNamedf creates an [ArgError] pointing at the value of the named
// argument with the given name.
func ArgErrorNamedf(name unique.Handle[string], format string, args ...any) *ArgError {
	return &ArgError{
		match: func(i int, arg Arg) (syntax.Span, bool) {
			narg, ok := arg.(*NamedArg)
			if ok && narg.name == name {
				return narg.expr.Span(), true
			}
			return syntax.Span{}, false
		},
		msg:   fmt.Sprintf(format, args...),
		hints: nil,
	}
}

// ArgErrorNamedPairf creates an ArgError that points at the entire named pair
// (key + value), used for unknown/unexpected named arguments.
func ArgErrorNamedPairf(name unique.Handle[string], format string, args ...any) *ArgError {
	return &ArgError{
		match: func(i int, arg Arg) (syntax.Span, bool) {
			narg, ok := arg.(*NamedArg)
			if ok && narg.name == name {
				return narg.Span(), true
			}
			return syntax.Span{}, false
		},
		msg:   fmt.Sprintf(format, args...),
		hints: nil,
	}
}

func (e *ArgError) Error() string { return e.msg }
func (e *ArgError) Hint(hint string) {
	e.hints = append(e.hints, hint)
}

// ArgErrors is a list of ArgError that implements error. It is used to report
// multiple argument-related errors (e.g. zip with exact: true and multiple
// mismatched arrays).
type ArgErrors []*ArgError

func (e ArgErrors) Error() string { return e[0].msg }

// IndexError //////////////////////////////////////////////////////////////////

type indexError struct {
	err error
	idx int
}

func (e *indexError) Error() string { return e.err.Error() }
func (e *indexError) Unwrap() error { return e.err }

// panics //////////////////////////////////////////////////////////////////////

type errWrapper struct {
	err []Error
}

func raise(err ...Error) {
	panic(&errWrapper{err})
}
