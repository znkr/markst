package ir

import (
	"fmt"
	"strings"
	"unicode"

	"znkr.io/writst/syntax"
)

func Format(content Content) string {
	var sb strings.Builder
	f := &formatter{sb: &sb, mode: syntax.ModeMarkup}
	for _, c := range content {
		c.format(f)
		sb.WriteString("\n")
	}
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

// contents formats multiple content blocks
func (f *formatter) contents(cs []Content) {
	for _, c := range cs {
		c.format(f)
	}
}

type arg struct {
	name       string
	positional bool
	value      any
}

func named(name string, v any) arg { return arg{name: name, value: v} }
func pos(v any) arg                { return arg{positional: true, value: v} }

// markup formats #name(args)[content] for markup nodes
func (f *formatter) markup(name string, args []arg, cblocks []Content) {
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

func (n Content) format(f *formatter) {
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

func (n *Label) format(f *formatter) {
	f.markup("label", []arg{pos(n.Name.Value())}, nil)
}

// Markup //////////////////////////////////////////////////////////////////////////////////////

func (n *Text) format(f *formatter) {
	f.markup("text", []arg{pos(n.Value)}, nil)
}

func (n *Heading) format(f *formatter) {
	f.markup("heading", []arg{named("level", n.Level)}, []Content{n.Body})
}

func (n *Strong) format(f *formatter) {
	f.markup("strong", nil, []Content{n.Body})
}

func (n *Emph) format(f *formatter) {
	f.markup("emph", nil, []Content{n.Body})
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
	f.markup("raw", args, nil)
}

func (n *Linebreak) format(f *formatter) {
	f.markup("linebreak", nil, nil)
}

func (n *Parbreak) format(f *formatter) {
	f.markup("parbreak", nil, nil)
}

func (n *Link) format(f *formatter) {
	f.markup("link", []arg{named("dest", n.Dest)}, []Content{n.Body})
}

func (n *Ref) format(f *formatter) {
	args := []arg{named("target", n.Target.Value())}
	var cblocks []Content
	if len(n.Supplement) > 0 {
		cblocks = []Content{n.Supplement}
	}
	f.markup("ref", args, cblocks)
}

func (n *List) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.markup("list", args, nil)
}

func (n *ListItem) format(f *formatter) {
	f.markup("list.item", nil, []Content{n.Body})
}

func (n *Enum) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.markup("enum", args, nil)
}

func (n *EnumItem) format(f *formatter) {
	f.markup("enum.item", []arg{pos(n.Number)}, []Content{n.Body})
}

func (n *Terms) format(f *formatter) {
	var args []arg
	for i := range n.Items {
		args = append(args, pos(&n.Items[i]))
	}
	f.markup("terms", args, nil)
}

func (n *TermItem) format(f *formatter) {
	f.markup("terms.item", nil, []Content{n.Term, n.Description})
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////

func (n *None) format(f *formatter) {
	f.prefix()
	f.str("none")
}

func (n *Auto) format(f *formatter) {
	f.prefix()
	f.str("auto")
}

func (n *Bool) format(f *formatter) {
	f.prefix()
	f.val(n.Value)
}

func (n *Int) format(f *formatter) {
	f.prefix()
	f.val(n.Value)
}

func (n *Float) format(f *formatter) {
	f.prefix()
	f.val(n.Value)
}

func (n *Numeric) format(f *formatter) {
	f.prefix()
	f.val(n.Value)
	f.str(n.Unit.String())
}

func (n *Str) format(f *formatter) {
	f.prefix()
	f.str("\"")
	for _, r := range n.Value {
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

func (n *Ident) format(f *formatter) {
	f.prefix()
	f.str(n.Name)
}

func (n *CodeBlock) format(f *formatter) {
	f.prefix()
	if len(n.Exprs) == 0 {
		f.str("{ }")
		return
	}
	prev := f.inlineCode
	f.inlineCode = true
	f.str("{ ")
	f.exprs("; ", n.Exprs)
	f.str(" }")
	f.inlineCode = prev
}

func (n *ContentBlock) format(f *formatter) {
	f.prefix()
	n.Body.format(f)
}

func (n *Underscore) format(f *formatter) {
	f.prefix()
	f.str("_")
}

func (n *Parenthesized) format(f *formatter) {
	f.prefix()
	f.str("(")
	f.expr(n.Body)
	f.str(")")
}

func (n *Array) format(f *formatter) {
	f.prefix()
	f.str("(")
	if len(n.Items) == 0 {
		f.str(")")
		return
	}
	if len(n.Items) == 1 {
		f.expr(n.Items[0])
		// Single element needs trailing comma unless it's a spread
		if _, isSpread := n.Items[0].(*Spread); !isSpread {
			f.str(",")
		}
		f.str(")")
		return
	}
	f.exprs(", ", n.Items)
	f.str(")")
}

func (n *Dict) format(f *formatter) {
	f.prefix()
	if len(n.Items) == 0 {
		f.str("(:)")
		return
	}
	f.str("(")
	f.exprs(", ", n.Items)
	f.str(")")
}

func (n *Named) format(f *formatter) {
	f.prefix()
	f.str(n.Name)
	f.str(": ")
	f.expr(n.Value)
}

func (n *Keyed) format(f *formatter) {
	f.prefix()
	f.expr(n.Key)
	f.str(": ")
	f.expr(n.Value)
}

func (n *Spread) format(f *formatter) {
	f.prefix()
	f.str("..")
	if n.Expr != nil {
		f.expr(n.Expr)
	}
}

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

func (n *FuncCall) format(f *formatter) {
	f.prefix()
	f.expr(n.Callee)
	f.str("(")
	f.exprs(", ", n.Args)
	f.str(")")
	f.contents(n.Content)
}

func (n *Closure) format(f *formatter) {
	f.prefix()
	if n.Name != "" {
		// Named function: name(params) = body
		f.str(n.Name)
		f.str("(")
		f.exprs(", ", n.Params)
		f.str(") = ")
		f.expr(n.Body)
		return
	}
	// Anonymous closure
	if len(n.Params) == 1 {
		// Check if param is a spread - if so, needs parens
		if _, isSpread := n.Params[0].(*Spread); !isSpread {
			f.expr(n.Params[0])
			f.str(" => ")
			f.expr(n.Body)
			return
		}
	}
	f.str("(")
	f.exprs(", ", n.Params)
	f.str(") => ")
	f.expr(n.Body)
}

func (n *LetBinding) format(f *formatter) {
	f.prefix()
	f.kw("let")
	if n.Value != nil {
		// Check if value is a named closure (let function)
		if closure, ok := n.Value.(*Closure); ok && closure.Name != "" {
			f.expr(n.Value)
			return
		}
		f.expr(n.Pattern)
		f.str(" = ")
		f.expr(n.Value)
		return
	}
	f.expr(n.Pattern)
}

func (n *SetRule) format(f *formatter) {
	f.prefix()
	f.kw("set")
	f.expr(n.Target)
	f.str("(")
	f.exprs(", ", n.Args)
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
	f.expr(n.Pattern)
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
	f.expr(n.Pattern)
	f.str(" = ")
	f.expr(n.Value)
}

func (n *Destructuring) format(f *formatter) {
	f.prefix()
	f.str("(")
	f.exprs(", ", n.Items)
	f.str(")")
}
