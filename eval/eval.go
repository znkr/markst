package eval

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"znkr.io/markst/builtin"
	"znkr.io/markst/expr"
	"znkr.io/markst/internal/graphemes"
	"znkr.io/markst/internal/joiner"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Option configures an [Eval] run.
type Option func(*session)

// WithNow fixes the instant the document is rendered at — what `datetime.today`
// reads the current date off. Without it the system clock is used; passing a
// fixed instant makes a render reproducible.
func WithNow(t time.Time) Option {
	return func(s *session) { s.now = t }
}

// Eval evaluates an SSA [Module] and returns the resulting document
// [value.Content]. Errors are returned as an [ErrorList]; non-fatal warnings
// are returned separately. Free names in the module have already been
// resolved to constants by the analyzer, so Eval needs no scope of its own.
//
// The content it returns is realized: paragraphs formed, style scopes
// resolved, and every heading labelled — see [session.realizeDocument].
func Eval(mod *expr.Module, opts ...Option) (doc *value.Document, warn []Error, err []Error) {
	s, v := runTop(mod, opts)
	// If the top-level value is itself an Error, the failure was already
	// recorded on the session; drop it and emit empty content.
	if _, ok := value.IsError(v); ok {
		v = &value.Sequence{}
	}
	// A top-level markup body of a single item skips the ContentResult
	// assembly (lowerMarkup's single-item shortcut), so a lone/trailing set or
	// show rule reaches here unfolded. Collapse it the same way assembly would.
	v = foldStyles(v)
	cc := value.ToContent(v)
	if cc == nil {
		cc = &value.Sequence{}
	}
	// Turn the recorded content tree into a realized document: paragraphs
	// formed, list/enum/term items grouped, styles resolved, wrapped in a
	// Document root.
	doc = s.realizeDocument(cc)
	warn = s.warnings
	err = s.errors
	return
}

// EvalExports evaluates a module analyzed with [analyzer.WithExports] and
// returns its top-level bindings as values, along with the body content the
// file would have produced. It is how a source file becomes something other
// files can be compiled against: the closures in exports are ordinary
// [value.Function]s, ready to be handed to [analyzer.WithBindings].
//
// The returned closures outlive this call. Each one remembers the module it
// was written in — for its constant pool, its nested functions and its
// [syntax.Origin] — but takes the session it runs under from its caller, so a
// later invocation records its diagnostics on whatever document is being
// compiled at the time.
func EvalExports(mod *expr.Module, opts ...Option) (body value.Value, exports *value.Dict, warn []Error, err []Error) {
	s, v := runTop(mod, opts)
	// The pair analyzer.WithExports emits. Anything else means the module was
	// analyzed without the option, which is a programming error in the host,
	// not something a document can cause — except when the top-level value is
	// poison, where the failure is already recorded and there is nothing to
	// take apart.
	if _, ok := value.IsError(v); !ok {
		pair, ok := v.(*value.Array)
		if !ok || len(pair.Elems) != 2 {
			panic(fmt.Sprintf("eval: EvalExports on a module analyzed without WithExports (top-level value is %T)", v))
		}
		body = foldStyles(pair.Elems[0])
		exports, _ = pair.Elems[1].(*value.Dict)
	}
	if exports == nil {
		exports = &value.Dict{}
	}
	return body, exports, s.warnings, s.errors
}

// runTop applies opts, reports the module's parse errors, and runs its
// top-level function. Shared by [Eval] and [EvalExports], which differ only in
// what they make of the value that comes back.
func runTop(mod *expr.Module, opts []Option) (*session, value.Value) {
	s := &session{}
	for _, opt := range opts {
		opt(s)
	}

	// Parse errors are diagnostics on the source, reported regardless of
	// which code paths run — and this is their only reporting channel: the
	// corresponding IR Error instructions are marked Reported and evaluate to
	// unrecorded poison values.
	for _, e := range mod.ParseErrors {
		e.Origin = mod.Origin
		s.recordError(e)
	}

	return s, runFunction(s, functionCall{mod: mod, fn: mod.Top})
}

// maxCallDepth bounds the number of nested user-closure calls before
// evaluation aborts with "maximum function call depth exceeded", guarding the
// Go stack against unbounded recursion (e.g. `let rec(n) = rec(n) + 1`).
const maxCallDepth = 64

