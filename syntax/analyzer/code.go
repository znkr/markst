package analyzer

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"unique"

	"znkr.io/writst/expr"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/convert"
	"znkr.io/writst/value"
)

func (a *analyzer) analyzeBool(n syntax.Node) *expr.ConstExpr {
	val := a.leaf(n, syntax.KindBool)
	v, ok := bools[val]
	if !ok {
		panic("invalid bool literal: " + val)
	}
	return expr.NewConstExpr(n.Span(), v)
}

func (a *analyzer) analyzeInt(n syntax.Node) *expr.ConstExpr {
	val := a.leaf(n, syntax.KindInt)
	iv, err := convert.ParseInt(val)
	if err != nil {
		panic(err.Error())
	}
	return expr.NewConstExpr(n.Span(), value.Int(iv))
}

func (a *analyzer) analyzeFloat(n syntax.Node) *expr.ConstExpr {
	val := a.leaf(n, syntax.KindFloat)
	fv, err := strconv.ParseFloat(val, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic("invalid float literal: " + val)
	}
	return expr.NewConstExpr(n.Span(), value.Float(fv))
}

func (a *analyzer) analyzeNumeric(n syntax.Node) *expr.ConstExpr {
	val := a.leaf(n, syntax.KindNumeric)
	// Find where the numeric part ends and unit begins
	idx := strings.IndexFunc(val, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if idx == -1 {
		panic("invalid numeric literal, no unit: " + val)
	}
	num, suffix := val[:idx], val[idx:]
	fv, err := strconv.ParseFloat(num, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic("invalid float literal: " + val)
	}
	var v value.Value
	switch suffix {
	case "pt":
		v = value.Length{Pt: fv}
	case "mm":
		v = value.Length{Pt: fv * (72.0 / 25.4)}
	case "cm":
		v = value.Length{Pt: fv * (72.0 / 2.54)}
	case "in":
		v = value.Length{Pt: fv * 72.0}
	case "em":
		v = value.Length{Em: fv}
	case "deg":
		v = value.Angle(fv / 180.0 * math.Pi)
	case "rad":
		v = value.Angle(fv)
	case "fr":
		v = value.Fraction(fv)
	case "%":
		v = value.Ratio(fv / 100.0)
	default:
		panic("invalid unit literal: " + val)
	}
	return expr.NewConstExpr(n.Span(), v)
}

func (a *analyzer) analyzeStr(n syntax.Node) *expr.ConstExpr {
	val := a.leaf(n, syntax.KindStr)
	return expr.NewConstExpr(n.Span(), value.Str(unquote(val)))
}

func (a *analyzer) analyzeIdent(n syntax.Node) *expr.Ident {
	name := unique.Make(a.leaf(n, syntax.KindIdent))
	return expr.NewIdent(n.Span(), name)
}

func (a *analyzer) analyzeCode(n syntax.Node) []expr.Expr {
	var exprs []expr.Expr
	for child := range a.inner(n, syntax.KindCode).all() {
		switch child.Kind() {
		case syntax.KindSemicolon:
			continue
		case syntax.KindError:
			a.error(child.(*syntax.Error))
			continue
		default:
			exprs = append(exprs, a.analyzeExpr(child))
		}
	}
	return exprs
}

func (a *analyzer) analyzeCodeBlock(n syntax.Node) *expr.CodeBlock {
	ns := a.inner(n, syntax.KindCodeBlock)
	defer ns.finish()
	var exprs []expr.Expr
	for child := range ns.inside(syntax.KindLeftBrace, syntax.KindRightBrace) {
		switch child.Kind() {
		case syntax.KindCode:
			exprs = append(exprs, a.analyzeCode(child)...)
		case syntax.KindError:
			a.error(child.(*syntax.Error))
			continue
		default:
			exprs = append(exprs, a.analyzeExpr(child))
		}
	}
	return expr.NewCodeBlock(n.Span(), exprs)
}

func (a *analyzer) analyzeParenthesized(n syntax.Node) *expr.Parenthesized {
	ns := a.inner(n, syntax.KindParenthesized)
	defer ns.finish()
	var x expr.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		x = a.analyzeExpr(child)
	}
	return expr.NewParenthesized(n.Span(), x)
}

