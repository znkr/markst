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
// A frame corresponds to one function: the document body or any closure nested
// inside it. Frames are the unit of SSA construction — every value reference,
// basic block, and capture lives in exactly one frame.
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
// # Escapes
//
// break, continue, and return are escapes: they abandon the value a block is
// building and jump away. break/continue unwind to the innermost loop on
// [frame.loops]; bare return to the frame itself (a closure body,
// [frame.isFn]).
//
// An escape does not emit its control transfer at its own site. Typst treats
// escapes as flow events: the statement containing the escape is evaluated to
// its end (sibling arguments, enclosing calls) and only then does control
// leave. Lowering mirrors that by recording a [pendingEscape] on the frame;
// statement sequences stop lowering once one is pending, and the innermost
// enclosing catch point — a conditional branch, a loop body, or the closure
// body — emits the terminator ([analyzer.firePending]). Before the transfer,
// the escape recovers the in-flight value built so far: the partial join of
// the frame's open join scopes from the escape target inward, with crossed
// loops contributing their accumulated iterations, so a bare return still
// folds through an enclosing loop's body (see [analyzer.partialJoin]).
//
// Recovering the value is shared ([analyzer.partialJoin]); only the delivery
// differs by escape kind. break/continue append the recovered value to the
// loop's runtime accumulator and jump to its exit/header block, while return
// hands it to a Return terminator directly.
//
// # Errors
//
// Lexical and parse errors are already represented as [syntax.Error] nodes
// embedded in the syntax tree; the analyzer threads them, alongside any
// semantic errors it discovers (unknown variables, illegal writes to captured
// variables, duplicate parameters, and so on), into the IR as [expr.Error]
// instructions. The evaluator surfaces them at eval time, so the rest of the
// document keeps rendering around the failure.
package analyzer

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"

	"znkr.io/writst/builtin"
	"znkr.io/writst/expr"
	"znkr.io/writst/internal/names"
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

// WithoutApproximations turns off the conservative approximations the analyzer
// uses to lower less than the general case would require — see
// [analyzer.ownsLine].
//
// An approximation is always static and one-sided: it may skip work only when a
// later, fully informed stage would undo that work anyway, and it does nothing
// whenever it cannot tell. So turning them off changes how large the module is,
// never the document it evaluates to. TestApproximationsPreserveOutput checks
// that over the whole corpus by analyzing every case both ways.
//
// A change that is not output-preserving does not belong behind this option. If
// disabling it changes a document, it is a semantic rule, not an approximation,
// and it must run unconditionally.
func WithoutApproximations() Option {
	return func(a *analyzer) {
		a.disableApprox = true
	}
}

// WithName sets the display name diagnostics about this source are reported
// under, e.g. "lib.wrt". It lands on [expr.Module.Origin] and travels with
// every span the module produces, so a failure inside a function defined here
// still names this source when it fires during another module's evaluation.
//
// Without it the origin still carries the source — spans remain resolvable to
// line and column — and is simply unnamed.
func WithName(name string) Option {
	return func(a *analyzer) {
		a.name = name
	}
}

// WithExports makes the top-level function return the file's top-level
// bindings alongside its body, so a host can compile a file for the values it
// defines rather than the document it produces. The returned value is a
// two-element array: the body content, and a dict mapping each top-level
// binding's name to its value as of the end of the file.
//
// Only bindings introduced at the top level are exported. Names bound inside a
// block have gone out of scope by the time the exports are read, and constants
// (the built-in universe, anything from [WithBindings]) name no variable and
// are not the file's to export.
func WithExports() Option {
	return func(a *analyzer) {
		a.exports = true
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
		bindings[name] = valueBinding{val: val}
	}
	a.scope = &scope{bindings: bindings}

	for _, opt := range opts {
		opt(a)
	}

	a.mb = expr.NewModuleBuilder()

	a.b = a.mb.NewBuilder()
	// Push a new scope to avoid confusion about global names.
	top := a.openScope()
	a.frames = []*frame{{b: a.b, scope: a.scope}}
	result := a.lowerMarkup(n)
	if a.exports {
		result = a.b.MakeArray(n.Span(), []expr.ArrayItem{
			{Value: result, Span: n.Span()},
			{Value: a.exportDict(top, n.Span()), Span: n.Span()},
		})
	}
	a.b.Return(n.Span(), result)
	a.b.Finalize()

	mod := a.mb.Module()
	mod.Top = a.b.Function()
	mod.ParseErrors = a.parseErrors
	mod.Origin = syntax.Origin{Name: a.name, Source: n.Source}
	return mod
}