// session is the [Eval]-wide state shared by every running SSA function: the
// document-level label set, the document properties collected from set rules,
// and the accumulated diagnostics. Each [frame] holds a back-pointer to its
// session so eval helpers don't have to thread it as a separate parameter.
//
// A session is per-render, and deliberately holds nothing about the *code*
// being run — that lives on [expr.Module], reached through [frame.mod]. The
// split is what lets a closure defined in one module be called during another
// module's evaluation: the closure keeps its own module for constants, nested
// functions and spans, but takes the running session from its caller, so its
// errors, warnings, labels and `set document` rules land on the document
// actually being compiled.
type session struct {
	labels map[name.Name]struct{}

	// metadataLabels is the subset of labels attached to a [value.Metadata].
	// A label may legitimately be shared by several elements — `#show <x>:`
	// styles every one of them — but two metadata under one label leave
	// znkr.io/markst.Query no way to tell them apart, so that case is warned
	// about; see [frame.attachLabel].
	metadataLabels map[name.Name]struct{}

	// now is the instant the document is rendered at, handed to every builtin
	// through [value.FunctionCallContext]. The zero value means the builtins
	// that care read the system clock themselves.
	now time.Time

	// stack is the chain of user-closure calls currently running, outermost
	// first. Pushed on entry to a closure in [frame.evalCall] and popped on
	// return; when it would exceed [maxCallDepth] the call is refused.
	//
	// It doubles as the source of [value.Error.Trace]: [session.trace] reads
	// off the entries where a call crossed from one [syntax.Origin] into
	// another, which is what tells a reader which document called the library
	// function that failed.
	stack []callSite

	// warnings collects informal diagnostics produced during evaluation.
	warnings []Error

	// doc accumulates the document properties collected from `set document(…)`
	// rules during the realization pass. Body is left alone here; it is filled
	// in by [session.realizeDocument], which returns a copy of this value as
	// the document root.
	doc value.Document

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

// callSite is one entry on [session.stack]: a call in progress, recorded where
// it was written.
type callSite struct {
	// mod is the *caller's* module — span points into its source, since that
	// is where the call expression is written. It also stands for the source's
	// identity in [session.trace]: comparing modules rather than the
	// [syntax.Source] interface keeps that comparison total, since Source is
	// exported and an implementation of it need not be comparable.
	mod    *expr.Module
	span   syntax.Span
	callee string
}

// trace returns the call sites that crossed a module boundary on the way into
// mod, outermost first. Calls within a single module contribute nothing: a
// failure two frames deep in the same file already points at the right line,
// and listing every frame would bury the one boundary crossing that matters
// under up to [maxCallDepth] notes.
func (s *session) trace(mod *expr.Module) []syntax.Frame {
	var frames []syntax.Frame
	inner := mod
	for i := len(s.stack) - 1; i >= 0; i-- {
		c := s.stack[i]
		if c.mod == inner {
			continue
		}
		frames = append(frames, syntax.Frame{Origin: c.mod.Origin, Span: c.span, Callee: c.callee})
		inner = c.mod
	}
	slices.Reverse(frames)
	return frames
}

// recordError inserts e into s.errors in sorted order unless an existing error
// with the same message already covers it (i.e. the existing span contains
// e's span). This collapses two kinds of redundancy:
//
//   - Exact duplicates: a comparator or predicate firing repeatedly on a
//     poisoned value produces the same (span, msg) over and over.
//   - Narrower restatements: destructure lowering emits a DestructArray
//     (span over the whole pattern) and per-ArrayElem reads (narrower
//     spans inside the pattern). When the value isn't destructurable,
//     all fire with the same message; only the outer one is kept.
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
// constructs ([ContentResult], [CodeJoin], [JoinAdd], [MakeArray],
// [MakeDict]) opt out because their semantics are to *hold* the operands,
// errors and all, rather than collapse to a single error. Without this
// opt-out, `(err, x)` would evaluate to err instead of an array
// containing err — defeating "continue past errors" for collections.
func propagatesFromOperands(inst expr.Instruction) bool {
	switch inst.(type) {
	case *expr.ContentResult, *expr.CodeJoin, *expr.JoinAdd,
		*expr.MakeArray, *expr.MakeDict:
		return false
	}
	return true
}

// frame is the per-call SSA evaluation state.
type frame struct {
	s *session

	// mod is the module fn was compiled from, and the one its Refs and Spans
	// are meaningful in. It is carried per-frame rather than per-session
	// because a single evaluation can run functions from several modules: a
	// closure exported by a library keeps resolving its module constants,
	// nested functions and source positions against the library, while
	// everything else about the call belongs to s.
	mod *expr.Module

	fn   *expr.Function
	vals []value.Value

	// iters holds the runtime state for iterator-producing instructions
	// (IterOpen). Keyed by the Ref of the IterOpen, since *iteratorState can't
	// satisfy [value.Value]'s unexported aValue() method.
	iters map[expr.Ref]*iteratorState
}

// get returns the value stored for ref. Module-constant refs are looked up
// in the frame's [expr.Module.Constants] pool. The builder rewrites every
// operand through the trivial-param rename map at [expr.Builder.Finalize]
// time, so no further resolution is needed here.
func (fr *frame) get(ref expr.Ref) value.Value {
	if ref.IsModConst() {
		return fr.mod.Constants[ref.ModConstID()]
	}
	if ref == expr.NoRef {
		return value.None{}
	}
	return fr.vals[ref]
}

// span returns the source span recorded for a value-producing producer of r.
// r must be a function-local Ref (not [expr.NoRef] or a module-constant).
// Used by handlers that need to attach errors to an instruction by its
// Result Ref now that [expr.instr] no longer carries the span.
func (fr *frame) span(r expr.Ref) syntax.Span { return fr.fn.RefSpans[r] }

// error constructs a [*value.Error] for msg at span, records it on the
// session, and returns it. Instruction handlers assign the returned error
// to their result slot (so downstream operations propagate it) and stop
// processing the current instruction. Replaces the previous panic-based
// raise mechanism.
func (fr *frame) error(span syntax.Span, msg string, hints ...string) *value.Error {
	ve := &value.Error{Span: span, Origin: fr.mod.Origin, Msg: msg, Hints: value.Hints(hints...), Trace: fr.s.trace(fr.mod)}
	fr.s.recordError(ve)
	return ve
}

// errorHints is [frame.error] for a diagnostic whose hints do not all belong
// at the diagnostic's own span.
func (fr *frame) errorHints(span syntax.Span, msg string, hints []value.Hint) *value.Error {
	ve := &value.Error{Span: span, Origin: fr.mod.Origin, Msg: msg, Hints: hints, Trace: fr.s.trace(fr.mod)}
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
	ve := &value.Error{Span: span, Origin: fr.mod.Origin, Msg: msg, Hints: value.Hints(hints...), Trace: fr.s.trace(fr.mod)}
	fr.s.recordWarning(ve)
}

// attachLabel binds lbl to c, registering it in the session's label set
// and warning at warnSpan if c was already labelled. Used by both
// [expr.AttachLabel] (explicit `<label>` markup) and [evalContentResult]
// (labels that appear as siblings to content in a markup body).
//
// This is also where a metadata label is checked for reuse. It has to be here:
// the label set is built during evaluation, which is the last stage that still
// has spans to report against — realization has none. The cost is that a label
// counts even when its content is later dropped by a show rule and never
// reaches the document; the `@ref` existence check that reads fr.s.labels has
// always had the same blind spot, and warning about a document that turns out
// not to have the conflict is better than staying silent about one that does.
func (fr *frame) attachLabel(c value.Content, lbl *value.Label, warnSpan syntax.Span) {
	if old := c.SetLabel(lbl); old != nil {
		fr.warn(warnSpan, "content labelled multiple times",
			"only the last label is used, the rest are ignored")
		delete(fr.s.labels, old.Name)
		delete(fr.s.metadataLabels, old.Name)
	}
	if fr.s.labels == nil {
		fr.s.labels = make(map[name.Name]struct{})
	}
	fr.s.labels[lbl.Name] = struct{}{}

	if _, isMetadata := c.(*value.Metadata); !isMetadata {
		return
	}
	if _, dup := fr.s.metadataLabels[lbl.Name]; dup {
		fr.warn(warnSpan, fmt.Sprintf("metadata label `<%s>` used more than once", lbl.Name),
			"a query for it finds the first one; label the others separately")
	}
	if fr.s.metadataLabels == nil {
		fr.s.metadataLabels = make(map[name.Name]struct{})
	}
	fr.s.metadataLabels[lbl.Name] = struct{}{}
}

// functionCall bundles the arguments for [runFunction] into a single struct
// to make call sites self-documenting.
type functionCall struct {
	// mod is the module fn was compiled from; see [frame.mod].
	mod *expr.Module
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
	fr := &frame{s: s, mod: call.mod, fn: fn, vals: make([]value.Value, fn.NumRefs())}
	// Pre-fill parameter, capture, and self slots.
	for i, p := range fn.Params {
		fr.vals[p.Ref] = args[i]
	}
	for i, r := range fn.Captures {
		fr.vals[r] = captures[i]
	}
	if fn.SelfRef != expr.NoRef {
		fr.vals[fn.SelfRef] = self
	}

	bb := expr.BlockID(0)
	for {
		block := fn.Blocks[bb]
		// Straight-line instructions.
		for _, inst := range block.Instrs {
			evalInst(fr, inst)
		}
		// Terminator: bind args directly into the successor's BlockParam
		// slots before transferring control. RHS values are resolved
		// against the current (predecessor) frame; LHS slots belong to
		// the successor's params and are not read again until after the
		// jump completes, so there is no parallel-copy hazard.
		switch t := block.Term.(type) {
		case *expr.Jump:
			bindArgs(fr, fn.Blocks[t.Target].Params, t.Args)
			bb = t.Target
		case *expr.Branch:
			cond := fr.get(t.Cond)
			// If the condition is itself an error value, the upstream
			// computation already recorded it. Pick the Else arm so
			// evaluation can continue *and* loops terminate (while/for
			// lower with the exit block as Else, so this drops out of the
			// loop instead of re-evaluating the failing condition every
			// iteration).
			if _, ok := value.IsError(cond); ok {
				bindArgs(fr, fn.Blocks[t.Else].Params, t.ElseArgs)
				bb = t.Else
				break
			}
			cb, ok := cond.(value.Bool)
			if !ok {
				// Non-bool condition: record the failure and fall through
				// to the Else arm. This terminates loops (their exit edge
				// is Else) and skips the Then arm of conditionals.
				fr.errorf(t.Span(), "expected boolean, found %s", cond.Type())
				bindArgs(fr, fn.Blocks[t.Else].Params, t.ElseArgs)
				bb = t.Else
				break
			}
			if bool(cb) {
				bindArgs(fr, fn.Blocks[t.Then].Params, t.ThenArgs)
				bb = t.Then
			} else {
				bindArgs(fr, fn.Blocks[t.Else].Params, t.ElseArgs)
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

// bindArgs writes each arg into the corresponding successor block param's
// slot in the frame's value table.
func bindArgs(fr *frame, params []*expr.BlockParam, args []expr.Ref) {
	for i, a := range args {
		fr.vals[params[i].Result()] = fr.get(a)
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
// mechanism. ContentResult, CodeJoin, and JoinAdd opt out of operand
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
			fr.vals[r] = fr.error(fr.span(r), err.Error())
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
			fr.vals[r] = fr.error(fr.span(r), err.Error())
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
					fr.vals[r] = fr.error(fr.span(r), fmt.Sprintf("cannot spread %s into dictionary", s.Type()))
					return
				}
				continue
			}
			keyStr, ok := fr.get(e.Key).(value.Str)
			if !ok {
				fr.vals[r] = fr.error(fr.span(r), "dictionary key must be a string")
				return
			}
			dict.Elems.Put(keyStr, fr.get(e.Value))
		}
		fr.vals[r] = dict
	case *expr.FieldRead:
		fr.vals[r] = fr.evalFieldRead(fr.get(i.Target), i.Field, fr.span(r), i.FieldSpan)
	case *expr.MethodField:
		fr.vals[r] = fr.evalMethodField(fr.get(i.Target), i.Field, fr.span(r), i.FieldSpan, i.TargetText, i.Math)
	case *expr.Call:
		fr.vals[r] = fr.evalCall(i)
	case *expr.CallSet:
		fr.evalCallSet(i)
	case *expr.FieldWrite:
		fr.evalFieldWrite(i)
	case *expr.DiscardCheck:
		if c, ok := fr.get(i.Value).(value.Content); ok {
			hints := []string{"try omitting the `return` to automatically join all values"}
			if containsIntrospection(c) {
				hints = append(hints, "state updates and metadata are content that must end up in the document to have an effect")
			}
			fr.warn(i.Span(), "this return unconditionally discards the content before it", hints...)
		}
	case *expr.Warn:
		fr.warn(i.Span(), i.Msg, i.Hints...)
	case *expr.MakeClosure:
		fr.vals[r] = makeClosureWithFrame(fr, i)
	case *expr.DestructArray:
		fr.vals[r] = fr.evalDestructArray(i, fr.span(r))
	case *expr.ArrayElem:
		fr.vals[r] = fr.evalArrayElem(fr.get(i.Source), i.Index, i.FromEnd, i.Key, fr.span(r))
	case *expr.ArraySlice:
		src := fr.get(i.Source)
		if arr, ok := src.(*value.Array); ok {
			end := len(arr.Elems) - i.After
			if i.Before > end {
				fr.vals[r] = &value.Array{}
				return
			}
			out := make([]value.Value, end-i.Before)
			copy(out, arr.Elems[i.Before:end])
			fr.vals[r] = &value.Array{Elems: out}
			return
		}
		if d, ok := src.(*value.Dict); ok && i.Hybrid {
			fr.vals[r] = dictExcluding(d, i.ExcludeKeys)
			return
		}
		fr.vals[r] = fr.error(fr.span(r), "destructure index out of range")
	case *expr.DestructDict:
		fr.vals[r] = fr.evalDestructDict(i, fr.span(r))
	case *expr.DictField:
		src := fr.get(i.Source)
		d, _ := src.(*value.Dict)
		if d == nil {
			fr.vals[r] = fr.error(fr.span(r), "destructure index out of range")
			return
		}
		fr.vals[r] = fr.destructDictKey(d, i.Field, i.FieldSpan)
	case *expr.DictRest:
		src := fr.get(i.Source)
		d, _ := src.(*value.Dict)
		if d == nil {
			fr.vals[r] = fr.error(fr.span(r), "destructure index out of range")
			return
		}
		fr.vals[r] = dictExcluding(d, i.Consumed)
	case *expr.IterOpen:
		state, e := fr.newIterator(fr.get(i.Iterable), fr.span(r), i.Destructuring, i.PatternSpan)
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
		// here, From is known to be non-error (or absent).
		if i.Reported {
			// Parse errors are already reported via Module.ParseErrors;
			// yield the poison value without recording again.
			fr.vals[r] = &value.Error{Span: fr.span(r), Msg: i.Msg, Hints: value.Hints(i.Hints...)}
		} else {
			fr.vals[r] = fr.error(fr.span(r), i.Msg, i.Hints...)
		}
	case *expr.AttachLabel:
		// Coerce the prior content to value.Content, attach the label, emit
		// a warning if overwriting, and register the label.
		v := fr.get(i.Content)
		c := value.ToContent(v)
		if c == nil {
			// The preceding value isn't content (e.g. a let-binding result of
			// None). Drop the label silently.
			fr.vals[r] = value.None{}
			break
		}
		if isStyleUpdate(c) {
			// A set/show update can never carry a label. The label attaches to
			// the update's *scope* siblings, not the transient node itself — so
			// with nothing else preceding it, the label is unattached.
			fr.warn(fr.span(r), fmt.Sprintf("label `<%s>` is not attached to anything", i.Label.String()))
			fr.vals[r] = value.None{}
			break
		}
		// Use the operand's defining instruction span for the attach
		// diagnostic. Module-const refs have no per-function span — use the
		// AttachLabel's own span instead, and skip the operand-slot writeback
		// (the pool is shared across uses and must stay immutable; the label is
		// still registered on the session).
		var attachSpan syntax.Span
		if i.Content.IsLocal() {
			attachSpan = fr.fn.RefSpans[i.Content]
		} else {
			attachSpan = fr.span(r)
		}
		fr.attachLabel(c, &value.Label{Name: i.Label}, attachSpan)
		if i.Content.IsLocal() {
			fr.vals[i.Content] = c
		}
		fr.vals[r] = value.None{}
	case *expr.CodeJoin:
		fr.vals[r] = fr.evalCodeJoin(i)
	case *expr.JoinBegin:
		fr.vals[r] = &value.Array{}
	case *expr.JoinAdd:
		arr, ok := fr.get(i.Acc).(*value.Array)
		if !ok {
			panic("join_add: accumulator is not an array")
		}
		arr.Elems = append(arr.Elems, fr.get(i.Item))
		fr.vals[r] = arr
	case *expr.JoinResult:
		arr, ok := fr.get(i.Acc).(*value.Array)
		if !ok {
			panic("join_result: accumulator is not an array")
		}
		fr.vals[r] = fr.joinValues(arr.Elems, fr.span(r))
	case *expr.Heading:
		fr.vals[r] = &value.Heading{Depth: i.Level, Body: contentOf(fr.get(i.Body))}
	case *expr.Strong:
		fr.vals[r] = &value.Strong{Body: contentOf(fr.get(i.Body))}
	case *expr.Emph:
		fr.vals[r] = &value.Emph{Body: contentOf(fr.get(i.Body))}
	case *expr.Link:
		fr.vals[r] = &value.Link{Dest: i.Dest, Body: contentOf(fr.get(i.Body))}
	case *expr.RefMarkup:
		if _, ok := fr.s.labels[i.Target]; !ok {
			fr.vals[r] = fr.errorf(fr.span(r), "label `<%s>` does not exist in the document", i.Target.String())
			return
		}
		v := &value.Ref{Target: i.Target}
		if i.Supplement != expr.NoRef {
			v.Supplement = contentOf(fr.get(i.Supplement))
		}
		fr.vals[r] = v
	case *expr.ListItem:
		fr.vals[r] = &value.ListItem{Body: contentOf(fr.get(i.Body))}
	case *expr.EnumItem:
		fr.vals[r] = &value.EnumItem{Number: i.Number, Body: contentOf(fr.get(i.Body))}
	case *expr.TermItem:
		fr.vals[r] = &value.TermItem{
			Term:        contentOf(fr.get(i.Term)),
			Description: contentOf(fr.get(i.Description)),
		}
	case *expr.Equation:
		fr.vals[r] = &value.Equation{Block: i.Block, Body: mathContentOf(fr.get(i.Body))}
	case *expr.MathAttach:
		a := &value.MathAttach{Base: mathContentOf(fr.get(i.Base))}
		if i.Top != expr.NoRef {
			a.Top = mathContentOf(fr.get(i.Top))
		}
		if i.Bottom != expr.NoRef {
			a.Bottom = mathContentOf(fr.get(i.Bottom))
		}
		fr.vals[r] = a
	case *expr.MathFrac:
		fr.vals[r] = &value.MathFrac{
			Num:   mathContentOf(fr.get(i.Num)),
			Denom: mathContentOf(fr.get(i.Denom)),
		}
	case *expr.MathRoot:
		root := &value.MathRoot{Radicand: mathContentOf(fr.get(i.Radicand))}
		if i.Index != expr.NoRef {
			root.Index = mathContentOf(fr.get(i.Index))
		}
		fr.vals[r] = root
	case *expr.MathPrimes:
		fr.vals[r] = &value.MathPrimes{Base: mathContentOf(fr.get(i.Base)), Count: i.Count}
	case *expr.MathDelimited:
		fr.vals[r] = &value.MathDelimited{
			Open:  mathContentOf(fr.get(i.Open)),
			Body:  mathContentOf(fr.get(i.Body)),
			Close: mathContentOf(fr.get(i.Close)),
		}
	case *expr.SetRule:
		fr.vals[r] = fr.evalSetRule(i)
	case *expr.ShowRule:
		fr.vals[r] = fr.evalShowRule(i)
	case *expr.ModuleInclude:
		// Parsed and lowered, but never implemented past that. Reachable from
		// ordinary source, so it has to be a diagnostic and not a crash.
		fr.vals[r] = fr.error(fr.span(r), "includes are not supported")
	case *expr.Contextual:
		panic(fmt.Sprintf("TODO: ssa eval %T", i))
	default:
		panic(fmt.Sprintf("ssa eval not implemented: %T", inst))
	}
}

// containsIntrospection reports whether content is, or (for a sequence)
// transitively contains, an element whose whole purpose is to be found again
// in the document: a state update or a metadata value. Discarding such content
// is worth a dedicated hint because it silently has no effect — there is no
// visible output missing to give the mistake away.
func containsIntrospection(c value.Content) bool {
	switch c := c.(type) {
	case *value.StateUpdate, *value.Metadata:
		return true
	case *value.Sequence:
		for _, child := range c.Children {
			if containsIntrospection(child) {
				return true
			}
		}
	}
	return false
}

// mathContentOf is [contentOf] in math context: symbols, strings and numbers
// become [value.MathText] rather than upright text, so that, e.g., `$s$` and
// `$sym.basic$` — a letter and a symbol resolving to the same character —
// compare equal.
func mathContentOf(v value.Value) value.Content {
	if c := value.ToMathContent(v); c != nil {
		return c
	}
	return &value.Sequence{}
}

// contentOf shows a value as content. The nothing that [value.None] shows as
// becomes an empty Sequence: an element's body has to hold something.
func contentOf(v value.Value) value.Content {
	if c := value.ToContent(v); c != nil {
		return c
	}
	return &value.Sequence{}
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
		sp := fr.span(c.Result())
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
	return foldStyles(j.Result())
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
	return foldStyles(j.Result())
}

// evalSetRule evaluates a `set target(args) [if cond]` rule to a transient
// [value.StyleUpdate] carrying the resolved [value.Set]. A false condition
// makes the rule a no-op ([value.None], stripped downstream). The target must
// resolve to a [value.Element], and the arguments are validated against that
// element's signature (a set rule overrides a subset of its properties).
func (fr *frame) evalSetRule(i *expr.SetRule) value.Value {
	if i.Condition != expr.NoRef {
		cond := fr.get(i.Condition)
		cb, ok := cond.(value.Bool)
		if !ok {
			return fr.errorf(fr.span(i.Result()), "expected boolean, found %s", cond.Type())
		}
		if !bool(cb) {
			return value.None{}
		}
	}
	target := fr.get(i.Target)
	elem, ok := target.(*value.Element)
	if !ok {
		return fr.errorf(fr.span(i.Result()), "expected element, found %s", target.Type())
	}
	args, e := fr.buildCallArgs(fr.span(i.Result()), i.Args, nil)
	if e != nil {
		return e
	}
	set, err := elem.BindStyleSet(&args)
	if err != nil {
		return fr.applyErr(&elem.Function, fr.span(i.Result()), i.Args, err)
	}
	return &value.StyleUpdate{Set: set}
}

// evalShowRule evaluates a `show selector: transform` rule to a transient
// [value.StyleUpdate] carrying the resolved [value.Recipe]. The transform is
// captured as-is; the realization pass applies it to matching content.
func (fr *frame) evalShowRule(i *expr.ShowRule) value.Value {
	sel, e := fr.evalSelector(i.Selector, fr.span(i.Result()))
	if e != nil {
		return e
	}
	transform := fr.get(i.Transform)
	return &value.StyleUpdate{Recipe: &value.Recipe{Selector: sel, Transform: transform}}
}

// evalSelector resolves a show-rule selector operand. A [expr.NoRef] operand is
// a bare `show: transform` (nil selector). An element, label, or existing
// selector value maps to the matching [value.Selector]; anything else errors.
func (fr *frame) evalSelector(ref expr.Ref, span syntax.Span) (value.Selector, *value.Error) {
	if ref == expr.NoRef {
		return nil, nil
	}
	v := fr.get(ref)
	switch s := v.(type) {
	case *value.Element:
		return &value.ElementSelector{Element: s}, nil
	case *value.Label:
		return &value.LabelSelector{Label: s.Name}, nil
	case value.Selector:
		return s, nil
	default:
		return nil, fr.errorf(span, "expected selector, found %s", v.Type())
	}
}

// whereMethod builds the `.where(label:)` selector constructor for an element
// target. Calling it with a label yields a [value.WhereSelector] matching that
// element carrying that label.
func whereMethod(e *value.Element) *value.Function {
	return &value.Function{
		Name:  "where",
		Named: value.NamedParams{names.Label: {Name: "label", Type: types.SetOf(types.Label)}},
		F: func(_ *value.FunctionCallContext, _ []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
			lbl, ok := named.Get(names.Label).(*value.Label)
			if !ok {
				return nil, value.ArgErrorNamedf(names.Label, "expected label")
			}
			return &value.WhereSelector{Element: e, Label: lbl.Name}, nil
		},
	}
}

// applyStyles folds each [value.StyleUpdate] into a [value.Styled] wrapper
// around the remaining siblings, giving one style scope per set/show scope. A
// trailing update (no following siblings) is dropped as a no-op.
func applyStyles(children []value.Content) []value.Content {
	for i, c := range children {
		tu, ok := c.(*value.StyleUpdate)
		if !ok {
			continue
		}
		tail := applyStyles(children[i+1:])
		if len(tail) == 0 {
			return children[:i:i]
		}
		w := &value.Styled{Body: seqOf(tail)}
		if tu.Set != nil {
			w.Sets = append(w.Sets, tu.Set)
		}
		if tu.Recipe != nil {
			w.Recipes = append(w.Recipes, tu.Recipe)
		}
		return append(children[:i:i], w)
	}
	return children
}

// seqOf wraps children in a Sequence, collapsing a single element to itself.
func seqOf(children []value.Content) value.Content {
	if len(children) == 1 {
		return children[0]
	}
	return &value.Sequence{Children: children}
}

// foldStyles applies [applyStyles] to a code-mode join result so a `set` or
// `show` inside a code block scopes over the trailing joined siblings. A lone
// trailing update collapses to [value.None] (a no-op).
func foldStyles(v value.Value) value.Value {
	switch c := v.(type) {
	case *value.Sequence:
		folded := applyStyles(c.Children)
		if len(folded) == len(c.Children) {
			return v // nothing folded
		}
		if len(folded) == 1 {
			return folded[0]
		}
		return &value.Sequence{Children: folded, Label: c.Label}
	case *value.StyleUpdate:
		return value.None{}
	}
	return v
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
	// Inside an equation the joiner is the math one, so that a symbol or string
	// in a run renders like the same character written on its own (which skips
	// the joiner entirely, see analyzer.lowerMathContent).
	toContent := value.ToContent
	if c.Math {
		toContent = value.ToMathContent
	}
	for _, r := range c.Items {
		v := fr.get(r)
		if _, ok := value.IsError(v); ok {
			continue
		}
		if lbl, ok := v.(*value.Label); ok {
			if len(ret) == 0 {
				continue
			}
			// A set/show update can never carry a label; a label landing on one
			// is unattached and warned about rather than silently dropped.
			if isStyleUpdate(ret[len(ret)-1]) {
				lblSpan := fr.span(c.Result())
				if r.IsLocal() {
					lblSpan = fr.fn.RefSpans[r]
				}
				fr.warn(lblSpan, fmt.Sprintf("label `<%s>` is not attached to anything", lbl.Name.String()))
				continue
			}
			fr.attachLabel(ret[len(ret)-1], lbl, lastSpan)
			continue
		}
		cv := toContent(v)
		if cv == nil {
			continue
		}
		ret = append(ret, cv)
		// Use the producing instruction's span so warnings/errors against
		// the most-recent content point at the right source location.
		// Module-const refs have no per-function span — fall back to the
		// consuming instruction's span.
		if r.IsLocal() {
			lastSpan = fr.fn.RefSpans[r]
		} else {
			lastSpan = fr.span(c.Result())
		}
	}
	ret = applyStyles(ret)
	if len(ret) == 1 {
		return ret[0]
	}
	return &value.Sequence{Children: ret}
}

// isStyleUpdate reports whether c is a transient set/show update — content
// that will be folded into a [value.Styled] wrapper by [applyStyles] and
// never survives on its own.
func isStyleUpdate(c value.Content) bool {
	_, ok := c.(*value.StyleUpdate)
	return ok
}

// evalDestructArray validates that the destructuring source is an array of
// the right shape (or, when Hybrid is set, a dict that can be destructured
// via ident shorthand). Returns the validated source on success, or a
// *value.Error on failure.
func (fr *frame) evalDestructArray(i *expr.DestructArray, span syntax.Span) value.Value {
	src := fr.get(i.Source)
	if arr, ok := src.(*value.Array); ok {
		need := i.Before + i.After
		got := len(arr.Elems)
		// With a sink, any length >= need is fine; without one, the length
		// must match exactly.
		if (i.HasSink && got < need) || (!i.HasSink && got != need) {
			quantifier := "not enough"
			if got > need {
				quantifier = "too many"
			}
			hint := fmt.Sprintf("the provided array has a length of %d, but the pattern expects %s",
				got, expectedElems(need, i.HasSink))
			return fr.error(span, quantifier+" elements to destructure", hint)
		}
		return arr
	}
	if d, ok := src.(*value.Dict); ok {
		if i.Hybrid {
			// Dict shorthand: each non-sink slot's Key must be present.
			// Per-key "does not contain" errors are surfaced by the
			// individual ArrayElem reads.
			return d
		}
		// A positional pattern with holes or nested sub-patterns has no key
		// names to look up, so it cannot destructure a dict.
		return fr.error(span, "cannot destructure a dictionary using a positional pattern",
			"use named fields like `(key: binding)`, or plain identifiers for shorthand")
	}
	return fr.errorf(span, "cannot destructure %s", src.Type())
}

// expectedElems describes how many elements a destructuring pattern expects,
// for the "the pattern expects ..." hint on a length mismatch.
func expectedElems(need int, hasSink bool) string {
	switch {
	case hasSink && need == 1:
		return "at least 1 element"
	case hasSink:
		return fmt.Sprintf("at least %d elements", need)
	case need == 0:
		return "an empty array"
	case need == 1:
		return "a single element"
	default:
		return fmt.Sprintf("%d elements", need)
	}
}

// evalDestructDict validates that the destructuring source is a dict. An
// array source raises the named-from-array error at the first named-pair
// span (i.FirstNamedSpan).
func (fr *frame) evalDestructDict(i *expr.DestructDict, span syntax.Span) value.Value {
	src := fr.get(i.Source)
	if _, ok := src.(*value.Array); ok {
		return fr.error(i.FirstNamedSpan, "cannot destructure named pattern from an array")
	}
	d, ok := src.(*value.Dict)
	if !ok {
		return fr.errorf(span, "cannot destructure %s", src.Type())
	}
	return d
}

// evalArrayElem reads a single element from an array (from the front, or from
// the back when fromEnd is set), or — when the source is a dict and key is
// non-zero — looks up the key.
func (fr *frame) evalArrayElem(src value.Value, index int, fromEnd bool, key name.Name, span syntax.Span) value.Value {
	if arr, ok := src.(*value.Array); ok {
		if index >= len(arr.Elems) {
			return fr.error(span, "destructure index out of range")
		}
		if fromEnd {
			return arr.Elems[len(arr.Elems)-1-index]
		}
		return arr.Elems[index]
	}
	if d, ok := src.(*value.Dict); ok && key != (name.Name{}) {
		return fr.destructDictKey(d, key, span)
	}
	return fr.error(span, "destructure index out of range")
}

// destructDictKey looks up key in d, returning a "dictionary does not contain
// key" error at span when absent. Shared by hybrid array reads and DictField.
func (fr *frame) destructDictKey(d *value.Dict, key name.Name, span syntax.Span) value.Value {
	v, found := d.Elems.Get(value.Str(key.String()))
	if !found {
		return fr.errorf(span, "dictionary does not contain key %q", key.String())
	}
	return v
}

// dictExcluding returns a new dict containing every entry of d whose key
// is not in exclude.
func dictExcluding(d *value.Dict, exclude []name.Name) *value.Dict {
	skip := make(map[value.Str]struct{}, len(exclude))
	for _, k := range exclude {
		// Sink/hole slots contribute an invalid (zero) name with no key to
		// exclude; skip it (and avoid panicking on String()).
		if k != (name.Name{}) {
			skip[value.Str(k.String())] = struct{}{}
		}
	}
	out := &value.Dict{}
	for k, v := range d.Elems.All() {
		if _, drop := skip[k]; !drop {
			out.Elems.Put(k, v)
		}
	}
	return out
}

// evalMethodField resolves the callee of a method call `target.method(...)`. It
// applies method-call error semantics: dictionary keys cannot be called
// directly, and a missing field on a content element or a method-bearing type
// is reported as a missing method rather than a missing field. targetText is the
// source text of the target expression, used to build the dictionary-key hints.
func (fr *frame) evalMethodField(target value.Value, fname name.Name, span, fieldSpan syntax.Span, targetText string, math bool) value.Value {
	// Symbols, modules, and reflected types resolve identically to a plain
	// field read (a symbol modifier or module definition is legitimately
	// callable; a type's method table is the same).
	switch t := target.(type) {
	case *value.Type:
		ms := builtin.TypeFields[t.Reflected]
		f := ms[fname]
		if f == nil {
			return fr.errorf(span, "type %s has no method `%s`", t.Reflected, fname.String())
		}
		return f
	case *value.Symbol:
		nsym, ok := t.Resolve(fname)
		if !ok {
			return fr.errorf(fieldSpan, "unknown symbol modifier")
		}
		return nsym
	case *value.Module:
		v := t.Def.Get(fname)
		if v == nil {
			return fr.errorf(fieldSpan, "module %s has no definition `%s`", t.Name, fname.String())
		}
		return v
	case *value.Element:
		// `.where(label:)` builds a selector; other names resolve against the
		// element's scope (e.g. `list.item`). Anything else is an invalid method.
		if fname == names.Where {
			return whereMethod(t)
		}
		if f, ok := t.Scope[fname]; ok {
			return f
		}
		return fr.errorf(span, "`%s` is not a valid method for element `%s`", fname.String(), t.Name)
	}
	// A built-in method always wins over a same-named dictionary key or content
	// field, and is dispatched by binding the receiver as the first argument.
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
		// A dictionary key is never directly callable via method syntax: the
		// key could shadow a built-in method name. Steer the user to either
		// call the stored value or read the key.
		if val, ok := t.Elems.Get(value.Str(fname.String())); ok {
			conflict := "dictionary keys cannot be used with method syntax as keys could conflict with built-in method names"
			var hints []string
			if _, isFunc := val.(*value.Function); isFunc {
				var call string
				if math {
					call = fmt.Sprintf("to call the stored function, use code mode and wrap the field access in parentheses: `#(%s.%s)(..)`", targetText, fname)
				} else {
					call = fmt.Sprintf("to call the stored function, wrap the field access in parentheses: `(%s.%s)(..)`", targetText, fname)
				}
				hints = []string{call, conflict}
			} else if math {
				hints = []string{conflict, "try adding a space before the parentheses"}
			} else {
				hints = []string{
					conflict,
					fmt.Sprintf("to access the `%s` key, remove the function arguments: `%s.%s`", fname, targetText, fname),
				}
			}
			return fr.error(span, "cannot directly call dictionary keys as functions", hints...)
		}
		return fr.errorf(span, "dictionary does not contain key %q", fname.String())
	case value.Content:
		name := t.Name()
		if t.HasField(fname) {
			// The field exists but holds a value, not a method — calling it
			// with method syntax would be ambiguous.
			var hint string
			if math {
				hint = fmt.Sprintf("to call the stored function, use code mode and wrap the field access in parentheses: `#(%s.%s)(..)`", targetText, fname)
			} else {
				hint = fmt.Sprintf("to call the stored function, wrap the field access in parentheses: `(%s([], %s: x => x + 1).%s)(..)`", name, fname, fname)
			}
			return fr.error(span,
				fmt.Sprintf("`%s` is not a valid method for element `%s`", fname, name),
				hint)
		}
		return fr.errorf(span, "element %s has no method `%s`", name, fname.String())
	case *value.Function:
		if f, ok := t.Scope[fname]; ok {
			return f
		}
		if t.Closure {
			return fr.errorf(fieldSpan, "cannot access fields on user-defined functions")
		}
		return fr.errorf(fieldSpan, "function `%s` does not contain field `%s`", t.Name, fname.String())
	}
	// The field is neither a method nor a value-specific field. If the type has
	// a method table at all, report the miss as an unknown method against the
	// whole access; otherwise the type has no accessible fields.
	if len(builtin.TypeFields[target.Type()]) > 0 {
		return fr.errorf(span, "type %s has no method `%s`", target.Type(), fname.String())
	}
	return fr.errorf(fieldSpan, "cannot access fields on type %s", target.Type())
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
			return fr.errorf(span, "type %s has no field `%s`", t.Reflected, fname.String())
		}
		return f
	case *value.Symbol:
		nsym, ok := t.Resolve(fname)
		if !ok {
			return fr.errorf(fieldSpan, "unknown symbol modifier")
		}
		return nsym
	case *value.Module:
		v := t.Def.Get(fname)
		if v == nil {
			return fr.errorf(fieldSpan, "module %s has no definition `%s`", t.Name, fname.String())
		}
		return v
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
	case *value.Function:
		if f, ok := t.Scope[fname]; ok {
			return f
		}
		if t.Closure {
			return fr.errorf(fieldSpan, "cannot access fields on user-defined functions")
		}
		return fr.errorf(fieldSpan, "function `%s` does not contain field `%s`", t.Name, fname.String())
	}
	// The field is neither a method nor a value-specific field. If the type has
	// a method table at all, report the miss as an unknown field against the
	// whole access; otherwise the type has no accessible fields.
	if len(builtin.TypeFields[target.Type()]) > 0 {
		return fr.errorf(span, "type %s has no field `%s`", target.Type(), fname.String())
	}
	return fr.errorf(fieldSpan, "cannot access fields on type %s", target.Type())
}

