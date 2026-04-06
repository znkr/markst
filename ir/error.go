package ir

import (
	"fmt"
	"unique"

	"znkr.io/writst/syntax"
)

type Error interface {
	Span() syntax.Span
	Error() string
	Hints() []string

	aError()
}

type ValueError struct {
	span  syntax.Span
	msg   string
	hints []string
}

func (err *ValueError) Span() syntax.Span { return err.span }
func (err *ValueError) Error() string     { return err.msg }
func (err *ValueError) Hints() []string   { return err.hints }
func (err *ValueError) aError()           {}

type ErrorList []Error

func (err ErrorList) Error() string { return err[0].Error() }
func (err ErrorList) Unwrap() []error {
	r := make([]error, 0, len(err))
	for _, e := range err {
		r = append(r, e)
	}
	return r
}

// ArgError ////////////////////////////////////////////////////////////////////////////////////////

type ArgError struct {
	match func(i int, arg Arg) (syntax.Span, bool)
	msg   string
	hints []string
}

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

// IndexError //////////////////////////////////////////////////////////////////////////////////////

type indexError struct {
	err error
	idx int
}

func (e *indexError) Error() string { return e.err.Error() }
func (e *indexError) Unwrap() error { return e.err }

// panics //////////////////////////////////////////////////////////////////////////////////////////

type errWrapper struct {
	err []Error
}

func raise(err ...Error) {
	panic(&errWrapper{err})
}
