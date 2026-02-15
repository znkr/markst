package ir

import (
	"fmt"
	"slices"
	"strings"
	"unique"

	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

func Eval(ec *EvalContext, exprs []Expr) (c Contents, err error) {
	ec.openScope()
	defer ec.closeScope()

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*errWrapper); ok {
				// propagate Error as an error return
				err = r.(*errWrapper).err
				c = nil
			} else {
				// re-panic other kinds of panic
				panic(r)
			}
		}
	}()
	c = evalContents(ec, exprs)
	return c, err
}

// Context /////////////////////////////////////////////////////////////////////////////////////////

type EvalContext struct {
	scope *scope
}

func NewEvalContext() *EvalContext {
	return &EvalContext{
		scope: &scope{
			parent: universe,
		},
	}
}

func (ec *EvalContext) openScope() {
	ec.scope = &scope{
		parent: ec.scope,
	}
}

func (ec *EvalContext) closeScope() {
	ec.scope = ec.scope.parent
}

func (ec *EvalContext) Lookup(name unique.Handle[string]) (Value, bool) {
	return ec.scope.lookup(name)
}

func (ec *EvalContext) Bind(name unique.Handle[string], val Value) {
	if ec.scope.bindings == nil {
		ec.scope.bindings = make(map[unique.Handle[string]]Value)
	}
	ec.scope.bindings[name] = val
}

// Scope ///////////////////////////////////////////////////////////////////////////////////////////

type scope struct {
	parent   *scope
	bindings map[unique.Handle[string]]Value
}

