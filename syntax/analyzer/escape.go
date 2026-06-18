package analyzer

import (
	"slices"

	"znkr.io/writst/expr"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// This file holds the join-scope and escape machinery the analyzer uses to
// give blocks, loops, and closures their joined value and to let break/
// continue/return recover the value built so far. See the "Escapes" section
// of the package doc in analyzer.go for the overview.

// A joinScope is one level of the frame's join stack ([frame.joinScopes]). A
// block scope collects the values a block produces, so the block's value —
// the join of its statement values — can be built: items are gathered as refs
// in source order (no IR per statement; see [analyzer.lowerJoinedBlock]) and
// combined into one static [expr.CodeJoin]/[expr.ContentResult] when the
// block completes (see [analyzer.joinScopeValue]).
//
// A loop contributes a second flavor of entry: an accumulator scope (acc set)
// standing for the iterations the loop has completed so far. Their values
// live in the runtime accumulator accVar rather than in static items;
// [analyzer.partialJoin] reads them from there. Keeping accumulator entries
// on the same stack keeps everything an escape recovers in one list, in
// source order.
type joinScope struct {
	items   []expr.Ref
	spans   []syntax.Span
	content bool

	// acc marks a loop-accumulator entry, whose value lives in accVar.
	// Accumulator entries never carry items.
	acc    bool
	accVar expr.Var
}

// pushJoinScope opens a fresh block join scope on the current frame. content
// selects the finalizer (content sequence vs code-mode join). The matching
// [analyzer.popJoinScope] must run once the scope's items are lowered.
func (a *analyzer) pushJoinScope(content bool) {
	f := a.frame()
	f.joinScopes = append(f.joinScopes, joinScope{content: content})
}

// popJoinScope removes and returns the frame's innermost join scope — the one
// a just-completed block opened.
func (a *analyzer) popJoinScope() joinScope {
	f := a.frame()
	s := f.joinScopes[len(f.joinScopes)-1]
	f.joinScopes = f.joinScopes[:len(f.joinScopes)-1]
	return s
}

// addJoinItem records one value-producing item on the frame's innermost join
// scope. No IR is emitted; the items are combined statically when the scope
// completes ([analyzer.joinScopeValue]) or an escape recovers them
// ([analyzer.partialJoin]).
func (a *analyzer) addJoinItem(ref expr.Ref, span syntax.Span) {
	f := a.frame()
	s := &f.joinScopes[len(f.joinScopes)-1]
	s.items = append(s.items, ref)
	s.spans = append(s.spans, span)
}

// loopInfo describes one enclosing loop of the current frame: the
// break/continue jump targets, the accumulator its iteration values flush
// into, and bodyScope — the index of the first join scope inside the loop,
// one past the loop's own accumulator entry on [frame.joinScopes].
type loopInfo struct {
	header, exit expr.BlockID
	accVar       expr.Var
	bodyScope    int
}

// pushLoop records header/exit targets and the accumulator for a loop on the
// current frame, and opens the loop's accumulator entry on the join stack.
// break/continue inside the body resolve to the innermost entry. The matching
// [analyzer.popLoop] must run once the body is lowered (and any escape from
// it is fired — the accumulator entry must still be on the stack so a return
// crossing the loop recovers the completed iterations).
func (a *analyzer) pushLoop(header, exit expr.BlockID, accVar expr.Var) {
	f := a.frame()
	f.joinScopes = append(f.joinScopes, joinScope{acc: true, accVar: accVar})
	f.loops = append(f.loops, loopInfo{header: header, exit: exit, accVar: accVar, bodyScope: len(f.joinScopes)})
}

// popLoop drops the innermost loop and its accumulator entry from the current
// frame.
func (a *analyzer) popLoop() {
	f := a.frame()
	f.joinScopes = f.joinScopes[:len(f.joinScopes)-1]
	f.loops = f.loops[:len(f.loops)-1]
}

// joinItems combines items into a single value. As content it is always a
// [expr.ContentResult] (so a single-item content block still yields content);
// otherwise it is the code-mode joiner: none for zero items, the item itself
// for one, [expr.CodeJoin] for more.
func (a *analyzer) joinItems(items []expr.Ref, spans []syntax.Span, content bool, span syntax.Span) expr.Ref {
	if content {
		return a.b.ContentResult(span, items)
	}
	switch len(items) {
	case 0:
		return a.b.Const(span, value.None{})
	case 1:
		return items[0]
	default:
		return a.b.CodeJoin(span, items, spans)
	}
}

// joinScopeValue finalizes one join scope into a single value.
func (a *analyzer) joinScopeValue(s joinScope, span syntax.Span) expr.Ref {
	return a.joinItems(s.items, s.spans, s.content, span)
}

// appendJoin appends item to a loop's runtime accumulator accVar in the current
// block (one [expr.JoinAdd], threaded via the builder's var machinery).
func (a *analyzer) appendJoin(accVar expr.Var, span syntax.Span, item expr.Ref) {
	prev := a.b.ReadVar(accVar, a.b.CurrentBlock())
	next := a.b.JoinAdd(span, prev, item)
	a.b.WriteVar(accVar, a.b.CurrentBlock(), next)
}

// partialJoin builds the in-flight value an escape recovers: the join of
// everything the frame's join scopes from fromScope upward have produced so
// far, in source order — block scopes contribute their collected items, loop
// accumulator entries the iterations completed so far. extra, when not NoRef,
// is appended last: the natural join value of a construct body whose scope is
// already popped (see [analyzer.firePending]). The first block scope's items
// spread directly and carry the finalizer; every other entry folds to a
// single value. Because the join operator is associative this is one static
// join, not one per scope. An empty join (an expression-bodied closure)
// yields none.
func (a *analyzer) partialJoin(fromScope int, extra expr.Ref, span syntax.Span) expr.Ref {
	f := a.frame()
	var items []expr.Ref
	var spans []syntax.Span
	content := false
	first := true
	for _, s := range f.joinScopes[fromScope:] {
		switch {
		case s.acc:
			acc := a.b.ReadVar(s.accVar, a.b.CurrentBlock())
			items = append(items, a.b.JoinResult(span, acc))
			spans = append(spans, span)
		case first:
			content = s.content
			items = slices.Concat(items, s.items)
			spans = slices.Concat(spans, s.spans)
			first = false
		default:
			items = append(items, a.joinScopeValue(s, span))
			spans = append(spans, span)
		}
	}
	if extra != expr.NoRef {
		items = append(items, extra)
		spans = append(spans, span)
	}
	if first && len(items) == 0 {
		return a.b.Const(span, value.None{})
	}
	return a.joinItems(items, spans, content, span)
}

// Pending escapes ////////////////////////////////////////////////////////////

// pendingEscape records a lowered break/continue/return whose control transfer
// has not been emitted yet. Escapes are flow events in Typst: the statement
// containing the escape is still evaluated to its end (sibling arguments,
// enclosing calls), and only then does control leave. Lowering mirrors that by
// deferring the terminator: the escape site records the event
// ([analyzer.setPending]) and evaluates to none; the statement sequence stops
// lowering further statements once an escape is pending ([analyzer.eachCodeItem],
// [analyzer.eachMarkupItem]); and the innermost enclosing catch point — a
// conditional branch, a loop body, or the closure body — emits the actual
// terminator ([analyzer.firePending]).
//
// Catch points fire only escapes that arose within their own construct
// (detected by comparing against the pending state at entry), so an escape
// pending from an earlier sibling argument passes through a nested construct
// untouched and the construct lowers as regular live code. This works because
// a pending escape never survives past a control-flow merge: every construct
// catches its own escapes before merging, so a pending escape at any
// statement boundary was unconditional within that statement.
type pendingEscape struct {
	kind escapeKind
	span syntax.Span
	// val is the explicit `return value`; NoRef for a bare return (which
	// yields the joined body value) and for break/continue.
	val expr.Ref
}

// escapeKind distinguishes the escape statements a [pendingEscape] defers.
type escapeKind int

const (
	escapeReturn escapeKind = iota
	escapeBreak
	escapeContinue
)

// setPending records an escape flow event on the current frame. The first
// event wins: an escape lowered while another is already pending is inert
// (its expression value is still none), matching Typst, where a flow event
// does not overwrite an existing one.
func (a *analyzer) setPending(p pendingEscape) {
	f := a.frame()
	if f.pending == nil {
		f.pending = &p
	}
}

// catchEscapes lowers a construct body via lower and, if an escape became
// pending inside it, fires it here — on the body's own path, before any
// control-flow merge (see [pendingEscape]). An escape already pending at
// entry passes through untouched, so a construct lowered after a sibling
// escape in the same statement stays regular live code. warnDiscard is
// forwarded to [analyzer.firePending].
func (a *analyzer) catchEscapes(warnDiscard bool, lower func() expr.Ref) expr.Ref {
	entry := a.frame().pending
	ref := lower()
	if a.frame().pending != entry {
		a.firePending(ref, warnDiscard)
	}
	return ref
}

// firePending emits the deferred control transfer for the frame's pending
// escape at a catch point and clears it. body is the natural join value of the
// construct body that just completed (conditional branch result, loop body
// value, or closure body value); its own join scope is already popped, so it
// is folded in after the still-open scopes recovered by [analyzer.partialJoin].
// warnDiscard marks the closure-body catch over a code-block body: the only
// site where an explicit return provably discards a code-mode body join on
// every call, and hence the only one that emits the [expr.DiscardCheck]
// warning. Whether the discarded value is worth warning about (non-empty
// content) is left to the runtime check; a check over a constant that can
// never warn is stripped by [expr.Builder.Finalize].
//
// firePending leaves the current block terminated and switches emission to a
// fresh unreachable block, so the caller's regular tail code (merge jumps,
// loop back-edges, the closure's fallthrough return) lands in dead code
// unchanged.
func (a *analyzer) firePending(body expr.Ref, warnDiscard bool) {
	f := a.frame()
	p := f.pending
	f.pending = nil
	switch p.kind {
	case escapeBreak, escapeContinue:
		// The innermost loop is the target. Recovery starts at its first
		// inner scope — one past the loop's own accumulator entry, which is
		// what the escape flushes into rather than a value it recovers.
		l := f.loops[len(f.loops)-1]
		a.appendJoin(l.accVar, p.span, a.partialJoin(l.bodyScope, body, p.span))
		if p.kind == escapeBreak {
			a.b.Jump(p.span, l.exit)
		} else {
			a.b.Jump(p.span, l.header)
		}
	case escapeReturn:
		val := p.val
		if val == expr.NoRef {
			// Bare return: yield the function body's joined-so-far value.
			// Loops the return crosses contribute their completed iterations
			// via their accumulator entries on the join stack.
			val = a.partialJoin(0, body, p.span)
		} else if warnDiscard {
			a.b.DiscardCheck(p.span, body)
		}
		a.b.Return(p.span, val)
	}
	dead := a.b.NewBlock()
	a.b.SetBlock(dead)
	a.b.SealBlock(dead)
}
