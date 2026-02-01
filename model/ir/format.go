package ir

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"znkr.io/writst/syntax"
)

func Format(content ContentExpr) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeMarkup}
	for _, c := range content {
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
			f.expr(e.Expr)
		case *ExprArg:
			f.expr(e.Expr)
		case *NamedArg:
			f.str(e.Name.Value())
			f.str(": ")
			f.expr(e.Expr)
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
			f.expr(p.Ident)
		case *NamedParam:
			f.str(p.Name.Value())
			if p.Default != nil {
				f.str(": ")
				f.expr(p.Default)
			}
		case *SpreadParam:
			f.str("..")
			f.expr(p.Ident)
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
func (f *formatter) funcCall(name string, args []arg, cblocks []ContentExpr) {
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
func (f *formatter) contents(cs []ContentExpr) {
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

func (n ContentExpr) format(f *formatter) {
	if len(n) == 0 {
		f.str("[]")
		return
	}
	prev := f.mode
	f.mode = syntax.ModeMarkup
	// Stay inline if there's only one item OR we're inside inline code
	if len(n) == 1 || f.inlineCode {
		f.str("[")
		for i, item := range n {
			if i > 0 {
				f.str(" ")
			}
			item.format(f)
		}
		f.str("]")
	} else {
		f.str("[")
		f.indent++
		for _, n := range n {
			f.nl()
			n.format(f)
		}
		f.indent--
		f.nl()
		f.str("]")
	}
	f.mode = prev
}

func (n *HeadingExpr) format(f *formatter) {
	f.funcCall("heading", []arg{named("level", n.Level)}, []ContentExpr{n.Body})
}

func (n *StrongExpr) format(f *formatter) {
	f.funcCall("strong", nil, []ContentExpr{n.Body})
}

func (n *EmphExpr) format(f *formatter) {
	f.funcCall("emph", nil, []ContentExpr{n.Body})
}

func (n *LinkExpr) format(f *formatter) {
	f.funcCall("link", []arg{named("dest", n.Dest)}, []ContentExpr{n.Body})
}

func (n *RefExpr) format(f *formatter) {
	args := []arg{named("target", n.Target.Value())}
	var cblocks []ContentExpr
	if len(n.Supplement) > 0 {
		cblocks = []ContentExpr{n.Supplement}
	}
	f.funcCall("ref", args, cblocks)
}

func (n *ListItemExpr) format(f *formatter) {
	f.funcCall("list.item", nil, []ContentExpr{n.Body})
}

func (n *EnumItemExpr) format(f *formatter) {
	f.funcCall("enum.item", []arg{pos(n.Number)}, []ContentExpr{n.Body})
}

func (n *TermItemExpr) format(f *formatter) {
	f.funcCall("terms.item", nil, []ContentExpr{n.Term, n.Description})
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Const) format(f *formatter) {
	n.Value.format(f)
}

func (n *Ident) format(f *formatter) {
	f.prefix()
	f.str(n.Name.Value())
}

func (n *CodeBlock) format(f *formatter) {
	f.prefix()
	if len(n.Body) == 0 {
		f.str("{ }")
		return
	}
	prev := f.inlineCode
	f.inlineCode = true
	f.str("{ ")
	f.exprs("; ", n.Body)
	f.str(" }")
	f.inlineCode = prev
}

func (n *ContentBlock) format(f *formatter) {
	f.prefix()
	n.Body.format(f)
}

func (n *Parenthesized) format(f *formatter) {
	f.prefix()
	f.str("(")
	f.expr(n.Body)
	f.str(")")
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) format(f *formatter) {
	f.prefix()
	f.str("(")
	if len(n.Elements) == 0 {
		f.str(")")
		return
	}
	f.exprs(", ", n.Elements)
	if len(n.Elements) == 1 {
		// trailing comma for single-element arrays
		f.str(",")
	}
	f.str(")")
}

func (n *DictExpr) format(f *formatter) {
	f.prefix()
	if len(n.Entries) == 0 {
		f.str("(:)")
		return
	}
	f.str("(")
	for i, ent := range n.Entries {
		if i > 0 {
			f.str(", ")
		}
		f.expr(ent.Key)
		f.str(": ")
		f.expr(ent.Value)
	}
	f.str(")")
}

// Operators ///////////////////////////////////////////////////////////////////////////////////////

func (n *Unary) format(f *formatter) {
	f.prefix()
	op := n.Op.String()
	f.str(op)
	// Add space after keyword operators like "not"
	if op == "not" {
		f.str(" ")
	}
	f.expr(n.Operand)
}

func (n *Binary) format(f *formatter) {
	f.prefix()
	f.expr(n.Left)
	f.op(n.Op)
	f.expr(n.Right)
}

func (n *FieldAccess) format(f *formatter) {
	f.prefix()
	f.expr(n.Target)
	f.str(".")
	f.str(n.Field)
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) format(f *formatter) {
	f.prefix()
	f.expr(n.Callee)
	f.str("(")
	f.arguments(", ", n.Args)
	f.str(")")
	f.contents(n.Content)
}

func (n *Closure) format(f *formatter) {
	f.prefix()
	if n.Name != nil {
		// Named function: name(params) = body
		f.str(n.Name.Name.Value())
		f.str("(")
		f.params(", ", n.Params)
		f.str(") = ")
		f.expr(n.Body)
		return
	}
	// Anonymous closure
	if len(n.Params) == 1 {
		// Check if param is a spread - if so, needs parens
		if _, isSpread := n.Params[0].(*SpreadParam); !isSpread {
			f.params(", ", n.Params)
			f.str(" => ")
			f.expr(n.Body)
			return
		}
	}
	f.str("(")
	f.params(", ", n.Params)
	f.str(") => ")
	f.expr(n.Body)
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) format(f *formatter) {
	f.prefix()
	f.kw("let")
	if n.Value != nil {
		// Check if value is a named closure (let function)
		if closure, ok := n.Value.(*Closure); ok && closure.Name != nil {
			f.expr(n.Value)
			return
		}
		f.destructPattern(n.Pattern)
		f.str(" = ")
		f.expr(n.Value)
		return
	}
	f.destructPattern(n.Pattern)
}

func (n *SetRule) format(f *formatter) {
	f.prefix()
	f.kw("set")
	f.expr(n.Target)
	f.str("(")
	f.arguments(", ", n.Args)
	f.str(")")
	if n.Condition != nil {
		f.kw(" if")
		f.expr(n.Condition)
	}
}

func (n *ShowRule) format(f *formatter) {
	f.prefix()
	f.str("show")
	if n.Selector != nil {
		f.str(" ")
		f.expr(n.Selector)
	}
	f.str(": ")
	f.expr(n.Transform)
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) format(f *formatter) {
	f.prefix()
	f.kw("if")
	f.expr(n.Condition)
	f.str(" ")
	f.expr(n.Then)
	if n.Else != nil {
		f.str(" else ")
		f.expr(n.Else)
	}
}

func (n *WhileLoop) format(f *formatter) {
	f.prefix()
	f.kw("while")
	f.expr(n.Condition)
	f.str(" ")
	f.expr(n.Body)
}

func (n *ForLoop) format(f *formatter) {
	f.prefix()
	f.kw("for")
	f.destructPattern(n.Pattern)
	f.kw(" in")
	f.expr(n.Iterable)
	f.str(" ")
	f.expr(n.Body)
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
	if n.Value != nil {
		f.str(" ")
		f.expr(n.Value)
	}
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) format(f *formatter) {
	f.prefix()
	f.kw("context")
	f.expr(n.Body)
}

func (n *ModuleInclude) format(f *formatter) {
	f.prefix()
	f.kw("include")
	f.expr(n.Source)
}

func (n *DestructAssignment) format(f *formatter) {
	f.prefix()
	f.destructPattern(n.Pattern)
	f.str(" = ")
	f.expr(n.Value)
}

func (n *DestructIdent) format(f *formatter) {
	f.str(n.Ident.Name.Value())
}

func (n *DestructNamed) format(f *formatter) {
	f.str(n.Name.Value())
	f.str(": ")
	f.str(n.Pattern.Name.Value())
}

func (n *DestructSink) format(f *formatter) {
	f.str("..")
	if n.Ident != nil {
		f.str(n.Ident.Name.Value())
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
