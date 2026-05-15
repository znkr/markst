package eval

import (
	"errors"
	"fmt"
	"maps"

	"znkr.io/writst/builtin"
	"znkr.io/writst/expr"
	"znkr.io/writst/internal/graphemes"
	"znkr.io/writst/internal/joiner"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// Eval evaluates an SSA [Module] and returns the resulting document
// [value.Content]. Errors are returned as an [ErrorList]; non-fatal warnings
// are returned separately. Free names in the module have already been
// resolved to constants by the analyzer, so Eval needs no scope of its own.
func Eval(mod *expr.Module) (c value.Content, warn []Error, err error) {
	s := &session{mod: mod}

	defer func() {
		if r := recover(); r != nil {
			if w, ok := r.(*errWrapper); ok {
				err = ErrorList(w.err)
				warn = s.warnings
				c = nil
			} else {
				panic(r)
			}
		}
	}()

	v := runFunction(s, functionCall{fn: mod.Top})
	cc, cerr := value.ToContent(v)
	if cerr != nil {
		raise(&ValueError{span: syntax.Span{}, msg: cerr.Error()})
	}
	if cc == nil {
		cc = &value.Sequence{}
	}
	c = cc
	warn = s.warnings
	return
}

// session is the [Eval]-wide state shared by every running SSA function: the
// module being evaluated, the document-level label set, and the accumulated
// warning list. Each [frame] holds a back-pointer to its session so eval
// helpers don't have to thread it as a separate parameter.
type session struct {
	mod      *expr.Module
	labels   map[name.Name]struct{}
	warnings []Error
}

func (s *session) warn(w Error) {
	if len(s.warnings) > 0 && s.warnings[len(s.warnings)-1].Span() == w.Span() {
		return
	}
	s.warnings = append(s.warnings, w)
}

// frame is the per-call SSA evaluation state.
type frame struct {
	s    *session
	fn   *expr.Function
	vals []value.Value

	// pred is the ID of the predecessor block, used to resolve phi nodes.
	pred expr.BlockID

	// iters holds the runtime state for iterator-producing instructions
	// (IterOpen). Keyed by the Ref of the IterOpen, since *iteratorState can't
	// satisfy [value.Value]'s unexported aValue() method.
	iters map[expr.Ref]*iteratorState
}

// get returns the value stored for ref, following [DefRedirect] chains
// installed by trivial-phi elimination.
func (fr *frame) get(ref expr.Ref) value.Value {
	ref = fr.fn.Resolve(ref)
	if ref == expr.NoRef {
		return nil
	}
	return fr.vals[ref]
}

// functionCall bundles the arguments for [runFunction] into a single struct
// to make call sites self-documenting.
type functionCall struct {
	// fn is the SSA function to execute.
	fn *expr.Function
	// args carries positional argument values in declaration order (with a sink
	// slot when the function has one).
	args []value.Value
	// captures carries the values supplied at MakeClosure time.
	captures []value.Value
	// self is the [value.Function] for the currently-executing closure, used to
	// materialise [expr.DefSelf] references. Nil for top-level functions.
	self value.Value
}

// runFunction drives the block dispatch loop for one [Function] invocation.
func runFunction(s *session, call functionCall) value.Value {
	fn := call.fn
	args, captures, self := call.args, call.captures, call.self
	fr := &frame{s: s, fn: fn, vals: make([]value.Value, len(fn.Defs))}
	// Pre-fill Param, Capture, and Self defs.
	for i, d := range fn.Defs {
		switch d := d.(type) {
		case *expr.DefParam:
			if d.Idx < len(args) {
				fr.vals[i] = args[d.Idx]
			} else {
				fr.vals[i] = value.None{}
			}
		case *expr.DefCapture:
			if d.Idx < len(captures) {
				fr.vals[i] = captures[d.Idx]
			}
		case *expr.DefSelf:
			fr.vals[i] = self
		}
	}

	bb := expr.BlockID(0)
	for {
		block := fn.Blocks[bb]
		// Resolve phis from the predecessor edge.
		for _, phi := range block.Phis {
			for _, op := range phi.Operands() {
				if op.Pred == fr.pred {
					fr.vals[phi.Result()] = fr.get(op.Value)
					break
				}
			}
		}
		// Straight-line instructions.
		for _, inst := range block.Instrs {
			evalInst(fr, inst)
		}
		// Terminator.
		switch t := block.Term.(type) {
		case *expr.Jump:
			fr.pred = bb
			bb = t.Target
		case *expr.Branch:
			cond := fr.get(t.Cond)
			cb, ok := cond.(value.Bool)
			if !ok {
				raise(&ValueError{span: t.Span(), msg: fmt.Sprintf("expected boolean, found %s", cond.Type())})
			}
			fr.pred = bb
			if bool(cb) {
				bb = t.Then
			} else {
				bb = t.Else
			}
		case *expr.Return:
			if t.Value == expr.NoRef {
				return value.None{}
			}
			return fr.get(t.Value)
		case *expr.Unreachable:
			panic("unreachable block reached at runtime")
		default:
			panic(fmt.Sprintf("unknown terminator: %T", t))
		}
	}
}

// evalInst dispatches on instruction kind and writes the result into the
// frame's value table.
func evalInst(fr *frame, inst expr.Instruction) {
	r := inst.Result()
	switch i := inst.(type) {
	case *expr.Const:
		fr.vals[r] = i.Value
	case *expr.Unary:
		v, err := value.UnaryOp(i.Op, fr.get(i.X))
		if err != nil {
			raise(&ValueError{span: i.Span(), msg: err.Error()})
		}
		fr.vals[r] = v
	case *expr.Binary:
		left := fr.get(i.L)
		if left.Type() == types.Bool {
			if i.Op == syntax.And && !bool(left.(value.Bool)) {
				fr.vals[r] = value.Bool(false)
				return
			}
			if i.Op == syntax.Or && bool(left.(value.Bool)) {
				fr.vals[r] = value.Bool(true)
				return
			}
		}
		right := fr.get(i.R)
		v, err := value.BinaryOp(i.Op, left, right)
		if err != nil {
			raise(&ValueError{span: i.Span(), msg: err.Error()})
		}
		fr.vals[r] = v
	case *expr.MakeArray:
		elems := make([]value.Value, 0, len(i.Items))
		for _, it := range i.Items {
			v := fr.get(it.Value)
			if it.Spread {
				switch s := v.(type) {
				case *value.Array:
					elems = append(elems, s.Elems...)
				case value.None:
				default:
					raise(&ValueError{span: it.Span, msg: fmt.Sprintf("cannot spread %s into array", v.Type())})
				}
			} else {
				elems = append(elems, v)
			}
		}
		fr.vals[r] = &value.Array{Elems: elems}
	case *expr.MakeDict:
		dict := new(value.Dict)
		for _, e := range i.Entries {
			if e.Spread {
				switch s := fr.get(e.Value).(type) {
				case *value.Dict:
					for k, v := range s.Elems.All() {
						dict.Elems.Put(k, v)
					}
				case value.None:
				default:
					raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("cannot spread %s into dictionary", s.Type())})
				}
				continue
			}
			keyStr, ok := fr.get(e.Key).(value.Str)
			if !ok {
				raise(&ValueError{span: i.Span(), msg: "dictionary key must be a string"})
			}
			dict.Elems.Put(keyStr, fr.get(e.Value))
		}
		fr.vals[r] = dict
	case *expr.FieldRead:
		fr.vals[r] = evalFieldRead(fr.get(i.Target), i.Field, i.Span(), i.FieldSpan)
	case *expr.Call:
		fr.vals[r] = evalCall(fr, i)
	case *expr.CallSet:
		evalCallSet(fr, i)
		fr.vals[r] = value.None{}
	case *expr.FieldWrite:
		evalFieldWrite(fr, i)
		fr.vals[r] = value.None{}
	case *expr.MakeClosure:
		fr.vals[r] = makeClosureWithFrame(fr, i)
	case *expr.Extract:
		src := fr.get(i.Source)
		arr, ok := src.(*value.Array)
		if !ok {
			raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("cannot destructure values of %s", src.Type())})
		}
		if i.Index >= len(arr.Elems) {
			raise(&ValueError{span: i.Span(), msg: "destructure index out of range"})
		}
		fr.vals[r] = arr.Elems[i.Index]
	case *expr.LengthCheck:
		src := fr.get(i.Source)
		arr, ok := src.(*value.Array)
		if !ok {
			raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("cannot destructure values of %s", src.Type())})
		}
		switch {
		case i.HasSink && len(arr.Elems) < i.Want:
			raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("need at least %d elements, got %d", i.Want, len(arr.Elems))})
		case !i.HasSink && len(arr.Elems) != i.Want:
			raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("need exactly %d elements, got %d", i.Want, len(arr.Elems))})
		}
	case *expr.IterOpen:
		if fr.iters == nil {
			fr.iters = make(map[expr.Ref]*iteratorState)
		}
		fr.iters[r] = newIterator(fr.get(i.Iterable), i.Span())
		// No meaningful value, but slot is non-nil to indicate "live".
		fr.vals[r] = value.None{}
	case *expr.IterHasNext:
		it := fr.iters[i.Iter]
		fr.vals[r] = value.Bool(it.hasNext())
	case *expr.IterAdvance:
		it := fr.iters[i.Iter]
		fr.vals[r] = it.advance()
	case *expr.ContentResult:
		fr.vals[r] = evalContentResult(fr, i)
	case *expr.RaiseError:
		raise(&ValueError{span: i.Span(), msg: i.Msg})
	case *expr.AttachLabel:
		// Coerce the prior content to value.Content, attach the label, emit
		// a warning if overwriting, and register the label.
		v := fr.get(i.Content)
		c, err := value.ToContent(v)
		if err != nil || c == nil {
			// The preceding value isn't content (e.g. a let-binding result of
			// None). Drop the label silently.
			fr.vals[r] = value.None{}
			break
		}
		labelVal := &value.Label{Name: i.Label}
		if old := c.SetLabel(labelVal); old != nil {
			fr.s.warn(&ValueError{
				span:  fr.fn.Defs[fr.fn.Resolve(i.Content)].Span(),
				msg:   "content labelled multiple times",
				hints: []string{"only the last label is used, the rest are ignored"},
			})
			delete(fr.s.labels, old.Name)
		}
		if fr.s.labels == nil {
			fr.s.labels = make(map[name.Name]struct{})
		}
		fr.s.labels[i.Label] = struct{}{}
		// Also overwrite the operand slot so the label is visible to any
		// subsequent consumer that re-fetches it; this keeps the value in
		// sync with the side-effect.
		fr.vals[fr.fn.Resolve(i.Content)] = c
		fr.vals[r] = value.None{}
	case *expr.CodeJoin:
		fr.vals[r] = evalCodeJoin(fr, i)
	case *expr.LoopAccBegin:
		fr.vals[r] = &value.Array{}
	case *expr.LoopAccAdd:
		arr, ok := fr.get(i.Acc).(*value.Array)
		if !ok {
			panic("loop_acc_add: accumulator is not an array")
		}
		arr.Elems = append(arr.Elems, fr.get(i.Item))
		fr.vals[r] = arr
	case *expr.LoopAccResult:
		arr, ok := fr.get(i.Acc).(*value.Array)
		if !ok {
			panic("loop_acc_result: accumulator is not an array")
		}
		fr.vals[r] = joinValues(arr.Elems, i.Span())
	case *expr.Heading:
		fr.vals[r] = &value.Heading{Depth: i.Level, Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.Strong:
		fr.vals[r] = &value.Strong{Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.Emph:
		fr.vals[r] = &value.Emph{Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.Link:
		fr.vals[r] = &value.Link{Dest: i.Dest, Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.RefMarkup:
		if _, ok := fr.s.labels[i.Target]; !ok {
			raise(&ValueError{span: i.Span(), msg: fmt.Sprintf("label `<%s>` does not exist in the document", i.Target.String())})
		}
		v := &value.Ref{Target: i.Target}
		if i.Supplement != expr.NoRef {
			v.Supplement = contentOf(fr.get(i.Supplement), i.Span())
		}
		fr.vals[r] = v
	case *expr.ListItem:
		fr.vals[r] = &value.ListItem{Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.EnumItem:
		fr.vals[r] = &value.EnumItem{Number: i.Number, Body: contentOf(fr.get(i.Body), i.Span())}
	case *expr.TermItem:
		fr.vals[r] = &value.TermItem{
			Term:        contentOf(fr.get(i.Term), i.Span()),
			Description: contentOf(fr.get(i.Description), i.Span()),
		}
	case *expr.SetRule, *expr.ShowRule, *expr.Contextual, *expr.ModuleInclude:
		panic(fmt.Sprintf("TODO: ssa eval %T", i))
	default:
		panic(fmt.Sprintf("ssa eval not implemented: %T", inst))
	}
}

// contentOf coerces a value to content, raising a ValueError on failure.
func contentOf(v value.Value, span syntax.Span) value.Content {
	c, err := value.ToContent(v)
	if err != nil {
		raise(&ValueError{span: span, msg: err.Error()})
	}
	if c == nil {
		return &value.Sequence{}
	}
	return c
}

// evalCodeJoin runs the code-mode joiner over a list of value Refs and
// returns the joined result. Per-item spans are used so type-mismatch
// errors point at the offending value (matching legacy behaviour).
func evalCodeJoin(fr *frame, c *expr.CodeJoin) value.Value {
	type item struct {
		v    value.Value
		span syntax.Span
	}
	var items []item
	for i, r := range c.Items {
		if r == expr.NoRef {
			continue
		}
		v := fr.get(r)
		if v == nil {
			continue
		}
		sp := c.Span()
		if i < len(c.ItemSpans) {
			sp = c.ItemSpans[i]
		}
		items = append(items, item{v: v, span: sp})
	}
	var sel joiner.Selector
	for _, it := range items {
		if err := sel.Add(it.v.Type()); err != nil {
			raise(&ValueError{span: it.span, msg: err.Error()})
		}
	}
	j := sel.Joiner()
	for _, it := range items {
		if err := j.Add(it.v); err != nil {
			raise(&ValueError{span: it.span, msg: err.Error()})
		}
	}
	return j.Result()
}

// joinValues runs the code-mode joiner over a slice of values. Returns the
// joined result, raising a ValueError on incompatible types.
func joinValues(values []value.Value, span syntax.Span) value.Value {
	var sel joiner.Selector
	for _, v := range values {
		if v == nil {
			continue
		}
		if err := sel.Add(v.Type()); err != nil {
			raise(&ValueError{span: span, msg: err.Error()})
		}
	}
	j := sel.Joiner()
	for _, v := range values {
		if v == nil {
			continue
		}
		if err := j.Add(v); err != nil {
			raise(&ValueError{span: span, msg: err.Error()})
		}
	}
	return j.Result()
}

// evalContentResult joins a list of value-producing items into a Sequence,
// applying labels to the preceding content element (mirroring evalContents).
func evalContentResult(fr *frame, c *expr.ContentResult) value.Value {
	ret := make([]value.Content, 0, len(c.Items))
	var lastSpan syntax.Span
	for _, r := range c.Items {
		v := fr.get(r)
		if lbl, ok := v.(*value.Label); ok {
			if len(ret) == 0 {
				continue
			}
			if old := ret[len(ret)-1].SetLabel(lbl); old != nil {
				fr.s.warn(&ValueError{
					span:  lastSpan,
					msg:   "content labelled multiple times",
					hints: []string{"only the last label is used, the rest are ignored"},
				})
				delete(fr.s.labels, old.Name)
			}
			if fr.s.labels == nil {
				fr.s.labels = make(map[name.Name]struct{})
			}
			fr.s.labels[lbl.Name] = struct{}{}
			continue
		}
		cv, err := value.ToContent(v)
		if err != nil {
			raise(&ValueError{span: c.Span(), msg: err.Error()})
		}
		if cv == nil {
			continue
		}
		ret = append(ret, cv)
		// Use the producing instruction's span so warnings/errors against
		// the most-recent content point at the right source location.
		resolved := fr.fn.Resolve(r)
		if resolved != expr.NoRef {
			lastSpan = fr.fn.Defs[resolved].Span()
		} else {
			lastSpan = c.Span()
		}
	}
	if len(ret) == 1 {
		return ret[0]
	}
	return &value.Sequence{Children: ret}
}

// evalFieldRead replicates the legacy FieldAccess.eval logic for SSA: type
// methods, module definitions, dict keys, and content fields are all
// supported. span covers the whole `target.field` expression and is used
// for type-level errors; fieldSpan covers just the `.field` portion and is
// used for content-field errors.
func evalFieldRead(target value.Value, fname name.Name, span, fieldSpan syntax.Span) value.Value {
	switch t := target.(type) {
	case *value.Type:
		ms := builtin.TypeFields[t.Reflected]
		f := ms[fname]
		if f == nil {
			raise(&ValueError{span: span, msg: fmt.Sprintf("type %s has no method `%s`", t.Reflected, fname.String())})
		}
		return f
	case *value.Module:
		def := t.Definitions[fname]
		if def == nil {
			raise(&ValueError{span: fieldSpan, msg: fmt.Sprintf("module %s has no definition `%s`", t.Name, fname.String())})
		}
		return def
	}
	if f := builtin.TypeFields[target.Type()][fname]; f != nil {
		switch f := f.(type) {
		case *value.Function:
			fn, err := f.With(&value.Arguments{Positional: []value.Value{target}})
			if err != nil {
				if fcerr, ok := err.(*value.FunctionCallError); ok {
					raise(&ValueError{span: fieldSpan, msg: fcerr.Msg, hints: fcerr.Hints})
				}
				raise(&ValueError{span: fieldSpan, msg: err.Error()})
			}
			return fn
		default:
			return f
		}
	}
	switch t := target.(type) {
	case *value.Dict:
		if val, ok := t.Elems.Get(value.Str(fname.String())); ok {
			return val
		}
		raise(&ValueError{span: fieldSpan, msg: fmt.Sprintf("dictionary does not have an entry %q", fname.String())})
	case value.Content:
		f := t.Field(fname)
		if f == nil {
			raise(&ValueError{span: fieldSpan, msg: fmt.Sprintf("content does not have field %q", fname.String())})
		}
		return f
	}
	raise(&ValueError{span: span, msg: fmt.Sprintf("type %s has no method `%s`", target.Type(), fname.String())})
	panic("unreachable")
}

// resolveCallee unwraps a callee value into the underlying function. Raises
// a value error if the value is not callable.
func resolveCallee(callee value.Value, span syntax.Span) *value.Function {
	switch cc := callee.(type) {
	case *value.Function:
		return cc
	case *value.Type:
		if cc.Constructor == nil {
			raise(&ValueError{span: span, msg: fmt.Sprintf("type %s is not callable", cc.Reflected)})
		}
		return cc.Constructor
	default:
		raise(&ValueError{span: span, msg: fmt.Sprintf("attempted to call a non-function value of type %s", callee.Type())})
	}
	panic("unreachable")
}

// buildCallArgs evaluates a SSA call's argument list and trailing content
// blocks into a runtime [value.Arguments].
func buildCallArgs(fr *frame, callSpan syntax.Span, callArgs []expr.CallArg, blocks []expr.Ref) value.Arguments {
	var args value.Arguments
	for _, a := range callArgs {
		if a.Value == expr.NoRef {
			continue
		}
		switch a.Kind {
		case expr.ArgPositional:
			args.Positional = append(args.Positional, fr.get(a.Value))
		case expr.ArgNamed:
			if args.Named == nil {
				args.Named = make(map[name.Name]value.Value)
			}
			args.Named[a.Name] = fr.get(a.Value)
		case expr.ArgSpread:
			v := fr.get(a.Value)
			switch sv := v.(type) {
			case *value.Array:
				args.Positional = append(args.Positional, sv.Elems...)
			case *value.Dict:
				if args.Named == nil {
					args.Named = make(map[name.Name]value.Value)
				}
				for k, vv := range sv.Elems.All() {
					args.Named[name.Make(string(k))] = vv
				}
			case *value.Arguments:
				args.Positional = append(args.Positional, sv.Positional...)
				if len(sv.Named) > 0 {
					if args.Named == nil {
						args.Named = make(map[name.Name]value.Value)
					}
					maps.Copy(args.Named, sv.Named)
				}
			case value.None:
			default:
				raise(&ValueError{span: callSpan, msg: fmt.Sprintf("cannot spread %s", v.Type())})
			}
		}
	}
	for _, b := range blocks {
		args.Positional = append(args.Positional, fr.get(b))
	}
	return args
}

// raiseApplyErr unwraps a function-call error and re-raises one [ValueError]
// per inner error, attributed to the source span of the offending argument
// (or callSpan when the location can't be matched).
func raiseApplyErr(fn *value.Function, callSpan syntax.Span, callArgs []expr.CallArg, err error) {
	var inErrs []error
	if wrapped, ok := err.(interface{ Unwrap() []error }); ok {
		inErrs = wrapped.Unwrap()
	} else {
		inErrs = []error{err}
	}
	var errs []Error
	for _, ie := range inErrs {
		if fcerr, ok := errors.AsType[*value.FunctionCallError](ie); ok {
			errs = append(errs, &ValueError{
				span:  locateArgErrSpan(fn, callSpan, callArgs, fcerr.Location),
				msg:   fcerr.Msg,
				hints: fcerr.Hints,
			})
		} else {
			errs = append(errs, &ValueError{span: callSpan, msg: ie.Error()})
		}
	}
	raise(errs...)
}

// evalCall lowers an SSA Call instruction to a [value.Function.Apply].
func evalCall(fr *frame, c *expr.Call) value.Value {
	fn := resolveCallee(fr.get(c.Callee), c.Span())
	args := buildCallArgs(fr, c.Span(), c.Args, c.Blocks)

	// Warn when a direct float literal is passed to decimal().
	if fn.Name == "decimal" && len(c.Args) > 0 && c.Args[0].DirectFloatLit {
		if f, ok := fr.get(c.Args[0].Value).(value.Float); ok {
			fr.s.warn(&ValueError{
				span:  c.Args[0].Span,
				msg:   "creating a decimal using imprecise float literal",
				hints: []string{"use a string in the decimal constructor to avoid loss of precision: `decimal(\"" + f.String() + "\")`"},
			})
		}
	}

	fcc := value.FunctionCallContext{Span: c.Span()}
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		raiseApplyErr(fn, c.Span(), c.Args, err)
	}
	return v
}

// evalCallSet implements lvalue-style assignment to a function call (e.g.
// `arr.at(i) = v`, `dict.at("k") += 1`). The callee is invoked with a
// [FunctionCallContext.Setter] pointer; if the function registers a setter,
// it is invoked with the (possibly op-combined) new value.
func evalCallSet(fr *frame, c *expr.CallSet) {
	fn := resolveCallee(fr.get(c.Callee), c.Span())
	args := buildCallArgs(fr, c.Span(), c.Args, c.Blocks)

	var setter func(value.Value)
	fcc := value.FunctionCallContext{Span: c.Span(), Setter: &setter}
	cur, err := fn.Apply(&fcc, &args)
	if err != nil {
		raiseApplyErr(fn, c.Span(), c.Args, err)
	}
	if setter == nil {
		raise(&ValueError{span: c.Span(), msg: "cannot mutate a temporary value"})
	}
	newVal := fr.get(c.NewVal)
	if c.Op != syntax.Assign {
		combined, err := value.BinaryOp(c.Op, cur, newVal)
		if err != nil {
			raise(&ValueError{span: c.Span(), msg: err.Error()})
		}
		newVal = combined
	}
	setter(newVal)
}

// evalFieldWrite implements `x.f = v` (and compound forms). For now it
// supports writing to dictionary fields; other field-writes (e.g. content
// fields) are uncommon as lvalues and will raise an error.
func evalFieldWrite(fr *frame, w *expr.FieldWrite) {
	target := fr.get(w.Target)
	newVal := fr.get(w.NewVal)
	switch t := target.(type) {
	case *value.Dict:
		key := value.Str(w.Field.String())
		if w.Op != syntax.Assign {
			cur, ok := t.Elems.Get(key)
			if !ok {
				raise(&ValueError{span: w.Span(), msg: fmt.Sprintf("dictionary does not have an entry %q", w.Field.String())})
			}
			combined, err := value.BinaryOp(w.Op, cur, newVal)
			if err != nil {
				raise(&ValueError{span: w.Span(), msg: err.Error()})
			}
			newVal = combined
		}
		t.Elems.Put(key, newVal)
	default:
		raise(&ValueError{span: w.Span(), msg: fmt.Sprintf("cannot assign to field of %s", target.Type())})
	}
}

// locateArgErrSpan walks the call's CallArg slice to find the source span
// matching the failed-argument location reported by [value.Function.Apply].
// Returns the call's span as a fallback when no match is found.
func locateArgErrSpan(fn *value.Function, callSpan syntax.Span, callArgs []expr.CallArg, loc value.ArgLoc) syntax.Span {
	if loc == nil {
		return callSpan
	}
	i := 0
	if fn.WithArgs != nil {
		i = len(fn.WithArgs.Positional)
	}
	for _, a := range callArgs {
		switch a.Kind {
		case expr.ArgPositional:
			if pos, ok := loc.(value.ArgLocPositional); ok && i == int(pos) {
				return a.Span
			}
			i++
		case expr.ArgNamed:
			switch lo := loc.(type) {
			case value.ArgLocNamed:
				if name.Name(lo) == a.Name {
					return a.Span
				}
			case value.ArgLocNamedPair:
				if name.Name(lo) == a.Name {
					return a.PairSpan
				}
			}
		}
	}
	return callSpan
}

// makeClosureWithFrame implements MakeClosure: it captures the closure's
// nested Function and the capture values, then hands the resulting F to the
// runtime to dispatch when invoked. Self-reference is handled by [expr.DefSelf]
// at call time, not by patching captures.
func makeClosureWithFrame(fr *frame, m *expr.MakeClosure) value.Value {
	innerFn := fr.s.mod.Functions[m.Func]
	caps := make([]value.Value, len(m.Captures))
	for i, r := range m.Captures {
		caps[i] = fr.get(r)
	}
	return buildFunctionValue(fr.s, innerFn, caps)
}

// buildFunctionValue creates a [value.Function] whose F invokes the SSA
// function with the supplied captures and an arg-slot layout matching the
// function's Params.
//
// fn.Params can interleave positional, named, and sink kinds in source order,
// but [value.Function.Positional] only carries positional and sink params. We
// build a posToParam mapping so the runtime can route each value.Function arg
// slot back to the correct fn.Params index.
func buildFunctionValue(s *session, fn *expr.Function, caps []value.Value) *value.Function {
	out := &value.Function{Name: fn.Name}
	posToParam := make([]int, 0, len(fn.Params))
	var sinkIdx *int
	for i, p := range fn.Params {
		switch p.Kind {
		case expr.ParamPositional:
			out.Positional = append(out.Positional, value.Param{
				Name: p.Name.String(),
				Type: types.Any,
			})
			posToParam = append(posToParam, i)
		case expr.ParamSink:
			idx := len(out.Positional)
			sinkIdx = &idx
			out.Positional = append(out.Positional, value.Param{
				Name: p.Name.String(),
				Type: types.SetOf(types.Arguments),
			})
			posToParam = append(posToParam, i)
		case expr.ParamNamed:
			if out.Named == nil {
				out.Named = make(map[name.Name]value.Param)
			}
			// Defaults are captured from the outer scope at MakeClosure time
			// and arrive as DefCapture entries.
			var defaultVal value.Value
			if p.Default != expr.NoRef && int(p.Default) < len(fn.Defs) {
				if d, ok := fn.Defs[p.Default].(*expr.DefCapture); ok && d.Idx < len(caps) {
					defaultVal = caps[d.Idx]
				}
			}
			out.Named[p.Name] = value.Param{
				Name:    p.Name.String(),
				Type:    types.Any,
				Default: defaultVal,
			}
		}
	}
	if sinkIdx != nil {
		out.Sink = sinkIdx
	}
	out.F = func(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
		paramArgs := make([]value.Value, len(fn.Params))
		for k, v := range args {
			paramArgs[posToParam[k]] = v
		}
		for i, p := range fn.Params {
			if p.Kind == expr.ParamNamed {
				paramArgs[i] = named.Get(p.Name)
			}
		}
		return runFunction(s, functionCall{fn: fn, args: paramArgs, captures: caps, self: out}), nil
	}
	return out
}

// Iterator state /////////////////////////////////////////////////////////////
//
// SSA's IterOpen/IterHasNext/IterAdvance instructions need a runtime
// iterator backing store. [value.Value] is a closed interface, so we keep
// the state in a sidecar map ([ssaFrame.iters]) instead.

type iteratorState struct {
	span syntax.Span
	// One of the four populated per kind:
	arr    *value.Array
	arrPos int
	dict   *value.Dict
	keys   []value.Str
	dictI  int
	str    []string // grapheme-segmented string; consumed front-to-back
	strI   int
}

func newIterator(v value.Value, span syntax.Span) *iteratorState {
	s := &iteratorState{span: span}
	if v == nil {
		raise(&ValueError{span: span, msg: "cannot loop over uninitialised value"})
	}
	switch v := v.(type) {
	case *value.Array:
		s.arr = v
	case *value.Dict:
		s.dict = v
		for k := range v.Elems.All() {
			s.keys = append(s.keys, k)
		}
	case value.Str:
		for g := range graphemes.Graphemes(string(v)) {
			s.str = append(s.str, g)
		}
	default:
		raise(&ValueError{span: span, msg: fmt.Sprintf("cannot loop over %s", v.Type())})
	}
	return s
}

func (s *iteratorState) hasNext() bool {
	switch {
	case s.arr != nil:
		return s.arrPos < len(s.arr.Elems)
	case s.dict != nil:
		return s.dictI < len(s.keys)
	default:
		return s.strI < len(s.str)
	}
}

func (s *iteratorState) advance() value.Value {
	switch {
	case s.arr != nil:
		v := s.arr.Elems[s.arrPos]
		s.arrPos++
		return v
	case s.dict != nil:
		k := s.keys[s.dictI]
		s.dictI++
		v, _ := s.dict.Elems.Get(k)
		return &value.Array{Elems: []value.Value{k, v}}
	default:
		g := s.str[s.strI]
		s.strI++
		return value.Str(g)
	}
}
