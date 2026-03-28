package ir

import (
	"fmt"
	"math"
	"slices"
	"unique"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

type EvalsOption func(*evalCtx)

func WithBindings(bindings map[unique.Handle[string]]Value) EvalsOption {
	return func(ec *evalCtx) {
		ec.scope.bindings = bindings
	}
}

func Eval(exprs []Expr, opts ...EvalsOption) (c Content, warn []Error, err error) {
	ec := &evalCtx{
		scope: &scope{
			parent: universe,
		},
	}
	for _, opt := range opts {
		opt(ec)
	}
	ec.openScope()
	defer ec.closeScope()

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*errWrapper); ok {
				// propagate Error as an error return
				err = r.(*errWrapper).err
				warn = ec.warnings
				c = nil
			} else {
				// re-panic other kinds of panic
				panic(r)
			}
		}
	}()
	c = evalContents(ec, exprs)
	warn = ec.warnings
	return
}

// Context /////////////////////////////////////////////////////////////////////////////////////////

type EvalContext struct {
	Bindings map[unique.Handle[string]]Value
}

type evalCtx struct {
	scope  *scope
	labels map[unique.Handle[string]]struct{} // Set of labels defined in the document

	warnings []Error
}

func (ec *evalCtx) openScope() {
	ec.scope = &scope{
		parent: ec.scope,
	}
}

func (ec *evalCtx) closeScope() {
	ec.scope = ec.scope.parent
}

func (ec *evalCtx) lookup(name unique.Handle[string]) (Value, bool) {
	return ec.scope.lookup(name)
}

func (ec *evalCtx) bind(name unique.Handle[string], val Value) {
	if ec.scope.bindings == nil {
		ec.scope.bindings = make(map[unique.Handle[string]]Value)
	}
	ec.scope.bindings[name] = val
}

