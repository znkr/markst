package expr

import (
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// resultSetter is implemented by every value-producing IR node (instructions
// embedding [instr], and [*BlockParam]). Void instructions (those embedding
// [voidInstr]) do not implement it, so the finalize loop naturally skips them
// via a type assertion.
type resultSetter interface {
	setResult(Ref)
}

// producer indexes the in-block def of a function-local Ref. Only set for
// in-block producers — block params and instructions. Function params,
// captures, and self never get an entry — their `exists` flag stays false,
// so [Builder.droppable] refuses to mark them dead.
type producer struct {
	block   BlockID
	idx     int // index into block.Params (isParam) or block.Instrs
	isParam bool
	exists  bool
}

// Finalize seals the function for consumption in a single combined pass:
//
//  1. Trivial-param rewrite: a reference to a param removed by
//     [tryRemoveTrivialParam] follows the rename map to the param's single
//     incoming value.
//  2. Dead-code elimination: pure instructions and block params whose result
//     Ref has no uses are dropped. When a param is dropped its slot is
//     spliced from every incoming terminator's arg list in lock-step. Cascades
//     to fixed point. A joiner fixup pre-pass strips constant-`none` items
//     from [ContentResult]/[CodeJoin] and drops [DiscardCheck]s over the
//     `none` constant (they can never warn).
//  3. Ref compaction: surviving Refs are renumbered into a dense
//     `[0, NumRefs)` range; [Function.RefSpans] shrinks to match.
//
// The three intents share one walker: a single `rename` function combines the
// trivial-param redirect with the dense-Ref remap. After Finalize, every
// operand in the IR points at the dense Ref of a surviving producer, the
// runtime `vals` table has no holes, and `b.redirects` is empty.
//
// Must be called once construction is complete, before the function is handed
// to the evaluator or formatter.
func (b *Builder) Finalize() {
	// Joiner fixups first: strip constant-`none` items from
	// ContentResult/CodeJoin and drop DiscardChecks that can never warn. This
	// runs before the NumRefs fast path below because a function whose values
	// are all module constants (no local Refs at all) can still carry a
	// droppable DiscardCheck. Use counts are unaffected: the removed operands
	// are ModConstRefs, which are never counted.
	if noneID, ok := b.mb.noneConstID(); ok {
		noneRef := ModConstRef(noneID)
		for _, block := range b.fn.Blocks {
			out := block.Instrs[:0]
			for _, inst := range block.Instrs {
				switch x := inst.(type) {
				case *ContentResult:
					x.Items = filterNone(x.Items, noneID)
				case *CodeJoin:
					x.Items, x.ItemSpans = filterNoneWithSpans(x.Items, x.ItemSpans, noneID)
				case *DiscardCheck:
					// A check over the none constant can never warn (none is
					// not content); drop it. This is what keeps the common
					// `{ return x }` body free of a useless check — the body
					// join of an otherwise empty block is the none constant.
					if x.Value == noneRef {
						continue
					}
				}
				out = append(out, inst)
			}
			block.Instrs = out
		}
	}

	if b.fn.NumRefs() == 0 {
		b.redirects = nil
		return
	}

	// Transitively close the trivial-param rename map so a single lookup
	// resolves to the terminal target.
	for ref, target := range b.redirects {
		seen := target
		for {
			t, ok := b.redirects[seen]
			if !ok || t == seen {
				break
			}
			seen = t
		}
		if seen != target {
			b.redirects[ref] = seen
		}
	}

	// redirect follows the trivial-param rename for a function-local Ref;
	// pass-through for NoRef and ModConstRef. Use counts and worklist DCE
	// both consult this so a dead param's incoming args attribute uses to
	// the trivial target, not the param itself.
	redirect := func(r Ref) Ref {
		if r < 0 {
			return r
		}
		if t, ok := b.redirects[r]; ok {
			return t
		}
		return r
	}

	// Walk 1: count uses (redirect-aware) and index producers.
	uses := make([]int32, b.fn.NumRefs())
	prods := make([]producer, b.fn.NumRefs())
	bumpUse := func(r Ref) {
		r = redirect(r)
		if r >= 0 {
			uses[r]++
		}
	}
	for _, p := range b.fn.Params {
		if p.Default != NoRef {
			bumpUse(p.Default)
		}
	}
	for _, r := range b.liveWrites {
		bumpUse(r)
	}
	for _, block := range b.fn.Blocks {
		for i, p := range block.Params {
			if r := p.Result(); r != NoRef {
				prods[r] = producer{block: block.ID, idx: i, isParam: true, exists: true}
			}
		}
		for i, inst := range block.Instrs {
			if r := inst.Result(); r != NoRef {
				prods[r] = producer{block: block.ID, idx: i, isParam: false, exists: true}
			}
			for _, op := range inst.Operands() {
				bumpUse(op)
			}
		}
		if block.Term != nil {
			for _, op := range block.Term.Operands() {
				bumpUse(op)
			}
		}
	}
	// Worklist DCE. Drops cascade: when an instruction or param is dropped,
	// its operands (instruction operands, or for params the incoming arg
	// slot from every predecessor) lose a use and may themselves become
	// droppable. Trivially-removed params (BlockParam.dead) arrive with
	// uses[result] == 0 (every direct use was rewritten through redirects)
	// and enter the worklist via the same condition as DCE.
	dropped := make(map[Ref]struct{})
	var worklist []Ref
	for r := Ref(0); int(r) < b.fn.NumRefs(); r++ {
		if uses[r] == 0 && b.droppable(prods, r) {
			worklist = append(worklist, r)
		}
	}
	visitDrop := func(op Ref) {
		op = redirect(op)
		if op < 0 {
			return
		}
		uses[op]--
		if uses[op] == 0 && b.droppable(prods, op) {
			worklist = append(worklist, op)
		}
	}
	for len(worklist) > 0 {
		r := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		if _, done := dropped[r]; done {
			continue
		}
		dropped[r] = struct{}{}
		p := prods[r]
		if p.isParam {
			blk := b.fn.Blocks[p.block]
			for _, predID := range blk.Preds {
				predTerm := b.fn.Blocks[predID].Term
				if predTerm == nil {
					continue
				}
				args := *edgeArgs(predTerm, blk.ID)
				if p.idx < len(args) {
					visitDrop(args[p.idx])
				}
			}
		} else {
			for _, op := range b.fn.Blocks[p.block].Instrs[p.idx].Operands() {
				visitDrop(op)
			}
		}
	}

	// Walk 2a: filter dropped params/instrs out of each block. For params,
	// the splice is synchronized with every incoming terminator's arg slot
	// for that block: the i-th param and the i-th arg are dropped together.
	for _, block := range b.fn.Blocks {
		if len(block.Params) > 0 {
			keep := make([]bool, len(block.Params))
			anyDrop := false
			out := block.Params[:0]
			for i, p := range block.Params {
				if _, dead := dropped[p.Result()]; dead {
					anyDrop = true
					continue
				}
				keep[i] = true
				out = append(out, p)
			}
			block.Params = out
			if anyDrop {
				for _, predID := range block.Preds {
					predTerm := b.fn.Blocks[predID].Term
					if predTerm == nil {
						continue
					}
					args := edgeArgs(predTerm, block.ID)
					filtered := (*args)[:0]
					for i, a := range *args {
						if i < len(keep) && keep[i] {
							filtered = append(filtered, a)
						}
					}
					*args = filtered
				}
			}
		}
		if len(block.Instrs) > 0 {
			out := block.Instrs[:0]
			for _, inst := range block.Instrs {
				r := inst.Result()
				if r == NoRef {
					out = append(out, inst)
					continue
				}
				if _, dead := dropped[r]; !dead {
					out = append(out, inst)
				}
			}
			block.Instrs = out
		}
	}

	// Walk 2b: build the dense remap in original Ref order. Iterating by
	// allocation order (i.e. Ref index) — rather than block-by-block —
	// preserves the construction-time ordering so a fully-dense IR comes
	// out unchanged and SSA dumps stay close to what they would have been
	// without compaction.
	isLive := make([]bool, b.fn.NumRefs())
	for _, p := range b.fn.Params {
		if p.Ref != NoRef {
			isLive[p.Ref] = true
		}
	}
	for _, r := range b.fn.Captures {
		isLive[r] = true
	}
	if b.fn.SelfRef != NoRef {
		isLive[b.fn.SelfRef] = true
	}
	for _, block := range b.fn.Blocks {
		for _, p := range block.Params {
			isLive[p.Result()] = true
		}
		for _, inst := range block.Instrs {
			if r := inst.Result(); r != NoRef {
				isLive[r] = true
			}
		}
	}
	remap := make([]Ref, b.fn.NumRefs())
	var next Ref
	for r := Ref(0); int(r) < b.fn.NumRefs(); r++ {
		if isLive[r] {
			remap[r] = next
			next++
		} else {
			remap[r] = NoRef
		}
	}

	// Combined rename: trivial-param redirect followed by dense remap. A
	// reference to a trivially-removed param lands directly on the dense
	// Ref of its terminal target.
	rename := func(r Ref) Ref {
		if r < 0 {
			return r
		}
		if t, ok := b.redirects[r]; ok {
			// Trivial-param target may itself be a ModConstRef (e.g. a param
			// whose only incoming arg was the module `none` constant).
			if t < 0 {
				return t
			}
			r = t
		}
		return remap[r]
	}

	// Walk 3: apply rename to every Ref-typed field, both producer-side (result
	// Refs) and operand-side. Void instructions don't implement resultSetter
	// and naturally skip the producer-side update.
	for i := range b.fn.Params {
		if b.fn.Params[i].Default != NoRef {
			b.fn.Params[i].Default = rename(b.fn.Params[i].Default)
		}
		if b.fn.Params[i].Ref != NoRef {
			b.fn.Params[i].Ref = rename(b.fn.Params[i].Ref)
		}
	}
	for i, r := range b.fn.Captures {
		b.fn.Captures[i] = rename(r)
	}
	if b.fn.SelfRef != NoRef {
		b.fn.SelfRef = rename(b.fn.SelfRef)
	}
	for _, block := range b.fn.Blocks {
		for _, p := range block.Params {
			p.setResult(rename(p.Result()))
		}
		for _, inst := range block.Instrs {
			if s, ok := inst.(resultSetter); ok {
				s.setResult(rename(inst.Result()))
			}
			inst.RemapOperands(rename)
		}
		if block.Term != nil {
			block.Term.RemapOperands(rename)
		}
	}

	// Rebuild RefSpans through the remap; its new length is the post-compaction
	// per-frame value-table size.
	newSpans := make([]syntax.Span, next)
	for old, new := range remap {
		if new == NoRef {
			continue
		}
		newSpans[new] = b.fn.RefSpans[old]
	}
	b.fn.RefSpans = newSpans
	b.redirects = nil
}

// droppable reports whether r has an in-block producer that is safe to
// remove. Params, captures, self, and impure instructions return false;
// phis are always droppable when unused.
func (b *Builder) droppable(prods []producer, r Ref) bool {
	if !r.IsLocal() {
		return false
	}
	p := prods[r]
	if !p.exists {
		return false
	}
	if p.isParam {
		return true
	}
	inst := b.fn.Blocks[p.block].Instrs[p.idx]
	if inst.Result() != r {
		return false
	}
	if IsPure(inst) {
		return true
	}
	// A Call to a function not marked as impure is droppable.
	if call, ok := inst.(*Call); ok {
		if !call.Callee.Ref.IsModConst() {
			return false
		}
		fn, ok := b.mb.mod.Constants[call.Callee.Ref.ModConstID()].(*value.Function)
		return ok && !fn.Impure
	}
	return false
}

// filterNone drops items equal to the `none` module constant.
func filterNone(items []Ref, noneID int32) []Ref {
	noneRef := ModConstRef(noneID)
	out := items[:0]
	for _, it := range items {
		if it != noneRef {
			out = append(out, it)
		}
	}
	return out
}

// filterNoneWithSpans drops items equal to the `none` module constant and
// keeps the parallel spans slice in lockstep.
func filterNoneWithSpans(items []Ref, spans []syntax.Span, noneID int32) ([]Ref, []syntax.Span) {
	noneRef := ModConstRef(noneID)
	if spans == nil {
		return filterNone(items, noneID), nil
	}
	outItems := items[:0]
	outSpans := spans[:0]
	for i, it := range items {
		if it == noneRef {
			continue
		}
		outItems = append(outItems, it)
		if i < len(spans) {
			outSpans = append(outSpans, spans[i])
		}
	}
	return outItems, outSpans
}
