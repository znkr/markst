package expr

import (
	"math"
	"slices"

	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// ModuleBuilder accumulates the per-module state shared by every [Builder]
// during analysis.
type ModuleBuilder struct {
	mod *Module

	// constIndex provides an index from a constant to [Module.Constants]
	// entries so repeated references to the same constant share a single module
	// constant entry.
	constIndex map[any]int32
}

// Builder constructs an SSA [Function]. The analyzer drives it as it walks the
// syntax tree, emitting instructions and tracking variable definitions per
// block. Join values are materialized as block parameters; the corresponding
// args on each predecessor terminator are populated as terminators are
// emitted and as new params are added to a block. The on-the-fly construction
// algorithm follows Braun, Buchwald & Hack (2013), "Simple and Efficient
// Construction of Static Single Assignment Form".
//
// Typical usage:
//
//		mb := expr.NewModuleBuilder()
//		b := mb.NewBuilder()
//		v := b.Const(span, value.Int(1))
//		b.WriteVar(name, b.CurrentBlock(), v)
//		then := b.NewBlock()
//		els  := b.NewBlock()
//		join := b.NewBlock()
//		b.Branch(span, cond, then, els)
//		... emit then and els blocks, each ending with b.Jump(join) ...
//		b.SealBlock(join)
//		r := b.ReadVar(name, join)   // inserts a block param at join if needed
//		fn := b.Function()
//	 mod := mb.Module()
type Builder struct {
	fn  *Function
	mb  *ModuleBuilder
	cur BlockID

	// Braun "current definition" table: per-variable, per-block, the SSA
	// Ref currently visible. Populated by [WriteVar] and consulted by
	// [ReadVar].
	currentDef map[Var]map[BlockID]Ref

	// Per unsealed block: block params created on demand for variables not
	// yet defined in the block's predecessors. Trivial-param elimination
	// is deferred to seal time so that the args set is complete.
	incompleteParams map[BlockID]map[Var]*BlockParam

	// paramVar records the source [Var] each param joins. Used by
	// [synthesizeArgs] and [SealBlock] to read the right value from each
	// predecessor; never escapes the builder. Sidecar map (rather than a
	// field on [BlockParam]) so the IR type stays free of build-time state.
	paramVar map[*BlockParam]Var

	// Set of blocks that have been sealed. A block is sealed when all of
	// its predecessors have been recorded; only then can incomplete block
	// params be checked for trivial elimination.
	sealed map[BlockID]struct{}

	// Use chains for trivial-param elimination: maps each Ref that is an
	// incoming arg slot value to the set of params receiving it. Updated
	// whenever an arg is appended or rewritten.
	paramUsers map[Ref]map[*BlockParam]struct{}

	// redirects records the rename target for each trivially-removed
	// block param. Populated by [tryRemoveTrivialParam]; consumed by
	// [Finalize], which transitively closes the map, rewrites every Ref
	// in the IR through it, and physically splices the dead param slots
	// from blocks and their incoming terminators. Never escapes the builder.
	redirects map[Ref]Ref

	// liveWrites collects every Ref ever passed as the value side of a
	// user-level [WriteVar] call. The SSA graph does not record these uses
	// (writes update an internal table, not an instruction operand), so DCE
	// counts each entry as +1 use to prevent dropping the RHS of a
	// write-only assignment — whose evaluation may surface a runtime error
	// even when the bound name is never read again. Builder-internal
	// caching writes (from [readVarRecursive]) bypass this slice via
	// [writeVarInternal] so they don't artificially pin param results.
	liveWrites []Ref

	// versions tracks how many [Var]s have been minted for each source
	// name in this function. Versions are per-name and 1-based, so
	// `let x; let x` yields x$1 then x$2, while a separate `let y` starts
	// at y$1. Used by [NewVar].
	versions map[name.Name]int

	// captures records the source names of captures registered via
	// [AddCapture], in the order they were added. Mirrors the indices of
	// [Function.Captures]. Returned by [Captures] for the analyzer's outer-Ref
	// wiring; the IR itself doesn't carry names because nothing consumes them
	// post-Finalize.
	captures []name.Name
}

// NewModuleBuilder starts a fresh module. The returned ModuleBuilder is the
// hub from which per-function [Builder]s are created.
func NewModuleBuilder() *ModuleBuilder {
	return &ModuleBuilder{mod: &Module{}}
}

// Module returns the [Module] under construction. Safe to call mid-build to
// e.g. attach the top-level function, but Constants is still growing until
// every [Builder] has finished emitting.
func (mb *ModuleBuilder) Module() *Module { return mb.mod }

// noneConstID returns the [Module.Constants] index of the interned
// [value.None] entry, if one has been emitted. Used by DCE to recognize
// constant-`none` joiner items.
func (mb *ModuleBuilder) noneConstID() (int32, bool) {
	id, ok := mb.constIndex[value.None{}]
	return id, ok
}

// RegisterFunction appends fn to the module's closure list and returns the
// assigned [FuncID]. Use this for closures hoisted out of the document body;
// the top-level body is attached via [Module.Top].
func (mb *ModuleBuilder) RegisterFunction(fn *Function) FuncID {
	id := FuncID(len(mb.mod.Functions))
	mb.mod.Functions = append(mb.mod.Functions, fn)
	return id
}

// NewBuilder starts construction of a fresh [Function] inside mb's module.
// The entry block (id 0) is created and made current. All builders created
// from the same ModuleBuilder share the constant pool.
func (mb *ModuleBuilder) NewBuilder() *Builder {
	b := &Builder{
		fn:               &Function{SelfRef: NoRef},
		mb:               mb,
		currentDef:       make(map[Var]map[BlockID]Ref),
		incompleteParams: make(map[BlockID]map[Var]*BlockParam),
		paramVar:         make(map[*BlockParam]Var),
		sealed:           make(map[BlockID]struct{}),
		paramUsers:       make(map[Ref]map[*BlockParam]struct{}),
	}
	entry := b.NewBlock()
	b.SetBlock(entry)
	// The entry block has no predecessors; it can be sealed immediately.
	b.SealBlock(entry)
	return b
}

// addConstant registers v in the module's constant pool and returns the
// resulting [ModConstRef]. Comparable values are deduplicated; non-comparable
// values are appended each call.
func (mb *ModuleBuilder) addConstant(v value.Value) Ref {
	key := internKey(v)
	if key == nil {
		return NoRef
	}
	if mb.constIndex == nil {
		mb.constIndex = make(map[any]int32)
	}
	if id, ok := mb.constIndex[key]; ok {
		return ModConstRef(id)
	}
	id := int32(len(mb.mod.Constants))
	mb.mod.Constants = append(mb.mod.Constants, v)
	mb.constIndex[key] = id
	return ModConstRef(id)
}

type float64key uint64

func internKey(v value.Value) any {
	switch v := v.(type) {
	case value.None, value.Bool, value.Int, value.Decimal, value.Str, value.Bytes, value.Ratio, value.Fraction, value.Length, value.Relative, value.Angle:
		return v
	case value.Float:
		// Use a bitwise representation for float64 to ensure that different
		// float values that compare equal (e.g. +0.0 and -0.0) get different
		// keys, and that NaNs work as expected.
		return float64key(math.Float64bits(float64(v)))
	case *value.Function, *value.Type:
		// Universe builtins and reflected types are global singletons, so
		// pointer identity is a sound intern key. User-defined closures and
		// partial applications (Function.With) only materialize at runtime and
		// never reach the pool.
		return v
	default:
		return nil
	}
}

// Function returns the function under construction.
func (b *Builder) Function() *Function { return b.fn }

// CurrentBlock returns the block that subsequent [Emit]/terminator calls
// will append to.
func (b *Builder) CurrentBlock() BlockID { return b.cur }

// NewBlock allocates a fresh empty block and returns its id. The new block
// is not made current; use [SetBlock].
func (b *Builder) NewBlock() BlockID {
	id := BlockID(len(b.fn.Blocks))
	b.fn.Blocks = append(b.fn.Blocks, &BasicBlock{ID: id})
	return id
}

// SetBlock makes id the current insertion point.
func (b *Builder) SetBlock(id BlockID) { b.cur = id }

// SealBlock marks a block as having no more predecessors to be added. While
// the block was unsealed, [Jump]/[Branch] emissions targeting it carried
// empty arg lists and [readVarRecursive] created params without filling
// their incoming args (cycle hazard: a back-edge can reach back into this
// block during pred-walk). At seal time we walk every incomplete param and
// append one arg per predecessor (computed by reading the param's associated
// [Var] from each pred), then run trivial-param elimination.
//
// Branch joins are typically sealed immediately after their predecessors'
// terminators are emitted. Loop-header blocks are sealed only after the
// back-edge has been added.
func (b *Builder) SealBlock(block BlockID) {
	if _, ok := b.sealed[block]; ok {
		return
	}
	b.sealed[block] = struct{}{}
	pending := b.incompleteParams[block]
	delete(b.incompleteParams, block)
	if len(pending) == 0 {
		return
	}
	// For each predecessor, walk block.Params in declaration order and
	// append one arg per param. block.Params order is fixed by the order
	// of [newParam] calls during analyzer construction (source order), so
	// the resulting IR shape is deterministic across runs. currentDef
	// already maps each pending param's var to the param's Ref, so a
	// back-edge ReadVar that reaches this block resolves directly without
	// creating duplicates.
	params := b.fn.Blocks[block].Params
	for _, predID := range b.fn.Blocks[block].Preds {
		predTerm := b.fn.Blocks[predID].Term
		if predTerm == nil {
			continue
		}
		args := edgeArgs(predTerm, block)
		for _, p := range params {
			v := b.paramVar[p]
			if _, incomplete := pending[v]; !incomplete {
				continue
			}
			op := b.ReadVar(v, predID)
			*args = append(*args, op)
			b.recordParamUser(op, p)
		}
	}
	// Trivial-param elimination must happen after all args are populated
	// across every pred. Iterate block.Params (not the map) for deterministic
	// processing order; skip already-eliminated entries (cascade may have
	// processed them).
	for _, p := range params {
		v := b.paramVar[p]
		if _, incomplete := pending[v]; !incomplete {
			continue
		}
		b.tryRemoveTrivialParam(p)
	}
}

// Terminators /////////////////////////////////////////////////////////////////

// Jump terminates the current block with an unconditional branch to target,
// passing one arg per [BlockParam] on the target (in declaration order).
// The target's predecessor list is updated.
func (b *Builder) Jump(span syntax.Span, target BlockID) {
	cur := b.cur
	args := b.synthesizeArgs(target, cur)
	jmp := &Jump{term: term{span: span}, Target: target, Args: args}
	b.fn.Blocks[cur].Term = jmp
	b.fn.Blocks[target].Preds = append(b.fn.Blocks[target].Preds, cur)
	for i, a := range args {
		b.recordParamUser(a, b.fn.Blocks[target].Params[i])
	}
}

// Branch terminates the current block with a two-way branch on cond. Each
// arm carries one arg per [BlockParam] on its respective target. The
// analyzer is responsible for ensuring thenBlk != elseBlk; this is asserted.
func (b *Builder) Branch(span syntax.Span, cond Ref, thenBlk, elseBlk BlockID) {
	if thenBlk == elseBlk {
		panic("Branch: Then and Else must be distinct blocks")
	}
	cur := b.cur
	thenArgs := b.synthesizeArgs(thenBlk, cur)
	elseArgs := b.synthesizeArgs(elseBlk, cur)
	br := &Branch{
		term:     term{span: span},
		Cond:     cond,
		Then:     thenBlk,
		ThenArgs: thenArgs,
		Else:     elseBlk,
		ElseArgs: elseArgs,
	}
	b.fn.Blocks[cur].Term = br
	b.fn.Blocks[thenBlk].Preds = append(b.fn.Blocks[thenBlk].Preds, cur)
	b.fn.Blocks[elseBlk].Preds = append(b.fn.Blocks[elseBlk].Preds, cur)
	for i, a := range thenArgs {
		b.recordParamUser(a, b.fn.Blocks[thenBlk].Params[i])
	}
	for i, a := range elseArgs {
		b.recordParamUser(a, b.fn.Blocks[elseBlk].Params[i])
	}
}

// Return terminates the current block with a return of v. Pass [NoRef] for
// a bare return.
func (b *Builder) Return(span syntax.Span, v Ref) {
	b.fn.Blocks[b.cur].Term = &Return{term: term{span: span}, Value: v}
}

// synthesizeArgs reads the values needed to satisfy target's current
// [BlockParam] list, reading each param's source [Var] from cur. Returns
// nil for unsealed targets: [SealBlock] runs a single batched backfill once
// the pred list is final, so eager filling here would conflict.
func (b *Builder) synthesizeArgs(target, cur BlockID) []Ref {
	if _, ok := b.sealed[target]; !ok {
		return nil
	}
	params := b.fn.Blocks[target].Params
	if len(params) == 0 {
		return nil
	}
	args := make([]Ref, len(params))
	for i, p := range params {
		args[i] = b.ReadVar(b.paramVar[p], cur)
	}
	return args
}

// Instructions ////////////////////////////////////////////////////////////////

// Const registers v in the module's constant pool (deduping when possible)
// and returns the resulting module-constant [Ref]. No instruction is emitted
// and no function-local [Def] slot is consumed. span is currently unused —
// spans on constants only matter at their use sites, which carry their own
// spans on the consuming instruction.
func (b *Builder) Const(span syntax.Span, v value.Value) Ref {
	if ref := b.mb.addConstant(v); ref != NoRef {
		return ref
	}
	return b.emit(span, func(ref Ref) Instruction {
		return &Const{instr: instr{result: ref}, Value: v}
	})
}

// PeekVar returns the SSA def currently visible for v in this builder's
// current block, without inserting any block params. Returns (NoRef, false) when
// the variable has no definition recorded in the current block — predecessors
// are not walked. Used by side-effect-free callers (e.g. the analyzer's
// capture-time const peek) that want a conservative "current value" lookup.
func (b *Builder) PeekVar(v Var) (Ref, bool) {
	refs, ok := b.currentDef[v]
	if !ok {
		return NoRef, false
	}
	r, ok := refs[b.cur]
	return r, ok
}

// Unary emits a unary-operator instruction.
func (b *Builder) Unary(span syntax.Span, op syntax.UnaryOp, x Ref) Ref {
	if x.IsModConst() {
		xv := b.mb.mod.Constants[x.ModConstID()]
		v, err := value.UnaryOp(op, xv)
		if err == nil {
			return b.Const(span, v)
		}
	}
	return b.emit(span, func(ref Ref) Instruction {
		return &Unary{instr: instr{result: ref}, Op: op, X: x}
	})
}

// Binary emits a non-assignment binary-operator instruction.
func (b *Builder) Binary(span syntax.Span, op syntax.BinaryOp, l, r Ref) Ref {
	if l.IsModConst() && r.IsModConst() {
		lv := b.mb.mod.Constants[l.ModConstID()]
		rv := b.mb.mod.Constants[r.ModConstID()]
		v, err := value.BinaryOp(op, lv, rv)
		if err == nil {
			return b.Const(span, v)
		}
	}
	return b.emit(span, func(ref Ref) Instruction {
		return &Binary{instr: instr{result: ref}, Op: op, L: l, R: r}
	})
}

// MakeArray emits an array-construction instruction.
func (b *Builder) MakeArray(span syntax.Span, items []ArrayItem) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeArray{instr: instr{result: ref}, Items: items}
	})
}

