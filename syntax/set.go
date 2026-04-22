package syntax

// Set is a compact bitset over [Kind] values, stored as three uint64 words
// to cover all ~150 Kind constants. It is used by the parser for efficient
// lookahead checks and token synchronization (e.g. "is this token a binary
// operator?" or "can this token start an expression?").
type Set [3]uint64

// SetOf creates a Set containing the given kinds.
func SetOf(kind ...Kind) Set {
	var s Set
	for _, k := range kind {
		i, bit := k/64, k%64
		s[i] |= 1 << bit
	}
	return s
}

// Contains reports whether the set contains the given kind.
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
)
