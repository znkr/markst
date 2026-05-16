package expr

import (
	"cmp"
	"maps"
	"slices"

	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// Builder constructs an SSA [Function]. The analyzer drives it as it walks
// the syntax tree, emitting instructions and tracking variable definitions
// per block. SSA phi nodes are inserted on demand by [Builder.ReadVar],
// following Braun, Buchwald & Hack (2013), "Simple and Efficient Construction
// of Static Single Assignment Form".
//
// Typical usage:
//
//	b := expr.NewBuilder()
//	v := b.Const(span, value.Int(1))
//	b.WriteVar(name, b.CurrentBlock(), v)
//	then := b.NewBlock()
//	els  := b.NewBlock()
//	join := b.NewBlock()
//	b.Branch(span, cond, then, els)
//	... emit then and els blocks, each ending with b.Jump(join) ...
//	b.SealBlock(join)
//	r := b.ReadVar(name, join)   // inserts a phi at join if needed
//	fn := b.Build()
type Builder struct {
	fn  *Function
	cur BlockID

	// Braun "current definition" table: per-variable, per-block, the SSA
	// Ref currently visible. Populated by [WriteVar] and consulted by
	// [ReadVar].
	currentDef map[Var]map[BlockID]Ref

	// Per unsealed block: phis created on demand for variables not yet
	// defined in the block's predecessors. Filled when the block is sealed.
	incompletePhis map[BlockID]map[Var]*Phi

	// Set of blocks that have been sealed. A block is sealed when all of
	// its predecessors have been recorded; only then can phi operands be
	// computed by walking predecessors.
	sealed map[BlockID]bool

	// Use chains for trivial-phi elimination: maps each Ref that is a phi
	// operand to the set of phis using it. Updated whenever a phi gains or
	// loses an operand.
	phiUsers map[Ref]map[*Phi]struct{}

	// selfRef caches the [DefSelf] Ref for this function. NoRef until the
	// first [Self] call allocates one.
	selfRef Ref

	// versions tracks how many [Var]s have been minted for each source
	// name in this function. Versions are per-name and 1-based, so
	// `let x; let x` yields x$1 then x$2, while a separate `let y` starts
	// at y$1. Used by [NewVar].
	versions map[name.Name]int
}

// NewBuilder starts construction of a fresh [Function]. The entry block (id 0)
// is created and made current.
func NewBuilder() *Builder {
	b := &Builder{
		fn:             &Function{},
		currentDef:     make(map[Var]map[BlockID]Ref),
		incompletePhis: make(map[BlockID]map[Var]*Phi),
		sealed:         make(map[BlockID]bool),
		phiUsers:       make(map[Ref]map[*Phi]struct{}),
		selfRef:        NoRef,
	}
	entry := b.NewBlock()
	b.SetBlock(entry)
	// The entry block has no predecessors; it can be sealed immediately.
	b.SealBlock(entry)
	return b
}

// Function returns the function under construction. Callers should generally
// not mutate it until [Build] has been called.
func (b *Builder) Function() *Function { return b.fn }

// Build finalizes and returns the function. The Builder must not be used
// after Build returns.
func (b *Builder) Build() *Function {
	return b.fn
}

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

// SealBlock marks a block as having no more predecessors to be added. Any
// incomplete phis on the block have their operands filled by reading the
// associated variable from each predecessor.
//
// Branch joins are typically sealed immediately after their predecessors'
// terminators are emitted. Loop-header blocks are sealed only after the
// back-edge has been added.
func (b *Builder) SealBlock(block BlockID) {
	if b.sealed[block] {
		return
	}
	// Iterate in sorted name order so the IR shape (Ref numbering, phi
	// operand order) is deterministic across runs even when multiple
	// incomplete phis exist on the same block.
	pending := b.incompletePhis[block]
	vars := slices.AppendSeq(make([]Var, 0, len(pending)), maps.Keys(pending))
	slices.SortFunc(vars, func(a, b Var) int {
		n := cmp.Compare(a.Name.String(), b.Name.String())
		if n != 0 {
			return n
		}
		return cmp.Compare(a.Version, b.Version)
	})
	for _, v := range vars {
		b.addPhiOperands(v, pending[v])
	}
	delete(b.incompletePhis, block)
	b.sealed[block] = true
}

// Terminators /////////////////////////////////////////////////////////////////

// Jump terminates the current block with an unconditional branch to target.
// The target's predecessor list is updated.
func (b *Builder) Jump(span syntax.Span, target BlockID) {
	b.fn.Blocks[b.cur].Term = &Jump{term: term{span: span}, Target: target}
	b.fn.Blocks[target].Preds = append(b.fn.Blocks[target].Preds, b.cur)
}

// Branch terminates the current block with a two-way branch on cond.
func (b *Builder) Branch(span syntax.Span, cond Ref, thenBlk, elseBlk BlockID) {
	b.fn.Blocks[b.cur].Term = &Branch{term: term{span: span}, Cond: cond, Then: thenBlk, Else: elseBlk}
	b.fn.Blocks[thenBlk].Preds = append(b.fn.Blocks[thenBlk].Preds, b.cur)
	b.fn.Blocks[elseBlk].Preds = append(b.fn.Blocks[elseBlk].Preds, b.cur)
}

// Return terminates the current block with a return of v. Pass [NoRef] for
// a bare return.
func (b *Builder) Return(span syntax.Span, v Ref) {
	b.fn.Blocks[b.cur].Term = &Return{term: term{span: span}, Value: v}
}

// Instructions ////////////////////////////////////////////////////////////////

// Const emits a [Const] instruction in the current block.
func (b *Builder) Const(span syntax.Span, v value.Value) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Const{instr: instr{result: ref, span: span}, Value: v}
	})
}