// MakeDict emits a dict-construction instruction.
func (b *Builder) MakeDict(span syntax.Span, entries []DictEntry) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeDict{instr: instr{result: ref}, Entries: entries}
	})
}

// FieldRead emits a field-access instruction. span is the full
// `target.field` span; fieldSpan is the `.field`/field-ident span.
func (b *Builder) FieldRead(span, fieldSpan syntax.Span, target Ref, field name.Name) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &FieldRead{instr: instr{result: ref}, Target: target, Field: field, FieldSpan: fieldSpan}
	})
}

// Call emits a function-call instruction.
func (b *Builder) Call(span syntax.Span, callee Callee, args []CallArg, blocks []Ref, allowSetter bool) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Call{
			instr:       instr{result: ref},
			Callee:      callee,
			Args:        args,
			Blocks:      blocks,
			AllowSetter: allowSetter,
		}
	})
}

// CallSet emits a function-call lvalue assignment. The instruction is
// side-effect-only: it has no SSA result. Errors during the call flow
// through the session, not through a value Ref.
func (b *Builder) CallSet(span syntax.Span, callee Callee, args []CallArg, blocks []Ref, newVal Ref, op syntax.BinaryOp) {
	b.emitVoid(func() Instruction {
		return &CallSet{
			voidInstr: voidInstr{span: span},
			Callee:    callee,
			Args:      args,
			Blocks:    blocks,
			NewVal:    newVal,
			Op:        op,
		}
	})
}

