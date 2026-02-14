package analyzer

import (
	"strconv"
	"strings"
	"unique"

	"znkr.io/writst/ir"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/convert"
)

func (a *analyzer) analyzeBool(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindBool)
	v, ok := bools[val]
	if !ok {
		panic("invalid bool literal: " + val)
	}
	return ir.NewConst(n.Span(), v)
}

func (a *analyzer) analyzeInt(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindInt)
	iv, err := convert.ParseInt(val)
	if err != nil {
		panic(err.Error())
	}
	return ir.NewConst(n.Span(), ir.Int(iv))
}

func (a *analyzer) analyzeFloat(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindFloat)
	fv, err := strconv.ParseFloat(val, 64)
	if err != nil {
		panic("invalid float literal: " + val)
	}
	return ir.NewConst(n.Span(), ir.Float(fv))
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
	return ir.NewConst(n.Span(), ir.Numeric{Value: fv, Unit: u})
}

func (a *analyzer) analyzeStr(n syntax.Node) *ir.Const {
	val := a.leaf(n, syntax.KindStr)
	return ir.NewConst(n.Span(), ir.String(unquote(val)))
}

func (a *analyzer) analyzeIdent(n syntax.Node) *ir.Ident {
	name := unique.Make(a.leaf(n, syntax.KindIdent))
	return ir.NewIdent(n.Span(), name)
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
	return ir.NewCodeBlock(n.Span(), exprs)
}

func (a *analyzer) analyzeParenthesized(n syntax.Node) *ir.Parenthesized {
	ns := a.inner(n, syntax.KindParenthesized)
	defer ns.finish()
	ns.take(syntax.KindLeftParen)
	n0 := ns.node()
	ns.take(syntax.KindRightParen)
	return ir.NewParenthesized(n.Span(), a.analyzeExpr(n0))
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
	return ir.NewArrayExpr(n.Span(), items)
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
			entries = append(entries, ir.NewDictItemExpr(
				ir.NewConst(child.Span(), ir.String(key)),
				value,
			))
		case syntax.KindKeyed:
			entry := a.inner(child, syntax.KindKeyed)
			key := a.analyzeExpr(entry.node())
			entry.take(syntax.KindColon)
			value := a.analyzeExpr(entry.node())
			entry.finish()
			entries = append(entries, ir.NewDictItemExpr(
				key,
				value,
			))
		default:
			panic("invalid dict entry: " + child.Kind().String())
		}
	}
	return ir.NewDictExpr(n.Span(), entries)
}

func (a *analyzer) analyzeUnary(n syntax.Node) *ir.Unary {
	ns := a.inner(n, syntax.KindUnary)
	defer ns.finish()
	op := syntax.UnaryOpFromKind(ns.node().Kind())
	operand := a.analyzeExpr(ns.node())
	return ir.NewUnary(n.Span(), op, operand)
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
	return ir.NewBinary(n.Span(), left, op, right)
}

func (a *analyzer) analyzeFieldAccess(n syntax.Node) *ir.FieldAccess {
	ns := a.inner(n, syntax.KindFieldAccess)
	defer ns.finish()
	target := a.analyzeExpr(ns.node())
	ns.take(syntax.KindDot)
	field := ns.take(syntax.KindIdent)
	return ir.NewFieldAccess(n.Span(), target, unique.Make(field))
}

func (a *analyzer) analyzeFuncCall(n syntax.Node) *ir.FuncCall {
	ns := a.inner(n, syntax.KindFuncCall)
	defer ns.finish()
	callee := a.analyzeExpr(ns.node())
	args, content := a.analyzeArgs(ns.node())
	return ir.NewFuncCall(n.Span(), callee, args, content)
}

func (a *analyzer) analyzeArgs(n syntax.Node) ([]ir.Arg, []*ir.ContentBlock) {
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
				args = append(args, ir.NewSpreadArg(expr))
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				key := unique.Make(named.take(syntax.KindIdent))
				named.take(syntax.KindColon)
				value := a.analyzeExpr(named.node())
				named.finish()
				args = append(args, ir.NewNamedArg(key, value))
			default:
				args = append(args, ir.NewExprArg(a.analyzeExpr(child)))
			}
		}
	}

	var content []*ir.ContentBlock
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
			params = []ir.Param{ir.NewPositionalParam(a.analyzeIdent(n))}
		}
	case syntax.KindUnderscore:
		// _ => body
		params = []ir.Param{ir.NewPositionalParam(ir.NewIdent(n.Span(), underscore))}
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
	return ir.NewClosure(n.Span(), name, params, body)
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
				params = append(params, ir.NewPositionalParam(ir.NewIdent(child.Span(), underscore)))
			default:
				params = append(params, ir.NewPositionalParam(a.analyzeIdent(child)))
			}
		}

	} else {
		hasSink := false
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindIdent:
				params = append(params, ir.NewPositionalParam(a.analyzeIdent(child)))
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				name := unique.Make(named.take(syntax.KindIdent))
				named.take(syntax.KindColon)
				defaultExpr := a.analyzeExpr(named.node())
				named.finish()
				params = append(params, ir.NewNamedParam(name, defaultExpr))
			case syntax.KindSpread:
				if hasSink {
					panic("only one sink parameter allowed")
				}
				hasSink = true
				ns := a.inner(child, syntax.KindSpread)
				defer ns.finish()
				ns.take(syntax.KindDots)
				ident := a.analyzeIdent(ns.node())
				params = append(params, ir.NewSpreadParam(ident))
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
		return ir.NewLetBinding(
			n.Span(),
			[]ir.DestructPattern{ir.NewDestructIdent(closure.Name())},
			closure,
		)
	}
	pattern := a.unpackDestructuringPattern(ns.node())
	var value ir.Expr
	if ns.at(syntax.KindEq) {
		ns.node() // consume eq
		value = a.analyzeExpr(ns.node())
	}
	return ir.NewLetBinding(n.Span(), pattern, value)
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
	return ir.NewSetRule(n.Span(), target, args, condition)
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
	return ir.NewShowRule(n.Span(), selector, transform)
}

