// Package syntax defines the core types shared across all stages of the Markst
// compilation pipeline: scanning, parsing, and semantic analysis.
//
// It provides the fundamental building blocks for representing Markst source
// code as a concrete syntax tree (CST).
//
//   - [Kind] classifies every token and node (~150 variants covering markup,
//     math, code, operators, and keywords).
//   - [Node] is the interface implemented by all tree nodes: [Leaf] (terminal
//     tokens), [Inner] (non-terminal nodes with children), and [Error] (invalid
//     syntax with an attached diagnostic message).
//   - [Span] and [Position] locate nodes in source text by byte offset and
//     line/column respectively. [Source] maps between the two.
//   - [Mode] distinguishes the three lexical modes of Markst: Markup, Math, and
//     Code.
//   - [Set] is a compact bitset over [Kind] values, used by the parser for
//     efficient lookahead and synchronization.
//
// The tree produced by [parser.Parse] is untyped: every node is a [Node] whose
// meaning is determined by its [Kind]. The [analyzer] package converts this
// untyped tree into the typed IR defined in package [ir].
//
// This package corresponds to the typst-syntax crate in the upstream Typst
// implementation.
package syntax
