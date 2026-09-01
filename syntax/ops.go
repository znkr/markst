package syntax

import "fmt"

// UnaryOp represents a unary operator in Markst's expression syntax.
//
// Each operator has a [Precedence] level used by the parser to resolve
// ambiguity in expressions like -x + y.
type UnaryOp int

const (
	Pos UnaryOp = iota // The plus operator: +
	Neg                // The negation operator: -
	Not                // The boolean 'not'.
)

// UnaryOpFromKind converts a token [Kind] to its corresponding UnaryOp.
//
// It panics if kind is not a unary operator token.
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

// Precedence returns the operator's binding strength. Higher values bind more
// tightly.
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

// Assoc represents operator associativity.
type Assoc int

const (
	AssocLeft  Assoc = iota // Left-to-right associativity.
	AssocRight              // Right-to-left associativity.
)

// BinaryOp represents a binary operator in Markst's expression syntax. Each
// operator has a [Precedence] level and [Assoc] (associativity) that the parser
// uses to build the correct expression tree.
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

// BinaryOpFromKind converts a token [Kind] to its corresponding BinaryOp.
// It panics if kind is not a binary operator token. Note that [KindNot] +
// [KindIn] (the "not in" operator) is handled by the parser, not here.
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

// Precedence returns the operator's binding strength. Higher values bind more
// tightly.
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

// Assoc returns the associativity of the operator. Assignment operators are
// right-associative; all others are left-associative.
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

// StripAssign converts a compound assignment operator to its arithmetic
// counterpart (e.g. AddAssign → Add). Non-assignment operators are returned
// unchanged.
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