// Unary emits a unary-operator instruction.
func (b *Builder) Unary(span syntax.Span, op syntax.UnaryOp, x Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Unary{instr: instr{result: ref, span: span}, Op: op, X: x}
	})
}

// Binary emits a non-assignment binary-operator instruction.
func (b *Builder) Binary(span syntax.Span, op syntax.BinaryOp, l, r Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Binary{instr: instr{result: ref, span: span}, Op: op, L: l, R: r}
	})
}

// MakeArray emits an array-construction instruction.
func (b *Builder) MakeArray(span syntax.Span, items []ArrayItem) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeArray{instr: instr{result: ref, span: span}, Items: items}
	})
}

// MakeDict emits a dict-construction instruction.
func (b *Builder) MakeDict(span syntax.Span, entries []DictEntry) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeDict{instr: instr{result: ref, span: span}, Entries: entries}
	})
}

// FieldRead emits a field-access instruction. span is the full
// `target.field` span; fieldSpan is the `.field`/field-ident span.
func (b *Builder) FieldRead(span, fieldSpan syntax.Span, target Ref, field name.Name) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &FieldRead{instr: instr{result: ref, span: span}, Target: target, Field: field, FieldSpan: fieldSpan}
	})
}

// Call emits a function-call instruction.
func (b *Builder) Call(span syntax.Span, callee Ref, args []CallArg, blocks []Ref, allowSetter bool) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Call{
			instr:       instr{result: ref, span: span},
			Callee:      callee,
			Args:        args,
			Blocks:      blocks,
			AllowSetter: allowSetter,
		}
	})
}

// CallSet emits a function-call lvalue assignment.
func (b *Builder) CallSet(span syntax.Span, callee Ref, args []CallArg, blocks []Ref, newVal Ref, op syntax.BinaryOp) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &CallSet{
			instr:  instr{result: ref, span: span},
			Callee: callee,
			Args:   args,
			Blocks: blocks,
			NewVal: newVal,
			Op:     op,
		}
	})
}

// FieldWrite emits a field-write instruction.
func (b *Builder) FieldWrite(span syntax.Span, target Ref, field name.Name, newVal Ref, op syntax.BinaryOp) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &FieldWrite{
			instr:  instr{result: ref, span: span},
			Target: target,
			Field:  field,
			NewVal: newVal,
			Op:     op,
		}
	})
}

// Extract emits a positional-extraction instruction (destructuring).
func (b *Builder) Extract(span syntax.Span, source Ref, index int) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Extract{instr: instr{result: ref, span: span}, Source: source, Index: index}
	})
}

// LengthCheck emits a runtime length-assertion instruction for destructuring.
// It produces no SSA value.
func (b *Builder) LengthCheck(span syntax.Span, source Ref, want int, hasSink bool) {
	b.emit(span, func(ref Ref) Instruction {
		return &LengthCheck{instr: instr{result: ref, span: span}, Source: source, Want: want, HasSink: hasSink}
	})
}