// FieldWrite emits a field-write instruction. The instruction is
// side-effect-only: it has no SSA result. Errors during the write flow
// through the session, not through a value Ref.
func (b *Builder) FieldWrite(span syntax.Span, target Ref, field name.Name, newVal Ref, op syntax.BinaryOp) {
	b.emitVoid(func() Instruction {
		return &FieldWrite{
			voidInstr: voidInstr{span: span},
			Target:    target,
			Field:     field,
			NewVal:    newVal,
			Op:        op,
		}
	})
}

// Extract emits a positional-extraction instruction (destructuring).
func (b *Builder) Extract(span syntax.Span, source Ref, index int) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Extract{instr: instr{result: ref}, Source: source, Index: index}
	})
}

// LengthCheck emits a runtime length-assertion instruction for destructuring.
// It produces no SSA value, so no Ref is allocated.
func (b *Builder) LengthCheck(span syntax.Span, source Ref, want int, hasSink bool) {
	b.emitVoid(func() Instruction {
		return &LengthCheck{voidInstr: voidInstr{span: span}, Source: source, Want: want, HasSink: hasSink}
	})
}

// IterOpen emits an iterator-opening instruction for the given iterable.
func (b *Builder) IterOpen(span syntax.Span, iterable Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterOpen{instr: instr{result: ref}, Iterable: iterable}
	})
}

