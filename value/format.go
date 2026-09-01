package value

import (
	"strings"
	"unicode"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/syntax"
)

// FormatContent returns a human-readable string representation of document
// content, used in test golden files. The output format mirrors Typst's
// repr() output for content values.
func FormatContent(c Content) string {
	f := formatter.New(syntax.ModeMarkup)
	c.Format(f)
	if f.Len() > 0 {
		f.Str("\n")
	}
	return f.String()
}

// FormatValue returns a string representation of a single value in code mode.
func FormatValue(value Value) string {
	f := formatter.New(syntax.ModeCode)
	value.Format(f)
	f.Str("\n")
	return f.String()
}

// Types ///////////////////////////////////////////////////////////////////////

func (n Type) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.Reflected.String())
}

// Scalars /////////////////////////////////////////////////////////////////////

func (n None) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("none")
}

func (n Auto) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("auto")
}

func (n Bool) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(bool(n))
}

func (n Int) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(int(n))
}

func (n Float) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(float64(n))
}

func (n Decimal) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(decimal128.Decimal(n))
}

func (n Ratio) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(n)
}

func (n Fraction) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(n)
}

func (n Length) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(n)
}

func (n Relative) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(n)
}

func (n Angle) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Print(n)
}

func (n Str) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("\"")
	for _, r := range n {
		switch r {
		case '\n':
			f.Str(`\n`)
		case '\t':
			f.Str(`\t`)
		case '\\':
			f.Str(`\\`)
		case '"':
			f.Str(`\"`)
		default:
			if !unicode.IsPrint(r) {
				f.Printf(`\u{%x}`, r)
			} else {
				f.Str(string(r))
			}
		}
	}
	f.Str("\"")
}

func (n Bytes) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("(")
	for i, b := range []byte(n) {
		if i > 0 {
			f.Str(", ")
		}
		f.Print(int(b))
	}
	f.Str(")")
}

// Dates ///////////////////////////////////////////////////////////////////////

func (n Datetime) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.String())
}

func (n Duration) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.String())
}

// Symbols /////////////////////////////////////////////////////////////////////

func (n *Symbol) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.String())
}

// Containers //////////////////////////////////////////////////////////////////

func (n *Array) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("(")
	if len(n.Elems) == 0 {
		f.Str(")")
		return
	}
	for i, item := range n.Elems {
		if i > 0 {
			f.Str(", ")
		}
		item.Format(f)
	}
	if len(n.Elems) == 1 {
		// trailing comma for single-element arrays
		f.Str(",")
	}
	f.Str(")")
}

func (n *Dict) Format(f *formatter.Formatter) {
	f.Prefix()
	if n.Elems.Len() == 0 {
		f.Str("(:)")
		return
	}
	f.Str("(")
	i := 0
	for k, v := range n.Elems.All() {
		if i > 0 {
			f.Str(", ")
		}
		k.Format(f)
		f.Str(": ")
		v.Format(f)
		i++
	}
	f.Str(")")
}

// Functions ///////////////////////////////////////////////////////////////////

func (n *Function) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.Name)
}

// Arguments ///////////////////////////////////////////////////////////////////

func (n *Arguments) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("arguments")
	if len(n.Positional) == 0 && n.Named.Len() == 0 {
		f.Str("()")
		return
	}
	f.Str("(")
	for i, v := range n.Positional {
		if i > 0 {
			f.Str(", ")
		}
		v.Format(f)
	}
	i := 0
	for k, v := range n.Named.All() {
		if len(n.Positional) > 0 || i > 0 {
			f.Str(", ")
		}
		f.Str(k.String())
		f.Str(": ")
		v.Format(f)
		i++
	}
	f.Str(")")
}

// Content /////////////////////////////////////////////////////////////////////

func (n *Sequence) Format(f *formatter.Formatter) {
	if n == nil || len(n.Children) == 0 {
		return
	}
	if len(n.Children) == 1 {
		n.Children[0].Format(f)
		return
	}
	for i, c := range n.Children {
		if i > 0 {
			f.Linebreak()
		}
		c.Format(f)
	}
}