func (a *analyzer) analyzeArray(n syntax.Node) *expr.ArrayExpr {
	ns := a.inner(n, syntax.KindArray)
	defer ns.finish()
	var items []expr.Expr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch kind := child.Kind(); kind {
		case syntax.KindComma:
		case syntax.KindSpread:
			entry := a.inner(child, syntax.KindSpread)
			entry.take(syntax.KindDots)
			x := a.analyzeExpr(entry.node())
			entry.finish()
			items = append(items, expr.NewSpreadExpr(child.Span(), x))
		case syntax.KindNamed, syntax.KindKeyed:
			a.error(syntax.NewError(child.Span(), "expected expression, found "+kind.Name()+" pair", child.Text()))
		default:
			items = append(items, a.analyzeExpr(child))
		}
	}
	return expr.NewArrayExpr(n.Span(), items)
}

func (a *analyzer) analyzeDict(n syntax.Node) *expr.DictExpr {
	ns := a.inner(n, syntax.KindDict)
	defer ns.finish()
	var entries []expr.DictItemExpr
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma, syntax.KindColon:
			continue
		case syntax.KindNamed:
			entry := a.inner(child, syntax.KindNamed)
			key := entry.take(syntax.KindIdent)
			entry.take(syntax.KindColon)
			val := a.analyzeExpr(entry.node())
			entry.finish()
			entries = append(entries, expr.NewDictItemExpr(
				expr.NewConstExpr(child.Span(), value.Str(key)),
				val,
			))
		case syntax.KindKeyed:
			entry := a.inner(child, syntax.KindKeyed)
			key := a.analyzeExpr(entry.node())
			entry.take(syntax.KindColon)
			value := a.analyzeExpr(entry.node())
			entry.finish()
			entries = append(entries, expr.NewDictItemExpr(
				key,
				value,
			))
		case syntax.KindError:
			a.error(child.(*syntax.Error))
		default:
			a.error(syntax.NewError(child.Span(), "expected named or keyed pair", child.Text()))
		}
	}
	return expr.NewDictExpr(n.Span(), entries)
}

func (a *analyzer) analyzeUnary(n syntax.Node) *expr.Unary {
	ns := a.inner(n, syntax.KindUnary)
	defer ns.finish()
	op := syntax.UnaryOpFromKind(ns.node().Kind())
	operand := a.analyzeExpr(ns.node())
	return expr.NewUnary(n.Span(), op, operand)
}

func (a *analyzer) analyzeBinary(n syntax.Node) *expr.Binary {
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
	return expr.NewBinary(n.Span(), left, op, right)
}

func (a *analyzer) analyzeFieldAccess(n syntax.Node) *expr.FieldAccess {
	ns := a.inner(n, syntax.KindFieldAccess)
	defer ns.finish()
	target := a.analyzeExpr(ns.node())
	ns.take(syntax.KindDot)
	field := a.analyzeIdent(ns.node())
	return expr.NewFieldAccess(n.Span(), target, field)
}

func (a *analyzer) analyzeFuncCall(n syntax.Node) *expr.FuncCall {
	ns := a.inner(n, syntax.KindFuncCall)
	defer ns.finish()
	callee := a.analyzeExpr(ns.node())
	args, content := a.analyzeArgs(ns.node())
	return expr.NewFuncCall(n.Span(), callee, args, content)
}

func (a *analyzer) analyzeArgs(n syntax.Node) ([]expr.Arg, []*expr.ContentBlock) {
	ns := a.inner(n, syntax.KindArgs)
	defer ns.finish()

	var args []expr.Arg
	if ns.at(syntax.KindLeftParen) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindSpread:
				ns := a.inner(child, syntax.KindSpread)
				defer ns.finish()
				ns.take(syntax.KindDots)
				x := a.analyzeExpr(ns.node())
				args = append(args, expr.NewSpreadArg(x))
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				key := unique.Make(named.take(syntax.KindIdent))
				named.take(syntax.KindColon)
				value := a.analyzeExpr(named.node())
				named.finish()
				args = append(args, expr.NewNamedArg(child.Span(), key, value))
			default:
				args = append(args, expr.NewExprArg(a.analyzeExpr(child)))
			}
		}
	}

	var content []*expr.ContentBlock
	for ns.at(syntax.KindContentBlock) {
		content = append(content, a.analyzeContentBlock(ns.node()))
	}
	return args, content
}

