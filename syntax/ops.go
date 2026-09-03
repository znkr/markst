// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package syntax

import "fmt"

// UnaryOp is one of Markst's prefix operators.
type UnaryOp int

const (
	Pos UnaryOp = iota // The plus operator: +
	Neg                // The negation operator: -
	Not                // The boolean 'not'.
)

// UnaryOpFromKind returns the operator a token stands for. It panics if kind
// is not a unary operator token.
func UnaryOpFromKind(kind Kind) UnaryOp {
	switch kind {
	case KindPlus:
		return Pos
	case KindMinus:
		return Neg
	case KindNot:
		return Not
	default:
		panic(fmt.Sprintf("not an unary op: %v", kind))
	}
}

// Precedence returns how tightly the operator binds. Higher binds tighter.
func (op UnaryOp) Precedence() int {
	switch op {
	case Pos, Neg:
		return 7
	case Not:
		return 4
	default:
		panic(fmt.Sprintf("invalid UnaryOp: %d", op))
	}
}

// String returns the operator as it is written in source.
func (op UnaryOp) String() string {
	switch op {
	case Pos:
		return "+"
	case Neg:
		return "-"
	case Not:
		return "not"
	default:
		return fmt.Sprintf("UnaryOp(%d)", op)
	}
}

// Assoc is which way a run of same-precedence operators groups.
type Assoc int

const (
	AssocLeft  Assoc = iota // Left-to-right associativity.
	AssocRight              // Right-to-left associativity.
)

// BinaryOp is one of Markst's infix operators. [BinaryOp.Precedence] and
// [BinaryOp.Assoc] are what the parser groups an expression by.
type BinaryOp int

const (
	Add       BinaryOp = iota // The addition operator: +
	Sub                       // The subtraction operator: -
	Mul                       // The multiplication operator: *
	Div                       // The division operator: /
	And                       // The short-circuiting boolean 'and'.
	Or                        // The short-circuiting boolean 'or'.
	Eq                        // The equality operator: ==
	Neq                       // The inequality operator: !=
	Lt                        // The less-than operator: <
	Leq                       // The less-than or equal operator: <=
	Gt                        // The greater-than operator: >
	Geq                       // The greater-than or equal operator: >=
	In                        // The containment operator: in
	NotIn                     // The inverse containment operator: not in
	Assign                    // The assignment operator: =
	AddAssign                 // The add-assign operator: +=
	SubAssign                 // The subtract-assign operator: -=
	MulAssign                 // The multiply-assign operator: *=
	DivAssign                 // The divide-assign operator: /=
)

// BinaryOpFromKind returns the operator a token stands for. It panics if kind
// is not a binary operator token. `not in` is two tokens, so the parser
// recognizes it rather than this function.
func BinaryOpFromKind(kind Kind) BinaryOp {
	switch kind {
	case KindPlus:
		return Add
	case KindMinus:
		return Sub
	case KindStar:
		return Mul
	case KindSlash:
		return Div
	case KindAnd:
		return And
	case KindOr:
		return Or
	case KindEqEq:
		return Eq
	case KindExclEq:
		return Neq
	case KindLt:
		return Lt
	case KindLtEq:
		return Leq
	case KindGt:
		return Gt
	case KindGtEq:
		return Geq
	case KindIn:
		return In
	case KindEq:
		return Assign
	case KindPlusEq:
		return AddAssign
	case KindHyphEq:
		return SubAssign
	case KindStarEq:
		return MulAssign
	case KindSlashEq:
		return DivAssign
	default:
		panic(fmt.Sprintf("not a binary op: %v", kind))
	}
}

// Precedence returns how tightly the operator binds. Higher binds tighter.
func (op BinaryOp) Precedence() int {
	switch op {
	case Mul, Div:
		return 6
	case Add, Sub:
		return 5
	case Eq, Neq, Lt, Leq, Gt, Geq, In, NotIn:
		return 4
	case And:
		return 3
	case Or:
		return 2
	case Assign, AddAssign, SubAssign, MulAssign, DivAssign:
		return 1
	default:
		panic(fmt.Sprintf("invalid BinaryOp: %d", op))
	}
}

// Assoc returns which way a run of this operator groups. Assignment groups to
// the right, everything else to the left.
func (op BinaryOp) Assoc() Assoc {
	switch op {
	case Assign, AddAssign, SubAssign, MulAssign, DivAssign:
		return AssocRight
	default:
		return AssocLeft
	}
}

// IsAssign reports whether the operator is an assignment (=, +=, -=, *=, /=).
func (op BinaryOp) IsAssign() bool {
	switch op {
	case Assign, AddAssign, SubAssign, MulAssign, DivAssign:
		return true
	default:
		return false
	}
}

// StripAssign returns the arithmetic operator behind a compound assignment,
// so [AddAssign] becomes [Add]. Any other operator is returned unchanged.
func (op BinaryOp) StripAssign() BinaryOp {
	switch op {
	case AddAssign:
		return Add
	case SubAssign:
		return Sub
	case MulAssign:
		return Mul
	case DivAssign:
		return Div
	default:
		return op
	}
}

// IsComparison reports whether the operator is a comparison or containment
// check (==, !=, <, <=, >, >=, in, not in).
func (op BinaryOp) IsComparison() bool {
	switch op {
	case Eq, Neq, Lt, Leq, Gt, Geq, In, NotIn:
		return true
	default:
		return false
	}
}

// String returns the operator as it is written in source.
func (op BinaryOp) String() string {
	switch op {
	case Add:
		return "+"
	case Sub:
		return "-"
	case Mul:
		return "*"
	case Div:
		return "/"
	case And:
		return "and"
	case Or:
		return "or"
	case Eq:
		return "=="
	case Neq:
		return "!="
	case Lt:
		return "<"
	case Leq:
		return "<="
	case Gt:
		return ">"
	case Geq:
		return ">="
	case In:
		return "in"
	case NotIn:
		return "not in"
	case Assign:
		return "="
	case AddAssign:
		return "+="
	case SubAssign:
		return "-="
	case MulAssign:
		return "*="
	case DivAssign:
		return "/="
	default:
		return fmt.Sprintf("BinaryOp(%d)", op)
	}
}