func (ec *evalCtx) warn(warn Error) {
	if len(ec.warnings) > 0 && ec.warnings[len(ec.warnings)-1].Span() == warn.Span() {
		// If the last warning has the same span as the new warning, we assume it's a duplicate and
		// ignore it.
		return
	}
	ec.warnings = append(ec.warnings, warn)
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

func (n *HeadingExpr) eval(ec *evalCtx) Value {
	return &Heading{
		Depth: n.level,
		Body:  evalContents(ec, n.body),
	}
}

func (n *StrongExpr) eval(ec *evalCtx) Value {
	return &Strong{
		Body: evalContents(ec, n.body),
	}
}

func (n *EmphExpr) eval(ec *evalCtx) Value {
	return &Emph{
		Body: evalContents(ec, n.body),
	}
}

func (n *LinkExpr) eval(ec *evalCtx) Value {
	return &Link{
		Dest: n.dest,
		Body: evalContents(ec, n.body),
	}
}

func (n *RefExpr) eval(ec *evalCtx) Value {
	if _, ok := ec.labels[n.target]; !ok {
		raise(&ValueError{
			span: n.Span(),
			msg:  fmt.Sprintf("label `<%s>` does not exist in the document", n.target.Value()),
		})
	}
	v := &Ref{
		Target: n.target,
	}
	if n.supplement != nil {
		v.Supplement = n.supplement.eval(ec).(Content)
	}
	return v
}

func (n *ListItemExpr) eval(ec *evalCtx) Value {
	return &ListItem{
		Body: evalContents(ec, n.body),
	}
}

func (n *EnumItemExpr) eval(ec *evalCtx) Value {
	return &EnumItem{
		Number: n.number,
		Body:   evalContents(ec, n.body),
	}
}

func (n *TermItemExpr) eval(ec *evalCtx) Value {
	return &TermItem{
		Term:        evalContents(ec, n.term),
		Description: evalContents(ec, n.description),
	}
}

func evalContents(ec *evalCtx, exprs []Expr) Content {
	ret := make([]Content, 0, len(exprs))
	var lastContentExpr Expr
	for _, expr := range exprs {
		switch v := expr.eval(ec).(type) {
		case *Label:
			if len(ret) == 0 {
				// TODO: warn of detached label
				continue
			}
			if old := ret[len(ret)-1].SetLabel(v); old != nil {
				ec.warn(&ValueError{
					span:  lastContentExpr.Span(),
					msg:   "content labelled multiple times",
					hints: []string{"only the last label is used, the rest are ignored"},
				})
				delete(ec.labels, old.Name) // Only the last label is used, the rest are ignored.
			}
			if ec.labels == nil {
				ec.labels = make(map[unique.Handle[string]]struct{})
			}
			ec.labels[v.Name] = struct{}{}
		default:
			lastContentExpr = expr
			c, err := toContent(v)
			if err != nil {
				raise(&ValueError{
					span: expr.Span(),
					msg:  err.Error(),
				})
			}
			if c == nil {
				continue
			}
			ret = append(ret, c)
		}
	}
	if len(ret) == 1 {
		return ret[0]
	}
	return &Sequence{Children: ret}
}

func toContent(v Value) (Content, error) {
	switch v := v.(type) {
	case Content:
		return v, nil
	case Str:
		return &Text{Text: string(v)}, nil
	case None:
		return nil, nil
	case Int:
		return &Raw{Lines: []string{fmt.Sprintf("%d", v)}}, nil
	case Float:
		var s string
		if math.IsInf(float64(v), 1) {
			s = "inf"
		} else if math.IsInf(float64(v), -1) {
			s = "-inf"
		} else if math.IsNaN(float64(v)) {
			s = "nan"
		} else {
			s = fmt.Sprintf("%g", v)
		}
		return &Raw{Lines: []string{s}}, nil
	case Decimal:
		d := decimal128.Decimal(v)
		var s string
		if d.IsInf(1) {
			s = "inf"
		} else if d.IsInf(-1) {
			s = "-inf"
		} else if d.IsNaN() {
			s = "nan"
		} else {
			s = d.String()
		}
		return &Raw{Lines: []string{s}}, nil
	default:
		return nil, fmt.Errorf("content expression evaluated to non-content value: %T", v)
	}
}

// Code ////////////////////////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) eval(ec *evalCtx) Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Ident) eval(ec *evalCtx) Value {
	val, ok := ec.lookup(n.name)
	if !ok {
		panic("undefined identifier: " + n.name.Value())
	}
	return val
}

func (n *CodeBlock) eval(ec *evalCtx) Value {
	ec.openScope()
	defer ec.closeScope()

	var values []Value
	var spans []syntax.Span
	for _, expr := range n.exprs {
		values = append(values, expr.eval(ec))
		spans = append(spans, expr.Span())
	}
	v, err := join(values)
	if err != nil {
		if idxErr, ok := err.(*indexError); ok {
			raise(&ValueError{
				span:  spans[idxErr.idx],
				msg:   idxErr.err.Error(),
				hints: nil,
			})
		} else {
			panic(err)
		}
	}
	return v
}

func (n *ContentBlock) eval(ec *evalCtx) Value {
	ec.openScope()
	defer ec.closeScope()
	return evalContents(ec, n.exprs)
}

func (n *Parenthesized) eval(ec *evalCtx) Value {
	return n.body.eval(ec)
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) eval(ec *evalCtx) Value {
	elems := make(Array, 0, len(n.elements))
	for _, expr := range n.elements {
		elems = append(elems, expr.eval(ec))
	}
	return elems
}

