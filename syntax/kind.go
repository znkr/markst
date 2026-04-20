package syntax

// Kind classifies tokens and syntax tree nodes. It serves a dual role: the
// scanner produces (Kind, text) pairs for individual tokens, and the parser
// groups those tokens into inner nodes that also carry a Kind. For example,
// [KindStar] is a token produced by the scanner, while [KindStrong] is a
// composite node assembled by the parser from a pair of KindStar tokens and
// the markup between them.
//
// Kinds are organized into groups: markup elements, math constructs,
// punctuation and operators, keywords, and code-level constructs. Each
// constant carries a comment describing its syntax.
//
// The [Name] method returns a human-readable name for use in diagnostics.
//
// Corresponds to SyntaxKind in the upstream Typst implementation.
//
//go:generate go tool golang.org/x/tools/cmd/stringer -type=Kind
type Kind int

const (
	KindInvalid Kind = iota // Invalid or unset kind. The zero value for Kind.
	KindEnd                 // The end of token stream.
	KindError               // An invalid sequence of characters.

	KindLineComment  // A line comment: // ...
	KindBlockComment // A block comment: /* ... */

	KindMarkup        // The contents of a file or content block.
	KindText          // Plain text without markup.
	KindSpace         // Whitespace. Contains at most one newline in markup, as more indicate a paragraph break.
	KindLinebreak     // A forced line break: \.
	KindParbreak      // A paragraph break, indicated by one or multiple blank lines.
	KindEscape        // An escape sequence: \#, \u{1F5FA}.
	KindShorthand     // A shorthand for a unicode codepoint. For example, ~ for non-breaking space or -? for a soft hyphen.
	KindSmartQuote    // A smart quote: ' or ".
	KindStrong        // Strong content: *Strong*.
	KindEmph          // Emphasized content: _Emphasized_.
	KindRaw           // Raw text with optional syntax highlighting: `...`.
	KindRawLang       // A language tag at the start of raw text: typ .
	KindRawDelim      // A raw delimiter consisting of 1 or 3+ backticks: `.
	KindRawTrimmed    // A sequence of whitespace to ignore in a raw text:     .
	KindLink          // A hyperlink: https://typst.org.
	KindLabel         // A label: <intro>.
	KindRef           // A reference: @target, @target[..].
	KindRefMarker     // Introduces a reference: @target.
	KindHeading       // A section heading: = Introduction.
	KindHeadingMarker // Introduces a section heading: =, ==, ...
	KindListItem      // An item in a bullet list: - ....
	KindListMarker    // Introduces a list item: -.
	KindEnumItem      // An item in an enumeration (numbered list): + ... or 1. ....
	KindEnumMarker    // Introduces an enumeration item: +, 1..
	KindTermItem      // An item in a term list: / Term: Details.
	KindTermMarker    // Introduces a term item: /.
	KindEquation      // A mathematical equation: $x$, $ x^2 $.

	KindMath           // The contents of a mathematical equation: x^2 + 1.
	KindMathText       // A lone text fragment in math: x, 25, 3.1415, =, |, [.
	KindMathIdent      // An identifier in math: pi.
	KindMathShorthand  // A shorthand for a unicode codepoint in math: a <= b.
	KindMathAlignPoint // An alignment point in math: &.
	KindMathDelimited  // Matched delimiters in math: [x + y].
	KindMathAttach     // A base with optional attachments in math: a_1^2.
	KindMathPrimes     // Grouped primes in math: a'''.
	KindMathFrac       // A fraction in math: x/2.
	KindMathRoot       // A root in math: √x, ∛x or ∜x.

	KindHash         // A hash that switches into code mode: #.
	KindLeftBrace    // A left curly brace, starting a code block: {.
	KindRightBrace   // A right curly brace, terminating a code block: }.
	KindLeftBracket  // A left square bracket, starting a content block: [.
	KindRightBracket // A right square bracket, terminating a content block: ].
	KindLeftParen    // A left round parenthesis, starting a grouped expression, collection, argument or parameter list: (.
	KindRightParen   // A right round parenthesis, terminating a grouped expression, collection, argument or parameter list: ).
	KindComma        // A comma separator in a sequence: ,.
	KindSemicolon    // A semicolon terminating an expression: ;.
	KindColon        // A colon between name/key and value in a dictionary, argument or parameter list, or between the term and body of a term list term: :.
	KindStar         // The strong text toggle, multiplication operator, and wildcard import symbol: *.
	KindUnderscore   // Toggles emphasized text and indicates a subscript in math: _.
	KindDollar       // Starts and ends a mathematical equation: $.
	KindPlus         // The unary plus and binary addition operator: +.
	KindMinus        // The unary negation and binary subtraction operator: -.
	KindSlash        // The division operator and fraction operator in math: /.
	KindHat          // The superscript operator in math: ^.
	KindDot          // The field access and method call operator: ..
	KindEq           // The assignment operator: =.
	KindEqEq         // The equality operator: ==.
	KindExclEq       // The inequality operator: !=.
	KindLt           // The less-than operator: <.
	KindLtEq         // The less-than or equal operator: <=.
	KindGt           // The greater-than operator: >.
	KindGtEq         // The greater-than or equal operator: >=.
	KindPlusEq       // The add-assign operator: +=.
	KindHyphEq       // The subtract-assign operator: -=.
	KindStarEq       // The multiply-assign operator: *=.
	KindSlashEq      // The divide-assign operator: /=.
	KindDots         // Indicates a spread or sink: ...
	KindArrow        // An arrow between a closure's parameters and body: =>.
	KindRoot         // A root: √, ∛ or ∜.
	KindBang         // An exclamation mark; groups with directly preceding text in math: !.

	KindNot      // The not operator.
	KindAnd      // The and operator.
	KindOr       // The or operator.
	KindNone     // The none literal.
	KindAuto     // The auto literal.
	KindLet      // The let keyword.
	KindSet      // The set keyword.
	KindShow     // The show keyword.
	KindContext  // The context keyword.
	KindIf       // The if keyword.
	KindElse     // The else keyword.
	KindFor      // The for keyword.
	KindIn       // The in keyword.
	KindWhile    // The while keyword.
	KindBreak    // The break keyword.
	KindContinue // The continue keyword.
	KindReturn   // The return keyword.
	KindImport   // The import keyword.
	KindInclude  // The include keyword.
	KindAs       // The as keyword.

	KindCode               // The contents of a code block.
	KindIdent              // An identifier: it.
	KindBool               // A boolean: true, false.
	KindInt                // An integer: 120.
	KindFloat              // A floating-point number: 1.2, 10e-4.
	KindNumeric            // A numeric value with a unit: 12pt, 3cm, 2em, 90deg, 50%.
	KindStr                // A quoted string: "...".
	KindCodeBlock          // A code block: { let x = 1; x + 2 }.
	KindContentBlock       // A content block: [*Hi* there!].
	KindParenthesized      // A grouped expression: (1 + 2).
	KindArray              // An array: (1, "hi", 12cm).
	KindDict               // A dictionary: (thickness: 3pt, dash: "solid").
	KindNamed              // A named pair: thickness: 3pt.
	KindKeyed              // A keyed pair: "spacy key": true.
	KindUnary              // A unary operation: -x.
	KindBinary             // A binary operation: a + b.
	KindFieldAccess        // A field access: properties.age.
	KindFuncCall           // An invocation of a function or method: f(x, y).
	KindArgs               // A function call's argument list: (12pt, y).
	KindSpread             // Spread arguments or an argument sink: ..x.
	KindClosure            // A closure: (x, y) => z.
	KindParams             // A closure's parameters: (x, y).
	KindLetBinding         // A let binding: let x = 1.
	KindSetRule            // A set rule: set text(...).
	KindShowRule           // A show rule: show heading: it => emph(it.body).
	KindContextual         // A contextual expression: context text.lang.
	KindConditional        // An if-else conditional: if x { y } else { z }.
	KindWhileLoop          // A while loop: while x { y }.
	KindForLoop            // A for loop: for x in y { z }.
	KindModuleImport       // A module import: import "utils.typ": a, b, c.
	KindImportItems        // Items to import from a module: a, b, c.
	KindImportItemPath     // A path to an imported name from a submodule: a.b.c.
	KindRenamedImportItem  // A renamed import item: a as d.
	KindModuleInclude      // A module include: include "chapter1.typ".
	KindLoopBreak          // A break from a loop: break.
	KindLoopContinue       // A continue in a loop: continue.
	KindFuncReturn         // A return from a function: return, return x + 1.
	KindDestructuring      // A destructuring pattern: (x, _, ..y).
	KindDestructAssignment // A destructuring assignment expression: (x, y) = (1, 2).

	numKinds
)