func (s *scope) lookup(name unique.Handle[string]) (Value, bool) {
	if val, ok := s.bindings[name]; ok {
		return val, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, false
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

func (n *HeadingExpr) eval(ec *EvalContext) Value {
	return &Heading{
		Level: n.level,
		Body:  evalContents(ec, n.body),
	}
}

func (n *StrongExpr) eval(ec *EvalContext) Value {
	return &Strong{
		Body: evalContents(ec, n.body),
	}
}

func (n *EmphExpr) eval(ec *EvalContext) Value {
	return &Emph{
		Body: evalContents(ec, n.body),
	}
}

func (n *LinkExpr) eval(ec *EvalContext) Value {
	return &Link{
		Dest: n.dest,
		Body: evalContents(ec, n.body),
	}
}

func (n *RefExpr) eval(ec *EvalContext) Value {
	return &Ref{
		Target:     n.target,
		Supplement: n.supplement.eval(ec).(Content),
	}
}

func (n *ListItemExpr) eval(ec *EvalContext) Value {
	return &ListItem{
		Body: evalContents(ec, n.body),
	}
}

func (n *EnumItemExpr) eval(ec *EvalContext) Value {
	return &EnumItem{
		Number: n.number,
		Body:   evalContents(ec, n.body),
	}
}

func (n *TermItemExpr) eval(ec *EvalContext) Value {
	return &TermItem{
		Term:        evalContents(ec, n.term),
		Description: evalContents(ec, n.description),
	}
}

func evalContents(ec *EvalContext, exprs []Expr) Contents {
	var ret Contents
	for _, expr := range exprs {
		v := toContent(expr.Span(), expr.eval(ec))
		if v == nil {
			continue
		}
		ret = append(ret, v)
	}
	return ret
}

func toContent(span syntax.Span, v Value) Content {
	switch v := v.(type) {
	case Content:
		return v
	case Str:
		return &Text{Value: string(v)}
	case None:
		return nil
	case Int:
		return &Raw{Lines: []string{fmt.Sprintf("%d", v)}}
	case Float:
		return &Raw{Lines: []string{fmt.Sprintf("%g", v)}}
	default:
		raise(&ValueError{
			span: span,
			msg:  fmt.Sprintf("content expression evaluated to non-content value: %T", v),
		})
		panic(fmt.Sprintf("content expression evaluated to non-element value: %T", v))
	}
}

// Code ////////////////////////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) eval(ec *EvalContext) Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Ident) eval(ec *EvalContext) Value {
	val, ok := ec.Lookup(n.name)
	if !ok {
		panic("undefined identifier: " + n.name.Value())
	}
	return val
}

var joinResultType = map[[2]types.Type]types.Type{
	{types.Str, types.Str}:         types.Str,
	{types.Bytes, types.Bytes}:     types.Bytes,
	{types.Array, types.Array}:     types.Array,
	{types.Dict, types.Dict}:       types.Dict,
	{types.Str, types.Content}:     types.Content,
	{types.Content, types.Content}: types.Content,
}

func (n *CodeBlock) eval(ec *EvalContext) Value {
	ec.openScope()
	defer ec.closeScope()

	var values []Value
	var spans []syntax.Span
	rtype := types.None
	for i, expr := range n.exprs {
		v := expr.eval(ec)
		if i == 0 {
			rtype = v.Type()
		} else if v.Type() != rtype {
			var at, bt types.Type
			at = rtype
			bt = v.Type()
			if at > bt {
				at, bt = bt, at
			}
			rtyp, ok := joinResultType[[2]types.Type{at, bt}]
			if !ok {
				raise(&ValueError{
					span: expr.Span(),
					msg:  fmt.Sprintf("cannot join %s with %s", rtype, v.Type()),
				})
			}
			rtype = rtyp
		}
		values = append(values, v)
		spans = append(spans, expr.Span())
	}
	switch rtype {
	case types.None:
	case types.Int, types.Float:
		return values[0]
	case types.Str:
		var sb strings.Builder
		for _, v := range values {
			sb.WriteString(string(v.(Str)))
		}
		return Str(sb.String())
	case types.Bytes:
		var sb strings.Builder
		for _, v := range values {
			sb.WriteString(string(v.(Bytes)))
		}
		return Bytes(sb.String())
	case types.Content:
		var contents Contents
		for i, v := range values {
			contents = append(contents, toContent(spans[i], v))
		}
		return contents
	case types.Array:
		var arr Array
		for _, v := range values {
			arr = append(arr, v.(Array)...)
		}
		return arr
	case types.Dict:
		dict := make(Dict)
		for _, v := range values {
			for k, val := range v.(Dict) {
				dict[k] = val
			}
		}
		return dict
	}
	panic("unsupported result type: " + rtype.String())
}

func (n *ContentBlock) eval(ec *EvalContext) Value {
	ec.openScope()
	defer ec.closeScope()
	return evalContents(ec, n.exprs)
}

func (n *Parenthesized) eval(ec *EvalContext) Value {
	return n.body.eval(ec)
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) eval(ec *EvalContext) Value {
	elems := make(Array, 0, len(n.elements))
	for _, expr := range n.elements {
		elems = append(elems, expr.eval(ec))
	}
	return elems
}

func (n *DictExpr) eval(ec *EvalContext) Value {
	dict := make(Dict, len(n.entries))
	for _, ent := range n.entries {
		keyVal := ent.key.eval(ec)
		keyStr, ok := keyVal.(Str)
		if !ok {
			panic("dictionary key did not evaluate to a string")
		}
		dict[keyStr] = ent.value.eval(ec)
	}
	return dict
}

// Operations //////////////////////////////////////////////////////////////////////////////////////

type unaryopKey struct {
	op  syntax.UnaryOp
	typ types.Type
}

var unaryops = map[unaryopKey]func(x Value) Value{
	{syntax.Not, types.Bool}: func(x Value) Value {
		return !x.(Bool)
	},
	{syntax.Neg, types.Int}: func(x Value) Value {
		return -x.(Int)
	},
	{syntax.Neg, types.Float}: func(x Value) Value {
		return -x.(Float)
	},
}

type binopKey struct {
	op        syntax.BinaryOp
	leftType  types.Type
	rightType types.Type
}

var binops = map[binopKey]func(x, y Value) Value{
	// Int operations
	{syntax.Add, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) + y.(Int)
	},
	{syntax.Sub, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) - y.(Int)
	},
	{syntax.Mul, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) * y.(Int)
	},
	{syntax.Div, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) / y.(Int)
	},

	// Float operations
	{syntax.Add, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) + y.(Float)
	},
	{syntax.Sub, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) - y.(Float)
	},
	{syntax.Mul, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) * y.(Float)
	},
	{syntax.Div, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) / y.(Float)
	},

	// String operations
	{syntax.Add, types.Str, types.Str}: func(x, y Value) Value {
		return Str(string(x.(Str)) + string(y.(Str)))
	},

	// Bytes operations
	{syntax.Add, types.Bytes, types.Bytes}: func(x, y Value) Value {
		return Bytes(string(x.(Bytes)) + string(y.(Bytes)))
	},

	// Array operations
	{syntax.Mul, types.Array, types.Int}: func(x, y Value) Value {
		arr, times := x.(Array), y.(Int)
		if times < 0 {
			panic("cannot multiply array by negative integer")
		}
		result := slices.Repeat(arr, int(times))
		return result
	},
}

