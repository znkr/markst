// Package eval executes the SSA-form IR produced by the analyzer and returns a
// [value.Content] document, along with any non-fatal warnings or fatal errors
// encountered during evaluation.
//
// The public API is a single function:
//
//   - [Eval] accepts an [expr.Module] — whose free names have already been
//     resolved to constants by the analyzer — and returns the top-level
//     document content together with warnings and errors.
//
// Internally, evaluation is driven by two data structures:
//
//   - [session] holds the module-wide state: the module being evaluated, the
//     accumulated error and warning lists, and the document-level label
//     registry. One session is created per [Eval] call.
//   - [frame] is the per-function-call state: the current [expr.Function] and
//     the SSA value table. Nested function calls each get their own frame; the
//     session is shared.
//
// The block-dispatch loop in runFunction drives each function's CFG: it
// executes straight-line [expr.Instruction]s via evalInst, then dispatches the
// terminator ([expr.Jump], [expr.Branch], [expr.Return]). Jump and Branch
// also bind the successor block's [expr.BlockParam] slots from the arg list
// carried on the terminator, replacing the predecessor-tracking and phi-
// resolution loop that the old IR shape required.
//
// # Error handling
//
// Eval uses a "poisoned value" model: when an instruction fails, it records a
// [*value.Error] on the session and writes the same error value into its SSA
// result slot via [frame.fail]. Downstream instructions that read a poisoned
// operand propagate it without re-recording, so the error list never accumulates
// duplicates from a single cascading failure.
//
// A small set of instructions ([expr.ContentResult], [expr.CodeJoin],
// [expr.JoinAdd]) opt out of automatic operand propagation so they can
// still produce a partial result — e.g. rendering the rest of a document even
// when one content block failed.
package eval
