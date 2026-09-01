package value

import (
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// Content is the interface for values that represent document content. It
// extends [Value] with label support and field access (for show/set rules).
// All Content values have [types.Content] as their type.
//
//go:generate go tool znkr.io/writst/internal/fieldaccessgen
type Content interface {
	Value

	SetLabel(*Label) *Label
	GetLabel() *Label

	Field(name.Name) Value
	HasField(name.Name) bool
	Fields() *Dict

	// Name returns the element name of the content value (e.g. "text",
	// "heading", "sequence"), matching the function name used to construct it.
	// Used in diagnostics such as "element <name> has no method `x`".
	Name() string

	// IsBlock reports whether the element is block-level: it occupies a line
	// of its own and breaks the paragraph flow around it. Everything else is
	// inline, and shares a paragraph with its neighbors.
	IsBlock() bool

	// walk yields this element and everything below it in document order,
	// stopping early when yield says so. It is generated for every element in
	// this file; see [All], the iterator built on it.
	walk(yield func(Content) bool) bool

	aContent()
}

// Sequence is a flat list of content elements, produced by concatenating
// content with the + operator or from markup blocks.
type Sequence struct {
	Children []Content `writst:"required"`
	Label    *Label
}

type Heading struct {
	Depth int     `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Strong struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Emph struct {
	Body  Content `writst:"required"`
	Label *Label
}

// Underline is underlined markup: underline[x].
type Underline struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Par struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Text struct {
	Text  string `writst:"required"`
	Label *Label
}

type Raw struct {
	Block bool `writst:"required"`
	Lang  string
	Text  string `writst:"required"`
}

type Linebreak struct{}

type Parbreak struct{}

// HSpace is horizontal spacing, produced by `h(amount)` and by the math
// spacing constants (`thin`, `med`, `thick`, `quad`, `wide`). Like the other
// break elements it carries no label, which also lets those constants be
// shared values.
type HSpace struct {
	Amount Length `writst:"required"`
}

// SmartQuote is a quotation mark written as ' or " in markup. Which glyph it
// stands for depends on the surrounding content, so it is emitted unresolved:
// present it with a [znkr.io/writst/smartquote.Quoter], which walks the content
// in document order and resolves the opening, closing, apostrophe, and prime
// forms. To get a literal quote instead, escape it in the source: \" or \'.
type SmartQuote struct {
	// Double reports whether this is a double quote (") rather than a single
	// one (').
	Double bool `writst:"required"`
	Label  *Label
}

type Link struct {
	Dest  string  `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Ref struct {
	Target     name.Name `writst:"required"`
	Supplement Content
	Label      *Label
}

type List struct {
	Children []*ListItem `writst:"required"`
	Label    *Label
}

type ListItem struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Enum struct {
	Children []*EnumItem `writst:"required"`
	Label    *Label
}

type EnumItem struct {
	Number int
	Body   Content `writst:"required"`
	Label  *Label
}

type Terms struct {
	Children []*TermItem `writst:"required"`
	Label    *Label
}

type TermItem struct {
	Term        Content `writst:"required"`
	Description Content `writst:"required"`
	Label       *Label
}

// Table is a grid of cells given in row-major order. Columns is how many cells
// make up a row; a [TableHeader] child contributes header rows instead of body
// rows.
type Table struct {
	Columns  int       `writst:"required"`
	Children []Content `writst:"required"`
	Label    *Label
}

// TableHeader is a run of header rows inside a [Table]. Its cells are laid out
// in the table's columns and start on a new row.
type TableHeader struct {
	Children []Content `writst:"required"`
	Label    *Label
}

type Footnote struct {
	Body  Content `writst:"required"`
	Label *Label
}

// Image is a picture loaded from a file. Path names the file and is resolved by
// the presenter, relative to whatever the presenter considers the document's
// root. Alt is an alternative description for assistive technology; it is empty
// when none was given. An image is inline: it shares the paragraph with the
// text around it.
type Image struct {
	Path  string `writst:"required"`
	Alt   string
	Label *Label
}

// HTMLElem is an HTML element written straight into a document with
// `html.elem(...)`, for the markup a document needs that writst has no element
// of its own for. Tag names it, Attrs holds its attributes in the order they
// were written, and Body is ordinary writst content: what goes inside an
// element is realized like anything else, so paragraphs form, emphasis and
// links work, and a label attaches.
//
// Block says whether the element occupies a line of its own. It defaults to
// what the tag implies (see [HtmlTagBlock]) and a document can override it.
// What the body is allowed to be does not follow Block but the tag alone (see
// [HtmlTagFlow]): `p` starts a line yet holds phrasing content, so realizing
// its body as flow content would nest a paragraph inside a paragraph.
//
// Body is nil for HTML's void elements ([HtmlTagVoid]), which have none.
//
// Escaping attribute values and text, and writing the tags themselves, is the
// presenter's job — writst carries the element, it does not serialize it.
type HTMLElem struct {
	Tag   string `writst:"required"`
	Attrs *Dict
	Body  Content
	Block bool `writst:"required"`
	Label *Label
}

// Metadata attaches a value to the document without producing any output. It
// survives realization as an invisible leaf, so it keeps its place in the
// document; a [Label] is what identifies it, and znkr.io/writst.Query finds it
// by that label.
type Metadata struct {
	Value Value `writst:"required"`
	Label *Label
}

// Equation is a mathematical equation, produced by `$...$`. Block reports
// whether it is displayed on its own line (block) or inline.
type Equation struct {
	Block bool    `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

// MathText is a text fragment inside math: a variable, number, or symbol. Bold,
// Italic, and Variant carry the font style folded in from an enclosing style
// function (`bold(x)`, `sans(x)`, …); each is nil when the style was never set,
// so a nearer style function always wins over an outer one.
type MathText struct {
	Text    string `writst:"required"`
	Bold    Value
	Italic  Value
	Variant Value
	Label   *Label
}

// MathOp is a text operator in math: `op("id")`, or one of the predefined
// operators (`sin`, `ln`, `lim`, …). Limits reports whether an attachment on
// the operator belongs above and below it in a block equation rather than
// beside it.
type MathOp struct {
	Text   Content `writst:"required"`
	Limits bool    `writst:"required"`
	Label  *Label
}

// MathAttach is a base with optional sub-/superscripts: a_1^2. Top and Bottom
// are optional.
type MathAttach struct {
	Base   Content `writst:"required"`
	Top    Content
	Bottom Content
	Label  *Label
}

// MathFrac is a fraction: x/2.
type MathFrac struct {
	Num   Content `writst:"required"`
	Denom Content `writst:"required"`
	Label *Label
}

// MathRoot is a root: √x or root(3, x). Index is optional (nil for a square
// root).
type MathRoot struct {
	Index    Content
	Radicand Content `writst:"required"`
	Label    *Label
}

// MathPrimes is a run of prime marks attached to a base (a with three primes).
type MathPrimes struct {
	Base  Content `writst:"required"`
	Count int     `writst:"required"`
	Label *Label
}

// MathAlignPoint is an alignment point in math: &.
type MathAlignPoint struct {
	Label *Label
}

// MathDelimited is a delimited group in math: [x + y] or (a). Open and Close
// hold the delimiter content.
type MathDelimited struct {
	Open  Content `writst:"required"`
	Body  Content `writst:"required"`
	Close Content `writst:"required"`
	Label *Label
}

// MathUnderline is an underlined expression in math: underline(x).
type MathUnderline struct {
	Body  Content `writst:"required"`
	Label *Label
}

// MathAccent is an accented expression in math: hat(x) or accent(x, \u{0302}).
// Accent holds the accent's combining codepoint — the accent argument is
// normalized to its combining form, so `hat(x)` and `accent(x, \u{0302})` are
// the same node. Size holds the user-supplied size override (nil when unset),
// retained so field access can retrieve it.
type MathAccent struct {
	Base    Content `writst:"required"`
	Accent  string  `writst:"required"`
	Size    Value
	Dotless bool `writst:"required"`
	Label   *Label
}

// MathCancel is a cancellation line over an expression in math:
// cancel(x, angle: ...). Angle holds the value passed for the `angle` field (a
// user-supplied angle or function), retained so field access can retrieve it.
type MathCancel struct {
	Body  Content `writst:"required"`
	Angle Value
	Label *Label
}

// MathVec is a column vector in math: vec(a, b, c).
type MathVec struct {
	Children []Content `writst:"required"`
	Label    *Label
}

// MathCases is a cases construct in math: cases(a, b).
type MathCases struct {
	Children []Content `writst:"required"`
	Label    *Label
}

// MathMat is a matrix in math: mat(a, b; c, d). Rows holds the rows, each a
// slice of cells.
type MathMat struct {
	Rows  [][]Content `writst:"required"`
	Label *Label
}

// Document is the realized document root, produced by the realization pass (it
// has no constructor). Fields are populated from set rules.
type Document struct {
	Title string
	Date  *Datetime
	Body  Content `writst:"required"`
}

func (*Sequence) aValue()    {}
func (*Heading) aValue()     {}
func (*Text) aValue()        {}
func (*Raw) aValue()         {}
func (*Strong) aValue()      {}
func (*Emph) aValue()        {}
func (*Underline) aValue()   {}
func (*Par) aValue()         {}
func (*Linebreak) aValue()   {}
func (*Parbreak) aValue()    {}
func (*HSpace) aValue()      {}
func (*SmartQuote) aValue()  {}
func (*Link) aValue()        {}
func (*Ref) aValue()         {}
func (*List) aValue()        {}
func (*ListItem) aValue()    {}
func (*Enum) aValue()        {}
func (*EnumItem) aValue()    {}
func (*Terms) aValue()       {}
func (*TermItem) aValue()    {}
func (*Table) aValue()       {}
func (*TableHeader) aValue() {}
func (*Footnote) aValue()    {}
func (*Image) aValue()       {}
func (*HTMLElem) aValue()    {}
func (*Metadata) aValue()    {}
func (*Document) aValue()    {}

func (*Equation) aValue()       {}
func (*MathText) aValue()       {}
func (*MathOp) aValue()         {}
func (*MathAttach) aValue()     {}
func (*MathFrac) aValue()       {}
func (*MathRoot) aValue()       {}
func (*MathPrimes) aValue()     {}
func (*MathAlignPoint) aValue() {}
func (*MathDelimited) aValue()  {}
func (*MathUnderline) aValue()  {}
func (*MathAccent) aValue()     {}
func (*MathCancel) aValue()     {}
func (*MathVec) aValue()        {}
func (*MathCases) aValue()      {}
func (*MathMat) aValue()        {}

func (Sequence) aContent()     {}
func (*Heading) aContent()     {}
func (*Text) aContent()        {}
func (*Raw) aContent()         {}
func (*Strong) aContent()      {}
func (*Emph) aContent()        {}
func (*Underline) aContent()   {}
func (*Par) aContent()         {}
func (*Linebreak) aContent()   {}
func (*Parbreak) aContent()    {}
func (*HSpace) aContent()      {}
func (*SmartQuote) aContent()  {}
func (*Link) aContent()        {}
func (*Ref) aContent()         {}
func (*List) aContent()        {}
func (*ListItem) aContent()    {}
func (*Enum) aContent()        {}
func (*EnumItem) aContent()    {}
func (*Terms) aContent()       {}
func (*TermItem) aContent()    {}
func (*Table) aContent()       {}
func (*TableHeader) aContent() {}
func (*Footnote) aContent()    {}
func (*Image) aContent()       {}
func (*HTMLElem) aContent()    {}
func (*Metadata) aContent()    {}
func (*Document) aContent()    {}

func (*Equation) aContent()       {}
func (*MathText) aContent()       {}
func (*MathOp) aContent()         {}
func (*MathAttach) aContent()     {}
func (*MathFrac) aContent()       {}
func (*MathRoot) aContent()       {}
func (*MathPrimes) aContent()     {}
func (*MathAlignPoint) aContent() {}
func (*MathDelimited) aContent()  {}
func (*MathUnderline) aContent()  {}
func (*MathAccent) aContent()     {}
func (*MathCancel) aContent()     {}
func (*MathVec) aContent()        {}
func (*MathCases) aContent()      {}
func (*MathMat) aContent()        {}

// Name returns the element name, matching the constructor function used to
// build it.
func (Sequence) Name() string     { return "sequence" }
func (*Heading) Name() string     { return "heading" }
func (*Strong) Name() string      { return "strong" }
func (*Emph) Name() string        { return "emph" }
func (*Underline) Name() string   { return "underline" }
func (*Par) Name() string         { return "par" }
func (*Text) Name() string        { return "text" }
func (*Raw) Name() string         { return "raw" }
func (*Linebreak) Name() string   { return "linebreak" }
func (*Parbreak) Name() string    { return "parbreak" }
func (*HSpace) Name() string      { return "h" }
func (*SmartQuote) Name() string  { return "smartquote" }
func (*Link) Name() string        { return "link" }
func (*Ref) Name() string         { return "ref" }
func (*List) Name() string        { return "list" }
func (*ListItem) Name() string    { return "list.item" }
func (*Enum) Name() string        { return "enum" }
func (*EnumItem) Name() string    { return "enum.item" }
func (*Terms) Name() string       { return "terms" }
func (*TermItem) Name() string    { return "terms.item" }
func (*Table) Name() string       { return "table" }
func (*TableHeader) Name() string { return "table.header" }
func (*Footnote) Name() string    { return "footnote" }
func (*Image) Name() string       { return "image" }
func (*HTMLElem) Name() string    { return "html.elem" }
func (*Metadata) Name() string    { return "metadata" }
func (*Document) Name() string    { return "document" }

func (*Equation) Name() string       { return "equation" }
func (*MathText) Name() string       { return "math.text" }
func (*MathOp) Name() string         { return "math.op" }
func (*MathAttach) Name() string     { return "math.attach" }
func (*MathFrac) Name() string       { return "math.frac" }
func (*MathRoot) Name() string       { return "math.root" }
func (*MathPrimes) Name() string     { return "math.primes" }
func (*MathAlignPoint) Name() string { return "math.align-point" }
func (*MathDelimited) Name() string  { return "math.lr" }
func (*MathUnderline) Name() string  { return "math.underline" }
func (*MathAccent) Name() string     { return "math.accent" }
func (*MathCancel) Name() string     { return "cancel" }
func (*MathVec) Name() string        { return "math.vec" }
func (*MathCases) Name() string      { return "math.cases" }
func (*MathMat) Name() string        { return "math.mat" }

// IsBlock reports whether the element occupies a line of its own. The item
// elements are block: a `- apples` entry breaks the paragraph before it exactly
// as a heading does, and its siblings being gathered into a [List] is a later
// step. A sequence is a flat splice rather than a box around its children, so
// the question is asked of each child once it is flattened. Raw and Equation
// answer from their Block field, set by the form they were written in.
func (Sequence) IsBlock() bool     { return false }
func (*Heading) IsBlock() bool     { return true }
func (*Strong) IsBlock() bool      { return false }
func (*Emph) IsBlock() bool        { return false }
func (*Underline) IsBlock() bool   { return false }
func (*Par) IsBlock() bool         { return true }
func (*Text) IsBlock() bool        { return false }
func (n *Raw) IsBlock() bool       { return n.Block }
func (*Linebreak) IsBlock() bool   { return false }
func (*Parbreak) IsBlock() bool    { return false }
func (*HSpace) IsBlock() bool      { return false }
func (*SmartQuote) IsBlock() bool  { return false }
func (*Link) IsBlock() bool        { return false }
func (*Ref) IsBlock() bool         { return false }
func (*List) IsBlock() bool        { return true }
func (*ListItem) IsBlock() bool    { return true }
func (*Enum) IsBlock() bool        { return true }
func (*EnumItem) IsBlock() bool    { return true }
func (*Terms) IsBlock() bool       { return true }
func (*TermItem) IsBlock() bool    { return true }
func (*Table) IsBlock() bool       { return true }
func (*TableHeader) IsBlock() bool { return true }
func (*Footnote) IsBlock() bool    { return false }
func (*Image) IsBlock() bool       { return false }
func (n *HTMLElem) IsBlock() bool  { return n.Block }
func (*Metadata) IsBlock() bool    { return false }
func (*Document) IsBlock() bool    { return true }

func (n *Equation) IsBlock() bool     { return n.Block }
func (*MathText) IsBlock() bool       { return false }
func (*MathOp) IsBlock() bool         { return false }
func (*MathAttach) IsBlock() bool     { return false }
func (*MathFrac) IsBlock() bool       { return false }
func (*MathRoot) IsBlock() bool       { return false }
func (*MathPrimes) IsBlock() bool     { return false }
func (*MathAlignPoint) IsBlock() bool { return false }
func (*MathDelimited) IsBlock() bool  { return false }
func (*MathUnderline) IsBlock() bool  { return false }
func (*MathAccent) IsBlock() bool     { return false }
func (*MathCancel) IsBlock() bool     { return false }
func (*MathVec) IsBlock() bool        { return false }
func (*MathCases) IsBlock() bool      { return false }
func (*MathMat) IsBlock() bool        { return false }

func (Sequence) Type() types.Type     { return types.Content }
func (*Heading) Type() types.Type     { return types.Content }
func (*Text) Type() types.Type        { return types.Content }
func (*Raw) Type() types.Type         { return types.Content }
func (*Strong) Type() types.Type      { return types.Content }
func (*Emph) Type() types.Type        { return types.Content }
func (*Underline) Type() types.Type   { return types.Content }
func (*Par) Type() types.Type         { return types.Content }
func (*Linebreak) Type() types.Type   { return types.Content }
func (*Parbreak) Type() types.Type    { return types.Content }
func (*HSpace) Type() types.Type      { return types.Content }
func (*SmartQuote) Type() types.Type  { return types.Content }
func (*Link) Type() types.Type        { return types.Content }
func (*Ref) Type() types.Type         { return types.Content }
func (*List) Type() types.Type        { return types.Content }
func (*ListItem) Type() types.Type    { return types.Content }
func (*Enum) Type() types.Type        { return types.Content }
func (*EnumItem) Type() types.Type    { return types.Content }
func (*Terms) Type() types.Type       { return types.Content }
func (*TermItem) Type() types.Type    { return types.Content }
func (*Table) Type() types.Type       { return types.Content }
func (*TableHeader) Type() types.Type { return types.Content }
func (*Footnote) Type() types.Type    { return types.Content }
func (*Image) Type() types.Type       { return types.Content }
func (*HTMLElem) Type() types.Type    { return types.Content }
func (*Metadata) Type() types.Type    { return types.Content }
func (*Document) Type() types.Type    { return types.Content }

func (*Equation) Type() types.Type       { return types.Content }
func (*MathText) Type() types.Type       { return types.Content }
func (*MathOp) Type() types.Type         { return types.Content }
func (*MathAttach) Type() types.Type     { return types.Content }
func (*MathFrac) Type() types.Type       { return types.Content }
func (*MathRoot) Type() types.Type       { return types.Content }
func (*MathPrimes) Type() types.Type     { return types.Content }
func (*MathAlignPoint) Type() types.Type { return types.Content }
func (*MathDelimited) Type() types.Type  { return types.Content }
func (*MathUnderline) Type() types.Type  { return types.Content }
func (*MathAccent) Type() types.Type     { return types.Content }
func (*MathCancel) Type() types.Type     { return types.Content }
func (*MathVec) Type() types.Type        { return types.Content }
func (*MathCases) Type() types.Type      { return types.Content }
func (*MathMat) Type() types.Type        { return types.Content }