func (n *Heading) Format(f *formatter.Formatter) {
	f.FuncCall("heading", []formatter.Arg{formatter.NamedArg("level", n.Depth)}, contentBlock{n.Body})
}

// contentItems flattens a content value into the items of a content block: a
// sequence contributes its children, anything else is a single item.
func contentItems(c Content) []formatter.Formattable {
	if seq, ok := c.(*Sequence); ok {
		items := make([]formatter.Formattable, len(seq.Children))
		for i, ch := range seq.Children {
			items[i] = ch
		}
		return items
	}
	return []formatter.Formattable{c}
}

// contentBlock wraps a Content value so FuncCall renders it as [...]. Its
// children stay concatenated inline when short, and break one-per-line when the
// block would otherwise be too long or contain a multi-line child (see
// [formatter.Formatter.BracketedList]).
type contentBlock struct{ c Content }

func (b contentBlock) Format(f *formatter.Formatter) {
	if b.c == nil {
		return
	}
	prev := f.Mode()
	f.SetMode(syntax.ModeMarkup)
	f.BracketedList(contentItems(b.c))
	f.SetMode(prev)
}

// codeValue wraps a Value so FuncCall renders it as the code-mode expression it
// is. Inside an argument list there is no transition into code left to mark, so
// the `#` that [formatter.Formatter.Prefix] writes in markup mode would be
// wrong. It is the mirror of [contentBlock].
type codeValue struct{ v Value }

func (a codeValue) Format(f *formatter.Formatter) {
	prev := f.Mode()
	f.SetMode(syntax.ModeCode)
	a.v.Format(f)
	f.SetMode(prev)
}

func (n *Strong) Format(f *formatter.Formatter) {
	f.FuncCall("strong", nil, contentBlock{n.Body})
}

func (n *Emph) Format(f *formatter.Formatter) {
	f.FuncCall("emph", nil, contentBlock{n.Body})
}

func (n *Underline) Format(f *formatter.Formatter) {
	f.FuncCall("underline", nil, contentBlock{n.Body})
}

func (n *Par) Format(f *formatter.Formatter) {
	f.FuncCall("par", nil, contentBlock{n.Body})
}

func (n *Text) Format(f *formatter.Formatter) {
	// In markup context text is written literally (its natural form); only in
	// code context does it need the explicit `text("…")` wrapper.
	if f.Mode() == syntax.ModeMarkup {
		f.Str(escapeMarkupText(n.Text))
		return
	}
	f.FuncCall("text", []formatter.Arg{formatter.PositionalArg(n.Text)})
}

// escapeMarkupText backslash-escapes the characters that would otherwise be
// structural in markup, so literal text round-trips unambiguously in a dump.
func escapeMarkupText(s string) string {
	const special = "\\[]#*_`<>@$\"'"
	if !strings.ContainsAny(s, special) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(special, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (n *Raw) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Block {
		args = append(args, formatter.NamedArg("block", n.Block))
	}
	if n.Lang != "" {
		args = append(args, formatter.NamedArg("lang", n.Lang))
	}
	args = append(args, formatter.PositionalArg(n.Text))
	f.FuncCall("raw", args)
}

func (n *Linebreak) Format(f *formatter.Formatter) {
	f.FuncCall("linebreak", nil)
}

func (n *Parbreak) Format(f *formatter.Formatter) {
	f.FuncCall("parbreak", nil)
}

func (n *HSpace) Format(f *formatter.Formatter) {
	// rawArg, not the length itself: a bare Length formats with the "#" that
	// carries it into markup, and this one is already inside a call.
	f.FuncCall("h", []formatter.Arg{formatter.PositionalArg(rawArg(n.Amount.String()))})
}

// rawArg is an argument that formats as itself: no "#" prefix, and none of the
// quoting a plain string argument gets.
type rawArg string

func (r rawArg) String() string { return string(r) }

func (n *SmartQuote) Format(f *formatter.Formatter) {
	// In markup the quote is written the way it was typed; escapeMarkupText
	// escapes a literal quote that came from text, so the two stay apart.
	if f.Mode() == syntax.ModeMarkup {
		if n.Double {
			f.Str(`"`)
		} else {
			f.Str("'")
		}
		return
	}
	f.FuncCall("smartquote", []formatter.Arg{formatter.NamedArg("double", n.Double)})
}

func (n *Link) Format(f *formatter.Formatter) {
	f.FuncCall("link", []formatter.Arg{formatter.NamedArg("dest", n.Dest)}, contentBlock{n.Body})
}

func (n *Ref) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.NamedArg("target", n.Target.String())}
	var blocks []formatter.Formattable
	if n.Supplement != nil {
		blocks = append(blocks, contentBlock{n.Supplement})
	}
	f.FuncCall("ref", args, blocks...)
}

