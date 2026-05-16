// Package analyzer is the bridge between Writst's concrete syntax tree and
// evaluation. It walks the tree produced by the parser and emits an
// [expr.Module] — a top-level function plus every closure nested inside it,
// each in SSA form — that the evaluator can run without ever looking at the
// syntax again.
//
// # Scopes and frames
//
// The analyzer keeps two notions of nesting distinct.
//
// A scope is a lexical region introduced by source-level constructs (code
// blocks, conditionals, loops, let-bindings, ...). Scopes only affect which
// source names are visible at a given point.
//
// A frame corresponds to one function: the document body or any closure
// nested inside it. Frames are the unit of SSA construction — every value
// reference, basic block, and capture lives in exactly one frame.
//
// Scopes nest inside frames. A frame opens with its own boundary scope and
// closes when its function is complete; lexical scopes inside come and go
// without affecting the frame.
//
// # Name resolution
//
// When the analyzer encounters an identifier, it resolves the source name
// against the scope chain and turns it into an SSA value reference. Four
// outcomes are possible:
//
//   - A builtin, or a name supplied via [WithBindings], materializes as a
//     constant inline.
//
//   - A name bound by a let, parameter, or destructuring pattern in the
//     same frame reads as a local SSA value.
//
//   - A name bound in an enclosing frame becomes a capture: the enclosing
//     function's value flows into the inner closure at construction time.
//     Captures that cross more than one frame are threaded through each
//     intermediate frame in turn.
//
//   - A let-bound closure may refer to itself by name. The reference
//     resolves to a self reference that the runtime materializes as the
//     currently-executing closure value, with no capture allocated. This
//     is what enables direct recursion.
//
// Unknown names produce an "unknown variable" error.
//
// # Errors
//
// Lexical and parse errors are already represented as [syntax.Error] nodes
// embedded in the syntax tree; the analyzer threads them, alongside any
// semantic errors it discovers (unknown variables, illegal writes to
// captured variables, duplicate parameters, and so on), into the IR as
// [expr.Error] instructions. The evaluator surfaces them at eval time, so
// the rest of the document keeps rendering around the failure.
package analyzer

