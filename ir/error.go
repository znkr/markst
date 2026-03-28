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

// ArgError ////////////////////////////////////////////////////////////////////////////////////////

type ArgError struct {
	match func(i int, arg Arg) Expr
	msg   string
	hints []string
}

func ArgErrorPosf(idx int, format string, args ...any) *ArgError {
	return &ArgError{
		match: func(i int, arg Arg) Expr {
			earg, ok := arg.(*ExprArg)
			if ok && i == idx {
				return earg.expr
			}
			return nil
		},
		msg:   fmt.Sprintf(format, args...),
		hints: nil,
	}
}

func ArgErrorNamedf(name unique.Handle[string], format string, args ...any) *ArgError {
	return &ArgError{
		match: func(i int, arg Arg) Expr {
			narg, ok := arg.(*NamedArg)
			if ok && narg.name == name {
				return narg.expr
			}
			return nil
		},
		msg:   fmt.Sprintf(format, args...),
		hints: nil,
	}
}

func (e *ArgError) Error() string { return e.msg }
func (e *ArgError) Hint(hint string) {
	e.hints = append(e.hints, hint)
}

// IndexError //////////////////////////////////////////////////////////////////////////////////////

type indexError struct {
	err error
	idx int
}

func (e *indexError) Error() string { return e.err.Error() }
func (e *indexError) Unwrap() error { return e.err }

// panics //////////////////////////////////////////////////////////////////////////////////////////

type errWrapper struct {
	err Error
}

func raise(err Error) {
	panic(&errWrapper{err})
}