// IterOpen emits an iterator-opening instruction for the given iterable.
func (b *Builder) IterOpen(span syntax.Span, iterable Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterOpen{instr: instr{result: ref, span: span}, Iterable: iterable}
	})
}

// IterHasNext emits a has-next check, producing a boolean SSA value.
func (b *Builder) IterHasNext(span syntax.Span, iter Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterHasNext{instr: instr{result: ref, span: span}, Iter: iter}
	})
}

// IterAdvance emits an iterator-advance, producing the next element.
func (b *Builder) IterAdvance(span syntax.Span, iter Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &IterAdvance{instr: instr{result: ref, span: span}, Iter: iter}
	})
}

// MakeClosure emits a closure-construction instruction.
func (b *Builder) MakeClosure(span syntax.Span, fn FuncID, captures []Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &MakeClosure{instr: instr{result: ref, span: span}, Func: fn, Captures: captures}
	})
}

// Function structure helpers /////////////////////////////////////////////////

// AddParam appends a parameter to the function and returns its SSA Ref.
// The Ref's Def is of kind [DefParam].
func (b *Builder) AddParam(name name.Name, kind ParamKind, defaultVal Ref, span syntax.Span) Ref {
	idx := len(b.fn.Params)
	b.fn.Params = append(b.fn.Params, Param{Name: name, Kind: kind, Default: defaultVal, Span: span})
	ref := Ref(len(b.fn.Defs))
	b.fn.Defs = append(b.fn.Defs, &DefParam{Idx: idx, def: def{span: span}})
	return ref
}

// AddCapture appends a capture entry to the function and returns its SSA Ref.
// The Ref's Def is of kind [DefCapture]. The runtime value flows in via the
// [MakeClosure] instruction at the call site.
func (b *Builder) AddCapture(v Var, span syntax.Span) Ref {
	idx := len(b.fn.Captures)
	b.fn.Captures = append(b.fn.Captures, v)
	ref := Ref(len(b.fn.Defs))
	b.fn.Defs = append(b.fn.Defs, &DefCapture{Idx: idx, def: def{span: span}})
	return ref
}

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

// Self returns the [DefSelf] Ref for the function under construction,
// allocating it on first call. References to this Ref materialize to the
// currently-executing closure value at runtime, enabling direct recursion
// without a capture.
func (b *Builder) Self(span syntax.Span) Ref {
	if b.selfRef != NoRef {
		return b.selfRef
	}
	ref := Ref(len(b.fn.Defs))
	b.fn.Defs = append(b.fn.Defs, &DefSelf{def: def{span: span}})
	b.selfRef = ref
	return ref
}

// Markup helpers /////////////////////////////////////////////////////////////

// ContentResult emits a content-join instruction over items.
func (b *Builder) ContentResult(span syntax.Span, items []Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ContentResult{instr: instr{result: ref, span: span}, Items: items}
	})
}

// AttachLabel emits a label-attachment instruction.
func (b *Builder) AttachLabel(span syntax.Span, content Ref, label name.Name) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &AttachLabel{instr: instr{result: ref, span: span}, Content: content, Label: label}
	})
}

// Error emits an instruction that, at eval time, raises a value error with
// msg. If from is non-NoRef and resolves to a [value.Error] at runtime, the
// upstream error is propagated as the result and msg is suppressed.
func (b *Builder) Error(span syntax.Span, msg string, from Ref, hints ...string) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Error{instr: instr{result: ref, span: span}, Msg: msg, Hints: hints, From: from}
	})
}

// CodeJoin emits a code-join instruction over items. itemSpans, if non-nil,
// must be parallel to items and provides per-item spans for error reporting.
func (b *Builder) CodeJoin(span syntax.Span, items []Ref, itemSpans []syntax.Span) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &CodeJoin{instr: instr{result: ref, span: span}, Items: items, ItemSpans: itemSpans}
	})
}

// LoopAccBegin emits an instruction starting a loop accumulator.
func (b *Builder) LoopAccBegin(span syntax.Span) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccBegin{instr: instr{result: ref, span: span}}
	})
}

// LoopAccAdd emits an instruction appending item to acc.
func (b *Builder) LoopAccAdd(span syntax.Span, acc, item Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccAdd{instr: instr{result: ref, span: span}, Acc: acc, Item: item}
	})
}

// LoopAccResult emits an instruction finalizing the accumulator.
func (b *Builder) LoopAccResult(span syntax.Span, acc Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &LoopAccResult{instr: instr{result: ref, span: span}, Acc: acc}
	})
}

