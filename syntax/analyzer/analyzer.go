// Package analyzer lowers a Markst syntax tree to the SSA IR the evaluator
// runs.
//
// [Analyze] walks the tree the parser produced and returns an [expr.Module]:
// the document body as a function, plus one function per closure nested in it.
// The evaluator never looks at the syntax again.
//
// # Scopes and frames
//
// A scope is a lexical region: a code block, a conditional, a loop body. It
// determines only which source names are visible where.
//
// A frame is one function, either the document body or a closure inside it.
// Every SSA value, basic block, and capture belongs to exactly one frame.
//
// Scopes nest inside frames. A frame begins with a scope of its own and ends
// when its function is complete; the scopes inside it open and close without
// affecting it.
//
// # Name resolution
//
// An identifier resolves against the scope chain to one of four things:
//
//   - A builtin, or a name from [WithBindings], becomes a constant in place.
//
//   - A name bound in the same frame, by a let, a parameter, or a
//     destructuring pattern, becomes a local SSA value.
//
//   - A name bound in an enclosing frame becomes a capture, threaded through
//     each frame in between and passed into the closure when it is built.
//
//   - A let-bound closure referring to itself becomes a self reference, which
//     the evaluator resolves to the closure currently running. Direct
//     recursion therefore needs no capture.
//
// A name that resolves to none of these is an "unknown variable" error.
//
// # Escapes
//
// break, continue, and return discard the value a block is building and
// transfer control elsewhere: break and continue to the innermost enclosing
// loop, a bare return to the closure body.
//
// The jump is not emitted where the escape is written. Typst finishes
// evaluating the statement containing an escape, including sibling arguments
// and enclosing calls, before control leaves. Lowering matches that by
// recording the escape and emitting the jump at the innermost construct that
// can catch it: a branch, a loop body, or the closure body. The escape takes
// the value built so far with it, joined across every scope it crosses, so a
// return inside a loop still includes that loop's completed iterations.
//
// # Errors
//
// Scanner and parser errors are already in the tree as [syntax.Error] nodes.
// The analyzer lowers those, along with the errors it finds itself (unknown
// variables, writes to captured variables, duplicate parameters), to
// [expr.Error] instructions. The evaluator raises them when it reaches them, so
// the rest of the document still renders.
package analyzer

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"sync"

	"znkr.io/markst/builtin"
	"znkr.io/markst/expr"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/internal/slab"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

// Option configures the analyzer.
type Option func(*analyzer)

// WithBindings makes name/value pairs resolve as if they were built in. Names
// resolve while analyzing and become constants in the IR, so the values must
// be known before [Analyze] runs. Values must be non-nil; to declare a name
// whose use must fail, bind a [value.Error].
func WithBindings(bindings map[name.Name]value.Value) Option {
	return func(a *analyzer) {
		for name, v := range bindings {
			a.bind(name, v)
		}
	}
}

// WithoutApproximations turns off the approximations the analyzer uses to lower
// less than the general case needs.
//
// An approximation only ever skips work a later stage would have undone, and
// does nothing when it cannot tell, so this option changes how large the module
// is and never the document it evaluates to. Anything that would change the
// document is a semantic rule rather than an approximation, and must run
// whatever this option says.
func WithoutApproximations() Option {
	return func(a *analyzer) {
		a.disableApprox = true
	}
}

// WithName sets the name diagnostics about this source are reported under,
// such as "lib.mst". It goes on [expr.Module.Origin] and travels with the
// module, so a failure inside a function defined here still names this source
// when it fires during another module's evaluation.
//
// Without it the origin is unnamed, but still carries the source, so spans
// still resolve to a line and column.
func WithName(name string) Option {
	return func(a *analyzer) {
		a.name = name
	}
}

// WithExports makes the top-level function return the file's bindings as well
// as its body, so a host can compile a file for the values it defines rather
// than the document it produces. The result is a two-element array: the body
// content, and a dict of each top-level binding's name to its value at the end
// of the file.
//
// Only top-level bindings are exported. A name bound inside a block is out of
// scope by then, and a constant — a builtin, or anything from [WithBindings] —
// names no variable of this file's.
func WithExports() Option {
	return func(a *analyzer) {
		a.exports = true
	}
}