func (a *analyzer) analyzeClosure(n syntax.Node) *expr.Closure {
	ns := a.inner(n, syntax.KindClosure)
	defer ns.finish()

	// Handle different closure forms:
	// - Named function: name(params) = body
	// - Anonymous with parens: (params) => body
	// - Anonymous single param: param => body
	var name *expr.Ident
	var params []expr.ClosureParam
	switch n := ns.node(); n.Kind() {
	case syntax.KindIdent:
		if ns.at(syntax.KindParams) {
			// Named function: name(params) = body
			name = a.analyzeIdent(n)
			params = a.analyzeClosureParams(ns.node())
		} else {
			// Single param: param => body
			params = []expr.ClosureParam{expr.NewPositionalClosureParam(a.analyzeIdent(n))}
		}
	case syntax.KindUnderscore:
		// _ => body
		params = []expr.ClosureParam{expr.NewPositionalClosureParam(expr.NewIdent(n.Span(), underscore))}
	case syntax.KindParams:
		params = a.analyzeClosureParams(n)
	case syntax.KindDestructuring:
		// Unsupported (pattern)(params) => body
		a.unexpected(syntax.NewError(n.Span(), "expected identifier or parameters", ""))
	default:
		panic("invalid closure syntax: " + n.Kind().String())
	}

	// Skip arrow or eq
	if ns.at(syntax.KindArrow) || ns.at(syntax.KindEq) {
		ns.node()
	}
	if ns.done() {
		a.expected(ns, "expression")
	}
	body := a.analyzeExpr(ns.node())

	// Compute captured variables: collect the parameter names as the initial
	// bound set, then find all free variables in the body.
	bound := make(map[unique.Handle[string]]bool)
	if name != nil {
		bound[name.Name()] = true
	}
	for _, p := range params {
		switch p := p.(type) {
		case *expr.PositionalClosureParam:
			bound[p.Name().Name()] = true
		case *expr.NamedClosureParam:
			bound[p.Name().Name()] = true
		case *expr.SpreadClosureParam:
			bound[p.Ident().Name()] = true
		}
	}
	captures := expr.FreeVars(body, bound)

	return expr.NewClosure(n.Span(), name, params, body, captures)
}

func (a *analyzer) analyzeClosureParams(n syntax.Node) []expr.ClosureParam {
	ns := a.inner(n, syntax.KindParams)
	defer ns.finish()
	var params []expr.ClosureParam
	names := make(map[unique.Handle[string]]bool)
	addName := func(ident *expr.Ident) {
		name := ident.Name()
		span := ident.Span()
		if names[name] {
			a.error(syntax.NewError(span, "duplicate parameter: "+name.Value(), name.Value()))
		}
		names[name] = true
	}
	if !ns.at(syntax.KindLeftParen) {
		// Single param without parens
		for child := range ns.all() {
			switch child.Kind() {
			case syntax.KindUnderscore:
				params = append(params, expr.NewPositionalClosureParam(expr.NewIdent(child.Span(), underscore)))
			default:
				ident := a.analyzeIdent(child)
				addName(ident)
				params = append(params, expr.NewPositionalClosureParam(ident))
			}
		}

	} else {
		hasSink := false
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindIdent:
				ident := a.analyzeIdent(child)
				addName(ident)
				params = append(params, expr.NewPositionalClosureParam(ident))
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				name := a.analyzeIdent(named.node())
				addName(name)
				named.take(syntax.KindColon)
				defaultExpr := a.analyzeExpr(named.node())
				named.finish()
				params = append(params, expr.NewNamedClosureParam(name, defaultExpr))
			case syntax.KindSpread:
				if hasSink {
					a.error(syntax.NewError(child.Span(), "only one arguments sink is allowed", child.Text()))
				}
				hasSink = true
				ns := a.inner(child, syntax.KindSpread)
				defer ns.finish()
				ns.take(syntax.KindDots)
				ident := a.analyzeIdent(ns.node())
				addName(ident)
				params = append(params, expr.NewSpreadClosureParam(ident))
			case syntax.KindError:
				a.error(child.(*syntax.Error))
			default:
				panic("invalid parameter: " + child.Kind().String())
			}
		}
	}
	return params
}

