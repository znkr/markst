package ir

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"
	"unique"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/syntax"
)

type formattable interface {
	format(f *formatter)
}

func FormatContent(c Content) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeMarkup}
	c.format(f)
	if sb.Len() > 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

func FormatExprs(exprs []Expr) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeMarkup}
	for _, c := range exprs {
		c.format(f)
		sb.WriteString("\n")
	}
	return sb.String()
}

func FormatValue(value Value) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeCode}
	value.format(f)
	sb.WriteString("\n")
	return sb.String()
}

// formatter provides a simple DSL for formatting IR nodes.
type formatter struct {
	sb         *strings.Builder
	mode       syntax.Mode
	indent     int
	inlineCode bool // true when formatting inline code like { expr }
}

func (f *formatter) str(s string)                   { f.sb.WriteString(s) }
func (f *formatter) nl()                            { f.str("\n"); f.writeIndent() }
func (f *formatter) writeIndent()                   { f.str(strings.Repeat("  ", f.indent)) }
func (f *formatter) val(v any)                      { fmt.Fprint(f.sb, v) }
func (f *formatter) printf(format string, a ...any) { fmt.Fprintf(f.sb, format, a...) }

// prefix writes # when in markup mode (transitioning into code)
func (f *formatter) prefix() {
	if f.mode == syntax.ModeMarkup {
		f.str("#")
	}
}

// kw writes a keyword followed by a space
func (f *formatter) kw(s string) {
	f.str(s)
	f.str(" ")
}

// op writes a binary operator with surrounding spaces
func (f *formatter) op(o syntax.BinaryOp) {
	f.str(" ")
	f.str(o.String())
	f.str(" ")
}

// expr formats an expression in code mode
func (f *formatter) expr(e Expr) {
	prev := f.mode
	f.mode = syntax.ModeCode
	e.format(f)
	f.mode = prev
}

// exprs formats expressions with a separator
func (f *formatter) exprs(sep string, es []Expr) {
	for i, e := range es {
		if i > 0 {
			f.str(sep)
		}
		f.expr(e)
	}
}

// destructPattern formats a slice of destruct patterns
func (f *formatter) destructPattern(patterns []DestructPattern) {
	if len(patterns) == 1 {
		patterns[0].format(f)
		return
	}
	f.str("(")
	for i, p := range patterns {
		if i > 0 {
			f.str(", ")
		}
		p.format(f)
	}
	f.str(")")
}

// args formats arguments with a separator
func (f *formatter) arguments(sep string, es []Arg) {
	for i, e := range es {
		if i > 0 {
			f.str(sep)
		}
		switch e := e.(type) {
		case *SpreadArg:
			f.str("..")
			f.expr(e.expr)
		case *ExprArg:
			f.expr(e.expr)
		case *NamedArg:
			f.str(e.name.Value())
			f.str(": ")
			f.expr(e.expr)
		default:
			panic(fmt.Sprintf("unknown arg type: %T", e))
		}
	}
}

// params formats parameters with a separator
func (f *formatter) params(sep string, ps []ClosureParam) {
	for i, p := range ps {
		if i > 0 {
			f.str(sep)
		}
		switch p := p.(type) {
		case *PositionalClosureParam:
			f.expr(p.ident)
		case *NamedClosureParam:
			f.str(p.name.Value())
			if p.def != nil {
				f.str(": ")
				f.expr(p.def)
			}
		case *SpreadClosureParam:
			f.str("..")
			f.expr(p.ident)
		default:
			panic(fmt.Sprintf("unknown param type: %T", p))
		}
	}
}

type arg struct {
	name       string
	positional bool
	value      any
}

func named(name string, v any) arg { return arg{name: name, value: v} }
func pos(v any) arg                { return arg{positional: true, value: v} }

