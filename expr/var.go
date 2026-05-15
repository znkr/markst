package expr

import (
	"strconv"

	"znkr.io/writst/name"
)

// Var names an SSA-mode variable in the [Builder]'s Braun construction.
// Each `let`, parameter, or synthetic intermediate (loop accumulator,
// conditional result, ...) allocates a fresh Var. Two Vars are equal iff
// both fields match, so distinct `let`s of the same source identifier
// produce distinct Vars and shadowing works without string mangling.
//
// Vars are a construction-time concept only: they do not appear in the
// final IR, which identifies SSA values by [Ref]. They show up in
// diagnostic SSA dumps via [Var.String].
type Var struct {
	// Name is the display name: the source identifier for `let`-introduced
	// vars, or a synthetic tag (e.g. "$cond", "$acc") for compiler-
	// generated ones.
	Name name.Name

	// Version disambiguates instances of the same Name. 0 is reserved for
	// "no disambiguation needed."
	Version int
}

// String renders the var as "name$version" (or just "name" when Version
// is 0). Used by SSA dumps and error messages.
func (v Var) String() string {
	if v.Version == 0 {
		return v.Name.String()
	}
	return v.Name.String() + "$" + strconv.Itoa(v.Version)
}