func (a *analyzer) analyzeLetBinding(n syntax.Node) *expr.LetBinding {
	ns := a.inner(n, syntax.KindLetBinding)
	defer ns.finish()
	ns.take(syntax.KindLet)
	if ns.at(syntax.KindClosure) {
		closure := a.analyzeClosure(ns.node())
		if closure == nil {
			return nil
		}
		return expr.NewLetBinding(
			n.Span(),
			[]expr.DestructPattern{expr.NewDestructIdent(closure.Name())},
			closure,
		)
	}
	pattern := a.unpackDestructuringPattern(ns.node())
	var value expr.Expr
	if ns.at(syntax.KindEq) {
		ns.node() // consume eq
		value = a.analyzeExpr(ns.node())
	} else if ns.at(syntax.KindError) {
		a.error(ns.node().(*syntax.Error))
	}
	return expr.NewLetBinding(n.Span(), pattern, value)
}

func (a *analyzer) analyzeSetRule(n syntax.Node) *expr.SetRule {
	ns := a.inner(n, syntax.KindSetRule)
	defer ns.finish()
	ns.take(syntax.KindSet)
	target := a.analyzeExpr(ns.node())
	args, _ := a.analyzeArgs(ns.node())
	var condition expr.Expr
	if ns.at(syntax.KindIf) {
		ns.node() // consume if
		condition = a.analyzeExpr(ns.node())
	}
	return expr.NewSetRule(n.Span(), target, args, condition)
}

func (a *analyzer) analyzeShowRule(n syntax.Node) *expr.ShowRule {
	ns := a.inner(n, syntax.KindShowRule)
	defer ns.finish()
	ns.take(syntax.KindShow)
	var selector expr.Expr
	if !ns.at(syntax.KindColon) {
		selector = a.analyzeExpr(ns.node())
	}
	ns.take(syntax.KindColon)
	transform := a.analyzeExpr(ns.node())
	return expr.NewShowRule(n.Span(), selector, transform)
}

func (a *analyzer) analyzeConditional(n syntax.Node) *expr.Conditional {
	var conditions []expr.Expr
	var blocks []expr.Expr
	var def expr.Expr

	var analyze func(n syntax.Node)
	analyze = func(n syntax.Node) {
		ns := a.inner(n, syntax.KindConditional)
		defer ns.finish()
		ns.take(syntax.KindIf)
		conditions = append(conditions, a.analyzeExpr(ns.node()))
		blocks = append(blocks, a.analyzeBlock(ns.node()))
		if !ns.at(syntax.KindElse) {
			return
		}
		ns.node() // consume else
		if ns.at(syntax.KindConditional) {
			analyze(ns.node())
		} else {
			def = a.analyzeBlock(ns.node())
		}
	}
	analyze(n)

	return expr.NewConditional(n.Span(), conditions, blocks, def)
}

func (a *analyzer) analyzeBlock(n syntax.Node) expr.Expr {
	switch n.Kind() {
	case syntax.KindCodeBlock:
		return a.analyzeCodeBlock(n)
	case syntax.KindContentBlock:
		return a.analyzeContentBlock(n)
	default:
		a.unexpected(n)
	}
	panic("never reached")
}

func (a *analyzer) analyzeWhileLoop(n syntax.Node) *expr.WhileLoop {
	ns := a.inner(n, syntax.KindWhileLoop)
	defer ns.finish()
	ns.take(syntax.KindWhile)
	condition := a.analyzeExpr(ns.node())
	body := a.analyzeCodeBlock(ns.node())
	return expr.NewWhileLoop(n.Span(), condition, body)
}

func (a *analyzer) analyzeForLoop(n syntax.Node) *expr.ForLoop {
	// Check for error nodes or incomplete structure in the ForLoop children.
	// This happens when the parser bails early due to syntax errors.
	hasErrors := false
	hasIn := false
	for _, child := range n.(*syntax.Inner).Children() {
		if err, ok := child.(*syntax.Error); ok {
			a.error(err)
			hasErrors = true
		}
		if child.Kind() == syntax.KindIn {
			hasIn = true
		}
	}
	// If there are errors, or the structure is incomplete (no `in` keyword),
	// skip further processing. An incomplete for loop without an error node
	// can happen when the parser bails out to avoid cascading errors.
	if hasErrors || !hasIn {
		return nil
	}

	ns := a.inner(n, syntax.KindForLoop)
	defer ns.finish()
	ns.take(syntax.KindFor)
	patternNode := ns.node()
	pattern := a.unpackDestructuringPattern(patternNode)
	ns.take(syntax.KindIn)
	iterable := a.analyzeExpr(ns.node())
	body := a.analyzeBlock(ns.node())
	return expr.NewForLoop(n.Span(), pattern, patternNode.Span(), iterable, body)
}

