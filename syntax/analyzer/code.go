package analyzer

import (
	"strconv"
	"strings"
	"unique"

	"znkr.io/writst/model/ir"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/convert"
)

func (a *analyzer) analyzeBool(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindBool)
	ret := bools[val]
	if ret == nil {
		panic("invalid bool literal: " + val)
	}
	return ret
}

func (a *analyzer) analyzeInt(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindInt)
	iv, err := convert.ParseInt(val)
	if err != nil {
		panic(err.Error())
	}
	return &ir.Const{Value: ir.Int(iv)}
}

func (a *analyzer) analyzeFloat(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindFloat)
	fv, err := strconv.ParseFloat(val, 64)
	if err != nil {
		panic("invalid float literal: " + val)
	}
	return &ir.Const{Value: ir.Float(fv)}
}

func (a *analyzer) analyzeNumeric(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindNumeric)
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
	return &ir.Const{Value: ir.Numeric{Value: fv, Unit: u}}
}

func (a *analyzer) analyzeStr(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindStr)
	return &ir.Const{Value: ir.String(unquote(val))}
}

func (a *analyzer) analyzeIdent(n syntax.Node) *ir.Ident {
	name := unique.Make(a.leaf(n, syntax.KindIdent))
	return &ir.Ident{Name: name}
}

func (a *analyzer) analyzeCode(n syntax.Node) []ir.Expr {
	var exprs []ir.Expr
	for child := range a.inner(n, syntax.KindCode).all() {
		if child.Kind() == syntax.KindSemicolon {
			continue
		}
		exprs = append(exprs, a.analyzeExpr(child))
	}
	return exprs
}

func (a *analyzer) analyzeCodeBlock(n syntax.Node) *ir.CodeBlock {
	ns := a.inner(n, syntax.KindCodeBlock)
	defer ns.finish()
	var exprs []ir.Expr
	for child := range ns.inside(syntax.KindLeftBrace, syntax.KindRightBrace) {
		switch child.Kind() {
		case syntax.KindCode:
			exprs = append(exprs, a.analyzeCode(child)...)
		default:
			exprs = append(exprs, a.analyzeExpr(child))
		}
	}
	return &ir.CodeBlock{Body: exprs}
}

func (a *analyzer) analyzeCodeContentBlock(n syntax.Node) *ir.ContentBlock {
	return &ir.ContentBlock{Body: a.analyzeContentBlock(n)}
}

func (a *analyzer) analyzeParenthesized(n syntax.Node) *ir.Parenthesized {
	ns := a.inner(n, syntax.KindParenthesized)
	defer ns.finish()
	ns.take(syntax.KindLeftParen)
	n0 := ns.node()
	ns.take(syntax.KindRightParen)
	return &ir.Parenthesized{Body: a.analyzeExpr(n0)}
}

func (a *analyzer) analyzeArray(n syntax.Node) *ir.ArrayExpr {
	ns := a.inner(n, syntax.KindArray)
	defer ns.finish()
	var items []ir.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		if child.Kind() == syntax.KindComma {
			continue
		}
		items = append(items, a.analyzeExpr(child))
	}
	return &ir.ArrayExpr{Elements: items}
}

func (a *analyzer) analyzeDict(n syntax.Node) *ir.DictExpr {
	ns := a.inner(n, syntax.KindDict)
	defer ns.finish()
	var entries []ir.DictItemExpr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma, syntax.KindColon:
			continue
		case syntax.KindNamed:
			entry := a.inner(child, syntax.KindNamed)
			key := entry.take(syntax.KindIdent)
			entry.take(syntax.KindColon)
			value := a.analyzeExpr(entry.node())
			entry.finish()
			entries = append(entries, ir.DictItemExpr{
				Key:   &ir.Const{Value: ir.String(key)},
				Value: value,
			})
		case syntax.KindKeyed:
			entry := a.inner(child, syntax.KindKeyed)
			key := a.analyzeExpr(entry.node())
			entry.take(syntax.KindColon)
			value := a.analyzeExpr(entry.node())
			entry.finish()
			entries = append(entries, ir.DictItemExpr{
				Key:   key,
				Value: value,
			})
		default:
			panic("invalid dict entry: " + child.Kind().String())
		}
	}
	return &ir.DictExpr{Entries: entries}
}

func (a *analyzer) analyzeUnary(n syntax.Node) *ir.Unary {
	ns := a.inner(n, syntax.KindUnary)
	defer ns.finish()
	op := syntax.UnaryOpFromKind(ns.node().Kind())
	operand := a.analyzeExpr(ns.node())
	return &ir.Unary{Op: op, Operand: operand}
}

func (a *analyzer) analyzeBinary(n syntax.Node) *ir.Binary {
	ns := a.inner(n, syntax.KindBinary)
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
		op = syntax.BinaryOpFromKind(ns.node().Kind())
	}
	right := a.analyzeExpr(ns.node())
	return &ir.Binary{Left: left, Op: op, Right: right}
}

