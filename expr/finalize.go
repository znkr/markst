package expr

import "znkr.io/writst/syntax"

// resultSetter is implemented by every value-producing IR node (instructions
// embedding [instr], and [*Phi]). Void instructions (those embedding
// [voidInstr]) do not implement it, so the finalize loop naturally skips them
// via a type assertion.
type resultSetter interface {
	setResult(Ref)
}

// producer indexes the in-block def of a function-local Ref. Only set for
// surviving in-block producers (phis and instructions still present after
// trivial-phi removal). Params, captures, self, and trivially-removed phis
// never get an entry — their `exists` flag stays false, so [Builder.droppable]
// refuses to mark them dead.
type producer struct {
	block  BlockID
	idx    int // index into block.Phis (isPhi) or block.Instrs
	isPhi  bool
	exists bool
}

// Finalize seals the function for consumption in a single combined pass:
//
//  1. Trivial-phi rewrite: a reference to a phi removed by
//     [tryRemoveTrivialPhi] follows the rename map to the phi's single
//     operand.
//  2. Dead-code elimination: pure instructions and phis whose result Ref
//     has no uses are dropped. Constant-`none` items are stripped from
//     [ContentResult]/[CodeJoin] joiners. Cascades to fixed point.
//  3. Ref compaction: surviving Refs are renumbered into a dense
//     `[0, NumRefs)` range; [Function.RefSpans] shrinks to match.
//
// The three intents share one walker: a single `rename` function combines the
// trivial-phi redirect with the dense-Ref remap. After Finalize, every operand
// in the IR points at the dense Ref of a surviving producer, the runtime `vals`
// table has no holes, and `b.redirects` is empty.
//
// Must be called once construction is complete, before the function is handed
// to the evaluator or formatter.
func (b *Builder) Finalize() {
	if b.fn.NumRefs == 0 {
		b.redirects = nil
		return
	}

	// Transitively close the trivial-phi rename map so a single lookup
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

	// redirect follows the trivial-phi rename for a function-local Ref;
	// pass-through for NoRef and ModConstRef. Use counts and worklist DCE both
	// consult this so a dead phi's operands attribute uses to the trivial
	// target, not the phi itself.
	redirect := func(r Ref) Ref {
		if r < 0 {
			return r
		}
		if t, ok := b.redirects[r]; ok {
			return t
		}
		return r
	}

	// Walk 1: count uses (redirect-aware), index producers, strip `none` items
	// from joiners.
	uses := make([]int32, b.fn.NumRefs)
	prods := make([]producer, b.fn.NumRefs)
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
		for i, phi := range block.Phis {
			if r := phi.Result(); r != NoRef {
				prods[r] = producer{block: block.ID, idx: i, isPhi: true, exists: true}
			}
			for _, op := range phi.Operands() {
				bumpUse(op.Value)
			}
		}
		for i, inst := range block.Instrs {
			if r := inst.Result(); r != NoRef {
				prods[r] = producer{block: block.ID, idx: i, isPhi: false, exists: true}
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
	if noneID, ok := b.mb.noneConstID(); ok {
		for _, block := range b.fn.Blocks {
			for _, inst := range block.Instrs {
				switch x := inst.(type) {
				case *ContentResult:
					x.Items = filterNone(x.Items, noneID)
				case *CodeJoin:
					x.Items, x.ItemSpans = filterNoneWithSpans(x.Items, x.ItemSpans, noneID)
				}
			}
		}
	}

	// Worklist DCE. Drops cascade: when an instruction is dropped, its operands
	// lose a use and may themselves become droppable.
	dropped := make(map[Ref]struct{})
	var worklist []Ref
	for r := Ref(0); int32(r) < b.fn.NumRefs; r++ {
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
		if p.isPhi {
			for _, op := range b.fn.Blocks[p.block].Phis[p.idx].Operands() {
				visitDrop(op.Value)
			}
		} else {
			for _, op := range b.fn.Blocks[p.block].Instrs[p.idx].Operands() {
				visitDrop(op)
			}
		}
	}

	// Walk 2a: filter dropped phis/instrs out of each block.
	for _, block := range b.fn.Blocks {
		if len(block.Phis) > 0 {
			out := block.Phis[:0]
			for _, phi := range block.Phis {
				if _, dead := dropped[phi.Result()]; !dead {
					out = append(out, phi)
				}
			}
			block.Phis = out
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
	isLive := make([]bool, b.fn.NumRefs)
	for _, p := range b.fn.Params {
		if p.Ref != NoRef {
			isLive[p.Ref] = true
		}
	}
	for _, r := range b.fn.CaptureRefs {
		if r != NoRef {
			isLive[r] = true
		}
	}
	if b.fn.SelfRef != NoRef {
		isLive[b.fn.SelfRef] = true
	}
	for _, block := range b.fn.Blocks {
		for _, phi := range block.Phis {
			isLive[phi.Result()] = true
		}
		for _, inst := range block.Instrs {
			if r := inst.Result(); r != NoRef {
				isLive[r] = true
			}
		}
	}
	remap := make([]Ref, b.fn.NumRefs)
	var next Ref
	for r := Ref(0); int32(r) < b.fn.NumRefs; r++ {
		if isLive[r] {
			remap[r] = next
			next++
		} else {
			remap[r] = NoRef
		}
	}

	// Combined rename: trivial-phi redirect followed by dense remap. A
	// reference to a trivially-removed phi lands directly on the dense Ref of
	// its terminal target.
	rename := func(r Ref) Ref {
		if r < 0 {
			return r
		}
		if t, ok := b.redirects[r]; ok {
			// Trivial-phi target may itself be a ModConstRef (e.g. a phi whose
			// only operand was the module `none` constant).
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
	for i, r := range b.fn.CaptureRefs {
		if r != NoRef {
			b.fn.CaptureRefs[i] = rename(r)
		}
	}
	if b.fn.SelfRef != NoRef {
		b.fn.SelfRef = rename(b.fn.SelfRef)
	}
	for _, block := range b.fn.Blocks {
		for _, phi := range block.Phis {
			phi.setResult(rename(phi.Result()))
			phi.RemapOperands(rename)
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

	// Rebuild RefSpans through the remap; resize NumRefs.
	newSpans := make([]syntax.Span, next)
	for old, new := range remap {
		if new == NoRef {
			continue
		}
		newSpans[new] = b.fn.RefSpans[old]
	}
	b.fn.RefSpans = newSpans
	b.fn.NumRefs = int32(next)
	b.redirects = nil
}

// droppable reports whether r has an in-block producer that is safe to
// remove. Params, captures, self, and impure instructions return false;
// phis are always droppable when unused.
func (b *Builder) droppable(prods []producer, r Ref) bool {
	if r < 0 || int32(r) >= b.fn.NumRefs {
		return false
	}
	p := prods[r]
	if !p.exists {
		return false
	}
	if p.isPhi {
		return true
	}
	inst := b.fn.Blocks[p.block].Instrs[p.idx]
	return inst.Result() == r && IsPure(inst)
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
