package syntax

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
