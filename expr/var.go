package expr

import (
	"strconv"

	"znkr.io/markst/name"
)

// Var names a variable while a [Builder] is constructing a function. Each
// `let`, each parameter, and each intermediate the analyzer needs — a loop
// accumulator, a conditional's result — gets its own Var. Two Vars are equal
// only if both fields match, so two `let`s of the same identifier are two
// different Vars and shadowing needs no renaming.
//
// A Var exists only during construction. The finished IR names values by
// [Ref].
type Var struct {
	// Name is the display name: the source identifier for `let`-introduced
	// vars, or a synthetic tag (e.g. "$cond", "$acc") for compiler-
	// generated ones.
	Name name.Name

	// Version disambiguates instances of the same Name. 0 is reserved for
	// "no disambiguation needed."
	Version int
}

// String returns the var as "name$version", or just "name" when the version is
// 0, which is how SSA dumps and diagnostics name it.
func (v Var) String() string {
	if v.Version == 0 {
		return v.Name.String()
	}
	return v.Name.String() + "$" + strconv.Itoa(v.Version)
}
