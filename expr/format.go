package expr

import (
	"fmt"

	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/syntax"
)

// FormatExprs returns a string representation of IR expressions, used as
// expected output in analyzer test golden files.
func FormatExprs(exprs []Expr) string {
	f := formatter.New(syntax.ModeMarkup)
	for _, c := range exprs {
		c.Format(f)
		f.Str("\n")
	}
	return f.String()
}

// op writes a binary operator with surrounding spaces
func formatOp(f *formatter.Formatter, o syntax.BinaryOp) {
	f.Str(" ")
	f.Str(o.String())
	f.Str(" ")
}

// expr formats an expression in code mode
func formatExpr(f *formatter.Formatter, e Expr) {
	prev := f.Mode()
	f.SetMode(syntax.ModeCode)
	e.Format(f)
	f.SetMode(prev)
}

// exprs formats expressions with a separator
func formatExprs(f *formatter.Formatter, sep string, es []Expr) {
	for i, e := range es {
		if i > 0 {
			f.Str(sep)
		}
		formatExpr(f, e)
	}
}

// destructPattern formats a slice of destruct patterns
func formatDestructPattern(f *formatter.Formatter, patterns []DestructPattern) {
	if len(patterns) == 1 {
		patterns[0].Format(f)
		return
	}
	f.Str("(")
	for i, p := range patterns {
		if i > 0 {
			f.Str(", ")
		}
		p.Format(f)
	}
	f.Str(")")
}

// args formats arguments with a separator
func formatArguments(f *formatter.Formatter, sep string, es []Arg) {
	for i, e := range es {
		if i > 0 {
			f.Str(sep)
		}
		switch e := e.(type) {
		case *SpreadArg:
			f.Str("..")
			formatExpr(f, e.expr)
		case *ExprArg:
			formatExpr(f, e.expr)
		case *NamedArg:
			f.Str(e.name.Value())
			f.Str(": ")
			formatExpr(f, e.expr)
		default:
			panic(fmt.Sprintf("unknown arg type: %T", e))
		}
	}
}

// params formats parameters with a separator
func formatParams(f *formatter.Formatter, sep string, ps []ClosureParam) {
	for i, p := range ps {
		if i > 0 {
			f.Str(sep)
		}
		switch p := p.(type) {
		case *PositionalClosureParam:
			formatExpr(f, p.name)
		case *NamedClosureParam:
			f.Str(p.name.name.Value())
			if p.def != nil {
				f.Str(": ")
				formatExpr(f, p.def)
			}
		case *SpreadClosureParam:
			f.Str("..")
			formatExpr(f, p.ident)
		default:
			panic(fmt.Sprintf("unknown param type: %T", p))
		}
	}
}

type exprBlock []Expr

func (b exprBlock) Format(f *formatter.Formatter) {
	formatCodeBlock(f, b)
}

// formatCodeBlock formats an []Expr block wrapped in [...].
func formatCodeBlock(f *formatter.Formatter, exprs []Expr) {
	if len(exprs) == 0 {
		return
	}
	prev := f.Mode()
	f.SetMode(syntax.ModeMarkup)
	f.Str("[")
	if len(exprs) == 1 {
		exprs[0].Format(f)
		f.Str("]")
		f.SetMode(prev)
		return
	}
	f.IncreaseIndent()
	for _, item := range exprs {
		f.Linebreak()
		item.Format(f)
	}
	f.DecreaseIndent()
	f.Linebreak()
	f.Str("]")
	f.SetMode(prev)
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

func (n *HeadingExpr) Format(f *formatter.Formatter) {
	f.FuncCall("heading", []formatter.Arg{formatter.NamedArg("level", n.level)}, exprBlock(n.body))
}

func (n *StrongExpr) Format(f *formatter.Formatter) {
	f.FuncCall("strong", nil, exprBlock(n.body))
}

func (n *EmphExpr) Format(f *formatter.Formatter) {
	f.FuncCall("emph", nil, exprBlock(n.body))
}

func (n *LinkExpr) Format(f *formatter.Formatter) {
	f.FuncCall("link", []formatter.Arg{formatter.NamedArg("dest", n.dest)}, exprBlock(n.body))
}

func (n *RefExpr) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.NamedArg("target", n.target.Value())}
	var blocks []formatter.Formattable
	if n.supplement != nil && len(n.supplement.exprs) > 0 {
		blocks = append(blocks, exprBlock(n.supplement.Body()))
	}
	f.FuncCall("ref", args, blocks...)
}