// IterHasNext emits a has-next check, producing a boolean SSA value.
func (b *Builder) IterHasNext(span syntax.Span, iter Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterHasNext{instr: instr{result: ref}, Iter: iter}
	})
}

// IterAdvance emits an iterator-advance, producing the next element.
func (b *Builder) IterAdvance(span syntax.Span, iter Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterAdvance{instr: instr{result: ref}, Iter: iter}
	})
}

// MakeClosure emits a closure-construction instruction.
func (b *Builder) MakeClosure(span syntax.Span, fn FuncID, captures []Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeClosure{instr: instr{result: ref}, Func: fn, Captures: captures}
	})
}

// Function structure helpers /////////////////////////////////////////////////

// AddParam appends a parameter to the function and returns its SSA Ref. The
// Ref is also stored on [Param.Ref] so callers iterating Params can find it.
func (b *Builder) AddParam(name name.Name, kind ParamKind, defaultVal Ref, span syntax.Span) Ref {
	ref := b.newRef(span)
	b.fn.Params = append(b.fn.Params, Param{Name: name, Kind: kind, Default: defaultVal, Ref: ref})
	return ref
}

// AddCapture appends a capture entry to the function and returns its SSA Ref.
// The source name is recorded on the builder (retrievable via
// [Builder.Captures]) but is not stored in the IR — the IR only needs
// the Ref. The runtime value flows in via the [MakeClosure] instruction at
// the call site.
func (b *Builder) AddCapture(source name.Name, span syntax.Span) Ref {
	ref := b.newRef(span)
	b.fn.Captures = append(b.fn.Captures, ref)
	b.captures = append(b.captures, source)
	return ref
}