func (a *analyzer) analyzeFieldAccess(n syntax.Node) *ir.FieldAccess {
	ns := a.inner(n, syntax.KindFieldAccess)
	defer ns.finish()
	target := a.analyzeExpr(ns.node())
	ns.take(syntax.KindDot)
	field := ns.take(syntax.KindIdent)
	return &ir.FieldAccess{Target: target, Field: field}
}

func (a *analyzer) analyzeFuncCall(n syntax.Node) *ir.FuncCall {
	ns := a.inner(n, syntax.KindFuncCall)
	defer ns.finish()
	callee := a.analyzeExpr(ns.node())
	args, content := a.analyzeArgs(ns.node())
	return &ir.FuncCall{Callee: callee, Args: args, Content: content}
}

func (a *analyzer) analyzeArgs(n syntax.Node) ([]ir.Arg, []ir.ContentExpr) {
	ns := a.inner(n, syntax.KindArgs)
	defer ns.finish()

	var args []ir.Arg
	if ns.at(syntax.KindLeftParen) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindSpread:
				ns := a.inner(child, syntax.KindSpread)
				defer ns.finish()
				ns.take(syntax.KindDots)
				expr := a.analyzeExpr(ns.node())
				args = append(args, &ir.SpreadArg{Expr: expr})
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				key := unique.Make(named.take(syntax.KindIdent))
				named.take(syntax.KindColon)
				value := a.analyzeExpr(named.node())
				named.finish()
				args = append(args, &ir.NamedArg{Name: key, Expr: value})
			default:
				args = append(args, &ir.ExprArg{Expr: a.analyzeExpr(child)})
			}
		}
	}

	var content []ir.ContentExpr
	for ns.at(syntax.KindContentBlock) {
		content = append(content, a.analyzeContentBlock(ns.node()))
	}
	return args, content
}

func (a *analyzer) analyzeClosure(n syntax.Node) *ir.Closure {
	ns := a.inner(n, syntax.KindClosure)
	defer ns.finish()

	// Handle different closure forms:
	// - Named function: name(params) = body
	// - Anonymous with parens: (params) => body
	// - Anonymous single param: param => body
	var name *ir.Ident
	var params []ir.Param
	switch n := ns.node(); n.Kind() {
	case syntax.KindIdent:
		if ns.at(syntax.KindParams) {
			// Named function: name(params) = body
			name = a.analyzeIdent(n)
			params = a.analyzeParams(ns.node())
		} else {
			// Single param: param => body
			params = []ir.Param{&ir.PositionalParam{Ident: a.analyzeIdent(n)}}
		}
	case syntax.KindUnderscore:
		// _ => body
		params = []ir.Param{&ir.PositionalParam{Ident: underscore}}
	case syntax.KindParams:
		params = a.analyzeParams(n)
	default:
		panic("invalid closure syntax: " + n.Kind().String())
	}

	// Skip arrow or eq
	if ns.at(syntax.KindArrow) || ns.at(syntax.KindEq) {
		ns.node()
	}
	body := a.analyzeExpr(ns.node())
	return &ir.Closure{Name: name, Params: params, Body: body}
}

func (a *analyzer) analyzeParams(n syntax.Node) []ir.Param {
	ns := a.inner(n, syntax.KindParams)
	defer ns.finish()
	var params []ir.Param
	if !ns.at(syntax.KindLeftParen) {
		// Single param without parens
		for child := range ns.all() {
			switch child.Kind() {
			case syntax.KindUnderscore:
				params = append(params, &ir.PositionalParam{Ident: underscore})
			default:
				params = append(params, &ir.PositionalParam{Ident: a.analyzeIdent(child)})
			}
		}

	} else {
		hasSink := false
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindIdent:
				params = append(params, &ir.PositionalParam{Ident: a.analyzeIdent(child)})
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				name := unique.Make(named.take(syntax.KindIdent))
				named.take(syntax.KindColon)
				defaultExpr := a.analyzeExpr(named.node())
				named.finish()
				params = append(params, &ir.NamedParam{Name: name, Default: defaultExpr})
			case syntax.KindSpread:
				if hasSink {
					panic("only one sink parameter allowed")
				}
				hasSink = true
				ns := a.inner(child, syntax.KindSpread)
				defer ns.finish()
				ns.take(syntax.KindDots)
				ident := a.analyzeIdent(ns.node())
				params = append(params, &ir.SpreadParam{Ident: ident})
			default:
				panic("invalid parameter: " + child.Kind().String())
			}
		}
	}
	return params
}

func (a *analyzer) analyzeLetBinding(n syntax.Node) *ir.LetBinding {
	ns := a.inner(n, syntax.KindLetBinding)
	defer ns.finish()
	ns.take(syntax.KindLet)
	if ns.at(syntax.KindClosure) {
		closure := a.analyzeClosure(ns.node())
		return &ir.LetBinding{
			Pattern: []ir.DestructPattern{&ir.DestructIdent{Ident: closure.Name}},
			Value:   closure,
		}
	}
	pattern := a.unpackDestructuringPattern(ns.node())
	var value ir.Expr
	if ns.at(syntax.KindEq) {
		ns.node() // consume eq
		value = a.analyzeExpr(ns.node())
	}
	return &ir.LetBinding{Pattern: pattern, Value: value}
}

