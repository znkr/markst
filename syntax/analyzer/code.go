package analyzer

import (
	"strconv"
	"strings"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
)

var bools = map[string]*ir.Bool{
	"true":  {Value: true},
	"false": {Value: false},
}

func (a *analyzer) analyzeBool(n syntax.Node) *ir.Bool {
	val := leaf(n, syntax.KindBool)
	ret := bools[val]
	if ret == nil {
		panic("invalid bool literal: " + val)
	}
	return ret
}

func (a *analyzer) analyzeInt(n syntax.Node) *ir.Int {
	val := leaf(n, syntax.KindInt)
	base := 10
	if len(val) >= 2 && val[0] == '0' {
		switch val[1] {
		case 'b':
			base = 2
			val = val[2:]
		case 'o':
			base = 8
			val = val[2:]
		case 'x':
			base = 16
			val = val[2:]
		}
	}
	iv, err := strconv.ParseInt(val, base, 64)
	if err != nil {
		panic("invalid int literal: " + val)
	}
	return &ir.Int{Value: iv}
}

func (a *analyzer) analyzeFloat(n syntax.Node) *ir.Float {
	val := leaf(n, syntax.KindFloat)
	fv, err := strconv.ParseFloat(val, 64)
	if err != nil {
		panic("invalid float literal: " + val)
	}
	return &ir.Float{Value: fv}
}

func (a *analyzer) analyzeNumeric(n syntax.Node) *ir.Numeric {
	val := leaf(n, syntax.KindNumeric)
	// Find where the numeric part ends and unit begins
	idx := strings.IndexFunc(val, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if idx == -1 {
		panic("invalid numeric literal, no unit: " + val)
	}
	num, suffix := val[:idx], val[idx:]
	fv, err := strconv.ParseFloat(num, 64)
	if err != nil {
		panic("invalid float literal: " + val)
	}
	u, ok := ir.ParseUnit(suffix)
	if !ok {
		panic("invalid unit literal: " + val)
	}
	return &ir.Numeric{Value: fv, Unit: u}
}

func (a *analyzer) analyzeStr(n syntax.Node) *ir.Str {
	val := leaf(n, syntax.KindStr)
	return &ir.Str{Value: unquote(val)}
}

func (a *analyzer) analyzeIdent(n syntax.Node) *ir.Ident {
	return &ir.Ident{Name: leaf(n, syntax.KindIdent)}
}

func (a *analyzer) analyzeCode(n syntax.Node) []ir.Expr {
	var exprs []ir.Expr
	for child := range inner(n, syntax.KindCode).all() {
		if child.Kind == syntax.KindSemicolon {
			continue
		}
		exprs = append(exprs, a.analyzeExpr(child))
	}
	return exprs
}

func (a *analyzer) analyzeCodeBlock(n syntax.Node) *ir.CodeBlock {
	ns := inner(n, syntax.KindCodeBlock)
	defer ns.finish()
	var exprs []ir.Expr
	for child := range ns.inside(syntax.KindLeftBrace, syntax.KindRightBrace) {
		switch child.Kind {
		case syntax.KindCode:
			exprs = append(exprs, a.analyzeCode(child)...)
		default:
			exprs = append(exprs, a.analyzeExpr(child))
		}
	}
	return &ir.CodeBlock{Exprs: exprs}
}

func (a *analyzer) analyzeCodeContentBlock(n syntax.Node) *ir.ContentBlock {
	return &ir.ContentBlock{Body: a.analyzeContentBlock(n)}
}

func (a *analyzer) analyzeParenthesized(n syntax.Node) *ir.Parenthesized {
	ns := inner(n, syntax.KindParenthesized)
	defer ns.finish()
	ns.take(syntax.KindLeftParen)
	n0 := ns.node()
	ns.take(syntax.KindRightParen)
	return &ir.Parenthesized{Body: a.analyzeExpr(n0)}
}

func (a *analyzer) analyzeArray(n syntax.Node) *ir.Array {
	ns := inner(n, syntax.KindArray)
	defer ns.finish()
	var items []ir.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		if child.Kind == syntax.KindComma {
			continue
		}
		items = append(items, a.analyzeExpr(child))
	}
	return &ir.Array{Items: items}
}

func (a *analyzer) analyzeDict(n syntax.Node) *ir.Dict {
	ns := inner(n, syntax.KindDict)
	defer ns.finish()
	var items []ir.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind {
		case syntax.KindComma, syntax.KindColon:
			continue
		default:
			items = append(items, a.analyzeExpr(child))
		}
	}
	return &ir.Dict{Items: items}
}

func (a *analyzer) analyzeNamed(n syntax.Node) *ir.Named {
	ns := inner(n, syntax.KindNamed)
	defer ns.finish()
	name := ns.take(syntax.KindIdent)
	ns.take(syntax.KindColon)
	value := a.analyzeExpr(ns.node())
	return &ir.Named{Name: name, Value: value}
}

func (a *analyzer) analyzeKeyed(n syntax.Node) *ir.Keyed {
	ns := inner(n, syntax.KindKeyed)
	defer ns.finish()
	key := a.analyzeExpr(ns.node())
	ns.take(syntax.KindColon)
	value := a.analyzeExpr(ns.node())
	return &ir.Keyed{Key: key, Value: value}
}