func (n *ListItemExpr) Format(f *formatter.Formatter) {
	f.FuncCall("list.item", nil, exprBlock(n.body))
}

func (n *EnumItemExpr) Format(f *formatter.Formatter) {
	f.FuncCall("enum.item", []formatter.Arg{formatter.PositionalArg(n.number)}, exprBlock(n.body))
}

func (n *TermItemExpr) Format(f *formatter.Formatter) {
	f.FuncCall("terms.item", nil, exprBlock(n.term), exprBlock(n.description))
}

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) Format(f *formatter.Formatter) {
	n.value.Format(f)
}

func (n *Ident) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.name.Value())
}

func (n *CodeBlock) Format(f *formatter.Formatter) {
	f.Prefix()
	if len(n.exprs) == 0 {
		f.Str("{ }")
		return
	}
	f.Inline(func(f *formatter.Formatter) {
		f.Str("{ ")
		formatExprs(f, "; ", n.exprs)
		f.Str(" }")
	})
}

func (n *ContentBlock) Format(f *formatter.Formatter) {
	f.Prefix()
	if len(n.exprs) == 0 {
		f.Str("[]")
		return
	}
	prev := f.Mode()
	f.SetMode(syntax.ModeMarkup)
	if len(n.exprs) == 1 {
		f.Str("[")
		for i, item := range n.exprs {
			if i > 0 {
				f.Str(" ")
			}
			item.Format(f)
		}
		f.Str("]")
	} else {
		f.Str("[")
		f.IncreaseIndent()
		for _, item := range n.exprs {
			f.Linebreak()
			item.Format(f)
		}
		f.DecreaseIndent()
		f.Linebreak()
		f.Str("]")
	}
	f.SetMode(prev)
}

func (n *Parenthesized) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("(")
	formatExpr(f, n.body)
	f.Str(")")
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("(")
	if len(n.elements) == 0 {
		f.Str(")")
		return
	}
	formatExprs(f, ", ", n.elements)
	if len(n.elements) == 1 {
		// trailing comma for single-element arrays
		f.Str(",")
	}
	f.Str(")")
}

func (n *SpreadExpr) Format(f *formatter.Formatter) {
	f.Str("..")
	formatExpr(f, n.inner)
}

func (n *DictExpr) Format(f *formatter.Formatter) {
	f.Prefix()
	if len(n.entries) == 0 {
		f.Str("(:)")
		return
	}
	f.Str("(")
	for i, ent := range n.entries {
		if i > 0 {
			f.Str(", ")
		}
		formatExpr(f, ent.key)
		f.Str(": ")
		formatExpr(f, ent.value)
	}
	f.Str(")")
}

// Operators ///////////////////////////////////////////////////////////////////////////////////////

func (n *Unary) Format(f *formatter.Formatter) {
	f.Prefix()
	op := n.op.String()
	f.Str(op)
	// Add space after keyword operators like "not"
	if op == "not" {
		f.Str(" ")
	}
	formatExpr(f, n.operand)
}

func (n *Binary) Format(f *formatter.Formatter) {
	f.Prefix()
	formatExpr(f, n.left)
	formatOp(f, n.op)
	formatExpr(f, n.right)
}

func (n *FieldAccess) Format(f *formatter.Formatter) {
	f.Prefix()
	formatExpr(f, n.target)
	f.Str(".")
	formatExpr(f, n.field)
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) Format(f *formatter.Formatter) {
	f.Prefix()
	formatExpr(f, n.callee)
	f.Str("(")
	formatArguments(f, ", ", n.args)
	f.Str(")")
	for _, block := range n.blocks {
		formatCodeBlock(f, block.exprs)
	}
}