func (n *DictExpr) eval(ec *evalCtx) Value {
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
	{syntax.Lt, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) < y.(Int))
	},
	{syntax.Gt, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) > y.(Int))
	},
	{syntax.Leq, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) <= y.(Int))
	},
	{syntax.Geq, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) >= y.(Int))
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
	{syntax.Lt, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) < y.(Float))
	},
	{syntax.Gt, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) > y.(Float))
	},
	{syntax.Leq, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) <= y.(Float))
	},
	{syntax.Geq, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) >= y.(Float))
	},

	// Decimal operations
	{syntax.Add, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Add(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Sub, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Sub(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Mul, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Mul(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Div, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Quo(decimal128.Decimal(y.(Decimal))))
	},

	// Ratio operations
	{syntax.Mul, types.Ratio, types.Ratio}: func(x, y Value) Value {
		r := x.(Numeric).Value * y.(Numeric).Value
		return Numeric{Value: r, Unit: UnitPercent}
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

	// Arguments operations
	{syntax.Add, types.Arguments, types.Arguments}: func(x, y Value) Value {
		a, b := x.(*Arguments), y.(*Arguments)
		return a.merge(b)
	},
}

func (n *Unary) eval(ec *evalCtx) Value {
	x := n.operand.eval(ec)
	op := unaryops[unaryopKey{n.op, x.Type()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported unary operation: %s %s", n.op, x.Type()))
	}
	return op(x)
}

func (n *Binary) eval(ec *evalCtx) Value {
	left, right := n.left.eval(ec), n.right.eval(ec)
	op := binops[binopKey{n.op, left.Type(), right.Type()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Type(), n.op, right.Type()))
	}
	return op(left, right)
}

func (n *FieldAccess) eval(ec *evalCtx) Value {
	t := n.target.eval(ec)
	fname := n.field.Name()
	switch t := t.(type) {
	case *Type:
		ms := typeFields[t.Reflected]
		f := ms[n.field.Name()]
		if f == nil {
			panic(fmt.Sprintf("type %s has no field named %s", t.Reflected, n.field.Name().Value()))
		}
		return f
	case Value:
		if f := typeFields[t.Type()][fname]; f != nil {
			switch f := f.(type) {
			case *Function:
				fn, err := f.With(&Arguments{
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
				return f
			}
		}
		if c, ok := t.(Content); ok {
			f := c.Field(fname)
			if f == nil {
				raise(&ValueError{
					span: n.field.Span(),
					msg:  fmt.Sprintf("content does not have field %q", fname.Value()),
				})
			}
			return f
		}
	default:
		panic(fmt.Sprintf("TODO: implement field access for %s", t.Type()))
	}
	panic(fmt.Sprintf("value of type %s has no field named %s", t.Type(), n.field.Name().Value()))
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) eval(ec *evalCtx) Value {
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
	for _, block := range n.blocks {
		args.Positional = append(args.Positional, block.eval(ec))
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

func (n *Closure) eval(ec *evalCtx) Value {
	panic("TODO: not actually an expression")
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) eval(ec *evalCtx) Value {
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			ec.bind(p.ident.name, n.value.eval(ec))
		default:
			panic("TODO: implement complex let patterns")
		}
	}
	return none
}

func (n *DestructAssignment) eval(ec *evalCtx) Value {
	panic("TODO: implement destruct assignment")
}

func (n *SetRule) eval(ec *evalCtx) Value {
	panic("TODO: implement set rules")
}

func (n *ShowRule) eval(ec *evalCtx) Value {
	panic("TODO: implement show rules")
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) eval(ec *evalCtx) Value {
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

func (n *ForLoop) eval(ec *evalCtx) Value {
	panic("TODO: implement for loops")
}

func (n *WhileLoop) eval(ec *evalCtx) Value {
	panic("TODO: implement while loops")
}

func (n *LoopBreak) eval(ec *evalCtx) Value {
	panic("TODO: not actually an expression")
}

func (n *LoopContinue) eval(ec *evalCtx) Value {
	panic("TODO: not actually an expression")
}

func (n *FuncReturn) eval(ec *evalCtx) Value {
	panic("TODO: not actually an expression")
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) eval(ec *evalCtx) Value { panic("TODO: implement contextual") }

func (n *ModuleInclude) eval(ec *evalCtx) Value { panic("TODO: implement module include") }