// Heading emits a heading instruction.
func (b *Builder) Heading(span syntax.Span, level int, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Heading{instr: instr{result: ref, span: span}, Level: level, Body: body}
	})
}

// Strong emits a strong (bold) instruction.
func (b *Builder) Strong(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Strong{instr: instr{result: ref, span: span}, Body: body}
	})
}

// Emph emits an emphasis (italic) instruction.
func (b *Builder) Emph(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Emph{instr: instr{result: ref, span: span}, Body: body}
	})
}

// Link emits a link instruction.
func (b *Builder) Link(span syntax.Span, dest string, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Link{instr: instr{result: ref, span: span}, Dest: dest, Body: body}
	})
}

// RefMarkup emits a label-reference instruction.
func (b *Builder) RefMarkup(span syntax.Span, target name.Name, supplement Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &RefMarkup{instr: instr{result: ref, span: span}, Target: target, Supplement: supplement}
	})
}

// ListItem emits a list-item instruction.
func (b *Builder) ListItem(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ListItem{instr: instr{result: ref, span: span}, Body: body}
	})
}

// EnumItem emits an enumerated-list-item instruction.
func (b *Builder) EnumItem(span syntax.Span, number int, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &EnumItem{instr: instr{result: ref, span: span}, Number: number, Body: body}
	})
}

// TermItem emits a definition-list-item instruction.
func (b *Builder) TermItem(span syntax.Span, term, description Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &TermItem{instr: instr{result: ref, span: span}, Term: term, Description: description}
	})
}

// SetRule emits a set-rule instruction.
func (b *Builder) SetRule(span syntax.Span, target Ref, args []CallArg, condition Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &SetRule{instr: instr{result: ref, span: span}, Target: target, Args: args, Condition: condition}
	})
}

// ShowRule emits a show-rule instruction.
func (b *Builder) ShowRule(span syntax.Span, selector, transform Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ShowRule{instr: instr{result: ref, span: span}, Selector: selector, Transform: transform}
	})
}

// Contextual emits a context-block instruction.
func (b *Builder) Contextual(span syntax.Span, body Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &Contextual{instr: instr{result: ref, span: span}, Body: body}
	})
}

// ModuleInclude emits a module-include instruction.
func (b *Builder) ModuleInclude(span syntax.Span, source Ref) Ref {
	return b.emit(span, func(ref Ref) Instruction {
		return &ModuleInclude{instr: instr{result: ref, span: span}, Source: source}
	})
}

// emit allocates a fresh Ref, hands it to mkInst to produce the instruction,
// then appends to the current block and registers the Def.
func (b *Builder) emit(span syntax.Span, mkInst func(result Ref) Instruction) Ref {
	ref := Ref(len(b.fn.Defs))
	d := &DefInstr{def: def{block: b.cur, span: span}}
	b.fn.Defs = append(b.fn.Defs, d)
	instance := mkInst(ref)
	d.Instr = instance
	b.fn.Blocks[b.cur].Instrs = append(b.fn.Blocks[b.cur].Instrs, instance)
	return ref
}

// Braun SSA construction //////////////////////////////////////////////////////

// WriteVar records that v has SSA value val visible from block onwards.
func (b *Builder) WriteVar(v Var, block BlockID, val Ref) {
	defs, ok := b.currentDef[v]
	if !ok {
		defs = make(map[BlockID]Ref)
		b.currentDef[v] = defs
	}
	defs[block] = val
}

// ReadVar returns the SSA value for v visible from block, inserting phi
// nodes as needed. Returns [NoRef] if v has never been written.
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
	if !b.sealed[block] {
		// Unsealed (e.g. loop header before back-edge is in): place an
		// incomplete phi that will be filled at SealBlock.
		phi := b.newPhi(block)
		val = phi.result
		if b.incompletePhis[block] == nil {
			b.incompletePhis[block] = make(map[Var]*Phi)
		}
		b.incompletePhis[block][v] = phi
	} else if preds := b.fn.Blocks[block].Preds; len(preds) == 1 {
		val = b.ReadVar(v, preds[0])
	} else if len(preds) == 0 {
		// No predecessors: var is undefined here. Return NoRef so the
		// caller (analyzer) can report an error.
		return NoRef
	} else {
		// Place phi, write before recursing to break cycles.
		phi := b.newPhi(block)
		val = phi.result
		b.WriteVar(v, block, val)
		val = b.addPhiOperands(v, phi)
	}
	b.WriteVar(v, block, val)
	return val
}

