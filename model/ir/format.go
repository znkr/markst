package ir

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"znkr.io/writst/syntax"
)

func Format(content *ContentExpr) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeMarkup}
	for _, c := range content.exprs {
		c.format(f)
		sb.WriteString("\n")
	}
	return sb.String()
}

func FormatValue(value Value) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeCode}
	value.format(f)
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
func (f *formatter) params(sep string, ps []Param) {
	for i, p := range ps {
		if i > 0 {
			f.str(sep)
		}
		switch p := p.(type) {
		case *PositionalParam:
			f.expr(p.ident)
		case *NamedParam:
			f.str(p.name.Value())
			if p.def != nil {
				f.str(": ")
				f.expr(p.def)
			}
		case *SpreadParam:
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

// funcCall formats #name(args)[funcCall] for funcCall nodes
func (f *formatter) funcCall(name string, args []arg, cblocks []*ContentExpr) {
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
	f.contents(cblocks)
	if len(args) == 0 && len(cblocks) == 0 {
		f.str("()")
	}
}

// contents formats multiple content blocks
func (f *formatter) contents(cs []*ContentExpr) {
	for _, c := range cs {
		c.format(f)
	}
}

// contentBlock formats a Content value with proper indentation for multi-item contents
func (f *formatter) contentBlock(c Content) {
	prev := f.mode
	f.mode = syntax.ModeMarkup
	if cs, ok := c.(Contents); ok && len(cs) > 1 {
		f.str("[")
		f.indent++
		for _, item := range cs {
			f.nl()
			item.format(f)
		}
		f.indent--
		f.nl()
		f.str("]")
	} else {
		f.str("[")
		c.format(f)
		f.str("]")
	}
	f.mode = prev
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

func (n *ContentExpr) format(f *formatter) {
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

func (n *HeadingExpr) format(f *formatter) {
	f.funcCall("heading", []arg{named("level", n.level)}, []*ContentExpr{n.body})
}

func (n *StrongExpr) format(f *formatter) {
	f.funcCall("strong", nil, []*ContentExpr{n.body})
}

func (n *EmphExpr) format(f *formatter) {
	f.funcCall("emph", nil, []*ContentExpr{n.body})
}

func (n *LinkExpr) format(f *formatter) {
	f.funcCall("link", []arg{named("dest", n.dest)}, []*ContentExpr{n.body})
}

func (n *RefExpr) format(f *formatter) {
	args := []arg{named("target", n.target.Value())}
	var cblocks []*ContentExpr
	if n.supplement != nil && len(n.supplement.exprs) > 0 {
		cblocks = []*ContentExpr{n.supplement}
	}
	f.funcCall("ref", args, cblocks)
}

func (n *ListItemExpr) format(f *formatter) {
	f.funcCall("list.item", nil, []*ContentExpr{n.body})
}

func (n *EnumItemExpr) format(f *formatter) {
	f.funcCall("enum.item", []arg{pos(n.number)}, []*ContentExpr{n.body})
}

func (n *TermItemExpr) format(f *formatter) {
	f.funcCall("terms.item", nil, []*ContentExpr{n.term, n.description})
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Const) format(f *formatter) {
	n.value.format(f)
}

func (n *Ident) format(f *formatter) {
	f.prefix()
	f.str(n.name.Value())
}

func (n *CodeBlock) format(f *formatter) {
	f.prefix()
	if n.body == nil || len(n.body.exprs) == 0 {
		f.str("{ }")
		return
	}
	prev := f.inlineCode
	f.inlineCode = true
	f.str("{ ")
	f.exprs("; ", n.body.exprs)
	f.str(" }")
	f.inlineCode = prev
}

func (n *ContentBlock) format(f *formatter) {
	f.prefix()
	n.body.format(f)
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
	f.str(n.field)
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) format(f *formatter) {
	f.prefix()
	f.expr(n.callee)
	f.str("(")
	f.arguments(", ", n.args)
	f.str(")")
	f.contents(n.content)
}

func (n *Closure) format(f *formatter) {
	f.prefix()
	if n.name != nil {
		// Named function: name(params) = body
		f.str(n.name.name.Value())
		f.str("(")
		f.params(", ", n.params)
		f.str(") = ")
		f.expr(n.body)
		return
	}
	// Anonymous closure
	if len(n.params) == 1 {
		// Check if param is a spread - if so, needs parens
		if _, isSpread := n.params[0].(*SpreadParam); !isSpread {
			f.params(", ", n.params)
			f.str(" => ")
			f.expr(n.body)
			return
		}
	}
	f.str("(")
	f.params(", ", n.params)
	f.str(") => ")
	f.expr(n.body)
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
	f.kw("if")
	f.expr(n.condition)
	f.str(" ")
	f.expr(n.then)
	if n.els != nil {
		f.str(" else ")
		f.expr(n.els)
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

func (n Numeric) format(f *formatter) {
	f.prefix()
	f.val(float64(n.Value))
	f.str(n.Unit.String())
}

func (n String) format(f *formatter) {
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

// Content /////////////////////////////////////////////////////////////////////////////////////////

func (n Contents) format(f *formatter) {
	if len(n) == 0 {
		return
	}
	if len(n) == 1 {
		n[0].format(f)
		return
	}
	for _, c := range n {
		f.nl()
		c.format(f)
	}
}

func (n *Heading) format(f *formatter) {
	f.prefix()
	f.printf("heading(level: %d)", n.Level)
	f.contentBlock(n.Body)
}

func (n *Strong) format(f *formatter) {
	f.prefix()
	f.str("strong")
	f.contentBlock(n.Body)
}

func (n *Emph) format(f *formatter) {
	f.prefix()
	f.str("emph")
	f.contentBlock(n.Body)
}

func (n *Text) format(f *formatter) {
	f.funcCall("text", []arg{pos(n.Value)}, nil)
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
	f.funcCall("raw", args, nil)
}

func (n *Linebreak) format(f *formatter) {
	f.funcCall("linebreak", nil, nil)
}

func (n *Parbreak) format(f *formatter) {
	f.funcCall("parbreak", nil, nil)
}

func (n *Link) format(f *formatter) {
	f.prefix()
	f.printf("link(dest: %q)", n.Dest)
	f.contentBlock(n.Body)
}

func (n *Ref) format(f *formatter) {
	f.prefix()
	f.printf("ref(target: %q)", n.Target.Value())
	if n.Supplement != nil {
		f.contentBlock(n.Supplement)
	}
}

func (n *List) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.funcCall("list", args, nil)
}

func (n *ListItem) format(f *formatter) {
	f.prefix()
	f.str("list.item")
	f.contentBlock(n.Body)
}

func (n *Enum) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.funcCall("enum", args, nil)
}

func (n *EnumItem) format(f *formatter) {
	f.prefix()
	f.printf("enum.item(%d)", n.Number)
	f.contentBlock(n.Body)
}

func (n *Terms) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.funcCall("terms", args, nil)
}

func (n *TermItem) format(f *formatter) {
	f.prefix()
	f.str("terms.item")
	f.contentBlock(n.Term)
	f.contentBlock(n.Description)
}

// Label ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Label) format(f *formatter) {
	f.funcCall("label", []arg{pos(n.Name.Value())}, nil)
}