// exportDict emits a dict of every variable bound in the top-level scope,
// keyed by source name; see [WithExports]. Each value is read at the current
// block — the end of the file — so a binding that was reassigned or defined
// conditionally exports the value it ended up with.
//
// The entries are sorted by name. Scope bindings live in a map, and the IR is
// golden-tested, so an arbitrary iteration order would make the module differ
// between runs of the same source.
func (a *analyzer) exportDict(top *scope, span syntax.Span) expr.Ref {
	names := make([]name.Name, 0, len(top.bindings))
	for n, b := range top.bindings {
		// Only variables are the file's to export: a valueBinding names a
		// constant it did not define (the universe, or [WithBindings]), and a
		// selfBinding cannot appear at the top level.
		if _, ok := b.(varBinding); ok {
			names = append(names, n)
		}
	}
	slices.SortFunc(names, func(x, y name.Name) int { return cmp.Compare(x.String(), y.String()) })

	entries := make([]expr.DictEntry, 0, len(names))
	for _, n := range names {
		v := top.bindings[n].(varBinding).v
		entries = append(entries, expr.DictEntry{
			Key:   a.b.Const(span, value.Str(n.String())),
			Value: a.b.ReadVar(v, a.b.CurrentBlock()),
		})
	}
	return a.b.MakeDict(span, entries)
}

type analyzer struct {
	source syntax.Source
	scope  *scope
	b      *expr.Builder
	mb     *expr.ModuleBuilder

	// parseErrors accumulates every syntax error lowered via [emitSyntaxError].
	parseErrors []*value.Error

	// frames is the stack of in-progress functions. frames[0] is the document
	// body; frames[len-1] is the innermost closure under construction. [a.b]
	// always aliases the innermost frame's builder, and [a.scope] is the
	// current lexical scope within it.
	frames []*frame

	// mathDepth is >0 while lowering math content, so an escape like `\(`
	// lowers to a math [value.Symbol] rather than markup text. Content blocks
	// reset it, since their bodies are markup even inside an equation.
	mathDepth int

	// disableApprox disables the lowering approximations; see
	// [WithoutApproximations].
	disableApprox bool

	// name is the display name for this source; see [WithName].
	name string

	// exports makes the top-level function return its bindings; see
	// [WithExports].
	exports bool
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

	// captures caches the capture Ref allocated for each captured
	// source name in this frame. A name is captured at most once per frame,
	// even if the body references it many times.
	captures map[name.Name]expr.Ref

	// isFn marks a closure-body frame, the target of bare return. It is false
	// for the document-body frame, where return is illegal. Per-frame (like
	// scopes, loops, and pending below) because escapes don't cross closure
	// boundaries: a break inside a nested closure that itself sits inside a
	// loop is invalid (and would jump to a block in the wrong function's CFG),
	// and a return resolves to its own closure.
	isFn bool

	// joinScopes is the stack of open join scopes (see [joinScope]),
	// innermost last: block scopes collecting in-flight items, interleaved in
	// source order with the accumulator entries of enclosing loops. Flattened
	// from any index up to the top they are the in-flight value an escape
	// recovers via [analyzer.partialJoin]. (Distinct from [frame.scope], the
	// bottom of the frame's lexical scope chain.)
	joinScopes []joinScope

	// loops is the stack of enclosing loops (see [loopInfo]), innermost last.
	// break/continue target the innermost entry.
	loops []loopInfo

	// pending is the frame's in-flight escape (see [pendingEscape]), recorded
	// at the escape site and emitted at the innermost enclosing catch point.
	pending *pendingEscape
}

// frame returns the innermost in-progress frame: the function (document body or
// closure) currently under construction.
func (a *analyzer) frame() *frame { return a.frames[len(a.frames)-1] }

// pushFrame opens a fresh builder and frame scope for a nested closure.
// Updates [a.b] and [a.scope] to point at the new frame.
func (a *analyzer) pushFrame() {
	b := a.mb.NewBuilder()
	s := &scope{parent: a.scope}
	a.b = b
	a.scope = s
	a.frames = append(a.frames, &frame{b: b, scope: s, isFn: true})
}