func (n *List) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("list", args)
}

func (n *ListItem) Format(f *formatter.Formatter) {
	f.FuncCall("list.item", nil, contentBlock{n.Body})
}

func (n *Enum) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("enum", args)
}

func (n *EnumItem) Format(f *formatter.Formatter) {
	f.FuncCall("enum.item", []formatter.Arg{formatter.PositionalArg(n.Number)}, contentBlock{n.Body})
}

func (n *Terms) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("terms", args)
}

func (n *TermItem) Format(f *formatter.Formatter) {
	f.FuncCall("terms.item", nil, contentBlock{n.Term}, contentBlock{n.Description})
}

func (n *Equation) Format(f *formatter.Formatter) {
	f.FuncCall("math.equation", []formatter.Arg{formatter.NamedArg("block", n.Block)}, contentBlock{n.Body})
}

// styleArg renders one of MathText's optional font-style properties. Their
// values are only ever booleans or strings (the equation element's parameter
// types enforce it), so they print as plain `bold: true` / `variant: "sans"`
// rather than the code-mode `#true` / `#"sans"` a Value would produce — matching
// how `math.equation(block: true)` already reads. ok is false when unset.
func styleArg(name string, v Value) (formatter.Arg, bool) {
	switch x := v.(type) {
	case nil:
		return formatter.Arg{}, false
	case Bool:
		return formatter.NamedArg(name, bool(x)), true
	case Str:
		return formatter.NamedArg(name, string(x)), true
	default:
		return formatter.NamedArg(name, v), true
	}
}

func (n *MathText) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.PositionalArg(n.Text)}
	for _, s := range []struct {
		name string
		val  Value
	}{{"bold", n.Bold}, {"italic", n.Italic}, {"variant", n.Variant}} {
		if a, ok := styleArg(s.name, s.val); ok {
			args = append(args, a)
		}
	}
	f.FuncCall("math.text", args)
}

func (n *MathOp) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Limits {
		args = append(args, formatter.NamedArg("limits", true))
	}
	f.FuncCall("math.op", args, contentBlock{n.Text})
}

func (n *MathAttach) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Top != nil {
		args = append(args, formatter.NamedArg("t", contentBlock{n.Top}))
	}
	if n.Bottom != nil {
		args = append(args, formatter.NamedArg("b", contentBlock{n.Bottom}))
	}
	f.FuncCall("math.attach", args, contentBlock{n.Base})
}

func (n *MathFrac) Format(f *formatter.Formatter) {
	f.FuncCall("math.frac", nil, contentBlock{n.Num}, contentBlock{n.Denom})
}

func (n *MathRoot) Format(f *formatter.Formatter) {
	var blocks []formatter.Formattable
	if n.Index != nil {
		blocks = append(blocks, contentBlock{n.Index})
	}
	blocks = append(blocks, contentBlock{n.Radicand})
	f.FuncCall("math.root", nil, blocks...)
}

func (n *MathPrimes) Format(f *formatter.Formatter) {
	f.FuncCall("math.primes", []formatter.Arg{formatter.NamedArg("count", n.Count)}, contentBlock{n.Base})
}

func (n *MathAlignPoint) Format(f *formatter.Formatter) {
	f.FuncCall("math.align-point", nil)
}

func (n *MathLr) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Size != nil {
		args = append(args, formatter.NamedArg("size", n.Size))
	}
	f.FuncCall("math.lr", args, contentBlock{n.Body})
}

func (n *MathMid) Format(f *formatter.Formatter) {
	f.FuncCall("math.mid", nil, contentBlock{n.Body})
}