// funcCall formats #name(args)[blocks...] for content nodes
func (f *formatter) funcCall(name string, args []arg, blocks ...any) {
	f.prefix()
	f.str(name)
	if len(args) > 0 {
		inline := len(args) <= 2
		f.str("(")
		if !inline {
			f.indent++
		}
		for i, a := range args {
			if !inline {
				f.nl()
			}
			if !a.positional {
				f.str(a.name)
				f.str(": ")
			}
			switch v := a.value.(type) {
			case Expr:
				f.expr(v)
			case Value:
				prev := f.mode
				f.mode = syntax.ModeCode
				v.format(f)
				f.mode = prev
			case string:
				f.printf("%q", v)
			default:
				f.val(v)
			}
			if inline {
				if i < len(args)-1 {
					f.str(", ")
				}
			} else {
				f.str(",")
			}
		}
		if !inline {
			f.indent--
			f.nl()
		}
		f.str(")")
	}
	for _, block := range blocks {
		switch b := block.(type) {
		case []Expr:
			f.contentBlock(b)
		case Content:
			f.valueBlock(b)
		}
	}
	if len(args) == 0 && len(blocks) == 0 {
		f.str("()")
	}
}

// writeBlock formats items wrapped in [...] with proper indentation for multiple items.
// For multi-item blocks, each item is preceded by a newline and indent.
func writeBlock[T formattable](f *formatter, items []T) {
	if len(items) == 0 {
		return
	}
	prev := f.mode
	f.mode = syntax.ModeMarkup
	if len(items) > 1 {
		f.str("[")
		f.indent++
		for _, item := range items {
			f.nl()
			item.format(f)
		}
		f.indent--
		f.nl()
		f.str("]")
	} else {
		f.str("[")
		items[0].format(f)
		f.str("]")
	}
	f.mode = prev
}

// contentBlock formats an []Expr block wrapped in [...]
func (f *formatter) contentBlock(exprs []Expr) {
	writeBlock(f, exprs)
}