// Captures returns the source names of captures registered via [AddCapture], in
// the order they were added. Used by the analyzer to resolve each capture to
// its outer-scope Ref before emitting [MakeClosure]. Index i in the returned
// slice corresponds to index i in [Function.Captures].
func (b *Builder) Captures() []name.Name { return b.captures }

// NewVar mints a fresh [Var] for source, assigning the next 1-based version
// for that name in this function. Distinct calls with the same source name
// (shadowing) yield distinct Vars; different names start at 1 independently.
func (b *Builder) NewVar(source name.Name) Var {
	if b.versions == nil {
		b.versions = make(map[name.Name]int)
	}
	b.versions[source]++
	return Var{Name: source, Version: b.versions[source]}
}

// Self returns the self-reference Ref for the function under construction,
// allocating it on first call. References to this Ref materialize to the
// currently-executing closure value at runtime, enabling direct recursion
// without a capture.
func (b *Builder) Self(span syntax.Span) Ref {
	if b.fn.SelfRef != NoRef {
		return b.fn.SelfRef
	}
	b.fn.SelfRef = b.newRef(span)
	return b.fn.SelfRef
}

// Markup helpers //////////////////////////////////////////////////////////////

// ContentResult emits a content-join instruction over items.
func (b *Builder) ContentResult(span syntax.Span, items []Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ContentResult{instr: instr{result: ref}, Items: items}
	})
}

