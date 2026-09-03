package eval

import (
	"znkr.io/markst/value"
)

// Error is a diagnostic from evaluation. It is an alias for [*value.Error], so
// the value that flows through evaluation is the same one [Eval] returns.
//
// It locates itself by byte offset rather than line and column, since the
// evaluator has no [syntax.Source] to resolve offsets against. Resolving
// happens once, in znkr.io/markst.Compile.
type Error = *value.Error
