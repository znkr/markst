// Package eval runs the SSA IR the analyzer produced and returns the document
// it builds.
//
// [Eval] takes an [expr.Module] and returns the realized document along with
// any warnings and errors. [EvalExports] does the same for a module compiled
// as a library, returning its top-level bindings as well.
//
// Free names are already resolved to constants by the analyzer, so evaluation
// keeps no scope of its own.
//
// # Error handling
//
// A failed instruction does not stop evaluation. It records a [value.Error] and
// writes that same error into its own result, so later instructions reading it
// produce it in turn without recording anything new. One failure is therefore
// reported once, however far its value travels.
//
// A few instructions — [expr.ContentResult], [expr.CodeJoin], [expr.JoinAdd] —
// opt out of that, so a document whose one content block failed still renders
// the rest.
package eval