func (a *analyzer) analyzeConditional(n syntax.Node) *ir.Conditional {
	var conditions []ir.Expr
	var blocks []*ir.CodeBlock
	var def *ir.CodeBlock

	var analyze func(n syntax.Node)
	analyze = func(n syntax.Node) {
		ns := a.inner(n, syntax.KindConditional)
		defer ns.finish()
		ns.take(syntax.KindIf)
		conditions = append(conditions, a.analyzeExpr(ns.node()))
		blocks = append(blocks, a.analyzeCodeBlock(ns.node()))
		if !ns.at(syntax.KindElse) {
			return
		}
		ns.node() // consume else
		if ns.at(syntax.KindConditional) {
			analyze(ns.node())
		} else {
			def = a.analyzeCodeBlock(ns.node())
		}
	}
	analyze(n)

	return ir.NewConditional(n.Span(), conditions, blocks, def)
}

func (a *analyzer) analyzeWhileLoop(n syntax.Node) *ir.WhileLoop {
	ns := a.inner(n, syntax.KindWhileLoop)
	defer ns.finish()
	ns.take(syntax.KindWhile)
	condition := a.analyzeExpr(ns.node())
	body := a.analyzeCodeBlock(ns.node())
	return ir.NewWhileLoop(n.Span(), condition, body)
}

func (a *analyzer) analyzeForLoop(n syntax.Node) *ir.ForLoop {
	ns := a.inner(n, syntax.KindForLoop)
	defer ns.finish()
	ns.take(syntax.KindFor)
	pattern := a.unpackDestructuringPattern(ns.node())
	ns.take(syntax.KindIn)
	iterable := a.analyzeExpr(ns.node())
	body := a.analyzeCodeBlock(ns.node())
	return ir.NewForLoop(n.Span(), pattern, iterable, body)
}

func (a *analyzer) analyzeLoopBreak(n syntax.Node) *ir.LoopBreak {
	ns := a.inner(n, syntax.KindLoopBreak)
	defer ns.advance()
	ns.take(syntax.KindBreak)
	return ir.NewLoopBreak(n.Span())
}

func (a *analyzer) analyzeLoopContinue(n syntax.Node) *ir.LoopContinue {
	ns := a.inner(n, syntax.KindLoopContinue)
	defer ns.finish()
	ns.take(syntax.KindContinue)
	return ir.NewLoopContinue(n.Span())
}

func (a *analyzer) analyzeFuncReturn(n syntax.Node) *ir.FuncReturn {
	ns := a.inner(n, syntax.KindFuncReturn)
	defer ns.finish()
	ns.take(syntax.KindReturn)
	var value ir.Expr
	if !ns.done() {
		value = a.analyzeExpr(ns.node())
	}
	return ir.NewFuncReturn(n.Span(), value)
}

func (a *analyzer) analyzeContextual(n syntax.Node) *ir.Contextual {
	ns := a.inner(n, syntax.KindContextual)
	defer ns.finish()
	ns.take(syntax.KindContext)
	body := a.analyzeExpr(ns.node())
	return ir.NewContextual(n.Span(), body)
}

func (a *analyzer) analyzeModuleInclude(n syntax.Node) *ir.ModuleInclude {
	ns := a.inner(n, syntax.KindModuleInclude)
	defer ns.finish()
	ns.take(syntax.KindInclude)
	source := a.analyzeExpr(ns.node())
	return ir.NewModuleInclude(n.Span(), source)
}

func (a *analyzer) analyzeDestructAssignment(n syntax.Node) *ir.DestructAssignment {
	ns := a.inner(n, syntax.KindDestructAssignment)
	defer ns.finish()
	pattern := a.unpackDestructuringPattern(ns.node())
	ns.take(syntax.KindEq)
	value := a.analyzeExpr(ns.node())
	return ir.NewDestructAssignment(n.Span(), pattern, value)
}

func (a *analyzer) unpackDestructuringPattern(n syntax.Node) []ir.DestructPattern {
	switch n.Kind() {
	case syntax.KindIdent:
		return []ir.DestructPattern{ir.NewDestructIdent(a.analyzeIdent(n))}
	case syntax.KindUnderscore:
		return []ir.DestructPattern{ir.NewDestructIdent(ir.NewIdent(n.Span(), underscore))}
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
			pattern = append(pattern, ir.NewDestructIdent(ir.NewIdent(child.Span(), underscore)))
		case syntax.KindIdent:
			pattern = append(pattern, ir.NewDestructIdent(a.analyzeIdent(child)))
		case syntax.KindNamed:
			named := a.inner(child, syntax.KindNamed)
			name := unique.Make(named.take(syntax.KindIdent))
			named.take(syntax.KindColon)
			patternIdent := a.analyzeIdent(named.node())
			named.finish()
			pattern = append(pattern, ir.NewDestructNamed(name, patternIdent))
		case syntax.KindSpread:
			if haveSink {
				panic("only one destruct sink allowed in destruct pattern")
			}
			haveSink = true
			ns := a.inner(child, syntax.KindSpread)
			defer ns.finish()
			ns.take(syntax.KindDots)
			ident := a.analyzeIdent(ns.node())
			pattern = append(pattern, ir.NewDestructSink(ident))
		default:
			panic("invalid destruct pattern: " + child.Kind().String())
		}
	}
	return pattern
}
