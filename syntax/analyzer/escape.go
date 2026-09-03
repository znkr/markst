// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package analyzer

import (
	"slices"

	"znkr.io/markst/expr"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

// This file holds the join scopes and escape handling the analyzer uses to give
// a block, loop, or closure its joined value, and to let break, continue, and
// return take the value built so far with them. See the "Escapes" section of
// the package doc in analyzer.go.

// A joinScope is one level of the frame's join stack ([frame.joinScopes]).
//
// A block scope collects the values a block's statements produce, which
// together make the block's value. The items are gathered as refs in source
// order, with no IR emitted per statement (see [analyzer.lowerJoinedBlock]),
// and combined into a single [expr.CodeJoin] or [expr.ContentResult] when the
// block ends (see [analyzer.joinScopeValue]).
//
// A loop adds a second kind of entry: an accumulator scope, with acc set,
// standing for the iterations completed so far. Its values are in the runtime
// accumulator accVar rather than in static items, and [analyzer.partialJoin]
// reads them from there. Both kinds share one stack so that everything an
// escape takes with it is in a single list, in source order.
type joinScope struct {
	items   []expr.Ref
	spans   []syntax.Span
	content bool

	// acc marks a loop-accumulator entry, whose value is in accVar. An
	// accumulator entry never has items.
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
// into, the error mark its back edges compare against, and bodyScope — the
// index of the first join scope inside the loop, one past the loop's own
// accumulator entry on [frame.joinScopes].
type loopInfo struct {
	header, exit expr.BlockID
	accVar       expr.Var
	errMark      expr.Ref
	bodyScope    int
}

// pushLoop records header/exit targets, the accumulator and the error mark for
// a loop on the current frame, and opens the loop's accumulator entry on the
// join stack. break/continue inside the body resolve to the innermost entry.
// The matching [analyzer.popLoop] must run once the body is lowered (and any
// escape from it is fired — the accumulator entry must still be on the stack so
// a return crossing the loop recovers the completed iterations).
func (a *analyzer) pushLoop(header, exit expr.BlockID, accVar expr.Var, errMark expr.Ref) {
	f := a.frame()
	f.joinScopes = append(f.joinScopes, joinScope{acc: true, accVar: accVar})
	f.loops = append(f.loops, loopInfo{header: header, exit: exit, accVar: accVar, errMark: errMark, bodyScope: len(f.joinScopes)})
}

// popLoop drops the innermost loop and its accumulator entry from the current
// frame.
func (a *analyzer) popLoop() {
	f := a.frame()
	f.joinScopes = f.joinScopes[:len(f.joinScopes)-1]
	f.loops = f.loops[:len(f.loops)-1]
}

// loopBackedge ends the current block with a loop back edge: control returns to
// header unless an error has been recorded since errMark was taken, in which
// case it leaves through exit. See [analyzer.lowerLoop] for why the check is
// here.
func (a *analyzer) loopBackedge(span syntax.Span, header, exit expr.BlockID, errMark expr.Ref) {
	failed := a.b.ErrorSince(span, errMark)
	a.b.Branch(span, failed, exit, header)
	a.b.MarkBackedge()
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

// partialJoin builds the value an escape takes with it: the join of everything
// the frame's join scopes from fromScope upward have produced so far, in source
// order. A block scope contributes its collected items, a loop accumulator
// entry the iterations completed so far. extra, when not NoRef, is appended
// last; it is the join value of a construct body whose scope has already been
// popped (see [analyzer.firePending]).
//
// The first block scope's items spread directly and carry the finalizer, and
// every other entry folds to a single value. The join operator is associative,
// so this is one static join rather than one per scope. An empty join, as in an
// expression-bodied closure, yields none.
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

// pendingEscape records a lowered break, continue, or return whose control
// transfer has not been emitted yet.
//
// In Typst the statement containing an escape is evaluated to its end,
// including sibling arguments and enclosing calls, before control leaves.
// Lowering matches that by deferring the terminator. The escape site records
// the escape ([analyzer.setPending]) and lowers to none; the statement sequence
// stops lowering further statements while an escape is pending
// ([analyzer.eachCodeItem], [analyzer.eachMarkupItem]); and the innermost
// construct that can catch it — a conditional branch, a loop body, or the
// closure body — emits the terminator ([analyzer.firePending]).
//
// A catch point fires only the escapes that arose inside its own construct,
// which it detects by comparing against the pending state at entry. An escape
// left pending by an earlier sibling argument therefore passes through a nested
// construct, and that construct lowers as ordinary live code. This is sound
// because a pending escape never survives a control-flow merge: every construct
// catches its own escapes first, so an escape pending at a statement boundary
// was unconditional within that statement.
type pendingEscape struct {
	kind escapeKind
	span syntax.Span
	// val is the value of an explicit `return value`. It is NoRef for a bare
	// return, which yields the joined body value, and for break and continue.
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

// catchEscapes lowers a construct body with lower and, if an escape became
// pending inside it, fires that escape here: on the body's own path, before any
// control-flow merge (see [pendingEscape]). An escape already pending at entry
// passes through, so a construct lowered after a sibling escape in the same
// statement remains ordinary live code. warnDiscard is passed on to
// [analyzer.firePending].
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
		// The innermost loop is the target. The recovered value starts at the
		// loop's first inner scope, one past its own accumulator entry, which
		// the escape appends to rather than reads.
		l := f.loops[len(f.loops)-1]
		a.appendJoin(l.accVar, p.span, a.partialJoin(l.bodyScope, body, p.span))
		if p.kind == escapeBreak {
			a.b.Jump(p.span, l.exit)
		} else {
			// A continue reaches the header without passing the ordinary back
			// edge, so it carries the same check.
			a.loopBackedge(p.span, l.header, l.exit, l.errMark)
		}
	case escapeReturn:
		val := p.val
		if val == expr.NoRef {
			// A bare return yields the function body's value so far. Each loop
			// the return passes through contributes its completed iterations
			// from its accumulator entry on the join stack.
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
