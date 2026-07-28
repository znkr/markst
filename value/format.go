package value

import (
	"unicode"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/syntax"
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
	f.FuncCall("heading", []formatter.Arg{formatter.NamedArg("level", n.Depth)})
}

// contentBlock wraps a Content value so FuncCall renders it as [...].
type contentBlock struct{ c Content }

func (b contentBlock) Format(f *formatter.Formatter) {
	if b.c == nil {
		return
	}
	prev := f.Mode()
	f.SetMode(syntax.ModeMarkup)
	f.Str("[")
	b.c.Format(f)
	f.Str("]")
	f.SetMode(prev)
}

func (n *Strong) Format(f *formatter.Formatter) {
	f.FuncCall("strong", nil, contentBlock{n.Body})
}

func (n *Emph) Format(f *formatter.Formatter) {
	f.FuncCall("emph", nil, contentBlock{n.Body})
}

func (n *Par) Format(f *formatter.Formatter) {
	f.FuncCall("par", nil, contentBlock{n.Body})
}

func (n *Text) Format(f *formatter.Formatter) {
	f.FuncCall("text", []formatter.Arg{formatter.PositionalArg(n.Text)})
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

func (n *Table) Format(f *formatter.Formatter) {
	var args []formatter.Arg
	for i := range n.Children {
		args = append(args, formatter.PositionalArg(n.Children[i]))
	}
	f.FuncCall("table", args)
}

// Label ///////////////////////////////////////////////////////////////////////

func (n *Label) Format(f *formatter.Formatter) {
	f.FuncCall("label", []formatter.Arg{formatter.PositionalArg(n.Name.String())})
}

// Module //////////////////////////////////////////////////////////////////////

func (n *Module) Format(f *formatter.Formatter) {
	f.Str("module")
}