func (n *Unary) eval(ec *EvalContext) Value {
	x := n.operand.eval(ec)
	op := unaryops[unaryopKey{n.op, x.Type()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported unary operation: %s %s", n.op, x.Type()))
	}
	return op(x)
}

func (n *Binary) eval(ec *EvalContext) Value {
	left, right := n.left.eval(ec), n.right.eval(ec)
	op := binops[binopKey{n.op, left.Type(), right.Type()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Type(), n.op, right.Type()))
	}
	return op(left, right)
}

func (n *FieldAccess) eval(ec *EvalContext) Value {
	t := n.target.eval(ec)
	switch t := t.(type) {
	case *Type:
		ms := methods[t.Reflected]
		fn := ms[n.field.Name()]
		if fn == nil {
			panic(fmt.Sprintf("type %s has no method named %s", t.Reflected, n.field.Name().Value()))
		}
		return fn
	case Value:
		ms := methods[t.Type()]
		fn := ms[n.field.Name()]
		if fn == nil {
			panic(fmt.Sprintf("type %s has no method named %s", t.Type(), n.field.Name().Value()))
		}
		fn, err := fn.With(&Arguments{
			Positional: []Value{t},
		})
		if err != nil {
			if argErr, ok := err.(*ArgError); ok {
				raise(&ValueError{
					span:  n.field.Span(),
					msg:   argErr.msg,
					hints: argErr.hints,
				})
			}
		}
		return fn
	default:
		panic(fmt.Sprintf("TODO: implement field access for %s", t.Type()))
	}
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) eval(ec *EvalContext) Value {
	callee := n.callee.eval(ec)
	var fn *Function
	switch callee := callee.(type) {
	case *Function:
		fn = callee
	case *Type:
		if callee.Constructor == nil {
			panic(fmt.Sprintf("type %s is not callable", callee.Reflected))
		}
		fn = callee.Constructor
	default:
		panic(fmt.Sprintf("attempted to call a non-function value of type %s", callee.Type()))
	}

	var args Arguments
	for _, arg := range n.args {
		switch a := arg.(type) {
		case *ExprArg:
			args.Positional = append(args.Positional, a.expr.eval(ec))
		case *NamedArg:
			if args.Named == nil {
				args.Named = make(map[unique.Handle[string]]Value)
			}
			args.Named[a.name] = a.expr.eval(ec)
		case *SpreadArg:
			panic("TODO: implement spread arguments")
		default:
			panic("unrecognized function argument type")
		}
	}
	fcc := FuncCallContext{
		Span: n.span,
	}
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		var hints []string
		if argErr, ok := err.(*ArgError); ok {
			hints = argErr.hints
		}
		raise(&ValueError{
			span:  n.locateArgErrSpan(err),
			msg:   err.Error(),
			hints: hints,
		})
	}
	return v
}

func (n *FuncCall) locateArgErrSpan(err error) syntax.Span {
	argErr, ok := err.(*ArgError)
	if !ok {
		return n.span
	}
	for i, arg := range n.args {
		if expr := argErr.match(i, arg); expr != nil {
			return expr.Span()
		}
	}
	return n.span
}

func (n *Closure) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) eval(ec *EvalContext) Value {
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			ec.Bind(p.ident.name, n.value.eval(ec))
		default:
			panic("TODO: implement complex let patterns")
		}
	}
	return none
}

func (n *DestructAssignment) eval(ec *EvalContext) Value {
	panic("TODO: implement destruct assignment")
}

func (n *SetRule) eval(ec *EvalContext) Value {
	panic("TODO: implement set rules")
}

func (n *ShowRule) eval(ec *EvalContext) Value {
	panic("TODO: implement show rules")
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) eval(ec *EvalContext) Value {
	for i, cond := range n.conditions {
		cond, ok := cond.eval(ec).(Bool)
		if !ok {
			panic("condition did not evaluate to a boolean")
		}
		if cond {
			return n.blocks[i].eval(ec)
		}
	}
	if n.def != nil {
		return n.def.eval(ec)
	}
	return none
}

func (n *ForLoop) eval(ec *EvalContext) Value {
	panic("TODO: implement for loops")
}

func (n *WhileLoop) eval(ec *EvalContext) Value {
	panic("TODO: implement while loops")
}

func (n *LoopBreak) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *LoopContinue) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *FuncReturn) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) eval(ec *EvalContext) Value { panic("TODO: implement contextual") }

func (n *ModuleInclude) eval(ec *EvalContext) Value { panic("TODO: implement module include") }