// resolveCallee unwraps a callee value into the underlying function.
// Returns (nil, err) when the value isn't callable.
func (fr *frame) resolveCallee(callee value.Value, span syntax.Span) (*value.Function, *value.Error) {
	fn, msg := calleeFunc(callee)
	if fn == nil {
		return nil, fr.error(span, msg)
	}
	return fn, nil
}

// calleeFunc returns the function a callee value denotes, or nil and the reason
// it denotes none. Reporting is left to the caller: a math call whose callee is
// not a function is juxtaposition rather than a mistake (see
// [frame.evalMathFallback]).
func calleeFunc(callee value.Value) (*value.Function, string) {
	switch cc := callee.(type) {
	case *value.Function:
		return cc, ""
	case *value.Type:
		if cc.Constructor == nil {
			return nil, fmt.Sprintf("type %s is not callable", cc.Reflected)
		}
		return cc.Constructor, ""
	case *value.Element:
		// A set-only element (e.g. `document`) has no implementation and can't be
		// called as a constructor.
		if cc.F == nil {
			return nil, fmt.Sprintf("element %s is not callable", cc.Name)
		}
		return &cc.Function, ""
	case *value.Symbol:
		// An accent symbol applies itself: `hat(f)` is `accent(f, hat)`. Every
		// other symbol is not callable.
		f, err := builtin.SymbolFunc(cc)
		if err != nil {
			return nil, err.Error()
		}
		return f, ""
	default:
		return nil, fmt.Sprintf("expected function, found %s", callee.Type())
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
			args.Named.Put(a.Name, fr.get(a.Value))
		case expr.ArgSpread:
			v := fr.get(a.Value)
			switch sv := v.(type) {
			case *value.Array:
				args.Positional = append(args.Positional, sv.Elems...)
			case *value.Dict:
				for k, vv := range sv.Elems.All() {
					args.Named.Put(name.Make(string(k)), vv)
				}
			case *value.Arguments:
				args.Positional = append(args.Positional, sv.Positional...)
				for k, vv := range sv.Named.All() {
					args.Named.Put(k, vv)
				}
			case value.None:
			default:
				return args, fr.errorf(a.Span, "cannot spread %s", v.Type())
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

// evalCall lowers an SSA Call instruction to a [value.Function.Apply], and
// returns a [*value.Error] when the callee can't be resolved, an argument
// spread fails, or the call itself reports an error. A math call whose callee
// turns out not to be a function renders as juxtaposition instead (see
// [frame.evalMathFallback]).
func (fr *frame) evalCall(c *expr.Call) value.Value {
	fn, msg := calleeFunc(fr.get(c.Callee.Ref))
	if fn == nil {
		// In math, calling something that isn't a function is juxtaposition:
		// `$sin(x)$` means what `$sin (x)$` means.
		if c.Fallback != nil {
			return fr.evalMathFallback(c)
		}
		return fr.error(c.Callee.Span, msg)
	}
	// A mutating method (Impure) needs a mutable place to write back. In math
	// mode there is none, so a resolved mutating method is rejected outright.
	// Otherwise, a mutating call on a temporary is an error. Both mutating-ness
	// and the receiver's place-ness are resolved here at runtime — see
	// [expr.MutCheck].
	if c.Mut != nil && fn.Impure {
		if c.Mut.Math {
			return fr.error(c.Mut.RecvSpan, "cannot call mutating methods in math",
				"try using code mode to call the method: `#"+c.Mut.MathCall+"`")
		}
		if !fr.receiverIsPlace(c.Mut) {
			return fr.error(c.Mut.RecvSpan, "cannot mutate a temporary value")
		}
	}
	args, e := fr.buildCallArgs(fr.span(c.Result()), c.Args, c.Blocks)
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

	// Guard the Go stack against unbounded user-closure recursion. Only
	// closures count toward the depth (matching where the runtime actually
	// recurses); built-ins do not nest evaluation this way.
	if fn.Closure {
		if len(fr.s.stack) >= maxCallDepth {
			return fr.error(fr.span(c.Result()), "maximum function call depth exceeded")
		}
		defer fr.pushCall(fr.span(c.Result()), fn)()
	}

	fcc := fr.callContext(fr.span(c.Result()))
	v, err := fn.Apply(&fcc, &args)
	if err != nil {
		return fr.applyErr(fn, fr.span(c.Result()), c.Args, err)
	}
	return v
}

// evalMathFallback renders a math call whose callee is not a function: the
// callee's content followed by the argument list read as plain content, which
// is what the same source without the parentheses hugging the callee produces.
func (fr *frame) evalMathFallback(c *expr.Call) value.Value {
	// Named and spread arguments only mean something in a real call.
	var first *value.Error
	for _, b := range c.Fallback.BadArgs {
		if e := fr.errorHints(b.Span, b.Msg, b.Hints); first == nil {
			first = e
		}
	}
	if first != nil {
		return first
	}

	callee := mathContentOf(fr.get(c.Callee.Ref))
	var body []value.Content
	for _, it := range c.Fallback.Items {
		if it.Ref == expr.NoRef {
			body = append(body, &value.MathText{Text: it.Text})
			continue
		}
		if cv := value.ToMathContent(fr.get(it.Ref)); cv != nil {
			body = append(body, cv)
		}
	}
	var inner value.Content = &value.Sequence{Children: body}
	if len(body) == 1 {
		inner = body[0]
	}
	parens := &value.MathDelimited{
		Open:  &value.MathText{Text: "("},
		Body:  inner,
		Close: &value.MathText{Text: ")"},
	}
	return &value.Sequence{Children: []value.Content{callee, parens}}
}

// callContext builds the context handed to a called function. Runtime carries
// the running session so that a closure — which remembers the module it was
// written in, but not the evaluation that created it — records what it does on
// the document being compiled now.
func (fr *frame) callContext(span syntax.Span) value.FunctionCallContext {
	return value.FunctionCallContext{Span: span, Now: fr.s.now, Runtime: fr.s}
}

// pushCall records a call in progress on the session stack and returns the
// function that pops it, for `defer fr.pushCall(...)()`. The entry holds the
// *calling* frame's origin, because span points at the call expression, which
// is written in the caller's source.
func (fr *frame) pushCall(span syntax.Span, fn *value.Function) func() {
	fr.s.stack = append(fr.s.stack, callSite{mod: fr.mod, span: span, callee: fn.Name})
	return func() { fr.s.stack = fr.s.stack[:len(fr.s.stack)-1] }
}

// receiverIsPlace reports whether a method call's receiver denotes a mutable
// place. The receiver chain must root in a variable (not a temporary) and every
// method link must resolve to an accessor — a method whose result is a mutable
// place into its receiver ([value.Function.Accessor]). An empty accessor list
// with RecvTemporary false is an unconditional place (a bare identifier or field
// chain).
func (fr *frame) receiverIsPlace(m *expr.MutCheck) bool {
	if m.RecvTemporary {
		return false
	}
	for _, aref := range m.RecvAccessors {
		f, ok := fr.get(aref).(*value.Function)
		if !ok || !f.Accessor {
			return false
		}
	}
	return true
}

// evalCallSet implements lvalue-style assignment to a function call (e.g.
// `arr.at(i) = v`, `dict.at("k") += 1`). The callee is invoked with a
// [FunctionCallContext.Setter] pointer; if the function registers a setter, it
// is invoked with the (possibly op-combined) new value. Returns the first error
// encountered, or none on success. evalCallSet implements `f(args) = v` (and
// compound forms). Side-effect only: all error paths record on the session, and
// the instruction has no SSA result.
func (fr *frame) evalCallSet(c *expr.CallSet) {
	fn, e := fr.resolveCallee(fr.get(c.Callee.Ref), c.Callee.Span)
	if e != nil {
		return
	}
	args, e := fr.buildCallArgs(c.Span(), c.Args, c.Blocks)
	if e != nil {
		return
	}

	var setter func(value.Value)
	fcc := fr.callContext(c.Span())
	fcc.Setter = &setter
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
	case value.None, value.Int, value.Float, value.Bool, value.Str:
		fr.errorf(w.Span(), "%s does not have accessible fields", target.Type())
	default:
		fr.errorf(w.Span(), "cannot mutate fields on %s", target.Type())
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
	innerFn := fr.mod.Functions[m.Func]
	caps := make([]value.Value, len(m.Captures))
	for i, r := range m.Captures {
		caps[i] = fr.get(r)
	}
	return buildFunctionValue(fr.mod, innerFn, caps)
}

// buildFunctionValue creates a [value.Function] whose F invokes the SSA
// function with the supplied captures and an arg-slot layout matching the
// function's Params.
//
// fn.Params can interleave positional, named, and sink kinds in source order,
// but [value.Function.Positional] only carries positional and sink params. We
// build a posToParam mapping so the runtime can route each value.Function arg
// slot back to the correct fn.Params index.
//
// The result closes over mod but not over any session: the module is what fn
// means (its constant pool, its nested functions, the source its spans index),
// while the session is whichever evaluation happens to be calling. Binding the
// creating session here instead would make a library function record its
// errors, labels and `set document` rules on the library's own long-finished
// evaluation rather than on the document being compiled. The live session
// arrives per call through [value.FunctionCallContext.Runtime]; see
// [frame.callContext].
func buildFunctionValue(mod *expr.Module, fn *expr.Function, caps []value.Value) *value.Function {
	out := &value.Function{Name: fn.Name, Closure: true}
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
			// and arrive in fn.Captures — or, for constant defaults, as
			// module-const refs that resolve directly from the pool.
			var defaultVal value.Value
			switch {
			case p.Default == expr.NoRef:
			case p.Default.IsModConst():
				defaultVal = mod.Constants[p.Default.ModConstID()]
			default:
				for idx, r := range fn.Captures {
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
		s, ok := call.Runtime.(*session)
		if !ok {
			// Every path that reaches a closure goes through the evaluator,
			// which always sets Runtime — including builtins invoking a
			// callback, which forward the context they were given. A missing
			// one means a host called the value directly, with no evaluation
			// for its diagnostics to land on.
			return nil, fmt.Errorf("cannot call a markst function outside an evaluation")
		}
		return runFunction(s, functionCall{mod: mod, fn: fn, args: paramArgs, captures: caps, self: out}), nil
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
func (fr *frame) newIterator(v value.Value, span syntax.Span, destructuring bool, patternSpan syntax.Span) (*iteratorState, *value.Error) {
	if v == nil {
		return nil, fr.error(span, "cannot loop over uninitialised value")
	}
	if destructuring {
		switch v.(type) {
		case value.Str:
			return nil, fr.errorf(patternSpan, "cannot destructure values of string")
		case value.Bytes:
			return nil, fr.errorf(patternSpan, "cannot destructure values of bytes")
		}
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
	case value.Bytes:
		i := 0
		b := string(val)
		s.hasNext = i < len(b)
		s.next = func() (value.Value, bool) {
			v := value.Int(b[i])
			i = min(i+1, len(b))
			return v, i < len(b)
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