func (a *analyzer) analyzeLoopBreak(n syntax.Node) *expr.LoopBreak {
	ns := a.inner(n, syntax.KindLoopBreak)
	defer ns.advance()
	ns.take(syntax.KindBreak)
	return expr.NewLoopBreak(n.Span())
}

func (a *analyzer) analyzeLoopContinue(n syntax.Node) *expr.LoopContinue {
	ns := a.inner(n, syntax.KindLoopContinue)
	defer ns.finish()
	ns.take(syntax.KindContinue)
	return expr.NewLoopContinue(n.Span())
}

func (a *analyzer) analyzeFuncReturn(n syntax.Node) *expr.FuncReturn {
	ns := a.inner(n, syntax.KindFuncReturn)
	defer ns.finish()
	ns.take(syntax.KindReturn)
	var value expr.Expr
	if !ns.done() {
		value = a.analyzeExpr(ns.node())
	}
	return expr.NewFuncReturn(n.Span(), value)
}

func (a *analyzer) analyzeContextual(n syntax.Node) *expr.Contextual {
	ns := a.inner(n, syntax.KindContextual)
	defer ns.finish()
	ns.take(syntax.KindContext)
	body := a.analyzeExpr(ns.node())
	return expr.NewContextual(n.Span(), body)
}

func (a *analyzer) analyzeModuleInclude(n syntax.Node) *expr.ModuleInclude {
	ns := a.inner(n, syntax.KindModuleInclude)
	defer ns.finish()
	ns.take(syntax.KindInclude)
	source := a.analyzeExpr(ns.node())
	return expr.NewModuleInclude(n.Span(), source)
}

func (a *analyzer) analyzeDestructAssignment(n syntax.Node) *expr.DestructAssignment {
	ns := a.inner(n, syntax.KindDestructAssignment)
	defer ns.finish()
	pattern := a.unpackDestructuringPattern(ns.node())
	ns.take(syntax.KindEq)
	value := a.analyzeExpr(ns.node())
	return expr.NewDestructAssignment(n.Span(), pattern, value)
}

func (a *analyzer) unpackDestructuringPattern(n syntax.Node) []expr.DestructPattern {
	switch n.Kind() {
	case syntax.KindIdent:
		return []expr.DestructPattern{expr.NewDestructIdent(a.analyzeIdent(n))}
	case syntax.KindUnderscore:
		return []expr.DestructPattern{expr.NewDestructIdent(expr.NewIdent(n.Span(), underscore))}
	default:
		// continue below
	}

	ns := a.inner(n, syntax.KindDestructuring)
	var pattern []expr.DestructPattern
	haveSink := false
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma:
			continue
		case syntax.KindUnderscore:
			pattern = append(pattern, expr.NewDestructIdent(expr.NewIdent(child.Span(), underscore)))
		case syntax.KindIdent:
			pattern = append(pattern, expr.NewDestructIdent(a.analyzeIdent(child)))
		case syntax.KindNamed:
			named := a.inner(child, syntax.KindNamed)
			name := unique.Make(named.take(syntax.KindIdent))
			named.take(syntax.KindColon)
			patternIdent := a.analyzeIdent(named.node())
			named.finish()
			pattern = append(pattern, expr.NewDestructNamed(name, patternIdent))
		case syntax.KindSpread:
			if haveSink {
				a.error(syntax.NewError(child.Span(), "only one destructuring sink is allowed", child.Text()))
			}
			haveSink = true
			ns := a.inner(child, syntax.KindSpread)
			defer ns.finish()
			ns.take(syntax.KindDots)
			ident := a.analyzeIdent(ns.node())
			pattern = append(pattern, expr.NewDestructSink(ident))
		case syntax.KindError:
			a.error(child.(*syntax.Error))
		default:
			panic("invalid destruct pattern: " + child.Kind().String())
		}
	}
	return pattern
}