func (a *analyzer) analyzeSetRule(n syntax.Node) *ir.SetRule {
	ns := a.inner(n, syntax.KindSetRule)
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
	ns := a.inner(n, syntax.KindShowRule)
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
	ns := a.inner(n, syntax.KindConditional)
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
	ns := a.inner(n, syntax.KindWhileLoop)
	defer ns.finish()
	ns.take(syntax.KindWhile)
	condition := a.analyzeExpr(ns.node())
	body := a.analyzeExpr(ns.node())
	return &ir.WhileLoop{Condition: condition, Body: body}
}

func (a *analyzer) analyzeForLoop(n syntax.Node) *ir.ForLoop {
	ns := a.inner(n, syntax.KindForLoop)
	defer ns.finish()
	ns.take(syntax.KindFor)
	pattern := a.unpackDestructuringPattern(ns.node())
	ns.take(syntax.KindIn)
	iterable := a.analyzeExpr(ns.node())
	body := a.analyzeExpr(ns.node())
	return &ir.ForLoop{Pattern: pattern, Iterable: iterable, Body: body}
}

func (a *analyzer) analyzeLoopBreak(n syntax.Node) *ir.LoopBreak {
	ns := a.inner(n, syntax.KindLoopBreak)
	defer ns.advance()
	ns.take(syntax.KindBreak)
	return &ir.LoopBreak{}
}

func (a *analyzer) analyzeLoopContinue(n syntax.Node) *ir.LoopContinue {
	ns := a.inner(n, syntax.KindLoopContinue)
	defer ns.finish()
	ns.take(syntax.KindContinue)
	return &ir.LoopContinue{}
}

func (a *analyzer) analyzeFuncReturn(n syntax.Node) *ir.FuncReturn {
	ns := a.inner(n, syntax.KindFuncReturn)
	defer ns.finish()
	ns.take(syntax.KindReturn)
	var value ir.Expr
	if !ns.done() {
		value = a.analyzeExpr(ns.node())
	}
	return &ir.FuncReturn{Value: value}
}

func (a *analyzer) analyzeContextual(n syntax.Node) *ir.Contextual {
	ns := a.inner(n, syntax.KindContextual)
	defer ns.finish()
	ns.take(syntax.KindContext)
	body := a.analyzeExpr(ns.node())
	return &ir.Contextual{Body: body}
}

func (a *analyzer) analyzeModuleInclude(n syntax.Node) *ir.ModuleInclude {
	ns := a.inner(n, syntax.KindModuleInclude)
	defer ns.finish()
	ns.take(syntax.KindInclude)
	source := a.analyzeExpr(ns.node())
	return &ir.ModuleInclude{Source: source}
}

func (a *analyzer) analyzeDestructAssignment(n syntax.Node) *ir.DestructAssignment {
	ns := a.inner(n, syntax.KindDestructAssignment)
	defer ns.finish()
	pattern := a.unpackDestructuringPattern(ns.node())
	ns.take(syntax.KindEq)
	value := a.analyzeExpr(ns.node())
	return &ir.DestructAssignment{Pattern: pattern, Value: value}
}

func (a *analyzer) unpackDestructuringPattern(n syntax.Node) []ir.DestructPattern {
	switch n.Kind() {
	case syntax.KindIdent:
		return []ir.DestructPattern{&ir.DestructIdent{Ident: a.analyzeIdent(n)}}
	case syntax.KindUnderscore:
		return []ir.DestructPattern{&ir.DestructIdent{Ident: underscore}}
	default:
		// continue below
	}

	ns := a.inner(n, syntax.KindDestructuring)
	var pattern []ir.DestructPattern
	haveSink := false
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma:
			continue
		case syntax.KindUnderscore:
			pattern = append(pattern, &ir.DestructIdent{Ident: underscore})
		case syntax.KindIdent:
			pattern = append(pattern, &ir.DestructIdent{Ident: a.analyzeIdent(child)})
		case syntax.KindNamed:
			named := a.inner(child, syntax.KindNamed)
			name := unique.Make(named.take(syntax.KindIdent))
			named.take(syntax.KindColon)
			patternIdent := a.analyzeIdent(named.node())
			named.finish()
			pattern = append(pattern, &ir.DestructNamed{Name: name, Pattern: patternIdent})
		case syntax.KindSpread:
			if haveSink {
				panic("only one destruct sink allowed in destruct pattern")
			}
			haveSink = true
			ns := a.inner(child, syntax.KindSpread)
			defer ns.finish()
			ns.take(syntax.KindDots)
			ident := a.analyzeIdent(ns.node())
			pattern = append(pattern, &ir.DestructSink{Ident: ident})
		default:
			panic("invalid destruct pattern: " + child.Kind().String())
		}
	}
	return pattern
}
