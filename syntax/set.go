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

// Set is a group of [Kind]s, as a bitset. The parser uses one to answer
// questions about a token in a single test: is it a binary operator, can it
// start an expression, should it be skipped.
type Set [3]uint64

// SetOf returns the set holding exactly the given kinds.
func SetOf(kind ...Kind) Set {
	var s Set
	for _, k := range kind {
		i, bit := k/64, k%64
		s[i] |= 1 << bit
	}
	return s
}

// Contains reports whether k is in the set.
func (s Set) Contains(k Kind) bool {
	i, bit := k/64, k%64
	return s[i]&(1<<bit) != 0
}

func (s Set) add(k ...Kind) Set {
	for _, kind := range k {
		i, bit := kind/64, kind%64
		s[i] |= 1 << bit
	}
	return s
}

// Remove returns the set without the given kinds. The receiver is unchanged.
func (s Set) Remove(k ...Kind) Set {
	for _, kind := range k {
		i, bit := kind/64, kind%64
		s[i] &^= 1 << bit
	}
	return s
}

func (s Set) union(o Set) Set {
	return Set{
		s[0] | o[0],
		s[1] | o[1],
		s[2] | o[2],
	}
}

// Predefined kind sets used by the parser for lookahead and synchronization.
var (
	// Trivia contains kinds that are skipped by the parser (whitespace,
	// comments).
	Trivia = SetOf(
		KindSpace,
		KindParbreak,
		KindLineComment,
		KindBlockComment,
	)

	Stmts = SetOf(
		KindLet,
		KindSet,
		KindShow,
		KindImport,
		KindInclude,
		KindReturn,
	)

	UnaryOps = SetOf(
		KindPlus,
		KindMinus,
		KindNot,
	)

	BinaryOps = SetOf(KindPlus,
		KindMinus,
		KindStar,
		KindSlash,
		KindAnd,
		KindOr,
		KindEqEq,
		KindExclEq,
		KindLt,
		KindLtEq,
		KindGt,
		KindGtEq,
		KindEq,
		KindIn,
		KindPlusEq,
		KindHyphEq,
		KindStarEq,
		KindSlashEq,
	)

	AtomicCodePrimary = SetOf(
		KindIdent,
		KindLeftBrace,
		KindLeftBracket,
		KindLeftParen,
		KindDollar,
		KindLet,
		KindSet,
		KindShow,
		KindContext,
		KindIf,
		KindWhile,
		KindFor,
		KindImport,
		KindInclude,
		KindBreak,
		KindContinue,
		KindReturn,
		KindNone,
		KindAuto,
		KindInt,
		KindFloat,
		KindBool,
		KindNumeric,
		KindStr,
		KindLabel,
		KindRaw,
	)
	AtomicCodeExpr = AtomicCodePrimary

	CodePrimary       = AtomicCodePrimary.add(KindUnderscore)
	CodeExpr          = CodePrimary.union(UnaryOps)
	ArrayOrDictItem   = CodeExpr.add(KindDots)
	Arg               = CodeExpr.add(KindDots)
	Param             = Pattern.add(KindDots)
	DestructuringItem = Pattern.add(KindDots)

	PatternLeaf = AtomicCodeExpr
	Pattern     = PatternLeaf.add(KindLeftParen, KindUnderscore)

	Keywords = SetOf(
		KindNot,
		KindAnd,
		KindOr,
		KindNone,
		KindAuto,
		KindLet,
		KindSet,
		KindShow,
		KindContext,
		KindIf,
		KindElse,
		KindFor,
		KindIn,
		KindWhile,
		KindBreak,
		KindContinue,
		KindReturn,
		KindImport,
		KindInclude,
		KindAs,
	)

	Terminator = SetOf(
		KindEnd,
		KindSemicolon,
		KindRightBrace,
		KindRightBracket,
		KindRightParen,
	)

	// MathExpr contains the kinds that can start a math expression. Used by the
	// parser to drive the loop over math contents.
	MathExpr = SetOf(
		KindHash,
		KindMathIdent,
		KindFieldAccess,
		KindDot,
		KindComma,
		KindSemicolon,
		KindLeftBrace,
		KindRightBrace,
		KindLeftParen,
		KindRightParen,
		KindMathText,
		KindMathShorthand,
		KindLinebreak,
		KindMathAlignPoint,
		KindMathPrimes,
		KindEscape,
		KindStr,
		KindRoot,
		KindBang,
	)
)