func (a *analyzer) analyzeSpread(n syntax.Node) *ir.Spread {
	ns := inner(n, syntax.KindSpread)
	defer ns.finish()
	ns.take(syntax.KindDots)
	expr := a.analyzeExpr(ns.node())
	return &ir.Spread{Expr: expr}
}

func (a *analyzer) analyzeUnary(n syntax.Node) *ir.Unary {
	ns := inner(n, syntax.KindUnary)
	defer ns.finish()
	op := syntax.UnaryOpFromKind(ns.node().Kind)
	operand := a.analyzeExpr(ns.node())
	return &ir.Unary{Op: op, Operand: operand}
}

func (a *analyzer) analyzeBinary(n syntax.Node) *ir.Binary {
	ns := inner(n, syntax.KindBinary)
	defer ns.finish()
	left := a.analyzeExpr(ns.node())
	// Check for "not in" operator (two tokens)
	var op syntax.BinaryOp
	if ns.at(syntax.KindNot) {
		// Must be not in, because "not" alone is unary
		ns.take(syntax.KindNot)
		ns.take(syntax.KindIn)
		op = syntax.NotIn
	} else {
		op = syntax.BinaryOpFromKind(ns.node().Kind)
	}
	right := a.analyzeExpr(ns.node())
	return &ir.Binary{Left: left, Op: op, Right: right}
}

func (a *analyzer) analyzeFieldAccess(n syntax.Node) *ir.FieldAccess {
	ns := inner(n, syntax.KindFieldAccess)
	defer ns.finish()
	target := a.analyzeExpr(ns.node())
	ns.take(syntax.KindDot)
	field := ns.take(syntax.KindIdent)
	return &ir.FieldAccess{Target: target, Field: field}
}

func (a *analyzer) analyzeFuncCall(n syntax.Node) *ir.FuncCall {
	ns := inner(n, syntax.KindFuncCall)
	defer ns.finish()
	callee := a.analyzeExpr(ns.node())
	args, content := a.analyzeArgs(ns.node())
	return &ir.FuncCall{Callee: callee, Args: args, Content: content}
}

func (a *analyzer) analyzeArgs(n syntax.Node) ([]ir.Expr, []ir.Content) {
	var args []ir.Expr
	var content []ir.Content
	// Args may or may not have parentheses - e.g. foo(1) vs foo[content]
	// Content blocks can appear after the closing paren: foo(1)[hello]
	for child := range inner(n, syntax.KindArgs).all() {
		switch child.Kind {
		case syntax.KindLeftParen, syntax.KindRightParen, syntax.KindComma:
			continue
		case syntax.KindContentBlock:
			content = append(content, a.analyzeContentBlock(child))
		default:
			args = append(args, a.analyzeExpr(child))
		}
	}
	return args, content
}

func (a *analyzer) analyzeClosure(n syntax.Node) *ir.Closure {
	ns := inner(n, syntax.KindClosure)
	defer ns.finish()

	// Handle different closure forms:
	// - Named function: name(params) = body
	// - Anonymous with parens: (params) => body
	// - Anonymous single param: param => body
	var name string
	var params []ir.Expr
	switch n := ns.node(); n.Kind {
	case syntax.KindIdent:
		if ns.at(syntax.KindParams) {
			// Named function: name(params) = body
			name = leaf(n, syntax.KindIdent)
			params = a.analyzeParams(ns.node())
		} else {
			// Single param: param => body
			params = []ir.Expr{a.analyzeIdent(n)}
		}
	case syntax.KindParams:
		params = a.analyzeParams(n)
	default:
		// Single expression as param (e.g., underscore)
		params = []ir.Expr{a.analyzeExpr(n)}
	}

	// Skip arrow or eq
	if ns.at(syntax.KindArrow) || ns.at(syntax.KindEq) {
		ns.node()
	}
	body := a.analyzeExpr(ns.node())
	return &ir.Closure{Name: name, Params: params, Body: body}
}

func (a *analyzer) analyzeParams(n syntax.Node) []ir.Expr {
	ns := inner(n, syntax.KindParams)
	defer ns.finish()
	var params []ir.Expr
	// Params may or may not have parentheses - single param closures don't need them
	if ns.at(syntax.KindLeftParen) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			if child.Kind == syntax.KindComma {
				continue
			}
			params = append(params, a.analyzeExpr(child))
		}
	} else {
		// Single param without parens
		for child := range ns.all() {
			params = append(params, a.analyzeExpr(child))
		}
	}
	return params
}

func (a *analyzer) analyzeLetBinding(n syntax.Node) *ir.LetBinding {
	ns := inner(n, syntax.KindLetBinding)
	defer ns.finish()
	ns.take(syntax.KindLet)
	if ns.at(syntax.KindClosure) {
		closure := a.analyzeClosure(ns.node())
		return &ir.LetBinding{
			Pattern: &ir.Ident{Name: closure.Name},
			Value:   closure,
		}
	}
	pattern := a.analyzeExpr(ns.node())
	var value ir.Expr
	if ns.at(syntax.KindEq) {
		ns.node() // consume eq
		value = a.analyzeExpr(ns.node())
	}
	return &ir.LetBinding{Pattern: pattern, Value: value}
}

