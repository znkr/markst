package eval

import (
	"znkr.io/writst/value"
)

// Error is the evaluator's diagnostic type. It is a type alias for
// [*value.Error] so the same runtime value that flows through the SSA value
// table is also what callers see in the returned error list — no wrapping
// or duplicate types.
type Error = *value.Error

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