// Name returns the human-readable name of the kind, suitable for use in
// error messages and diagnostics. For operators it returns the symbol (e.g.
// "=="), for keywords the keyword text (e.g. "let"), and for composite nodes
// a descriptive label (e.g. "function call").
func (k Kind) Name() string { return kinds[k] }

var kinds = [...]string{
	KindInvalid: "invalid",
	KindEnd:     "end",
	KindError:   "error",

	KindLineComment:  "comment",
	KindBlockComment: "comment",

	KindMarkup:        "markup",
	KindText:          "text",
	KindSpace:         "space",
	KindLinebreak:     "line break",
	KindParbreak:      "paragraph break",
	KindEscape:        "escape",
	KindShorthand:     "shorthand",
	KindSmartQuote:    "smartquote",
	KindStrong:        "strong",
	KindEmph:          "emph",
	KindRaw:           "raw",
	KindRawLang:       "raw language",
	KindRawDelim:      "raw delim",
	KindRawTrimmed:    "raw trimmed",
	KindLink:          "link",
	KindLabel:         "label",
	KindRef:           "reference",
	KindRefMarker:     "reference marker",
	KindHeading:       "heading",
	KindHeadingMarker: "heading marker",
	KindListItem:      "list item",
	KindListMarker:    "list marker",
	KindEnumItem:      "enum item",
	KindEnumMarker:    "enum marker",
	KindTermItem:      "term item",
	KindTermMarker:    "term marker",
	KindEquation:      "equation",

	KindMath:           "math",
	KindMathText:       "text",
	KindMathIdent:      "identifier",
	KindMathShorthand:  "shorthand",
	KindMathAlignPoint: "align point",
	KindMathDelimited:  "delimited",
	KindMathAttach:     "attach",
	KindMathPrimes:     "primes",
	KindMathFrac:       "frac",
	KindMathRoot:       "root",

	KindHash:         "#",
	KindLeftBrace:    "opening brace",
	KindRightBrace:   "closing brace",
	KindLeftBracket:  "opening bracket",
	KindRightBracket: "closing bracket",
	KindLeftParen:    "opening parenthesis",
	KindRightParen:   "closing parenthesis",
	KindComma:        "comma",
	KindSemicolon:    "semicolon",
	KindColon:        "colon",
	KindStar:         "star",
	KindUnderscore:   "underscore",
	KindDollar:       "dollar",
	KindPlus:         "plus",
	KindMinus:        "minus",
	KindSlash:        "slash",
	KindHat:          "hat",
	KindDot:          "dot",
	KindEq:           "equals sign",
	KindEqEq:         "==",
	KindExclEq:       "!=",
	KindLt:           "<",
	KindLtEq:         "<=",
	KindGt:           ">",
	KindGtEq:         ">=",
	KindPlusEq:       "+=",
	KindHyphEq:       "-=",
	KindStarEq:       "*=",
	KindSlashEq:      "/=",
	KindDots:         "dots",
	KindArrow:        "=>",
	KindRoot:         "root",
	KindBang:         "bang",

	KindNot:      "not",
	KindAnd:      "and",
	KindOr:       "or",
	KindNone:     "none",
	KindAuto:     "auto",
	KindLet:      "let",
	KindSet:      "set",
	KindShow:     "show",
	KindContext:  "context",
	KindIf:       "if",
	KindElse:     "else",
	KindFor:      "for",
	KindIn:       "in",
	KindWhile:    "while",
	KindBreak:    "break",
	KindContinue: "continue",
	KindReturn:   "return",
	KindImport:   "import",
	KindInclude:  "include",
	KindAs:       "as",

	KindCode:               "code",
	KindIdent:              "identifier",
	KindBool:               "boolean",
	KindInt:                "int",
	KindFloat:              "float",
	KindNumeric:            "numeric",
	KindStr:                "string",
	KindCodeBlock:          "code_block",
	KindContentBlock:       "content_block",
	KindParenthesized:      "parenthesized",
	KindArray:              "array",
	KindDict:               "dict",
	KindNamed:              "named",
	KindKeyed:              "keyed",
	KindUnary:              "unary",
	KindBinary:             "binary",
	KindFieldAccess:        "field access",
	KindFuncCall:           "function call",
	KindArgs:               "args",
	KindSpread:             "spread",
	KindClosure:            "closure",
	KindParams:             "params",
	KindLetBinding:         "let binding",
	KindSetRule:            "set rule",
	KindShowRule:           "show rule",
	KindContextual:         "contextual",
	KindConditional:        "conditional",
	KindWhileLoop:          "while loop",
	KindForLoop:            "for loop",
	KindModuleImport:       "module import",
	KindImportItems:        "import items",
	KindImportItemPath:     "import item path",
	KindRenamedImportItem:  "renamed import item",
	KindModuleInclude:      "module include",
	KindLoopBreak:          "loop break",
	KindLoopContinue:       "loop continue",
	KindFuncReturn:         "function return",
	KindDestructuring:      "destructuring",
	KindDestructAssignment: "destructuring assignment",
}
