package eval

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

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
func Eval(mod *expr.Module) (c value.Content, warn []Error, err []Error) {
	s := &session{mod: mod}

	v := runFunction(s, functionCall{fn: mod.Top})
	// If the top-level value is itself an Error, the failure was already
	// recorded on the session; drop it and emit empty content.
	if _, ok := value.IsError(v); ok {
		v = &value.Sequence{}
	}
	cc, cerr := value.ToContent(v)
	// Only surface a content-coercion failure if no other errors were
	// recorded during eval. When other errors exist, the "non-content
	// value" diagnostic is almost always a cascading consequence and
	// would just add noise.
	if cerr != nil && len(s.errors) == 0 {
		s.recordError(&value.Error{Msg: cerr.Error()})
	}
	if cc == nil {
		cc = &value.Sequence{}
	}
	c = cc
	warn = s.warnings
	err = s.errors
	return
}

// session is the [Eval]-wide state shared by every running SSA function: the
// module being evaluated, the document-level label set, and the accumulated
// warning list. Each [frame] holds a back-pointer to its session so eval
// helpers don't have to thread it as a separate parameter.
type session struct {
	mod    *expr.Module
	labels map[name.Name]struct{}

	// warnings collects informal diagnostics produced during evaluation.
	warnings []Error

	// errors collects every diagnostic produced during evaluation. Every
	// failure surfaces here via [session.recordError]; instructions that
	// fail also write a [*value.Error] to their SSA result slot so
	// downstream operations can propagate it through the value table.
	//
	// Sorted by (Span.Start ascending, Span.End descending). Insertions
	// almost always land at the end (errors arrive in roughly source
	// order), so the slice-shift cost amortizes; binary search keeps the
	// containment lookup off the hot path.
	errors []Error
}

// recordError inserts e into s.errors in sorted order unless an existing error
// with the same message already covers it (i.e. the existing span contains
// e's span). This collapses two kinds of redundancy:
//
//   - Exact duplicates: a comparator or predicate firing repeatedly on a
//     poisoned value produces the same (span, msg) over and over.
//   - Narrower restatements: destructure lowering emits a LengthCheck (span
//     over the whole pattern) and per-Extract instructions (narrower spans
//     inside the pattern). When the value isn't destructurable, all fire
//     with the same message; only the outer one is kept.
//
// Implementation: errors is sorted by (Span.Start asc, Span.End desc). Binary
// search locates the insertion point in O(log n); the cover check then
// scans earlier entries (Start <= e.Span.Start) backward. Any error that
// covers e must appear before the insertion point in this ordering, so the
// scan only needs to walk that prefix. In practice it terminates within a
// handful of steps — at most the source nesting depth.
func (s *session) recordError(e Error) {
	if len(s.errors) == 0 {
		s.errors = append(s.errors, e)
		return
	}
	idx, _ := slices.BinarySearchFunc(s.errors, e, errCmp)
	// Scan backward from idx to find an entry that covers e. Two cases:
	//   - Same-span entries: leftmost-insertion semantics puts identical-key
	//     entries *at* idx (and beyond), so the existing duplicate sits at
	//     idx itself when idx < len.
	//   - Broader-span entries: earlier in the slice (Start <= e.Span.Start
	//     by sort order); End >= e.Span.End determines coverage.
	// min(idx, len-1) starts at the duplicate cluster for case 1, or the
	// last entry for case 2 (append at end).
	for i := min(idx, len(s.errors)-1); i >= 0; i-- {
		ex := s.errors[i]
		if ex.Span.Start <= e.Span.Start && ex.Span.End >= e.Span.End && ex.Msg == e.Msg {
			return
		}
	}
	s.errors = slices.Insert(s.errors, idx, e)
}

// recordWarning appends w to the session's warning list, deduplicating
// consecutive warnings at the same span. Warnings are non-fatal: they are
// returned to the caller alongside the document content rather than
// aborting evaluation.
func (s *session) recordWarning(w Error) {
	if len(s.warnings) > 0 && s.warnings[len(s.warnings)-1].Span == w.Span {
		return
	}
	s.warnings = append(s.warnings, w)
}

// errCmp orders errors by Span.Start ascending, then by Span.End
// descending so broader spans come before narrower ones at the same Start.
// Returns the three-way comparison expected by [slices.BinarySearchFunc].
func errCmp(a, b Error) int {
	if c := cmp.Compare(a.Span.Start, b.Span.Start); c != 0 {
		return c
	}
	return cmp.Compare(b.Span.End, a.Span.End)
}