import (
	"fmt"
	"regexp"

	"znkr.io/writst/builtin"
	"znkr.io/writst/expr"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// Option configures the analyzer.
type Option func(*analyzer)

// WithBindings makes name→value pairs available to the program as if they
// were members of the built-in universe. References resolve at analyze time
// and lower to inline [expr.Const] instructions, so values must be known
// before [Analyze] runs. Values must be non-nil; pass [value.Error] for
// "declared but evaluation must fail" placeholders.
func WithBindings(bindings map[name.Name]value.Value) Option {
	return func(a *analyzer) {
		for name, v := range bindings {
			a.bind(name, v)
		}
	}
}

// Analyze converts the syntax tree rooted at n into an SSA [expr.Module].
// Errors discovered during lowering (along with embedded scanner/parser
// errors from the syntax tree) are emitted into the module as [expr.Error]
// instructions and surfaced by [eval.Eval] at run time. The returned module
// is always non-nil.
func Analyze(n syntax.RootNode, opts ...Option) *expr.Module {
	a := &analyzer{source: n.Source}
	bindings := make(map[name.Name]binding, len(builtin.Universe))
	for name, val := range builtin.Universe {
		bindings[name] = binding{value: val}
	}
	a.scope = &scope{bindings: bindings}

	for _, opt := range opts {
		opt(a)
	}

	a.mod = &expr.Module{}
	a.b = expr.NewBuilder()
	a.mod.Top = a.b.Function()

	// Push a new scope to avoid confusion about global names.
	a.openScope()
	a.frames = []*frame{{b: a.b, scope: a.scope}}

	result := a.lowerMarkup(n)
	a.b.Return(syntax.Span{}, result)
	return a.mod
}

type analyzer struct {
	source syntax.Source
	scope  *scope
	b      *expr.Builder
	mod    *expr.Module

	// frames is the stack of in-progress functions. frames[0] is the
	// document body; frames[len-1] is the innermost closure under
	// construction. [a.b] always aliases the innermost frame's builder, and
	// [a.scope] is the current lexical scope within it.
	frames []*frame
}

// frame is one nesting level of SSA construction. Pushed at closure entry by
// [pushFrame] and popped by [popFrame].
type frame struct {
	b *expr.Builder
	// scope is the bottom of the frame's lexical scope chain: any further
	// scopes the frame opens chain off it, and its parent is the outer
	// frame's scope at push time. Name resolution stops treating bindings
	// as locals once it walks past this scope.
	scope *scope

	// captures caches the [expr.DefCapture] Ref allocated for each captured
	// source name in this frame. A name is captured at most once per frame,
	// even if the body references it many times.
	captures map[name.Name]expr.Ref

	// loops is the stack of enclosing loops within this frame, used to
	// resolve break/continue targets. Per-frame because loops don't cross
	// closure boundaries: a break inside a nested closure that itself sits
	// inside a loop is invalid (and would jump to a block in the wrong
	// function's CFG).
	loops []loop
}

// pushFrame opens a fresh builder and frame scope for a nested closure.
// Updates [a.b] and [a.scope] to point at the new frame.
func (a *analyzer) pushFrame() {
	b := expr.NewBuilder()
	s := &scope{parent: a.scope}
	a.b = b
	a.scope = s
	a.frames = append(a.frames, &frame{b: b, scope: s})
}

// popFrame discards the innermost frame, restoring [a.b] and [a.scope] to
// the enclosing frame's state. Any intermediate non-closure scopes opened
// inside the popped frame are dropped along with it.
func (a *analyzer) popFrame() {
	f := a.frames[len(a.frames)-1]
	a.frames = a.frames[:len(a.frames)-1]
	a.b = a.frames[len(a.frames)-1].b
	a.scope = f.scope.parent
}

// closureDepth reports how many nested closures are currently being lowered
// (0 at the top level).
func (a *analyzer) closureDepth() int { return len(a.frames) - 1 }

// pushLoop records header/exit targets for the innermost enclosing loop on
// the current frame. Break/continue inside the body resolve to these.
func (a *analyzer) pushLoop(header, exit expr.BlockID) {
	f := a.frames[len(a.frames)-1]
	f.loops = append(f.loops, loop{header: header, exit: exit})
}

// popLoop drops the innermost loop from the current frame.
func (a *analyzer) popLoop() {
	f := a.frames[len(a.frames)-1]
	f.loops = f.loops[:len(f.loops)-1]
}

// currentLoop returns the innermost loop on the current frame, or (loop{},
// false) if the frame has no enclosing loop. break/continue inside a
// closure do not see loops in outer frames.
func (a *analyzer) currentLoop() (loop, bool) {
	f := a.frames[len(a.frames)-1]
	if len(f.loops) == 0 {
		return loop{}, false
	}
	return f.loops[len(f.loops)-1], true
}

// loop describes an enclosing loop's targets for break/continue jumps.
type loop struct {
	header expr.BlockID // continue target
	exit   expr.BlockID // break target
}

// emitError emits an [expr.Error] instruction at the current insertion
// point and returns its Ref. The Ref carries a [*value.Error] at eval time;
// callers that produce a value at their site should return the Ref so any
// enclosing construct sees an error operand and short-circuits via
// [eval.propagatesFromOperands]. Callers in statement position discard the
// Ref — the instruction stays in the block and still records the error.
func (a *analyzer) emitError(span syntax.Span, msg string, hints ...string) expr.Ref {
	return a.b.Error(span, msg, expr.NoRef, hints...)
}

// emitSyntaxError adopts a parser/scanner [*syntax.Error] node from the CST
// into the IR.
func (a *analyzer) emitSyntaxError(e *syntax.Error) expr.Ref {
	return a.b.Error(e.Span(), e.Error(), expr.NoRef, e.Hints()...)
}

// Scope ///////////////////////////////////////////////////////////////////////

type scope struct {
	parent *scope

	// bindings maps a source name to its current SSA-mangled name within
	// this scope. Used by the SSA lowering so that shadowing `let x = ...` in
	// nested scopes doesn't collide with outer bindings in the Builder's flat
	// currentDef table.
	bindings map[name.Name]binding
}

type binding struct {
	// The SSA variable backing this binding; zero for value/self bindings
	variable expr.Var

	// Optional value-binding (builtin, WithBindings)
	value value.Value

	// self, when true, means this name refers to the closure-under-construction
	// itself. Resolution materializes a [expr.DefSelf] on demand via the
	// frame's [expr.Builder.Self], so no SSA is emitted unless the body
	// actually mentions the name.
	self bool
}

func (a *analyzer) openScope() *scope {
	a.scope = &scope{parent: a.scope}
	return a.scope
}

func (a *analyzer) closeScope() {
	a.scope = a.scope.parent
}

// couldBeSubtractionRe matches identifiers like "x-1" that might be
// subtraction with missing spaces.
var couldBeSubtractionRe = regexp.MustCompile(`(-)(\d+)$`)

// checkIdent reports an "unknown variable" error when n is not in scope.
// Returns [expr.NoRef] when n is bound; otherwise returns the Ref of the
// emitted [expr.Error] so callers in expression position can use it as the
// value of the failing identifier.
func (a *analyzer) checkIdent(n name.Name, span syntax.Span) expr.Ref {
	if _, ok := a.lookup(n); ok {
		return expr.NoRef
	}
	var hints []string
	if m := couldBeSubtractionRe.FindAllStringSubmatch(n.String(), -1); m != nil {
		sign := m[0][1]
		num := m[0][2]
		hints = append(hints, fmt.Sprintf("if you meant to use subtraction, try adding spaces around the minus sign: `%s %s %s`", n.String()[:len(n.String())-len(m[0][0])], sign, num))
	}
	return a.emitError(span, fmt.Sprintf("unknown variable: %s", n.String()), hints...)
}

// SSA variable allocation /////////////////////////////////////////////////////
//
// Source-level names can shadow each other (`let x = 1; { let x = 2; ... }`).
// The Braun-style builder uses a flat per-function variable table keyed by
// [expr.Var], so each `let` allocates a fresh, versioned Var for the source
// name. Scopes track only the Var currently visible for each source name;
// closing a scope drops its mappings. Assignments without a `let` walk the
// scope chain and rebind the existing Var, which is exactly what we want
// for Braun's phi insertion.

// allocVar allocates a fresh versioned [expr.Var] for source and binds it in
// the current scope.
func (a *analyzer) allocVar(source name.Name) expr.Var {
	v := a.b.NewVar(source)
	if a.scope.bindings == nil {
		a.scope.bindings = make(map[name.Name]binding)
	}
	a.scope.bindings[source] = binding{variable: v}
	return v
}

// bind associates source with val in the current scope, without mangling.
func (a *analyzer) bind(source name.Name, val value.Value) {
	if val == nil {
		panic("binding value cannot be nil")
	}
	if a.scope.bindings == nil {
		a.scope.bindings = make(map[name.Name]binding)
	}
	a.scope.bindings[source] = binding{value: val}
}

// lookup walks the scope chain for source's current mangled name.
func (a *analyzer) lookup(source name.Name) (binding, bool) {
	for s := a.scope; s != nil; s = s.parent {
		if m, ok := s.bindings[source]; ok {
			return m, true
		}
	}
	return binding{}, false
}
