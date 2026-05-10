package expr

import (
	"errors"
	"fmt"
	"maps"
	"unique"

	"znkr.io/writst/builtin"
	"znkr.io/writst/internal/joiner"
	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// EvalsOption configures the evaluation context for [Eval].
type EvalsOption func(*evalCtx)

// WithBindings adds initial variable bindings to the evaluation scope.
func WithBindings(bindings map[unique.Handle[string]]value.Value) EvalsOption {
	return func(ec *evalCtx) {
		ec.scope.bindings = bindings
	}
}

// Eval evaluates a slice of IR expressions and returns the resulting document
// [Content].
//
// Evaluation errors are returned as an [ErrorList]; non-fatal warnings are
// returned separately. The evaluation operates in a scope that inherits from
// the built-in universe (global functions, types, and constructors).
func Eval(exprs []Expr, opts ...EvalsOption) (c value.Content, warn []Error, err error) {
	ec := &evalCtx{
		scope: &scope{
			parent: &scope{
				bindings: builtin.Universe,
			},
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

// Context /////////////////////////////////////////////////////////////////////

type EvalContext struct {
	Bindings map[unique.Handle[string]]value.Value
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

func (ec *evalCtx) lookup(name unique.Handle[string]) (value.Value, setter, bool) {
	return ec.scope.lookup(name)
}

func (ec *evalCtx) bind(name unique.Handle[string], val value.Value) {
	ec.scope.bind(name, val)
}

func (ec *evalCtx) warn(warn Error) {
	if len(ec.warnings) > 0 && ec.warnings[len(ec.warnings)-1].Span() == warn.Span() {
		// If the last warning has the same span as the new warning, we assume it's a duplicate and
		// ignore it.
		return
	}
	ec.warnings = append(ec.warnings, warn)
}

// Scope ///////////////////////////////////////////////////////////////////////

type scope struct {
	parent   *scope
	bindings map[unique.Handle[string]]value.Value
}

func (s *scope) bind(name unique.Handle[string], val value.Value) {
	if s.bindings == nil {
		s.bindings = make(map[unique.Handle[string]]value.Value)
	}
	s.bindings[name] = val
}

func (s *scope) lookup(name unique.Handle[string]) (value.Value, setter, bool) {
	if val, ok := s.bindings[name]; ok {
		return val, func(v value.Value) { s.bindings[name] = v }, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, nil, false
}

// Content Expressions /////////////////////////////////////////////////////////

func (n *HeadingExpr) eval(ec *evalCtx) value.Value {
	return &value.Heading{
		Depth: n.level,
		Body:  evalContents(ec, n.body),
	}
}

func (n *StrongExpr) eval(ec *evalCtx) value.Value {
	return &value.Strong{
		Body: evalContents(ec, n.body),
	}
}

func (n *EmphExpr) eval(ec *evalCtx) value.Value {
	return &value.Emph{
		Body: evalContents(ec, n.body),
	}
}

func (n *LinkExpr) eval(ec *evalCtx) value.Value {
	return &value.Link{
		Dest: n.dest,
		Body: evalContents(ec, n.body),
	}
}

func (n *RefExpr) eval(ec *evalCtx) value.Value {
	if _, ok := ec.labels[n.target]; !ok {
		raise(&ValueError{
			span: n.Span(),
			msg:  fmt.Sprintf("label `<%s>` does not exist in the document", n.target.Value()),
		})
	}
	v := &value.Ref{
		Target: n.target,
	}
	if n.supplement != nil {
		v.Supplement = n.supplement.eval(ec).(value.Content)
	}
	return v
}

func (n *ListItemExpr) eval(ec *evalCtx) value.Value {
	return &value.ListItem{
		Body: evalContents(ec, n.body),
	}
}

func (n *EnumItemExpr) eval(ec *evalCtx) value.Value {
	return &value.EnumItem{
		Number: n.number,
		Body:   evalContents(ec, n.body),
	}
}

func (n *TermItemExpr) eval(ec *evalCtx) value.Value {
	return &value.TermItem{
		Term:        evalContents(ec, n.term),
		Description: evalContents(ec, n.description),
	}
}

func evalContents(ec *evalCtx, exprs []Expr) value.Content {
	ret := make([]value.Content, 0, len(exprs))
	var lastContentExpr Expr
	for _, expr := range exprs {
		switch v := expr.eval(ec).(type) {
		case *value.Label:
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
			c, err := value.ToContent(v)
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
	return &value.Sequence{Children: ret}
}

// Code ////////////////////////////////////////////////////////////////////////

func (n *ConstExpr) eval(ec *evalCtx) value.Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////

func (n *Ident) eval(ec *evalCtx) value.Value {
	val, _ := n.evalL(ec)
	return val
}

func (n *Ident) evalL(ec *evalCtx) (value.Value, setter) {
	val, set, ok := ec.lookup(n.name)
	if !ok {
		panic(fmt.Sprintf("unknown variable %q not caught by analyzer", n.name.Value()))
	}
	return val, set
}

func (n *CodeBlock) eval(ec *evalCtx) value.Value {
	ec.openScope()
	defer ec.closeScope()

	values := make([]value.Value, 0, len(n.exprs))
	var joinSel joiner.Selector
	for _, expr := range n.exprs {
		v := expr.eval(ec)
		if err := joinSel.Add(v.Type()); err != nil {
			raise(&ValueError{
				span: expr.Span(),
				msg:  err.Error(),
			})
		}
		values = append(values, v)
	}
	joiner := joinSel.Joiner()
	for i, v := range values {
		if err := joiner.Add(v); err != nil {
			raise(&ValueError{
				span: n.exprs[i].Span(),
				msg:  err.Error(),
			})
		}
	}
	return joiner.Result()
}

func (n *ContentBlock) eval(ec *evalCtx) value.Value {
	ec.openScope()
	defer ec.closeScope()
	return evalContents(ec, n.exprs)
}

func (n *Parenthesized) eval(ec *evalCtx) value.Value {
	return n.body.eval(ec)
}

func (n *Parenthesized) evalL(ec *evalCtx) (value.Value, setter) {
	if l, ok := n.body.(lvalueExpr); ok {
		return l.evalL(ec)
	}
	return n.body.eval(ec), nil
}

// Collections /////////////////////////////////////////////////////////////////

func (n *ArrayExpr) eval(ec *evalCtx) value.Value {
	elems := make([]value.Value, 0, len(n.elements))
	for _, expr := range n.elements {
		if spread, ok := expr.(*SpreadExpr); ok {
			val := spread.inner.eval(ec)
			switch v := val.(type) {
			case *value.Array:
				elems = append(elems, v.Elems...)
			case value.None:
				// spreading none produces no elements
			case *value.Dict:
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
	return &value.Array{Elems: elems}
}

func (n *SpreadExpr) eval(ec *evalCtx) value.Value {
	panic("spread expr should not be evaluated directly")
}

func (n *DictExpr) eval(ec *evalCtx) value.Value {
	dict := new(value.Dict)
	for _, ent := range n.entries {
		keyVal := ent.key.eval(ec)
		keyStr, ok := keyVal.(value.Str)
		if !ok {
			panic("dictionary key did not evaluate to a string")
		}
		dict.Elems.Put(keyStr, ent.value.eval(ec))
	}
	return dict
}

// Operations //////////////////////////////////////////////////////////////////

func (n *Unary) eval(ec *evalCtx) value.Value {
	v, err := value.UnaryOp(n.op, n.operand.eval(ec))
	if err != nil {
		raise(&ValueError{
			span: n.span,
			msg:  err.Error(),
		})
	}
	return v
}

func (n *Binary) eval(ec *evalCtx) value.Value {
	if n.op.IsAssign() {
		lexpr, ok := n.left.(lvalueExpr)
		if !ok {
			// Evaluate the left hand side to surface any errors in the expression. Only report the
			// error below if the expression is valid, to avoid confusing error messages.
			_ = n.left.eval(ec)
			raise(&ValueError{
				span:  n.left.Span(),
				msg:   "cannot mutate a temporary value",
				hints: nil,
			})
		}
		v := n.right.eval(ec)
		left, set := lexpr.evalL(ec)
		if n.op != syntax.Assign {
			var err error
			v, err = value.BinaryOp(n.op.StripAssign(), left, v)
			if err != nil {
				raise(&ValueError{
					span: n.span,
					msg:  err.Error(),
				})
			}
		}
		set(v)
		return value.None{}
	} else {
		left := n.left.eval(ec)
		if left.Type() == types.Bool {
			if n.op == syntax.And && !left.(value.Bool) {
				return value.Bool(false)
			}
			if n.op == syntax.Or && left.(value.Bool) {
				return value.Bool(true)
			}
		}
		right := n.right.eval(ec)
		v, err := value.BinaryOp(n.op, left, right)
		if err != nil {
			raise(&ValueError{
				span: n.span,
				msg:  err.Error(),
			})
		}
		return v
	}
}

func (n *FieldAccess) eval(ec *evalCtx) value.Value {
	t := n.target.eval(ec)
	fname := n.field.Name()
	switch t := t.(type) {
	case *value.Type:
		ms := builtin.TypeFields[t.Reflected]
		f := ms[n.field.Name()]
		if f == nil {
			raise(&ValueError{
				span: n.span,
				msg:  fmt.Sprintf("type %s has no method `%s`", t.Reflected, n.field.Name().Value()),
			})
		}
		return f
	case *value.Module:
		def := t.Definitions[n.field.Name()]
		if def == nil {
			raise(&ValueError{
				span: n.span,
				msg:  fmt.Sprintf("module %s has no definition `%s`", t.Name, n.field.Name().Value()),
			})
		}
		return def
	case value.Value:
		if f := builtin.TypeFields[t.Type()][fname]; f != nil {
			switch f := f.(type) {
			case *value.Function:
				fn, err := f.With(&value.Arguments{
					Positional: []value.Value{t},
				})
				if err != nil {
					if fcerr, ok := err.(*value.FunctionCallError); ok {
						raise(&ValueError{
							span:  n.field.Span(),
							msg:   fcerr.Msg,
							hints: fcerr.Hints,
						})
					}
					raise(&ValueError{
						span: n.field.Span(),
						msg:  err.Error(),
					})
				}
				return fn
			default:
				return f
			}
		}
		switch t := t.(type) {
		case *value.Dict:
			if val, ok := t.Elems.Get(value.Str(fname.Value())); ok {
				return val
			} else {
				raise(&ValueError{
					span: n.field.Span(),
					msg:  fmt.Sprintf("dictionary does not have an entry %q", fname.Value()),
				})
			}
		case value.Content:
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

// Functions ///////////////////////////////////////////////////////////////////

func (n *FuncCall) eval0(ec *evalCtx, setter *func(value.Value)) value.Value {
	callee := n.callee.eval(ec)
	var fn *value.Function
	switch callee := callee.(type) {
	case *value.Function:
		fn = callee
	case *value.Type:
		if callee.Constructor == nil {
			panic(fmt.Sprintf("type %s is not callable", callee.Reflected))
		}
		fn = callee.Constructor
	default:
		panic(fmt.Sprintf("attempted to call a non-function value of type %s", callee.Type()))
	}

	var args value.Arguments
	for _, arg := range n.args {
		switch a := arg.(type) {
		case *ExprArg:
			if len(args.Positional) >= len(fn.Positional) && fn.Sink == nil {
				raise(&ValueError{
					span: a.expr.Span(),
					msg:  "unexpected argument",
				})
			}
			args.Positional = append(args.Positional, a.expr.eval(ec))
		case *NamedArg:
			if args.Named == nil {
				args.Named = make(map[unique.Handle[string]]value.Value)
			}
			args.Named[a.name] = a.expr.eval(ec)
		case *SpreadArg:
			val := a.expr.eval(ec)
			switch v := val.(type) {
			case *value.Array:
				args.Positional = append(args.Positional, v.Elems...)
			case *value.Dict:
				if args.Named == nil {
					args.Named = make(map[unique.Handle[string]]value.Value)
				}
				for k, v := range v.Elems.All() {
					args.Named[unique.Make(string(k))] = v
				}
			case *value.Arguments:
				args.Positional = append(args.Positional, v.Positional...)
				if len(v.Named) > 0 {
					if args.Named == nil {
						args.Named = make(map[unique.Handle[string]]value.Value)
					}
					maps.Copy(args.Named, v.Named)
				}
			case value.None:
				// spreading none produces no arguments
			default:
				raise(&ValueError{
					span: a.expr.Span(),
					msg:  fmt.Sprintf("cannot spread %s", val.Type()),
				})
			}
		default:
			panic("unrecognized function argument type")
		}
	}
	for _, block := range n.blocks {
		args.Positional = append(args.Positional, block.eval(ec))
	}
	// Warn when a literal float is passed to decimal().
	if fn.Name == "decimal" && len(n.args) > 0 {
		if ea, ok := n.args[0].(*ExprArg); ok {
			if ce, ok := ea.expr.(*ConstExpr); ok {
				if f, ok := ce.value.(value.Float); ok {
					ec.warn(&ValueError{
						span:  ea.expr.Span(),
						msg:   "creating a decimal using imprecise float literal",
						hints: []string{"use a string in the decimal constructor to avoid loss of precision: `decimal(\"" + f.String() + "\")`"},
					})
				}
			}
		}
	}

	fcc := value.FunctionCallContext{
		Span:   n.span,
		Setter: setter,
	}
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		var inErrs []error
		if wrapped, ok := err.(interface{ Unwrap() []error }); ok {
			inErrs = wrapped.Unwrap()
		} else {
			inErrs = []error{err}
		}
		var errs []Error
		for _, err := range inErrs {
			if fcerr, ok := errors.AsType[*value.FunctionCallError](err); ok {
				errs = append(errs, &ValueError{
					span:  n.locateArgErrSpan(fn, fcerr.Location),
					msg:   fcerr.Msg,
					hints: fcerr.Hints,
				})
			} else {
				errs = append(errs, &ValueError{
					span: n.span,
					msg:  err.Error(),
				})

			}

		}
		raise(errs...)
	}
	return v
}

func (n *FuncCall) evalL(ec *evalCtx) (value.Value, setter) {
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

func (n *FuncCall) eval(ec *evalCtx) value.Value {
	return n.eval0(ec, nil)
}

func (n *FuncCall) locateArgErrSpan(fn *value.Function, loc value.ArgLoc) syntax.Span {
	if loc == nil {
		return n.span
	}
	i := 0
	if fn.WithArgs != nil {
		i = len(fn.WithArgs.Positional)
	}
	for _, arg := range n.args {
		switch arg := arg.(type) {
		case *ExprArg:
			if loc, ok := loc.(value.ArgLocPositional); ok && i == int(loc) {
				return arg.expr.Span()
			}
			i++
		case *NamedArg:
			switch loc := loc.(type) {
			case value.ArgLocNamed:
				if unique.Handle[string](loc) == arg.name {
					return arg.expr.Span()
				}
			case value.ArgLocNamedPair:
				if unique.Handle[string](loc) == arg.name {
					return arg.Span()
				}
			}
		}
	}
	return n.span
}

func (n *Closure) eval(ec *evalCtx) value.Value {
	f := &value.Function{}
	if n.name != nil {
		f.Name = n.name.name.Value()
	}

	// positional tracks the name handles for each entry in f.Positional,
	// including the sink slot (which stores its own name separately).
	var positional []unique.Handle[string]
	var sinkName unique.Handle[string]
	var hasSink bool
	for _, p := range n.params {
		switch p := p.(type) {
		case *PositionalClosureParam:
			positional = append(positional, p.Name().Name())
			f.Positional = append(f.Positional, value.Param{
				Name: p.Name().Name().Value(),
				Type: types.Any,
			})
		case *NamedClosureParam:
			if f.Named == nil {
				f.Named = make(map[unique.Handle[string]]value.Param)
			}
			f.Named[p.Name().Name()] = value.Param{
				Name:    p.Name().Name().Value(),
				Type:    types.Any,
				Default: p.Default().eval(ec),
			}
		case *SpreadClosureParam:
			hasSink = true
			sinkIdx := len(f.Positional)
			f.Sink = &sinkIdx
			name := "sink"
			if p.Ident() != nil {
				sinkName = p.Ident().Name()
				name = sinkName.Value()
			}
			positional = append(positional, sinkName)
			f.Positional = append(f.Positional, value.Param{
				Name: name,
				Type: types.SetOf(types.Arguments),
			})
		default:
			panic(fmt.Sprintf("not implemented for %T", p))
		}
	}

	captures := make(map[unique.Handle[string]]value.Value, len(n.captures))
	for name := range n.captures {
		val, _, ok := ec.lookup(name)
		if !ok {
			panic(fmt.Sprintf("capture %q not found; should have been caught by analyzer", name.Value()))
		}
		captures[name] = val
	}
	scope := &scope{
		bindings: captures,
	}
	f.F = func(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
		origScope := ec.scope
		ec.scope = scope
		ec.openScope()
		defer func() {
			ec.scope = origScope
		}()

		for i, arg := range args {
			if hasSink && i == *f.Sink {
				// Bind the sink parameter if it has a name.
				if sinkName != (unique.Handle[string]{}) {
					ec.bind(sinkName, arg)
				}
				continue
			}
			ec.bind(positional[i], arg)
		}
		for name := range f.Named {
			val := named.Get(name)
			ec.bind(name, val)
		}

		v := n.body.eval(ec)
		return v, nil
	}
	return f
}

// Bindings & Rules ////////////////////////////////////////////////////////////

func (n *LetBinding) eval(ec *evalCtx) value.Value {
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			var v value.Value
			if n.value != nil {
				// #let x = ...
				v = n.value.eval(ec)
			} else {
				// #let x
				v = value.None{}
			}
			ec.bind(p.ident.name, v)
		default:
			panic("TODO: implement complex let patterns")
		}
	}
	return value.None{}
}

func (n *DestructAssignment) eval(ec *evalCtx) value.Value {
	panic("TODO: implement destruct assignment")
}

func (n *SetRule) eval(ec *evalCtx) value.Value {
	panic("TODO: implement set rules")
}

func (n *ShowRule) eval(ec *evalCtx) value.Value {
	panic("TODO: implement show rules")
}

// Control Flow ////////////////////////////////////////////////////////////////

func (n *Conditional) eval(ec *evalCtx) value.Value {
	for i, cond := range n.conditions {
		condV := cond.eval(ec)
		if condV.Type() != types.Bool {
			raise(&ValueError{
				span: cond.Span(),
				msg:  fmt.Sprintf("expected boolean, found %s", condV.Type()),
			})
		}
		if condV.(value.Bool) {
			return n.blocks[i].eval(ec)
		}
	}
	if n.def != nil {
		return n.def.eval(ec)
	}
	return value.None{}
}

func (n *ForLoop) eval(ec *evalCtx) value.Value {
	iterable := n.iterable.eval(ec)

	ec.openScope()
	defer ec.closeScope()
	var setters []setter
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			setters = append(setters, func(v value.Value) {
				ec.bind(p.ident.name, v)
			})
		default:
			panic(fmt.Sprintf("not implemented for %T", p))
		}
	}

	switch iterable := iterable.(type) {
	case *value.Array:
		set := func(v value.Value) {
			switch v := v.(type) {
			case *value.Array:
				if len(setters) != len(v.Elems) {
					panic("mismatched number of setters and array elements")
				}
				for i, elem := range v.Elems {
					setters[i](elem)
				}
			default:
				if len(setters) != 1 {
					panic("multiple setters for non-array value")
				}
				setters[0](v)
			}
		}
		values := make([]value.Value, 0, len(iterable.Elems))
		var joinSel joiner.Selector
		for _, e := range iterable.Elems {
			set(e)
			v := n.body.eval(ec)
			values = append(values, v)
			joinSel.Add(v.Type())
		}
		joiner := joinSel.Joiner()
		for _, e := range values {
			joiner.Add(e)
		}
		return joiner.Result()
	case *value.Dict:
		set := func(k, v value.Value) {
			switch len(setters) {
			case 1:
				setters[0](&value.Array{Elems: []value.Value{k, v}})
			case 2:
				setters[0](k)
				setters[1](v)
			default:
				panic("expected exactly one or two setters for dict destructuring")
			}
		}
		values := make([]value.Value, 0, iterable.Elems.Len())
		var joinSel joiner.Selector
		for k, v := range iterable.Elems.All() {
			set(k, v)
			v := n.body.eval(ec)
			values = append(values, v)
			joinSel.Add(v.Type())
		}
		joiner := joinSel.Joiner()
		for _, e := range values {
			joiner.Add(e)
		}
		return joiner.Result()
	case value.Str:
		if len(setters) != 1 {
			raise(&ValueError{
				span: n.PatternSpan(),
				msg:  "cannot destructure values of string",
			})
			panic("unreachable")
		}
		set := setters[0]
		var values []value.Value
		var joinSel joiner.Selector
		for g := range graphemes(string(iterable)) {
			set(value.Str(g))
			v := n.body.eval(ec)
			values = append(values, v)
			joinSel.Add(v.Type())
		}
		joiner := joinSel.Joiner()
		for _, e := range values {
			joiner.Add(e)
		}
		return joiner.Result()
	default:
		raise(&ValueError{
			span: n.iterable.Span(),
			msg:  fmt.Sprintf("cannot loop over %s", iterable.Type()),
		})
		panic("unreachable")
	}
}

func (n *WhileLoop) eval(ec *evalCtx) value.Value {
	panic("TODO: implement while loops")
}

func (n *LoopBreak) eval(ec *evalCtx) value.Value {
	panic("TODO: not actually an expression")
}

func (n *LoopContinue) eval(ec *evalCtx) value.Value {
	panic("TODO: not actually an expression")
}

func (n *FuncReturn) eval(ec *evalCtx) value.Value {
	panic("TODO: not actually an expression")
}

// Other ///////////////////////////////////////////////////////////////////////

func (n *Contextual) eval(ec *evalCtx) value.Value { panic("TODO: implement contextual") }

func (n *ModuleInclude) eval(ec *evalCtx) value.Value { panic("TODO: implement module include") }
