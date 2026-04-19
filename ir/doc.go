// Package ir defines the intermediate representation (IR) for Writst documents.j
//
// The IR is produced by [analyzer.Analyze] from the untyped syntax tree and
// consumed by [Eval] to produce document [Content]. It consists of three main
// hierarchies:
//
//   - [Value]: the runtime value types of Writst
//   - [Content]: a subset of Value representing document content
//   - [Expr]: expression nodes that evaluate to Values
//
// The [Eval] function walks a slice of Expr and produces Content, collecting
// warnings and errors along the way.
package ir