// valueBlock formats a Content value wrapped in [...]
func (f *formatter) valueBlock(c Content) {
	if cs, ok := c.(*Sequence); ok {
		writeBlock(f, cs.Children)
	} else {
		writeBlock(f, []Content{c})
	}
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

func (n *HeadingExpr) format(f *formatter) {
	f.funcCall("heading", []arg{named("level", n.level)}, n.body)
}

func (n *StrongExpr) format(f *formatter) {
	f.funcCall("strong", nil, n.body)
}

func (n *EmphExpr) format(f *formatter) {
	f.funcCall("emph", nil, n.body)
}

func (n *LinkExpr) format(f *formatter) {
	f.funcCall("link", []arg{named("dest", n.dest)}, n.body)
}

func (n *RefExpr) format(f *formatter) {
	args := []arg{named("target", n.target.Value())}
	var blocks []any
	if n.supplement != nil && len(n.supplement.exprs) > 0 {
		blocks = append(blocks, n.supplement.Body())
	}
	f.funcCall("ref", args, blocks...)
}

func (n *ListItemExpr) format(f *formatter) {
	f.funcCall("list.item", nil, n.body)
}

func (n *EnumItemExpr) format(f *formatter) {
	f.funcCall("enum.item", []arg{pos(n.number)}, n.body)
}

func (n *TermItemExpr) format(f *formatter) {
	f.funcCall("terms.item", nil, n.term, n.description)
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) format(f *formatter) {
	n.value.format(f)
}

func (n *Ident) format(f *formatter) {
	f.prefix()
	f.str(n.name.Value())
}

func (n *CodeBlock) format(f *formatter) {
	f.prefix()
	if len(n.exprs) == 0 {
		f.str("{ }")
		return
	}
	prev := f.inlineCode
	f.inlineCode = true
	f.str("{ ")
	f.exprs("; ", n.exprs)
	f.str(" }")
	f.inlineCode = prev
}

func (n *ContentBlock) format(f *formatter) {
	f.prefix()
	if len(n.exprs) == 0 {
		f.str("[]")
		return
	}
	prev := f.mode
	f.mode = syntax.ModeMarkup
	// Stay inline if there's only one item OR we're inside inline code
	if len(n.exprs) == 1 || f.inlineCode {
		f.str("[")
		for i, item := range n.exprs {
			if i > 0 {
				f.str(" ")
			}
			item.format(f)
		}
		f.str("]")
	} else {
		f.str("[")
		f.indent++
		for _, item := range n.exprs {
			f.nl()
			item.format(f)
		}
		f.indent--
		f.nl()
		f.str("]")
	}
	f.mode = prev
}

func (n *Parenthesized) format(f *formatter) {
	f.prefix()
	f.str("(")
	f.expr(n.body)
	f.str(")")
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) format(f *formatter) {
	f.prefix()
	f.str("(")
	if len(n.elements) == 0 {
		f.str(")")
		return
	}
	f.exprs(", ", n.elements)
	if len(n.elements) == 1 {
		// trailing comma for single-element arrays
		f.str(",")
	}
	f.str(")")
}

func (n *DictExpr) format(f *formatter) {
	f.prefix()
	if len(n.entries) == 0 {
		f.str("(:)")
		return
	}
	f.str("(")
	for i, ent := range n.entries {
		if i > 0 {
			f.str(", ")
		}
		f.expr(ent.key)
		f.str(": ")
		f.expr(ent.value)
	}
	f.str(")")
}

// Operators ///////////////////////////////////////////////////////////////////////////////////////

func (n *Unary) format(f *formatter) {
	f.prefix()
	op := n.op.String()
	f.str(op)
	// Add space after keyword operators like "not"
	if op == "not" {
		f.str(" ")
	}
	f.expr(n.operand)
}

func (n *Binary) format(f *formatter) {
	f.prefix()
	f.expr(n.left)
	f.op(n.op)
	f.expr(n.right)
}

func (n *FieldAccess) format(f *formatter) {
	f.prefix()
	f.expr(n.target)
	f.str(".")
	f.expr(n.field)
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) format(f *formatter) {
	f.prefix()
	f.expr(n.callee)
	f.str("(")
	f.arguments(", ", n.args)
	f.str(")")
	for _, block := range n.blocks {
		f.contentBlock(block.exprs)
	}
}

func (n *Closure) format(f *formatter) {
	f.prefix()
	if n.name != nil {
		// Named function: name(params) = body
		f.str(n.name.name.Value())
		f.str("(")
		f.params(", ", n.params)
		f.str(") = ")
		f.closureBody(n.body)
		return
	}
	// Anonymous closure
	if len(n.params) == 1 {
		// Check if param is a spread - if so, needs parens
		if _, isSpread := n.params[0].(*SpreadClosureParam); !isSpread {
			f.params(", ", n.params)
			f.str(" => ")
			f.closureBody(n.body)
			return
		}
	}
	f.str("(")
	f.params(", ", n.params)
	f.str(") => ")
	f.closureBody(n.body)
}

// closureBody formats a closure body, unwrapping single-expression code blocks.
func (f *formatter) closureBody(body Expr) {
	if cb, ok := body.(*CodeBlock); ok && len(cb.exprs) == 1 {
		f.expr(cb.exprs[0])
	} else {
		f.expr(body)
	}
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) format(f *formatter) {
	f.prefix()
	f.kw("let")
	if n.value != nil {
		// Check if value is a named closure (let function)
		if closure, ok := n.value.(*Closure); ok && closure.name != nil {
			f.expr(n.value)
			return
		}
		f.destructPattern(n.pattern)
		f.str(" = ")
		f.expr(n.value)
		return
	}
	f.destructPattern(n.pattern)
}

func (n *SetRule) format(f *formatter) {
	f.prefix()
	f.kw("set")
	f.expr(n.target)
	f.str("(")
	f.arguments(", ", n.args)
	f.str(")")
	if n.condition != nil {
		f.kw(" if")
		f.expr(n.condition)
	}
}

func (n *ShowRule) format(f *formatter) {
	f.prefix()
	f.str("show")
	if n.selector != nil {
		f.str(" ")
		f.expr(n.selector)
	}
	f.str(": ")
	f.expr(n.transform)
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) format(f *formatter) {
	f.prefix()
	for i, cond := range n.conditions {
		if i == 0 {
			f.kw("if")
		} else {
			f.str(" ")
			f.kw("else if")
		}
		f.expr(cond)
		f.str(" ")
		f.expr(n.blocks[i])
	}
	if n.def != nil {
		f.str(" ")
		f.kw("else")
		f.expr(n.def)
	}
}

func (n *WhileLoop) format(f *formatter) {
	f.prefix()
	f.kw("while")
	f.expr(n.condition)
	f.str(" ")
	f.expr(n.body)
}

func (n *ForLoop) format(f *formatter) {
	f.prefix()
	f.kw("for")
	f.destructPattern(n.pattern)
	f.kw(" in")
	f.expr(n.iterable)
	f.str(" ")
	f.expr(n.body)
}

func (n *LoopBreak) format(f *formatter) {
	f.prefix()
	f.str("break")
}

func (n *LoopContinue) format(f *formatter) {
	f.prefix()
	f.str("continue")
}

func (n *FuncReturn) format(f *formatter) {
	f.prefix()
	f.str("return")
	if n.value != nil {
		f.str(" ")
		f.expr(n.value)
	}
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) format(f *formatter) {
	f.prefix()
	f.kw("context")
	f.expr(n.body)
}

func (n *ModuleInclude) format(f *formatter) {
	f.prefix()
	f.kw("include")
	f.expr(n.source)
}

func (n *DestructAssignment) format(f *formatter) {
	f.prefix()
	f.destructPattern(n.pattern)
	f.str(" = ")
	f.expr(n.value)
}

func (n *DestructIdent) format(f *formatter) {
	f.str(n.ident.name.Value())
}

func (n *DestructNamed) format(f *formatter) {
	f.str(n.name.Value())
	f.str(": ")
	f.str(n.pattern.name.Value())
}

func (n *DestructSink) format(f *formatter) {
	f.str("..")
	if n.ident != nil {
		f.str(n.ident.name.Value())
	}
}

// Types ///////////////////////////////////////////////////////////////////////////////////////////

func (n Type) format(f *formatter) {
	f.prefix()
	f.str(n.Reflected.String())
}

// Scalars /////////////////////////////////////////////////////////////////////////////////////////

func (n None) format(f *formatter) {
	f.prefix()
	f.str("none")
}

func (n Auto) format(f *formatter) {
	f.prefix()
	f.str("auto")
}

func (n Bool) format(f *formatter) {
	f.prefix()
	f.val(bool(n))
}

func (n Int) format(f *formatter) {
	f.prefix()
	f.val(int(n))
}

func (n Float) format(f *formatter) {
	f.prefix()
	f.val(float64(n))
}

func (n Decimal) format(f *formatter) {
	f.prefix()
	f.val(decimal128.Decimal(n))
}

func (n Numeric) format(f *formatter) {
	f.prefix()
	f.val(float64(n.Value))
	f.str(n.Unit.String())
}

func (n Str) format(f *formatter) {
	f.prefix()
	f.str("\"")
	for _, r := range n {
		switch r {
		case '\n':
			f.str(`\n`)
		case '\t':
			f.str(`\t`)
		case '\\':
			f.str(`\\`)
		case '"':
			f.str(`\"`)
		default:
			if !unicode.IsPrint(r) {
				f.printf(`\u{%x}`, r)
			} else {
				f.sb.WriteRune(r)
			}
		}
	}
	f.str("\"")
}

func (n Bytes) format(f *formatter) {
	f.prefix()
	f.str("(")
	for i, b := range []byte(n) {
		if i > 0 {
			f.str(", ")
		}
		f.val(int(b))
	}
	f.str(")")
}

// Containers //////////////////////////////////////////////////////////////////////////////////////

func (n Array) format(f *formatter) {
	f.prefix()
	f.str("(")
	if len(n) == 0 {
		f.str(")")
		return
	}
	for i, item := range n {
		if i > 0 {
			f.str(", ")
		}
		item.format(f)
	}
	if len(n) == 1 {
		// trailing comma for single-element arrays
		f.str(",")
	}
	f.str(")")
}

func (n Dict) format(f *formatter) {
	f.prefix()
	if len(n) == 0 {
		f.str("(:)")
		return
	}
	f.str("(")
	for i, key := range slices.Sorted(maps.Keys(n)) {
		value := n[key]
		if i > 0 {
			f.str(", ")
		}
		key.format(f)
		f.str(": ")
		value.format(f)
	}
	f.str(")")
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *Function) format(f *formatter) {
	f.prefix()
	f.str(n.Name)
}

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

func (n *Arguments) format(f *formatter) {
	f.prefix()
	f.str("arguments")
	if len(n.Positional) == 0 && len(n.Named) == 0 {
		f.str("()")
		return
	}
	f.str("(")
	for i, v := range n.Positional {
		if i > 0 {
			f.str(", ")
		}
		v.format(f)
	}
	keys := slices.SortedFunc(maps.Keys(n.Named), func(a, b unique.Handle[string]) int { return strings.Compare(a.Value(), b.Value()) })
	for i, name := range keys {
		value := n.Named[name]
		if len(n.Positional) > 0 || i > 0 {
			f.str(", ")
		}
		f.str(name.Value())
		f.str(": ")
		value.format(f)
	}
	f.str(")")
}

// Content /////////////////////////////////////////////////////////////////////////////////////////

func (n *Sequence) format(f *formatter) {
	if n == nil || len(n.Children) == 0 {
		return
	}
	if len(n.Children) == 1 {
		n.Children[0].format(f)
		return
	}
	for i, c := range n.Children {
		if i > 0 {
			f.nl()
		}
		c.format(f)
	}
}

func (n *Heading) format(f *formatter) {
	f.funcCall("heading", []arg{named("level", n.Depth)})
}

func (n *Strong) format(f *formatter) {
	f.funcCall("strong", nil, n.Body)
}

func (n *Emph) format(f *formatter) {
	f.funcCall("emph", nil, n.Body)
}

func (n *Text) format(f *formatter) {
	f.funcCall("text", []arg{pos(n.Text)})
}

func (n *Raw) format(f *formatter) {
	var args []arg
	if n.Block {
		args = append(args, named("block", n.Block))
	}
	if n.Lang != "" {
		args = append(args, named("lang", n.Lang))
	}
	for _, line := range n.Lines {
		args = append(args, pos(line))
	}
	f.funcCall("raw", args)
}

func (n *Linebreak) format(f *formatter) {
	f.funcCall("linebreak", nil)
}

func (n *Parbreak) format(f *formatter) {
	f.funcCall("parbreak", nil)
}

func (n *Link) format(f *formatter) {
	f.funcCall("link", []arg{named("dest", n.Dest)}, n.Body)
}

func (n *Ref) format(f *formatter) {
	args := []arg{named("target", n.Target.Value())}
	var blocks []any
	if n.Supplement != nil {
		blocks = append(blocks, n.Supplement)
	}
	f.funcCall("ref", args, blocks...)
}

func (n *List) format(f *formatter) {
	var args []arg
	for i := range n.Children {
		args = append(args, pos(n.Children[i]))
	}
	f.funcCall("list", args)
}

func (n *ListItem) format(f *formatter) {
	f.funcCall("list.item", nil, n.Body)
}

func (n *Enum) format(f *formatter) {
	var args []arg
	for i := range n.Children {
		args = append(args, pos(n.Children[i]))
	}
	f.funcCall("enum", args)
}

func (n *EnumItem) format(f *formatter) {
	f.funcCall("enum.item", []arg{pos(n.Number)}, n.Body)
}

func (n *Terms) format(f *formatter) {
	var args []arg
	for i := range n.Children {
		args = append(args, pos(n.Children[i]))
	}
	f.funcCall("terms", args)
}

func (n *TermItem) format(f *formatter) {
	f.funcCall("terms.item", nil, n.Term, n.Description)
}

// Label ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Label) format(f *formatter) {
	f.funcCall("label", []arg{pos(n.Name.Value())})
}