// AttachLabel emits a label-attachment instruction.
func (b *Builder) AttachLabel(span syntax.Span, content Ref, label name.Name) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &AttachLabel{instr: instr{result: ref}, Content: content, Label: label}
	})
}

// Error emits an instruction that, at eval time, raises a value error with
// msg. If from is non-NoRef and resolves to a [value.Error] at runtime, the
// upstream error is propagated as the result and msg is suppressed.
func (b *Builder) Error(span syntax.Span, msg string, from Ref, hints ...string) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Error{instr: instr{result: ref}, Msg: msg, Hints: hints, From: from}
	})
}

// CodeJoin emits a code-join instruction over items. itemSpans, if non-nil,
// must be parallel to items and provides per-item spans for error reporting.
func (b *Builder) CodeJoin(span syntax.Span, items []Ref, itemSpans []syntax.Span) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &CodeJoin{instr: instr{result: ref}, Items: items, ItemSpans: itemSpans}
	})
}

// LoopAccBegin emits an instruction starting a loop accumulator.
func (b *Builder) LoopAccBegin(span syntax.Span) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccBegin{instr: instr{result: ref}}
	})
}

// LoopAccAdd emits an instruction appending item to acc.
func (b *Builder) LoopAccAdd(span syntax.Span, acc, item Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccAdd{instr: instr{result: ref}, Acc: acc, Item: item}
	})
}

// LoopAccResult emits an instruction finalizing the accumulator.
func (b *Builder) LoopAccResult(span syntax.Span, acc Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccResult{instr: instr{result: ref}, Acc: acc}
	})
}

// Heading emits a heading instruction.
func (b *Builder) Heading(span syntax.Span, level int, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Heading{instr: instr{result: ref}, Level: level, Body: body}
	})
}

// Strong emits a strong (bold) instruction.
func (b *Builder) Strong(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Strong{instr: instr{result: ref}, Body: body}
	})
}

// Emph emits an emphasis (italic) instruction.
func (b *Builder) Emph(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Emph{instr: instr{result: ref}, Body: body}
	})
}

// Link emits a link instruction.
func (b *Builder) Link(span syntax.Span, dest string, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Link{instr: instr{result: ref}, Dest: dest, Body: body}
	})
}

// RefMarkup emits a label-reference instruction.
func (b *Builder) RefMarkup(span syntax.Span, target name.Name, supplement Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &RefMarkup{instr: instr{result: ref}, Target: target, Supplement: supplement}
	})
}

// ListItem emits a list-item instruction.
func (b *Builder) ListItem(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ListItem{instr: instr{result: ref}, Body: body}
	})
}

// EnumItem emits an enumerated-list-item instruction.
func (b *Builder) EnumItem(span syntax.Span, number int, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &EnumItem{instr: instr{result: ref}, Number: number, Body: body}
	})
}

// TermItem emits a definition-list-item instruction.
func (b *Builder) TermItem(span syntax.Span, term, description Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &TermItem{instr: instr{result: ref}, Term: term, Description: description}
	})
}

// SetRule emits a set-rule instruction.
func (b *Builder) SetRule(span syntax.Span, target Ref, args []CallArg, condition Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &SetRule{instr: instr{result: ref}, Target: target, Args: args, Condition: condition}
	})
}

// ShowRule emits a show-rule instruction.
func (b *Builder) ShowRule(span syntax.Span, selector, transform Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ShowRule{instr: instr{result: ref}, Selector: selector, Transform: transform}
	})
}

// Contextual emits a context-block instruction.
func (b *Builder) Contextual(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Contextual{instr: instr{result: ref}, Body: body}
	})
}

// ModuleInclude emits a module-include instruction.
func (b *Builder) ModuleInclude(span syntax.Span, source Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ModuleInclude{instr: instr{result: ref}, Source: source}
	})
}

// newRef allocates a fresh function-local Ref and records its span.
func (b *Builder) newRef(span syntax.Span) Ref {
	ref := Ref(len(b.fn.RefSpans))
	b.fn.RefSpans = append(b.fn.RefSpans, span)
	return ref
}