func (n *MathUnderline) Format(f *formatter.Formatter) {
	f.FuncCall("math.underline", nil, contentBlock{n.Body})
}

func (n *MathAccent) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.NamedArg("accent", n.Accent)}
	if n.Size != nil {
		args = append(args, formatter.NamedArg("size", n.Size))
	}
	// dotless defaults to true; only a disabling override is worth showing.
	if !n.Dotless {
		args = append(args, formatter.NamedArg("dotless", false))
	}
	f.FuncCall("math.accent", args, contentBlock{n.Base})
}

func (n *MathCancel) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Angle != nil {
		args = append(args, formatter.NamedArg("angle", n.Angle))
	}
	f.FuncCall("math.cancel", args, contentBlock{n.Body})
}

// mathCellBlocks renders each content cell as its own `[…]` block.
func mathCellBlocks(cells []Content) []formatter.Formattable {
	blocks := make([]formatter.Formattable, len(cells))
	for i, c := range cells {
		blocks[i] = contentBlock{c}
	}
	return blocks
}

func (n *MathVec) Format(f *formatter.Formatter) {
	f.FuncCall("math.vec", nil, mathCellBlocks(n.Children)...)
}

func (n *MathCases) Format(f *formatter.Formatter) {
	f.FuncCall("math.cases", nil, mathCellBlocks(n.Children)...)
}

func (n *MathMat) Format(f *formatter.Formatter) {
	// Each row is one content block, its cells rendered as a sequence.
	blocks := make([]formatter.Formattable, len(n.Rows))
	for i, row := range n.Rows {
		blocks[i] = contentBlock{rowContent(row)}
	}
	f.FuncCall("math.mat", nil, blocks...)
}

// rowContent collapses a matrix row into a single content value for rendering
// as one block.
func rowContent(row []Content) Content {
	if len(row) == 1 {
		return row[0]
	}
	return &Sequence{Children: row}
}

func (n *Table) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.NamedArg("columns", n.Columns)}
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("table", args)
}

func (n *TableHeader) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("table.header", args)
}

func (n *Footnote) Format(f *formatter.Formatter) {
	f.FuncCall("footnote", nil, contentBlock{n.Body})
}

func (n *Image) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.PositionalArg(n.Path)}
	if n.Alt != "" {
		args = append(args, formatter.NamedArg("alt", n.Alt))
	}
	f.FuncCall("image", args)
}

func (n *HTMLElem) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.PositionalArg(n.Tag)}
	if n.Attrs != nil && n.Attrs.Elems.Len() > 0 {
		args = append(args, formatter.NamedArg("attrs", codeValue{n.Attrs}))
	}
	args = append(args, formatter.NamedArg("block", n.Block))
	if n.Body == nil {
		f.FuncCall("html.elem", args)
		return
	}
	f.FuncCall("html.elem", args, contentBlock{n.Body})
}

// Format renders metadata with its label, which no other element does: a label
// elsewhere is styling and reference bookkeeping, but a metadata value is
// *identified* by its label — without it there is no telling one entry from
// another.
func (n *Metadata) Format(f *formatter.Formatter) {
	// Content carried as data still reads best as content, the way it does
	// everywhere else; anything else is a code-mode value.
	arg := formatter.PositionalArg(codeValue{n.Value})
	if c, ok := n.Value.(Content); ok {
		arg = formatter.PositionalArg(contentBlock{c})
	}
	f.FuncCall("metadata", []formatter.Arg{arg})
	if n.Label != nil {
		f.Str(" <")
		f.Str(n.Label.Name.String())
		f.Str(">")
	}
}

func (n *Document) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	if n.Title != "" {
		args = append(args, formatter.NamedArg("title", n.Title))
	}
	if n.Date != nil {
		args = append(args, formatter.NamedArg("date", *n.Date))
	}
	f.FuncCall("document", args, contentBlock{n.Body})
}

// Label ///////////////////////////////////////////////////////////////////////

func (n *Label) Format(f *formatter.Formatter) {
	f.FuncCall("label", []formatter.Arg{formatter.PositionalArg(n.Name.String())})
}

// Module //////////////////////////////////////////////////////////////////////

func (n *Module) Format(f *formatter.Formatter) {
	f.Str("module")
}