// newPhi creates an empty phi attached to block and registers a Def for it.
func (b *Builder) newPhi(block BlockID) *Phi {
	ref := Ref(len(b.fn.Defs))
	phi := &Phi{result: ref}
	b.fn.Defs = append(b.fn.Defs, &DefPhi{Phi: phi, def: def{block: block}})
	b.fn.Blocks[block].Phis = append(b.fn.Blocks[block].Phis, phi)
	return phi
}

// addPhiOperands fills a phi's operand list by reading v from each
// predecessor, then runs trivial-phi elimination. Returns the Ref that the
// phi resolved to (which may differ from phi.result if it was trivial).
func (b *Builder) addPhiOperands(v Var, phi *Phi) Ref {
	block := b.fn.Defs[phi.result].Block()
	for _, pred := range b.fn.Blocks[block].Preds {
		operand := b.ReadVar(v, pred)
		phi.operands = append(phi.operands, PhiOperand{Pred: pred, Value: operand})
		b.recordPhiUser(operand, phi)
	}
	return b.tryRemoveTrivialPhi(phi)
}

// tryRemoveTrivialPhi: a phi is trivial if all its operands are the same
// value (ignoring self-references). In that case the phi can be replaced by
// its single operand. Returns the Ref to use in place of the (possibly
// removed) phi.
//
// Rather than rewriting every instruction operand that references the phi
// (which would require type-switching every Instruction kind), we install a
// [DefRedirect] entry pointing the phi's Ref at its single operand. The
// evaluator and formatter both call [Function.Resolve] to follow these.
func (b *Builder) tryRemoveTrivialPhi(phi *Phi) Ref {
	same := NoRef
	for _, op := range phi.operands {
		if op.Value == same || op.Value == phi.result {
			continue
		}
		if same != NoRef {
			return phi.result // multiple distinct operands: non-trivial
		}
		same = op.Value
	}
	if same == NoRef {
		// Phi sits on unreachable code (no real defining operands). Leave it
		// alone; the user will be reading NoRef-tainted data and that's the
		// analyzer's problem.
		return phi.result
	}
	users := b.phiUsers[phi.result]
	delete(b.phiUsers, phi.result)
	b.replaceUses(phi.result, same)
	// Convert the phi's Def into a redirect to `same` so downstream readers
	// (eval, format, future passes) follow through transparently. The phi
	// itself stays in the block list but is no longer a live def.
	b.fn.Defs[phi.result] = &DefRedirect{Redirect: same, def: def{block: b.fn.Defs[phi.result].Block(), span: b.fn.Defs[phi.result].Span()}}
	b.removePhi(phi)
	for user := range users {
		if user != phi {
			b.tryRemoveTrivialPhi(user)
		}
	}
	return same
}

// recordPhiUser tracks that phi uses operand, for later use chains.
func (b *Builder) recordPhiUser(operand Ref, phi *Phi) {
	if operand == NoRef {
		return
	}
	users := b.phiUsers[operand]
	if users == nil {
		users = make(map[*Phi]struct{})
		b.phiUsers[operand] = users
	}
	users[phi] = struct{}{}
}

// replaceUses rewrites every phi operand from old to new, plus every
// currentDef entry. (Non-phi instructions cannot be rewritten through this
// path because trivial-phi removal must run *before* any consumers see the
// phi, which is true while the builder is the only emitter.)
func (b *Builder) replaceUses(old, new Ref) {
	// Rewrite phi operands that pointed at old.
	for phi := range b.phiUsers[old] {
		for i, op := range phi.operands {
			if op.Value == old {
				phi.operands[i].Value = new
				b.recordPhiUser(new, phi)
			}
		}
	}
	delete(b.phiUsers, old)
	// Rewrite currentDef entries.
	for _, perBlock := range b.currentDef {
		for blk, ref := range perBlock {
			if ref == old {
				perBlock[blk] = new
			}
		}
	}
}

// removePhi detaches phi from its block.
func (b *Builder) removePhi(phi *Phi) {
	block := b.fn.Defs[phi.result].Block()
	phis := b.fn.Blocks[block].Phis
	for i, p := range phis {
		if p == phi {
			b.fn.Blocks[block].Phis = append(phis[:i], phis[i+1:]...)
			break
		}
	}
}