// emit allocates a fresh Ref, hands it to mkInstr to produce the instruction,
// then appends to the current block.
func (b *Builder) emit(span syntax.Span, mkInstr func(result Ref) Instruction) Ref {
	ref := b.newRef(span)
	instance := mkInstr(ref)
	b.fn.Blocks[b.cur].Instrs = append(b.fn.Blocks[b.cur].Instrs, instance)
	return ref
}

// emitVoid appends a side-effect-only instruction to the current block
// without allocating a Ref. The instruction's [Instruction.Result] must be
// [NoRef]; the evaluator skips the result-slot write for these kinds.
func (b *Builder) emitVoid(mkInstr func() Instruction) {
	b.fn.Blocks[b.cur].Instrs = append(b.fn.Blocks[b.cur].Instrs, mkInstr())
}

// Braun SSA construction //////////////////////////////////////////////////////

// WriteVar records that v has SSA value val visible from block onwards.
// This is the user-level entry point (called from analyzer lowering); the
// write is also recorded in [Builder.liveWrites] so DCE keeps val alive
// even when v is never read again. Internal SSA-construction writes
// (cycle-breaking caches in [readVarRecursive]) call [writeVarInternal]
// instead so they don't artificially pin block-param results.
func (b *Builder) WriteVar(v Var, block BlockID, val Ref) {
	b.writeVarInternal(v, block, val)
	if val >= 0 {
		b.liveWrites = append(b.liveWrites, val)
	}
}

// writeVarInternal is the unbookkept WriteVar used by SSA construction.
func (b *Builder) writeVarInternal(v Var, block BlockID, val Ref) {
	defs, ok := b.currentDef[v]
	if !ok {
		defs = make(map[BlockID]Ref)
		b.currentDef[v] = defs
	}
	defs[block] = val
}

// ReadVar returns the SSA value for v visible from block, inserting block
// params as needed. Returns [NoRef] if v has never been written.
func (b *Builder) ReadVar(v Var, block BlockID) Ref {
	if v.Name == name.Invalid {
		panic("invalid var")
	}
	if ref, ok := b.currentDef[v][block]; ok {
		return ref
	}
	return b.readVarRecursive(v, block)
}

func (b *Builder) readVarRecursive(v Var, block BlockID) Ref {
	var val Ref
	if _, ok := b.sealed[block]; !ok {
		// Unsealed (e.g. loop header before back-edge is in): place an
		// incomplete param. Args are filled at SealBlock when the pred
		// list is final. Deferring avoids the cycle hazard from a back-edge
		// reaching this block during pred-walk.
		p := b.allocateParam(block, v)
		val = p.result
		if b.incompleteParams[block] == nil {
			b.incompleteParams[block] = make(map[Var]*BlockParam)
		}
		b.incompleteParams[block][v] = p
	} else if preds := b.fn.Blocks[block].Preds; len(preds) == 1 {
		val = b.ReadVar(v, preds[0])
	} else if len(preds) == 0 {
		// No predecessors: var is undefined here. Return NoRef so the
		// caller (analyzer) can report an error.
		return NoRef
	} else {
		// Sealed multi-pred: allocate param, write currentDef *before*
		// the backfill walk so a recursive ReadVar that reaches this
		// block via a pred finds the param and terminates.
		p := b.allocateParam(block, v)
		val = p.result
		b.writeVarInternal(v, block, val)
		b.backfillParamArgs(p)
		val = b.tryRemoveTrivialParam(p)
	}
	b.writeVarInternal(v, block, val)
	return val
}

// allocateParam appends a fresh block parameter for v on block without
// touching any predecessor terminator's args. The (param → var) mapping is
// recorded in [Builder.paramVar] for later arg synthesis. Use
// [backfillParamArgs] (for sealed blocks) or [SealBlock] (for unsealed
// blocks) to populate args.
func (b *Builder) allocateParam(block BlockID, v Var) *BlockParam {
	ref := b.newRef(syntax.Span{})
	p := &BlockParam{result: ref, block: block}
	b.paramVar[p] = v
	b.fn.Blocks[block].Params = append(b.fn.Blocks[block].Params, p)
	return p
}