// popFrame discards the innermost frame, restoring [a.b] and [a.scope] to
// the enclosing frame's state. Any intermediate non-closure scopes opened
// inside the popped frame are dropped along with it.
func (a *analyzer) popFrame() {
	f := a.frame()
	a.frames = a.frames[:len(a.frames)-1]
	a.b = a.frame().b
	a.scope = f.scope.parent
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

// emitSyntaxError adopts a parser/scanner [*syntax.Error] node from the CST:
// it records the diagnostic in [analyzer.parseErrors] — the single reporting
// channel for parse errors, surfaced by eval whether or not the containing
// code runs — and emits a poison [expr.Error] whose value propagates through
// enclosing computations without being reported again.
func (a *analyzer) emitSyntaxError(e *syntax.Error) expr.Ref {
	a.parseErrors = append(a.parseErrors, &value.Error{Span: e.Span(), Msg: e.Error(), Hints: value.Hints(e.Hints()...)})
	return a.b.SyntaxError(e.Span(), e.Error(), e.Hints()...)
}

// adoptParseErrors records every [*syntax.Error] in n's subtree in
// [analyzer.parseErrors] without emitting IR. Used for statements skipped
// after a pending escape: they can never execute, but their parse errors are
// diagnostics on the source, not on a code path, and must still surface.
func (a *analyzer) adoptParseErrors(n syntax.Node) {
	if e, ok := n.(*syntax.Error); ok {
		a.parseErrors = append(a.parseErrors, &value.Error{Span: e.Span(), Msg: e.Error(), Hints: value.Hints(e.Hints()...)})
		return
	}
	if inner, ok := n.(*syntax.Inner); ok {
		for _, c := range inner.Children() {
			a.adoptParseErrors(c)
		}
	}
}

// Scope ///////////////////////////////////////////////////////////////////////

type scope struct {
	parent *scope

	// bindings maps a source name to its current SSA-mangled name within
	// this scope. Used by the SSA lowering so that shadowing `let x = ...` in
	// nested scopes doesn't collide with outer bindings in the Builder's flat
	// currentDef table.
	bindings map[name.Name]binding

	// mathScope, when set, is a module consulted as a low-precedence fallback
	// during name resolution: it is checked only after the entire scope chain's
	// bindings miss. The analyzer installs it on the scope wrapping an equation
	// body so math identifiers (`pi`, `frac`, …) resolve through the normal
	// resolution machinery while still being shadowed by any local binding.
	mathScope value.ModuleDef
}

// binding is the kind of in-scope association recorded for a source name. The
// three concrete kinds are mutually exclusive; the type split lets resolution
// switch on them directly instead of inspecting sentinel-valued fields.
type binding interface{ aBinding() }

// varBinding names an SSA variable allocated by the current function's
// Builder. Plain `let x = ...` and reassignments both produce these.
type varBinding struct{ v expr.Var }

// valueBinding names a constant value resolved at analysis time (the built-in
// universe, or values pre-bound via [WithBindings]).
type valueBinding struct{ val value.Value }

// selfBinding marks a closure's recursion name. Resolution materializes the
// self Ref on demand via [expr.Builder.Self], so no SSA is emitted unless the
// body actually mentions the name.
type selfBinding struct{}

func (varBinding) aBinding()   {}
func (valueBinding) aBinding() {}
func (selfBinding) aBinding()  {}

// lookupMath resolves a name the way a math identifier sees it: the bindings
// written around the equation, then the math module. The universe is not in
// that chain — math has its own vocabulary, and a name that only the universe
// has is a mistake in math rather than a call to something else.
func (a *analyzer) lookupMath(source name.Name) (binding, bool) {
	var fallback value.ModuleDef
	for s := a.scope; s.parent != nil; s = s.parent {
		if m, ok := s.bindings[source]; ok {
			return m, true
		}
		if s.mathScope != nil {
			fallback = s.mathScope
		}
	}
	if fallback != nil {
		if v := fallback.Get(source); v != nil {
			return valueBinding{val: v}, true
		}
		// `std` is the way into the universe from math, so math can see it.
		if source == names.Std {
			if v := builtin.Universe[names.Std]; v != nil {
				return valueBinding{val: v}, true
			}
		}
	}
	return nil, false
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

// assignVar resolves an assignment target's binding to the SSA variable it
// names. A [valueBinding] — a constant resolved at analysis time, either from
// the universe (`emph = 1`) or from the math module reached through
// [scope.mathScope] (`$#(pi = 1)$`) — names no variable and cannot be assigned
// to; it reports the error and returns its Ref so callers in expression
// position can use it as the value of the failing assignment.
func (a *analyzer) assignVar(bnd binding, source name.Name, span syntax.Span) (expr.Var, expr.Ref, bool) {
	vb, ok := bnd.(varBinding)
	if !ok {
		return expr.Var{}, a.emitError(span, fmt.Sprintf("cannot mutate a constant: %s", source.String())), false
	}
	return vb.v, expr.NoRef, true
}

// SSA variable allocation /////////////////////////////////////////////////////
//
// Source-level names can shadow each other (`let x = 1; { let x = 2; ... }`).
// The Braun-style builder uses a flat per-function variable table keyed by
// [expr.Var], so each `let` allocates a fresh, versioned Var for the source
// name. Scopes track only the Var currently visible for each source name;
// closing a scope drops its mappings. Assignments without a `let` walk the
// scope chain and rebind the existing Var, which is exactly what we want
// for Braun's block-param insertion.

// allocVar allocates a fresh versioned [expr.Var] for source and binds it in
// the current scope.
func (a *analyzer) allocVar(source name.Name) expr.Var {
	v := a.b.NewVar(source)
	if a.scope.bindings == nil {
		a.scope.bindings = make(map[name.Name]binding)
	}
	a.scope.bindings[source] = varBinding{v: v}
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
	a.scope.bindings[source] = valueBinding{val: val}
}

// lookup walks the scope chain for source's current mangled name. If no
// binding is found, an in-scope [scope.mathScope] fallback is consulted last.
func (a *analyzer) lookup(source name.Name) (binding, bool) {
	var fallback value.ModuleDef
	for s := a.scope; s != nil; s = s.parent {
		if m, ok := s.bindings[source]; ok {
			return m, true
		}
		if s.mathScope != nil {
			fallback = s.mathScope
		}
	}
	if fallback != nil {
		if v := fallback.Get(source); v != nil {
			return valueBinding{val: v}, true
		}
	}
	return nil, false
}