func (a *analyzer) analyzeSetRule(n syntax.Node) *ir.SetRule {
	ns := inner(n, syntax.KindSetRule)
	defer ns.finish()
	ns.take(syntax.KindSet)
	target := a.analyzeExpr(ns.node())
	args, _ := a.analyzeArgs(ns.node())
	var condition ir.Expr
	if ns.at(syntax.KindIf) {
		ns.node() // consume if
		condition = a.analyzeExpr(ns.node())
	}
	return &ir.SetRule{Target: target, Args: args, Condition: condition}
}

func (a *analyzer) analyzeShowRule(n syntax.Node) *ir.ShowRule {
	ns := inner(n, syntax.KindShowRule)
	defer ns.finish()
	ns.take(syntax.KindShow)
	var selector ir.Expr
	if !ns.at(syntax.KindColon) {
		selector = a.analyzeExpr(ns.node())
	}
	ns.take(syntax.KindColon)
	transform := a.analyzeExpr(ns.node())
	return &ir.ShowRule{Selector: selector, Transform: transform}
}

func (a *analyzer) analyzeConditional(n syntax.Node) *ir.Conditional {
	ns := inner(n, syntax.KindConditional)
	defer ns.finish()
	ns.take(syntax.KindIf)
	condition := a.analyzeExpr(ns.node())
	then := a.analyzeExpr(ns.node())
	var elseExpr ir.Expr
	if ns.at(syntax.KindElse) {
		ns.node() // consume else
		elseExpr = a.analyzeExpr(ns.node())
	}
	return &ir.Conditional{Condition: condition, Then: then, Else: elseExpr}
}

func (a *analyzer) analyzeWhileLoop(n syntax.Node) *ir.WhileLoop {
	ns := inner(n, syntax.KindWhileLoop)
	defer ns.finish()
	ns.take(syntax.KindWhile)
	condition := a.analyzeExpr(ns.node())
	body := a.analyzeExpr(ns.node())
	return &ir.WhileLoop{Condition: condition, Body: body}
}

func (a *analyzer) analyzeForLoop(n syntax.Node) *ir.ForLoop {
	ns := inner(n, syntax.KindForLoop)
	defer ns.finish()
	ns.take(syntax.KindFor)
	pattern := a.analyzeExpr(ns.node())
	ns.take(syntax.KindIn)
	iterable := a.analyzeExpr(ns.node())
	body := a.analyzeExpr(ns.node())
	return &ir.ForLoop{Pattern: pattern, Iterable: iterable, Body: body}
}

func (a *analyzer) analyzeLoopBreak(n syntax.Node) *ir.LoopBreak {
	ns := inner(n, syntax.KindLoopBreak)
	defer ns.advance()
	ns.take(syntax.KindBreak)
	return &ir.LoopBreak{}
}

func (a *analyzer) analyzeLoopContinue(n syntax.Node) *ir.LoopContinue {
	ns := inner(n, syntax.KindLoopContinue)
	defer ns.finish()
	ns.take(syntax.KindContinue)
	return &ir.LoopContinue{}
}

func (a *analyzer) analyzeFuncReturn(n syntax.Node) *ir.FuncReturn {
	ns := inner(n, syntax.KindFuncReturn)
	defer ns.finish()
	ns.take(syntax.KindReturn)
	var value ir.Expr
	if !ns.done() {
		value = a.analyzeExpr(ns.node())
	}
	return &ir.FuncReturn{Value: value}
}

func (a *analyzer) analyzeContextual(n syntax.Node) *ir.Contextual {
	ns := inner(n, syntax.KindContextual)
	defer ns.finish()
	ns.take(syntax.KindContext)
	body := a.analyzeExpr(ns.node())
	return &ir.Contextual{Body: body}
}

func (a *analyzer) analyzeModuleInclude(n syntax.Node) *ir.ModuleInclude {
	ns := inner(n, syntax.KindModuleInclude)
	defer ns.finish()
	ns.take(syntax.KindInclude)
	source := a.analyzeExpr(ns.node())
	return &ir.ModuleInclude{Source: source}
}

func (a *analyzer) analyzeDestructAssignment(n syntax.Node) *ir.DestructAssignment {
	ns := inner(n, syntax.KindDestructAssignment)
	defer ns.finish()
	pattern := a.analyzeExpr(ns.node())
	ns.take(syntax.KindEq)
	value := a.analyzeExpr(ns.node())
	return &ir.DestructAssignment{Pattern: pattern, Value: value}
}

func (a *analyzer) analyzeDestructuring(n syntax.Node) *ir.Destructuring {
	ns := inner(n, syntax.KindDestructuring)
	defer ns.finish()
	var items []ir.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		if child.Kind == syntax.KindComma {
			continue
		}
		items = append(items, a.analyzeExpr(child))
	}
	return &ir.Destructuring{Items: items}
}