// backfillParamArgs walks p.block.Preds and appends one arg per predecessor
// to the matching edge's args list, computed as ReadVar(paramVar[p], pred).
// Must only be called when p.block is sealed (preds final). Caller is
// responsible for writing currentDef[v][block] = p.result before invoking,
// to break the recursion cycle through back-edges.
func (b *Builder) backfillParamArgs(p *BlockParam) {
	v := b.paramVar[p]
	for _, predID := range b.fn.Blocks[p.block].Preds {
		predTerm := b.fn.Blocks[predID].Term
		if predTerm == nil {
			continue
		}
		args := edgeArgs(predTerm, p.block)
		op := b.ReadVar(v, predID)
		*args = append(*args, op)
		b.recordParamUser(op, p)
	}
}

// tryRemoveTrivialParam: a block param is trivial if all the values flowing
// into it (across every incoming edge) are the same ref (ignoring
// self-references). In that case it can be replaced by that single value.
// Returns the Ref to use in place of the (possibly removed) param.
//
// Rather than rewriting every instruction operand that references the param
// — which would require type-switching every Instruction kind — we record a
// rename in [Builder.redirects] pointing the param's Ref at its single
// value. [Builder.Finalize] closes the map, rewrites every Ref in the IR
// through it, and physically splices the dead param slot. The "already
// trivialized" check uses [Builder.redirects] directly (instead of a dead
// flag on the IR type), keeping cascade idempotent without leaking build
// state into [BlockParam].
//
// Must only be called on a param whose block is sealed (so the incoming
// args set is final).
func (b *Builder) tryRemoveTrivialParam(p *BlockParam) Ref {
	if t, done := b.redirects[p.result]; done {
		return t
	}
	same := NoRef
	paramIdx := b.paramIndex(p)
	for _, predID := range b.fn.Blocks[p.block].Preds {
		predTerm := b.fn.Blocks[predID].Term
		if predTerm == nil {
			continue
		}
		args := *edgeArgs(predTerm, p.block)
		if paramIdx >= len(args) {
			continue
		}
		op := args[paramIdx]
		if op == same || op == p.result {
			continue
		}
		if same != NoRef {
			return p.result // multiple distinct operands: non-trivial
		}
		same = op
	}
	if same == NoRef {
		// Sits on unreachable code (no real defining operands). Leave alone;
		// the analyzer surfaces undefined-var errors via NoRef-tainted reads.
		return p.result
	}
	users := b.paramUsers[p.result]
	delete(b.paramUsers, p.result)
	b.replaceUses(p.result, same)
	if b.redirects == nil {
		b.redirects = make(map[Ref]Ref)
	}
	b.redirects[p.result] = same
	for user := range users {
		if user != p {
			b.tryRemoveTrivialParam(user)
		}
	}
	return same
}

// paramIndex returns p's position in p.block.Params. Result is stable
// during construction: params are only appended (trivial removals are
// flagged via [BlockParam.dead], not spliced).
func (b *Builder) paramIndex(p *BlockParam) int {
	return slices.Index(b.fn.Blocks[p.block].Params, p)
}

// recordParamUser tracks that p receives operand on at least one incoming
// edge, for later use chains during trivial-param elimination cascade.
func (b *Builder) recordParamUser(operand Ref, p *BlockParam) {
	if operand == NoRef || operand.IsModConst() {
		return
	}
	users := b.paramUsers[operand]
	if users == nil {
		users = make(map[*BlockParam]struct{})
		b.paramUsers[operand] = users
	}
	users[p] = struct{}{}
}

// replaceUses rewrites every incoming-arg slot referencing old to new (for
// every param recorded in paramUsers[old]), plus every currentDef entry.
// Non-param consumers (regular instructions, terminator operands) cannot be
// rewritten through this path; trivial-param elimination relies on the
// invariant that no such consumer has observed the param's Ref yet, which
// holds while the builder is the only emitter.
func (b *Builder) replaceUses(old, new Ref) {
	for p := range b.paramUsers[old] {
		idx := b.paramIndex(p)
		if idx < 0 {
			continue
		}
		for _, predID := range b.fn.Blocks[p.block].Preds {
			predTerm := b.fn.Blocks[predID].Term
			if predTerm == nil {
				continue
			}
			args := edgeArgs(predTerm, p.block)
			if idx < len(*args) && (*args)[idx] == old {
				(*args)[idx] = new
				b.recordParamUser(new, p)
			}
		}
	}
	delete(b.paramUsers, old)
	for _, perBlock := range b.currentDef {
		for blk, ref := range perBlock {
			if ref == old {
				perBlock[blk] = new
			}
		}
	}
}