// propagatesFromOperands reports whether [evalInst]'s pre-dispatch
// propagation should short-circuit this instruction when any operand is a
// [*value.Error]. Most value-producing instructions opt in; aggregation
// constructs ([ContentResult], [CodeJoin], [LoopAccAdd], [MakeArray],
// [MakeDict]) opt out because their semantics are to *hold* the operands,
// errors and all, rather than collapse to a single error. Without this
// opt-out, `(err, x)` would evaluate to err instead of an array
// containing err — defeating "continue past errors" for collections.
func propagatesFromOperands(inst expr.Instruction) bool {
	switch inst.(type) {
	case *expr.ContentResult, *expr.CodeJoin, *expr.LoopAccAdd,
		*expr.MakeArray, *expr.MakeDict:
		return false
	}
	return true
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

// get returns the value stored for ref. Module-constant refs are looked up
// in the session's [expr.Module.Constants] pool. The builder rewrites every
// operand through the trivial-phi rename map at [expr.Builder.Finalize]
// time, so no further resolution is needed here.
func (fr *frame) get(ref expr.Ref) value.Value {
	if ref.IsModConst() {
		return fr.s.mod.Constants[ref.ModConstID()]
	}
	if ref == expr.NoRef {
		return value.None{}
	}
	return fr.vals[ref]
}

// error constructs a [*value.Error] for msg at span, records it on the
// session, and returns it. Instruction handlers assign the returned error
// to their result slot (so downstream operations propagate it) and stop
// processing the current instruction. Replaces the previous panic-based
// raise mechanism.
func (fr *frame) error(span syntax.Span, msg string, hints ...string) *value.Error {
	ve := &value.Error{Span: span, Msg: msg, Hints: hints}
	fr.s.recordError(ve)
	return ve
}

// errorf is a formatting wrapper around [frame.error].
func (fr *frame) errorf(span syntax.Span, format string, args ...any) *value.Error {
	return fr.error(span, fmt.Sprintf(format, args...))
}

// warn records a non-fatal diagnostic on the session. Unlike [frame.error],
// it does not write a [*value.Error] to the value table, so evaluation of
// the surrounding expression continues normally.
func (fr *frame) warn(span syntax.Span, msg string, hints ...string) {
	ve := &value.Error{Span: span, Msg: msg, Hints: hints}
	fr.s.recordWarning(ve)
}

// attachLabel binds lbl to c, registering it in the session's label set
// and warning at warnSpan if c was already labelled. Used by both
// [expr.AttachLabel] (explicit `<label>` markup) and [evalContentResult]
// (labels that appear as siblings to content in a markup body).
func (fr *frame) attachLabel(c value.Content, lbl *value.Label, warnSpan syntax.Span) {
	if old := c.SetLabel(lbl); old != nil {
		fr.warn(warnSpan, "content labelled multiple times",
			"only the last label is used, the rest are ignored")
		delete(fr.s.labels, old.Name)
	}
	if fr.s.labels == nil {
		fr.s.labels = make(map[name.Name]struct{})
	}
	fr.s.labels[lbl.Name] = struct{}{}
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
	// materialize [expr.Function.SelfRef] reads. Nil for top-level functions.
	self value.Value
}

// runFunction drives the block dispatch loop for one [Function] invocation.
func runFunction(s *session, call functionCall) value.Value {
	fn := call.fn
	args, captures, self := call.args, call.captures, call.self
	fr := &frame{s: s, fn: fn, vals: make([]value.Value, fn.NumRefs)}
	// Pre-fill parameter, capture, and self slots.
	for i, p := range fn.Params {
		fr.vals[p.Ref] = args[i]
	}
	for i, r := range fn.CaptureRefs {
		fr.vals[r] = captures[i]
	}
	if fn.SelfRef != expr.NoRef {
		fr.vals[fn.SelfRef] = self
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
			// If the condition is itself an error value, the upstream
			// computation already recorded it. Pick the Else arm so
			// evaluation can continue *and* loops terminate (while/for
			// lower with the exit block as Else, so this drops out of the
			// loop instead of re-evaluating the failing condition every
			// iteration). The surrounding expression's result will reflect
			// the Else arm; routing Error through the join phi properly
			// requires a follow-up analyzer change.
			if _, ok := value.IsError(cond); ok {
				fr.pred = bb
				bb = t.Else
				break
			}
			cb, ok := cond.(value.Bool)
			if !ok {
				// Non-bool condition: record the failure and fall through
				// to the Else arm. This terminates loops (their exit edge
				// is Else) and skips the Then arm of conditionals.
				fr.errorf(t.Span(), "expected boolean, found %s", cond.Type())
				fr.pred = bb
				bb = t.Else
				break
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
//
// Error propagation: if any operand is a [*value.Error], opted-in
// instructions short-circuit and write the same error to the result slot
// without re-recording it on the session. Each handler that can fail uses
// [frame.fail] to record + return a [*value.Error] that gets assigned to
// the result; downstream instructions then propagate it via the same
// mechanism. ContentResult, CodeJoin, and LoopAccAdd opt out of operand
// propagation so they can produce a partial result instead.
func evalInst(fr *frame, inst expr.Instruction) {
	r := inst.Result()

	if propagatesFromOperands(inst) {
		for _, op := range inst.Operands() {
			if e, ok := value.IsError(fr.get(op)); ok {
				if r != expr.NoRef {
					fr.vals[r] = e
				}
				return
			}
		}
	}

	switch i := inst.(type) {
	case *expr.Const:
		fr.vals[r] = i.Value
	case *expr.Unary:
		v, err := value.UnaryOp(i.Op, fr.get(i.X))
		if err != nil {
			fr.vals[r] = fr.error(i.Span(), err.Error())
			return
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
			fr.vals[r] = fr.error(i.Span(), err.Error())
			return
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
					fr.vals[r] = fr.errorf(it.Span, "cannot spread %s into array", v.Type())
					return
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
					fr.vals[r] = fr.error(i.Span(), fmt.Sprintf("cannot spread %s into dictionary", s.Type()))
					return
				}
				continue
			}
			keyStr, ok := fr.get(e.Key).(value.Str)
			if !ok {
				fr.vals[r] = fr.error(i.Span(), "dictionary key must be a string")
				return
			}
			dict.Elems.Put(keyStr, fr.get(e.Value))
		}
		fr.vals[r] = dict
	case *expr.FieldRead:
		fr.vals[r] = fr.evalFieldRead(fr.get(i.Target), i.Field, i.Span(), i.FieldSpan)
	case *expr.Call:
		fr.vals[r] = fr.evalCall(i)
	case *expr.CallSet:
		fr.evalCallSet(i)
	case *expr.FieldWrite:
		fr.evalFieldWrite(i)
	case *expr.MakeClosure:
		fr.vals[r] = makeClosureWithFrame(fr, i)
	case *expr.Extract:
		src := fr.get(i.Source)
		arr, ok := src.(*value.Array)
		if !ok {
			fr.vals[r] = fr.error(i.Span(), fmt.Sprintf("cannot destructure values of %s", src.Type()))
			return
		}
		if i.Index >= len(arr.Elems) {
			fr.vals[r] = fr.error(i.Span(), "destructure index out of range")
			return
		}
		fr.vals[r] = arr.Elems[i.Index]
	case *expr.LengthCheck:
		src := fr.get(i.Source)
		arr, ok := src.(*value.Array)
		if !ok {
			fr.error(i.Span(), fmt.Sprintf("cannot destructure values of %s", src.Type()))
			return
		}
		switch {
		case i.HasSink && len(arr.Elems) < i.Want:
			fr.error(i.Span(), fmt.Sprintf("need at least %d elements, got %d", i.Want, len(arr.Elems)))
		case !i.HasSink && len(arr.Elems) != i.Want:
			fr.error(i.Span(), fmt.Sprintf("need exactly %d elements, got %d", i.Want, len(arr.Elems)))
		}
	case *expr.IterOpen:
		state, e := fr.newIterator(fr.get(i.Iterable), i.Span())
		if e != nil {
			fr.vals[r] = e
			return
		}
		if fr.iters == nil {
			fr.iters = make(map[expr.Ref]*iteratorState)
		}
		fr.iters[r] = state
		// No meaningful value, but slot is non-nil to indicate "live".
		fr.vals[r] = value.None{}
	case *expr.IterHasNext:
		it := fr.iters[i.Iter]
		fr.vals[r] = value.Bool(it.hasNext)
	case *expr.IterAdvance:
		it := fr.iters[i.Iter]
		fr.vals[r] = it.advance()
	case *expr.ContentResult:
		fr.vals[r] = fr.evalContentResult(i)
	case *expr.Error:
		// The cascade-suppression rule (when From is already a *value.Error,
		// propagate it and suppress Msg) is handled by the generic operand-
		// error short-circuit at the top of evalInst; by the time we reach
		// here, From is known to be non-error (or absent), so we always
		// record the Msg.
		fr.vals[r] = fr.error(i.Span(), i.Msg, i.Hints...)
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
		// Use the operand's defining instruction span for the attach
		// diagnostic. Module-const refs have no per-function span — use the
		// AttachLabel's own span instead, and skip the operand-slot writeback
		// (the pool is shared across uses and must stay immutable; the label is
		// still registered on the session).
		var attachSpan syntax.Span
		if i.Content >= 0 && int32(i.Content) < fr.fn.NumRefs {
			attachSpan = fr.fn.RefSpans[i.Content]
		} else {
			attachSpan = i.Span()
		}
		fr.attachLabel(c, &value.Label{Name: i.Label}, attachSpan)
		if i.Content >= 0 && int32(i.Content) < fr.fn.NumRefs {
			fr.vals[i.Content] = c
		}
		fr.vals[r] = value.None{}
	case *expr.CodeJoin:
		fr.vals[r] = fr.evalCodeJoin(i)
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
		fr.vals[r] = fr.joinValues(arr.Elems, i.Span())
	case *expr.Heading:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.Heading{Depth: i.Level, Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.Strong:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.Strong{Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.Emph:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.Emph{Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.Link:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.Link{Dest: i.Dest, Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.RefMarkup:
		if _, ok := fr.s.labels[i.Target]; !ok {
			fr.vals[r] = fr.errorf(i.Span(), "label `<%s>` does not exist in the document", i.Target.String())
			return
		}
		v := &value.Ref{Target: i.Target}
		if i.Supplement != expr.NoRef {
			body, e := fr.contentOf(fr.get(i.Supplement), i.Span())
			if e != nil {
				fr.vals[r] = e
				return
			}
			v.Supplement = body
		}
		fr.vals[r] = v
	case *expr.ListItem:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.ListItem{Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.EnumItem:
		if body, e := fr.contentOf(fr.get(i.Body), i.Span()); e == nil {
			fr.vals[r] = &value.EnumItem{Number: i.Number, Body: body}
		} else {
			fr.vals[r] = e
		}
	case *expr.TermItem:
		term, e := fr.contentOf(fr.get(i.Term), i.Span())
		if e != nil {
			fr.vals[r] = e
			return
		}
		desc, e := fr.contentOf(fr.get(i.Description), i.Span())
		if e != nil {
			fr.vals[r] = e
			return
		}
		fr.vals[r] = &value.TermItem{Term: term, Description: desc}
	case *expr.SetRule, *expr.ShowRule, *expr.Contextual, *expr.ModuleInclude:
		panic(fmt.Sprintf("TODO: ssa eval %T", i))
	default:
		panic(fmt.Sprintf("ssa eval not implemented: %T", inst))
	}
}

// contentOf coerces a value to content. Returns (nil-content, error) when
// the value isn't content-coercible; on success the second return is nil.
// A nil content from value.ToContent is normalized to an empty Sequence.
func (fr *frame) contentOf(v value.Value, span syntax.Span) (value.Content, *value.Error) {
	c, err := value.ToContent(v)
	if err != nil {
		return nil, fr.error(span, err.Error())
	}
	if c == nil {
		return &value.Sequence{}, nil
	}
	return c, nil
}

// evalCodeJoin runs the code-mode joiner over a list of value Refs and
// returns the joined result. Per-item spans are used so type-mismatch
// errors point at the offending value (matching legacy behavior).
//
// Returns (nil, err) when the joiner rejects an operand; (joined, nil)
// otherwise. Items whose value is already a [*value.Error] are skipped so a
// previously-recorded failure doesn't block a partial join; when at least
// one error operand was dropped, a subsequent type-mismatch among the
// remaining operands is also suppressed (the user already has a real
// diagnostic, the cascade adds no information).
func (fr *frame) evalCodeJoin(c *expr.CodeJoin) value.Value {
	type item struct {
		v    value.Value
		span syntax.Span
	}
	var items []item
	dropped := false
	for i, r := range c.Items {
		if r == expr.NoRef {
			continue
		}
		v := fr.get(r)
		if v == nil {
			continue
		}
		if _, ok := value.IsError(v); ok {
			dropped = true
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
			if dropped {
				return value.None{}
			}
			return fr.error(it.span, err.Error())
		}
	}
	j := sel.Joiner()
	for _, it := range items {
		if err := j.Add(it.v); err != nil {
			if dropped {
				return value.None{}
			}
			return fr.error(it.span, err.Error())
		}
	}
	return j.Result()
}

// joinValues runs the code-mode joiner over a slice of values. Returns
// *value.Error on incompatible types. Already-failed values (those carrying
// a [*value.Error] in the slot) are dropped: the underlying error is
// already on the session, and including it would either cascade into
// "cannot join error with …" or replace the real diagnostic with a
// vacuous one. When at least one operand is dropped this way, an
// incompatible-types failure among the remaining operands is also
// suppressed — the user already has a diagnostic to act on and the
// secondary "cannot join X with Y" is just noise about the corrupted
// remainder.
func (fr *frame) joinValues(values []value.Value, span syntax.Span) value.Value {
	dropped := false
	var sel joiner.Selector
	for _, v := range values {
		if v == nil {
			continue
		}
		if _, ok := value.IsError(v); ok {
			dropped = true
			continue
		}
		if err := sel.Add(v.Type()); err != nil {
			if dropped {
				return value.None{}
			}
			return fr.error(span, err.Error())
		}
	}
	j := sel.Joiner()
	for _, v := range values {
		if v == nil {
			continue
		}
		if _, ok := value.IsError(v); ok {
			continue
		}
		if err := j.Add(v); err != nil {
			if dropped {
				return value.None{}
			}
			return fr.error(span, err.Error())
		}
	}
	return j.Result()
}

// evalContentResult joins a list of value-producing items into a Sequence,
// applying labels to the preceding content element (mirroring evalContents).
// Items whose value is a [*value.Error] are skipped: the error is already on
// the session's error list, and dropping the item lets the rest of the
// document still render. Non-content items that can't be coerced are
// surfaced via [frame.fail] and skipped from the output.
func (fr *frame) evalContentResult(c *expr.ContentResult) value.Value {
	ret := make([]value.Content, 0, len(c.Items))
	var lastSpan syntax.Span
	for _, r := range c.Items {
		v := fr.get(r)
		if _, ok := value.IsError(v); ok {
			continue
		}
		if lbl, ok := v.(*value.Label); ok {
			if len(ret) == 0 {
				continue
			}
			fr.attachLabel(ret[len(ret)-1], lbl, lastSpan)
			continue
		}
		cv, err := value.ToContent(v)
		if err != nil {
			fr.error(c.Span(), err.Error())
			continue
		}
		if cv == nil {
			continue
		}
		ret = append(ret, cv)
		// Use the producing instruction's span so warnings/errors against
		// the most-recent content point at the right source location.
		// Module-const refs have no per-function span — fall back to the
		// consuming instruction's span.
		if r >= 0 && int32(r) < fr.fn.NumRefs {
			lastSpan = fr.fn.RefSpans[r]
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
// used for content-field errors. Returns (nil, err) when the access fails.
func (fr *frame) evalFieldRead(target value.Value, fname name.Name, span, fieldSpan syntax.Span) value.Value {
	switch t := target.(type) {
	case *value.Type:
		ms := builtin.TypeFields[t.Reflected]
		f := ms[fname]
		if f == nil {
			return fr.errorf(span, "type %s has no method `%s`", t.Reflected, fname.String())
		}
		return f
	case *value.Module:
		def := t.Definitions[fname]
		if def == nil {
			return fr.errorf(fieldSpan, "module %s has no definition `%s`", t.Name, fname.String())
		}
		return def
	}
	if f := builtin.TypeFields[target.Type()][fname]; f != nil {
		switch f := f.(type) {
		case *value.Function:
			fn, err := f.With(&value.Arguments{Positional: []value.Value{target}})
			if err != nil {
				if fcerr, ok := err.(*value.FunctionCallError); ok {
					return fr.error(fieldSpan, fcerr.Msg, fcerr.Hints...)
				}
				return fr.error(fieldSpan, err.Error())
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
		return fr.errorf(fieldSpan, "dictionary does not have an entry %q", fname.String())
	case value.Content:
		f := t.Field(fname)
		if f == nil {
			return fr.errorf(fieldSpan, "content does not have field %q", fname.String())
		}
		return f
	}
	return fr.errorf(span, "type %s has no method `%s`", target.Type(), fname.String())
}

// resolveCallee unwraps a callee value into the underlying function.
// Returns (nil, err) when the value isn't callable.
func (fr *frame) resolveCallee(callee value.Value, span syntax.Span) (*value.Function, *value.Error) {
	switch cc := callee.(type) {
	case *value.Function:
		return cc, nil
	case *value.Type:
		if cc.Constructor == nil {
			return nil, fr.errorf(span, "type %s is not callable", cc.Reflected)
		}
		return cc.Constructor, nil
	default:
		return nil, fr.errorf(span, "attempted to call a non-function value of type %s", callee.Type())
	}
}

// buildCallArgs evaluates a SSA call's argument list and trailing content
// blocks into a runtime [value.Arguments]. Returns (args, err) where err is
// non-nil when a spread operand has an unsupported type.
func (fr *frame) buildCallArgs(callSpan syntax.Span, callArgs []expr.CallArg, blocks []expr.Ref) (value.Arguments, *value.Error) {
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
				return args, fr.errorf(callSpan, "cannot spread %s", v.Type())
			}
		}
	}
	for _, b := range blocks {
		args.Positional = append(args.Positional, fr.get(b))
	}
	return args, nil
}

// applyErr unwraps a function-call error returned by [value.Function.Apply]
// and records one diagnostic per inner error, each attributed to the source
// span of the offending argument (or callSpan when the location can't be
// matched). Returns a [*value.Error] suitable for assigning to the call's
// result slot (the first recorded error).
func (fr *frame) applyErr(fn *value.Function, callSpan syntax.Span, callArgs []expr.CallArg, err error) *value.Error {
	var inErrs []error
	if wrapped, ok := err.(interface{ Unwrap() []error }); ok {
		inErrs = wrapped.Unwrap()
	} else {
		inErrs = []error{err}
	}
	var first *value.Error
	for _, ie := range inErrs {
		var span syntax.Span
		var msg string
		var hints []string
		if fcerr, ok := errors.AsType[*value.FunctionCallError](ie); ok {
			span = locateArgErrSpan(fn, callSpan, callArgs, fcerr.Location)
			msg = fcerr.Msg
			hints = fcerr.Hints
		} else {
			span = callSpan
			msg = ie.Error()
		}
		ve := fr.error(span, msg, hints...)
		if first == nil {
			first = ve
		}
	}
	return first
}

// evalCall lowers an SSA Call instruction to a [value.Function.Apply].
// Returns (nil, err) when the callee can't be resolved, an argument spread
// fails, or the call itself reports an error.
func (fr *frame) evalCall(c *expr.Call) value.Value {
	fn, e := fr.resolveCallee(fr.get(c.Callee), c.Span())
	if e != nil {
		return e
	}
	args, e := fr.buildCallArgs(c.Span(), c.Args, c.Blocks)
	if e != nil {
		return e
	}

	// Warn when a direct float literal is passed to decimal().
	if fn.Name == "decimal" && len(c.Args) > 0 && c.Args[0].DirectFloatLit {
		if f, ok := fr.get(c.Args[0].Value).(value.Float); ok {
			fr.warn(
				c.Args[0].Span,
				"creating a decimal using imprecise float literal",
				"use a string in the decimal constructor to avoid loss of precision: `decimal(\""+f.String()+"\")`",
			)
		}
	}

	fcc := value.FunctionCallContext{Span: c.Span()}
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		return fr.applyErr(fn, c.Span(), c.Args, err)
	}
	return v
}

// evalCallSet implements lvalue-style assignment to a function call (e.g.
// `arr.at(i) = v`, `dict.at("k") += 1`). The callee is invoked with a
// [FunctionCallContext.Setter] pointer; if the function registers a setter,
// it is invoked with the (possibly op-combined) new value. Returns the
// first error encountered, or none on success.
// evalCallSet implements `f(args) = v` (and compound forms). Side-effect
// only: all error paths record on the session, and the instruction has no
// SSA result.
func (fr *frame) evalCallSet(c *expr.CallSet) {
	fn, e := fr.resolveCallee(fr.get(c.Callee), c.Span())
	if e != nil {
		return
	}
	args, e := fr.buildCallArgs(c.Span(), c.Args, c.Blocks)
	if e != nil {
		return
	}

	var setter func(value.Value)
	fcc := value.FunctionCallContext{Span: c.Span(), Setter: &setter}
	cur, err := fn.Apply(&fcc, &args)
	if err != nil {
		fr.applyErr(fn, c.Span(), c.Args, err)
		return
	}
	if setter == nil {
		fr.error(c.Span(), "cannot mutate a temporary value")
		return
	}
	newVal := fr.get(c.NewVal)
	if c.Op != syntax.Assign {
		combined, err := value.BinaryOp(c.Op, cur, newVal)
		if err != nil {
			fr.error(c.Span(), err.Error())
			return
		}
		newVal = combined
	}
	setter(newVal)
}

// evalFieldWrite implements `x.f = v` (and compound forms). For now it
// supports writing to dictionary fields; other field-writes (e.g. content
// fields) are uncommon as lvalues and record an error. Side-effect only:
// all error paths record on the session, and the instruction has no SSA
// result.
func (fr *frame) evalFieldWrite(w *expr.FieldWrite) {
	target := fr.get(w.Target)
	newVal := fr.get(w.NewVal)
	switch t := target.(type) {
	case *value.Dict:
		key := value.Str(w.Field.String())
		if w.Op != syntax.Assign {
			cur, ok := t.Elems.Get(key)
			if !ok {
				fr.errorf(w.Span(), "dictionary does not have an entry %q", w.Field.String())
				return
			}
			combined, err := value.BinaryOp(w.Op, cur, newVal)
			if err != nil {
				fr.error(w.Span(), err.Error())
				return
			}
			newVal = combined
		}
		t.Elems.Put(key, newVal)
	default:
		fr.errorf(w.Span(), "cannot assign to field of %s", target.Type())
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
// runtime to dispatch when invoked. Self-reference is handled by
// [expr.Function.SelfRef] at call time, not by patching captures.
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
			// and arrive in fn.CaptureRefs — or, for constant defaults, as
			// module-const refs that resolve directly from the pool.
			var defaultVal value.Value
			switch {
			case p.Default == expr.NoRef:
			case p.Default.IsModConst():
				defaultVal = s.mod.Constants[p.Default.ModConstID()]
			default:
				for idx, r := range fn.CaptureRefs {
					if r == p.Default {
						defaultVal = caps[idx]
						break
					}
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

// iteratorState is the runtime cursor for one IterOpen/IterHasNext/IterAdvance
// triple. Only one of the three value fields (arr, dict, str) is non-nil at a
// time, depending on the type of the iterable passed to IterOpen.
type iteratorState struct {
	hasNext bool
	next    func() (value.Value, bool)
}

// newIterator creates an [iteratorState] for the given value. Arrays, dicts,
// and strings are supported; any other type yields an error. For dicts the
// key slice is snapshotted at open time so concurrent mutations don't affect
// iteration order.
func (fr *frame) newIterator(v value.Value, span syntax.Span) (*iteratorState, *value.Error) {
	if v == nil {
		return nil, fr.error(span, "cannot loop over uninitialised value")
	}
	s := &iteratorState{}
	switch val := v.(type) {
	case *value.Array:
		i := 0
		s.hasNext = i < len(val.Elems)
		s.next = func() (value.Value, bool) {
			v := val.Elems[i]
			i = min(i+1, len(val.Elems))
			return v, i < len(val.Elems)
		}
	case *value.Dict:
		i := 0
		keys := val.Elems.UnsafeKeys()
		s.hasNext = i < len(keys)
		s.next = func() (value.Value, bool) {
			k := keys[i]
			i = min(i+1, len(keys))
			v, _ := val.Elems.Get(k)
			return &value.Array{Elems: []value.Value{k, v}}, i < len(keys)
		}
	case value.Str:
		i := 0
		str := string(val)
		s.hasNext = i < len(str)
		s.next = func() (value.Value, bool) {
			r, size := graphemes.Decode(str[i:])
			i = min(i+size, len(str))
			return value.Str(r), i < len(str)
		}
	default:
		return nil, fr.errorf(span, "cannot loop over %s", val.Type())
	}
	return s, nil
}

// advance returns the next element and advances the cursor. Callers must
// check [iteratorState.hasNext] before calling advance.
func (s *iteratorState) advance() value.Value {
	v, hasNext := s.next()
	s.hasNext = hasNext
	return v
}
