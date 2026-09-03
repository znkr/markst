// Package syntax defines the types the scanner, parser, and analyzer share.
//
//   - [Kind] says what a token or node is; there are about 150 of them,
//     covering markup, math, code, operators, and keywords.
//   - [Node] is a node of the syntax tree: a [Leaf] token, an [Inner] node with
//     children, or an [Error] standing for invalid source.
//   - [Span] locates a node by byte offset, [Position] by line and column, and
//     [Source] converts between the two.
//   - [Mode] is one of the three ways Markst source can be read: markup, math,
//     or code.
//   - [Set] is a bitset of [Kind]s, which is how the parser looks ahead.
//
// The tree is untyped: every node is a [Node], and its [Kind] is what says what
// it means. Package znkr.io/markst/syntax/analyzer lowers it to the SSA IR in
// package znkr.io/markst/expr.
//
// This package corresponds to the typst-syntax crate in Typst.
package syntax