func (n *Closure) Format(f *formatter.Formatter) {
	f.Prefix()
	if n.name != nil {
		// Named function: name(params) = body
		f.Str(n.name.name.Value())
		f.Str("(")
		formatParams(f, ", ", n.params)
		f.Str(") = ")
		formatClosureBody(f, n.body)
		return
	}
	// Anonymous closure
	if len(n.params) == 1 {
		// Check if param is a spread - if so, needs parens
		if _, isSpread := n.params[0].(*SpreadClosureParam); !isSpread {
			formatParams(f, ", ", n.params)
			f.Str(" => ")
			formatClosureBody(f, n.body)
			return
		}
	}
	f.Str("(")
	formatParams(f, ", ", n.params)
	f.Str(") => ")
	formatClosureBody(f, n.body)
}

// closureBody formats a closure body, unwrapping single-expression code blocks.
func formatClosureBody(f *formatter.Formatter, body Expr) {
	if cb, ok := body.(*CodeBlock); ok && len(cb.exprs) == 1 {
		formatExpr(f, cb.exprs[0])
	} else {
		formatExpr(f, body)
	}
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("let")
	if n.value != nil {
		// Check if value is a named closure (let function)
		if closure, ok := n.value.(*Closure); ok && closure.name != nil {
			formatExpr(f, n.value)
			return
		}
		formatDestructPattern(f, n.pattern)
		f.Str(" = ")
		formatExpr(f, n.value)
		return
	}
	formatDestructPattern(f, n.pattern)
}

func (n *SetRule) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("set")
	formatExpr(f, n.target)
	f.Str("(")
	formatArguments(f, ", ", n.args)
	f.Str(")")
	if n.condition != nil {
		f.Str(" if ")
		formatExpr(f, n.condition)
	}
}

func (n *ShowRule) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("show")
	if n.selector != nil {
		f.Str(" ")
		formatExpr(f, n.selector)
	}
	f.Str(": ")
	formatExpr(f, n.transform)
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) Format(f *formatter.Formatter) {
	f.Prefix()
	for i, cond := range n.conditions {
		if i == 0 {
			f.Keyword("if")
		} else {
			f.Str("else ")
			f.Keyword("if")
		}
		formatExpr(f, cond)
		f.Str(" ")
		formatExpr(f, n.blocks[i])
		if i < len(n.conditions)-1 || n.def != nil {
			f.Str(" ")
		}
	}
	if n.def != nil {
		f.Str("else ")
		formatExpr(f, n.def)
	}
}

func (n *WhileLoop) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("while")
	formatExpr(f, n.condition)
	f.Str(" ")
	formatExpr(f, n.body)
}

func (n *ForLoop) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("for")
	formatDestructPattern(f, n.pattern)
	f.Str(" in ")
	formatExpr(f, n.iterable)
	f.Str(" ")
	formatExpr(f, n.body)
}

func (n *LoopBreak) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("break")
}

func (n *LoopContinue) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("continue")
}

func (n *FuncReturn) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("return")
	if n.value != nil {
		f.Str(" ")
		formatExpr(f, n.value)
	}
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("context")
	formatExpr(f, n.body)
}

func (n *ModuleInclude) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Keyword("include")
	formatExpr(f, n.source)
}

func (n *DestructAssignment) Format(f *formatter.Formatter) {
	f.Prefix()
	formatDestructPattern(f, n.pattern)
	f.Str(" = ")
	formatExpr(f, n.value)
}

func (n *DestructIdent) Format(f *formatter.Formatter) {
	f.Str(n.ident.name.Value())
}

func (n *DestructNamed) Format(f *formatter.Formatter) {
	f.Str(n.name.Value())
	f.Str(": ")
	f.Str(n.pattern.name.Value())
}

func (n *DestructSink) Format(f *formatter.Formatter) {
	f.Str("..")
	if n.ident != nil {
		f.Str(n.ident.name.Value())
	}
}
