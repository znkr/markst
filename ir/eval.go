package ir

import (
	"fmt"
	"math"
	"unique"

	"github.com/woodsbury/decimal128"
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
			if w, ok := r.(*errWrapper); ok {
				// propagate Error as an error return
				err = ErrorList(w.err)
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

func (ec *evalCtx) lookup(name unique.Handle[string]) (Value, setter, bool) {
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

func (s *scope) lookup(name unique.Handle[string]) (Value, setter, bool) {
	if val, ok := s.bindings[name]; ok {
		return val, func(v Value) { s.bindings[name] = v }, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, nil, false
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
	case *Array:
		return &Raw{Lines: []string{FormatValue(v)}}, nil
	default:
		return nil, fmt.Errorf("content expression evaluated to non-content value: %T", v)
	}
}

// Code ////////////////////////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) eval(ec *evalCtx) Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Ident) eval(ec *evalCtx) Value {
	val, _ := n.evalL(ec)
	return val
}

func (n *Ident) evalL(ec *evalCtx) (Value, setter) {
	val, set, ok := ec.lookup(n.name)
	if !ok {
		panic("undefined identifier: " + n.name.Value())
	}
	return val, set
}

func (n *CodeBlock) eval(ec *evalCtx) Value {
	ec.openScope()
	defer ec.closeScope()

	values := make([]Value, 0, len(n.exprs))
	var joinSel joinerSelector
	for _, expr := range n.exprs {
		v := expr.eval(ec)
		if err := joinSel.add(v.Type()); err != nil {
			raise(&ValueError{
				span: expr.Span(),
				msg:  err.Error(),
			})
		}
		values = append(values, v)
	}
	joiner := joinSel.joiner()
	for i, v := range values {
		if err := joiner.add(v); err != nil {
			raise(&ValueError{
				span: n.exprs[i].Span(),
				msg:  err.Error(),
			})
		}
	}
	return joiner.result()
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
	elems := make([]Value, 0, len(n.elements))
	for _, expr := range n.elements {
		if spread, ok := expr.(*SpreadExpr); ok {
			val := spread.inner.eval(ec)
			switch v := val.(type) {
			case *Array:
				elems = append(elems, v.Elems...)
			case None:
				// spreading none produces no elements
			case Dict:
				raise(&ValueError{
					span: spread.span,
					msg:  "cannot spread dictionary into array",
				})
			default:
				raise(&ValueError{
					span: spread.span,
					msg:  fmt.Sprintf("cannot spread %s into array", val.Type()),
				})
			}
		} else {
			elems = append(elems, expr.eval(ec))
		}
	}
	return &Array{elems}
}

func (n *SpreadExpr) eval(ec *evalCtx) Value {
	panic("spread expr should not be evaluated directly")
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

func (n *Unary) eval(ec *evalCtx) Value {
	x := n.operand.eval(ec)
	op := unaryops[unaryopKey{n.op, x.Type()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported unary operation: %s %s", n.op, x.Type()))
	}
	return op(x)
}

func (n *Binary) eval(ec *evalCtx) Value {
	if n.op.IsAssign() {
		lexpr, ok := n.left.(lvalueExpr)
		if !ok {
			panic("left-hand side of assignment must be an LValue")
		}
		left, set := lexpr.evalL(ec)
		v := n.right.eval(ec)
		if n.op != syntax.Assign {
			op := binops[binopKey{n.op.StripAssign(), left.Type(), v.Type()}]
			if op == nil {
				panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Type(), n.op, v.Type()))
			}
			v = op(left, v)
		}
		set(v)
		return none
	} else {
		left, right := n.left.eval(ec), n.right.eval(ec)
		switch n.op {
		case syntax.Eq:
			return Bool(left.Equal(right))
		case syntax.Neq:
			return Bool(!left.Equal(right))
		}
		op := binops[binopKey{n.op, left.Type(), right.Type()}]
		if op == nil {
			panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Type(), n.op, right.Type()))
		}
		return op(left, right)
	}
}

func (n *FieldAccess) eval(ec *evalCtx) Value {
	t := n.target.eval(ec)
	fname := n.field.Name()
	switch t := t.(type) {
	case *Type:
		ms := typeFields[t.Reflected]
		f := ms[n.field.Name()]
		if f == nil {
			raise(&ValueError{
				span: n.span,
				msg:  fmt.Sprintf("type %s has no method `%s`", t.Reflected, n.field.Name().Value()),
			})
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
		switch t := t.(type) {
		case Dict:
			if val, ok := t[Str(fname.Value())]; ok {
				return val
			} else {
				raise(&ValueError{
					span: n.field.Span(),
					msg:  fmt.Sprintf("dictionary does not have an entry %q", fname.Value()),
				})
			}
		case Content:
			f := t.Field(fname)
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
	raise(&ValueError{
		span: n.span,
		msg:  fmt.Sprintf("type %s has no method `%s`", t.Type(), n.field.Name().Value()),
	})
	panic("unreachable")
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) eval0(ec *evalCtx, setter *setter) Value {
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
		Span:   n.span,
		setter: setter,
	}
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		switch err := err.(type) {
		case ArgErrors:
			var errs []Error
			for _, e := range err {
				errs = append(errs, &ValueError{
					span:  n.locateArgErrSpan(fn, e),
					msg:   e.Error(),
					hints: e.hints,
				})
			}
			raise(errs...)
		case *ArgError:
			raise(&ValueError{
				span:  n.locateArgErrSpan(fn, err),
				msg:   err.Error(),
				hints: err.hints,
			})
		default:
			raise(&ValueError{
				span: n.span,
				msg:  err.Error(),
			})
		}

	}
	return v
}

func (n *FuncCall) evalL(ec *evalCtx) (Value, setter) {
	var setter setter
	v := n.eval0(ec, &setter)
	if setter == nil {
		raise(&ValueError{
			span:  n.span,
			msg:   "cannot mutate a temporary value",
			hints: nil,
		})
	}
	return v, setter
}

func (n *FuncCall) eval(ec *evalCtx) Value {
	return n.eval0(ec, nil)
}

func (n *FuncCall) locateArgErrSpan(fn *Function, err *ArgError) syntax.Span {
	offset := 0
	if fn.WithArgs != nil {
		offset = len(fn.WithArgs.Positional)
	}
	for i, arg := range n.args {
		if span, ok := err.match(i+offset, arg); ok {
			return span
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
