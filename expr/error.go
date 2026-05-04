package expr

import (
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