// Analyze lowers a syntax tree to an SSA [expr.Module], which is never nil.
//
// Analyze never fails. Errors it finds, along with the scanner and parser
// errors already in the tree, become [expr.Error] instructions in the module,
// raised by the evaluator when it reaches them.
func Analyze(n syntax.RootNode, opts ...Option) *expr.Module {
	a := &analyzer{source: n.Source, text: string(n.Src)}
	// The universe is shared rather than copied per analysis: it holds a
	// binding per builtin, and building that map is among the most expensive
	// things an analysis does. The scope pushed on top of it below is where
	// the options bind, so nothing ever writes to the shared one.
	a.scope = universe()
	a.openScope()

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

// exportDict emits a dict of every variable bound in the top-level scope, keyed
// by source name; see [WithExports]. Each value is read at the current block,
// the end of the file, so a binding that was reassigned or defined
// conditionally exports its final value.
//
// The entries are sorted by name. Scope bindings are held in a map and the IR
// is golden-tested, so map iteration order would make the module differ between
// runs over the same source.
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

	// texts and mathTexts allocate the content values a markup or math token
	// lowers to. There is one per token, and the constant pool cannot hold
	// them because a content value carries a label the evaluator may set, so
	// these are worth allocating in blocks rather than one at a time.
	texts     slab.Of[value.Text]
	mathTexts slab.Of[value.MathText]

	// text is the source as a string, converted once. The syntax tree holds
	// bytes, so scanning allocates nothing per token, but the analyzer needs
	// strings: a name to intern, a literal to store in a value. A node's text
	// is exactly the source it spans, so [analyzer.str] returns slices of this
	// string rather than copying per node.
	text  string
	scope *scope
	b     *expr.Builder
	mb    *expr.ModuleBuilder

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

	// scope is the bottom of the frame's lexical scope chain. Scopes the
	// frame opens chain off it, and its parent is the outer frame's scope at
	// push time. Name resolution stops treating bindings as locals once it
	// passes this scope.
	scope *scope

	// captures caches the capture Ref allocated for each captured
	// source name in this frame. A name is captured at most once per frame,
	// even if the body references it many times.
	captures map[name.Name]expr.Ref

	// isFn marks a closure-body frame, the target of a bare return. It is
	// false for the document-body frame, where return is not allowed. Like
	// scopes, loops, and pending below, it is per-frame because escapes do
	// not cross closure boundaries: a break inside a closure nested in a loop
	// is invalid, and would otherwise jump to a block in another function's
	// CFG, and a return resolves to its own closure.
	isFn bool

	// joinScopes is the stack of open join scopes (see [joinScope]),
	// innermost last: block scopes collecting items so far, interleaved in
	// source order with the accumulator entries of enclosing loops. Flattened
	// from any index to the top, they give the value an escape takes with it
	// via [analyzer.partialJoin]. This is not [frame.scope], which is the
	// bottom of the frame's lexical scope chain.
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

// emitError emits an [expr.Error] instruction at the current insertion point
// and returns its Ref, which holds a [*value.Error] at eval time.
//
// A caller that produces a value should return this Ref, so an enclosing
// construct sees an error operand and short-circuits through
// [eval.propagatesFromOperands]. A caller in statement position discards it;
// the instruction stays in the block and still records the error.
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

	// bindings maps a source name to the [expr.Var] currently standing for it
	// in this scope, so that a shadowing `let x = ...` in a nested scope does
	// not collide with an outer binding in the builder's flat currentDef
	// table.
	bindings map[name.Name]binding

	// mathScope, when set, is a module consulted during name resolution only
	// after every binding in the scope chain has missed. The analyzer installs
	// it on the scope around an equation body, so math identifiers such as
	// `pi` and `frac` resolve through the normal machinery while any local
	// binding still shadows them.
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
		// `std` reaches the universe from math, so math resolves it too.
		if source == names.Std {
			if v := builtin.Universe[names.Std]; v != nil {
				return valueBinding{val: v}, true
			}
		}
	}
	return nil, false
}

// universe is the scope every analysis starts from: one binding per name in the
// built-in universe, built once and read by every analysis after that. Nothing
// writes to it; see [Analyze].
var universe = sync.OnceValue(func() *scope {
	bindings := make(map[name.Name]binding, len(builtin.Universe))
	for n, val := range builtin.Universe {
		bindings[n] = valueBinding{val: val}
	}
	return &scope{bindings: bindings}
})

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
// name. A scope tracks only the Var visible for each source name, and closing
// it drops those mappings. An assignment without a `let` walks the scope chain
// and rebinds the existing Var, which is what Braun's block-param insertion
// needs.

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
