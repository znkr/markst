package eval

import (
	"znkr.io/writst/value"
)

// Error is the evaluator's diagnostic type. It is a type alias for
// [*value.Error] so the same runtime value that flows through the SSA value
// table is also what [Eval] returns — no wrapping or duplicate types.
//
// It carries a [syntax.Span], not a resolved position: the evaluator works in
// byte offsets and has no [syntax.Source] to resolve them against. Resolving
// happens once, at the boundary, where znkr.io/writst.Compile turns these into
// the self-describing diagnostics it hands to callers.
type Error = *value.Error
