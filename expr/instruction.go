package expr

import (
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

// Instruction is one SSA operation. Instructions are held in
// [BasicBlock.Instrs] in program order.
//
// Most instructions produce a value; the ones that only have an effect produce
// [NoRef] instead. The span an instruction came from is in [Function.RefSpans],
// keyed by its result.
type Instruction interface {
	// Result returns the SSA value this instruction produces, or [NoRef] if it
	// produces none.
	Result() Ref

	// Operands appends the instruction's Ref operands to dst and returns the
	// result. Passing a scratch slice back in avoids an allocation per call.
	Operands(dst []Ref) []Ref

	// RemapOperands replaces every Ref operand with rename of it.
	RemapOperands(rename func(Ref) Ref)

	aInstruction()
}

// IsPure reports whether instr can be dropped when nothing uses its result. A
// pure instruction does nothing but produce that result; an impure one — a
// call, a recorded error, a step of an iterator — has to run either way.
func IsPure(instr Instruction) bool {
	switch instr.(type) {
	case *Const, *Unary, *Binary,
		*MakeArray, *MakeDict, *FieldRead, *MethodField,
		*ArrayElem, *ArraySlice,
		*DictField, *DictRest,
		*MakeClosure,
		*Heading, *Strong, *Emph, *Link, *RefMarkup,
		*ListItem, *EnumItem, *TermItem,
		*Equation, *MathAttach, *MathFrac, *MathRoot, *MathPrimes, *MathDelimited,
		*ContentResult, *CodeJoin,
		*JoinBegin, *JoinResult:
		return true
	}
	return false
}

// instr is embedded by the instruction kinds that produce a value. It supplies
// Result and setResult, and having setResult is what marks a kind as
// value-producing for [Builder.Finalize].
type instr struct {
	result Ref
}

func (i *instr) Result() Ref     { return i.result }
func (i *instr) aInstruction()   {}
func (i *instr) setResult(r Ref) { i.result = r }

// voidInstr is embedded by the instruction kinds that only have an effect. It
// has no result field and no setResult, so passes that rewrite result Refs skip
// these kinds. It carries the span instead, which [instr] leaves to
// [Function.RefSpans].
type voidInstr struct {
	span syntax.Span
}

func (i *voidInstr) Result() Ref       { return NoRef }
func (i *voidInstr) Span() syntax.Span { return i.span }
func (i *voidInstr) aInstruction()     {}

// Terminator ends a [BasicBlock]. Every block has exactly one.
type Terminator interface {
	// Successors returns the blocks control can reach from here.
	Successors() []BlockID

	// Operands appends the terminator's Ref operands to dst and returns the
	// result, including the args flowing into each successor's [BlockParam]s.
	Operands(dst []Ref) []Ref

	// RemapOperands replaces every Ref operand with rename of it, args
	// included.
	RemapOperands(rename func(Ref) Ref)

	// Span returns the source this terminator came from.
	Span() syntax.Span

	aTerminator()
}

type term struct {
	span syntax.Span
}

func (t *term) Span() syntax.Span { return t.span }
func (t *term) aTerminator()      {}

// Jump passes control to Target, binding Args to Target's [BlockParam]s in
// order. There is always exactly one arg per param.
type Jump struct {
	term
	Target BlockID
	Args   []Ref
}

func (t *Jump) Successors() []BlockID    { return []BlockID{t.Target} }
func (t *Jump) Operands(dst []Ref) []Ref { return append(dst, t.Args...) }
func (t *Jump) RemapOperands(f func(Ref) Ref) {
	for i := range t.Args {
		t.Args[i] = f(t.Args[i])
	}
}

// Branch passes control to Then or Else depending on Cond, binding ThenArgs or
// ElseArgs to that block's [BlockParam]s. Then and Else are always different
// blocks.
type Branch struct {
	term
	Cond     Ref
	Then     BlockID
	ThenArgs []Ref
	Else     BlockID
	ElseArgs []Ref
}

func (t *Branch) Successors() []BlockID { return []BlockID{t.Then, t.Else} }
func (t *Branch) Operands(dst []Ref) []Ref {
	dst = append(dst, t.Cond)
	dst = append(dst, t.ThenArgs...)
	dst = append(dst, t.ElseArgs...)
	return dst
}
func (t *Branch) RemapOperands(f func(Ref) Ref) {
	t.Cond = f(t.Cond)
	for i := range t.ThenArgs {
		t.ThenArgs[i] = f(t.ThenArgs[i])
	}
	for i := range t.ElseArgs {
		t.ElseArgs[i] = f(t.ElseArgs[i])
	}
}

// Return exits the enclosing [Function]. Value is the returned SSA value, or
// [NoRef] for a bare return (which yields none at runtime).
type Return struct {
	term
	Value Ref
}

func (t *Return) Successors() []BlockID { return nil }
func (t *Return) Operands(dst []Ref) []Ref {
	if t.Value == NoRef {
		return dst
	}
	return append(dst, t.Value)
}
func (t *Return) RemapOperands(f func(Ref) Ref) {
	if t.Value != NoRef {
		t.Value = f(t.Value)
	}
}

// Unreachable marks a control-flow position that the analyzer has proved
// cannot be reached (e.g. straight-line code after a Return). Evaluating it
// is a bug.
type Unreachable struct {
	term
}

func (t *Unreachable) Successors() []BlockID         { return nil }
func (t *Unreachable) Operands(dst []Ref) []Ref      { return dst }
func (t *Unreachable) RemapOperands(_ func(Ref) Ref) {}

// edgeArgs returns a pointer to the args slice on pred's terminator that
// flows into target. The pointer remains valid for the lifetime of the
// terminator and is the canonical handle for appending or splicing args.
// Panics if pred does not branch to target.
func edgeArgs(pred Terminator, target BlockID) *[]Ref {
	switch t := pred.(type) {
	case *Jump:
		if t.Target == target {
			return &t.Args
		}
	case *Branch:
		if t.Then == target {
			return &t.ThenArgs
		}
		if t.Else == target {
			return &t.ElseArgs
		}
	}
	panic("edgeArgs: predecessor does not target block")
}

// Instructions ////////////////////////////////////////////////////////////////
//
// The instruction kinds. Each embeds [instr] or, when it produces no value,
// [voidInstr].

// Const materializes a constant runtime value.
type Const struct {
	instr
	Value value.Value
}

func (c *Const) Operands(dst []Ref) []Ref      { return dst }
func (c *Const) RemapOperands(_ func(Ref) Ref) {}

// Unary applies a unary operator to a single operand.
type Unary struct {
	instr
	Op syntax.UnaryOp
	X  Ref
}

func (u *Unary) Operands(dst []Ref) []Ref      { return append(dst, u.X) }
func (u *Unary) RemapOperands(f func(Ref) Ref) { u.X = f(u.X) }

// Binary applies a non-assignment binary operator. Assignment forms
// (`=`, `+=`, etc.) lower to a write of the LHS, not to Binary.
type Binary struct {
	instr
	Op syntax.BinaryOp
	L  Ref
	R  Ref
}

func (b *Binary) Operands(dst []Ref) []Ref      { return append(dst, b.L, b.R) }
func (b *Binary) RemapOperands(f func(Ref) Ref) { b.L = f(b.L); b.R = f(b.R) }

// MakeArray constructs an array. Items may be plain values or spread sources;
// the [ArrayItem.Spread] flag distinguishes them.
type MakeArray struct {
	instr
	Items []ArrayItem
}

func (m *MakeArray) Operands(dst []Ref) []Ref {
	for _, it := range m.Items {
		dst = append(dst, it.Value)
	}
	return dst
}

func (m *MakeArray) RemapOperands(f func(Ref) Ref) {
	for i := range m.Items {
		m.Items[i].Value = f(m.Items[i].Value)
	}
}

// ArrayItem is one element of a [MakeArray] (plain value or spread source).
type ArrayItem struct {
	Value  Ref
	Spread bool
	Span   syntax.Span // span of the item (used for error reporting on spread)
}

// MakeDict constructs a dictionary. Entries may be key/value pairs or spread
// sources (`Key == NoRef && Spread == true`).
type MakeDict struct {
	instr
	Entries []DictEntry
}

func (m *MakeDict) Operands(dst []Ref) []Ref {
	out := make([]Ref, 0, 2*len(m.Entries))
	for _, e := range m.Entries {
		if e.Key != NoRef {
			out = append(out, e.Key)
		}
		out = append(out, e.Value)
	}
	return out
}

func (m *MakeDict) RemapOperands(f func(Ref) Ref) {
	for i := range m.Entries {
		if m.Entries[i].Key != NoRef {
			m.Entries[i].Key = f(m.Entries[i].Key)
		}
		m.Entries[i].Value = f(m.Entries[i].Value)
	}
}

// DictEntry is one entry of a [MakeDict].
type DictEntry struct {
	Key    Ref // NoRef when Spread is true
	Value  Ref
	Spread bool
}

// fieldAccess is the shared shape of a field-access instruction: reading the
// named field Field from Target. It is embedded by [FieldRead] and
// [MethodField]; field resolution lives in the runtime, not the IR.
//
// FieldSpan covers just the `.field` portion of the source (used by errors
// pointing at the field name, e.g. "content does not have field X");
// [inst.span] covers the whole `target.field` expression (used by errors that
// report against the access as a whole, e.g. "type T has no method").
type fieldAccess struct {
	instr
	Target    Ref
	Field     name.Name
	FieldSpan syntax.Span
}

func (f *fieldAccess) Operands(dst []Ref) []Ref       { return append(dst, f.Target) }
func (f *fieldAccess) RemapOperands(rn func(Ref) Ref) { f.Target = rn(f.Target) }

// FieldRead reads a named field from a value (dictionary, content, or function
// scope).
type FieldRead struct {
	fieldAccess
}

// MethodField reads the callee of a method call (`target.field(...)`). It uses
// the same field resolution as [FieldRead] but applies method-call error
// semantics: dictionary keys are not directly callable, and a missing field on
// a content element or a method-bearing type is reported as a missing method
// rather than a missing field. TargetText is the source text of the target
// expression, used to build the dictionary-key call hints.
type MethodField struct {
	fieldAccess
	TargetText string
	// Math is true when the method call is in math mode (`$ target.field(...) $`).
	// A math-mode call reports the same errors with math-specific hints, which
	// point at code mode, and for a dictionary key holding a non-function suggest
	// adding a space before the parentheses rather than removing them.
	Math bool
}

// ArgKind discriminates the variants of [CallArg].
type ArgKind uint8

const (
	ArgPositional ArgKind = iota
	ArgNamed
	ArgSpread
)

// CallArg is one argument of a [Call].
type CallArg struct {
	Kind     ArgKind
	Name     name.Name // valid for ArgNamed
	Value    Ref
	Span     syntax.Span // span of the value expression (used for error reporting)
	PairSpan syntax.Span // span of the `name: value` pair (ArgNamed only)
	// DirectFloatLit is true when the source argument was a syntactic float
	// literal (e.g. `decimal(1.32523)`). Used to gate the decimal-literal
	// precision warning so it doesn't fire when the value is computed (e.g.
	// laundered through a code block).
	DirectFloatLit bool
}

// Callee bundles the SSA Ref of a call's callee with the source span of the
// callee expression itself (as opposed to the whole call). Mirrors
// [CallArg]'s (Value, Span) pairing.
type Callee struct {
	Ref  Ref
	Span syntax.Span
}

// Call invokes a function value. Args carry the positional, named, and spread
// arguments in source order; Blocks holds the Refs of any trailing content
// blocks (markup form: `f[...]`). AllowSetter is true when this call is the LHS
// of an assignment, signaling that the callee's runtime should surface a
// setter via FunctionCallContext. Mut, set for method calls, carries the
// receiver place check that reports "cannot mutate a temporary value" when the
// resolved method is mutating.
type Call struct {
	instr
	Callee      Callee
	Args        []CallArg
	Blocks      []Ref
	AllowSetter bool
	Mut         *MutCheck
	Fallback    *MathFallback
}

// MathFallback is the juxtaposition form of a math call, rendered when the
// callee turns out not to be a function: `$sin(x)$` then means what `$sin (x)$`
// means, the callee's content followed by a parenthesized group. Items are the
// argument list's cells and separators in source order. BadArgs are the named
// and spread arguments, which have no meaning outside a real call and are
// reported when the fallback is taken. The analyzer leaves [Call.Fallback] nil
// when it already knows the callee is callable.
type MathFallback struct {
	Items   []MathItem
	BadArgs []MathBadArg
}

// MathItem is one piece of a [MathFallback]: an argument cell, or a literal
// separator when Ref is [NoRef].
type MathItem struct {
	Ref  Ref
	Text string // `,` or `;`, for a separator
}

// MathBadArg is a named or spread argument of a math call together with the
// diagnostic to report if the call falls back to juxtaposition.
type MathBadArg struct {
	Span  syntax.Span
	Msg   string
	Hints []value.Hint
}

// MutCheck describes a method call's receiver, so the evaluator can tell
// whether the call is allowed to mutate it.
//
// A mutating method ([value.Function.Impure]) needs a receiver that names a
// place rather than a temporary; if it does not, the call reports "cannot
// mutate a temporary value" at RecvSpan. Whether it does can only be settled at
// run time: the receiver has to be rooted in a variable (RecvTemporary false),
// and every method in the chain has to be an accessor (RecvAccessors, checked
// with [value.Function.Accessor]). A bare identifier or field chain has no
// accessors to check and is always a place.
type MutCheck struct {
	RecvSpan      syntax.Span
	RecvTemporary bool
	RecvAccessors []Ref

	// Math is set for method calls in math mode. A math-mode call whose resolved
	// callee is mutating is rejected outright ("cannot call mutating methods in
	// math") — there is no mutable place to write the result back to. MathCall is
	// the call's source text, used to build the "use code mode" hint.
	Math     bool
	MathCall string
}

func (c *Call) Operands(dst []Ref) []Ref {
	n := 1 + len(c.Args) + len(c.Blocks)
	if c.Mut != nil {
		n += len(c.Mut.RecvAccessors)
	}
	if c.Fallback != nil {
		n += len(c.Fallback.Items)
	}
	out := make([]Ref, 0, n)
	out = append(out, c.Callee.Ref)
	for _, a := range c.Args {
		out = append(out, a.Value)
	}
	out = append(out, c.Blocks...)
	if c.Mut != nil {
		out = append(out, c.Mut.RecvAccessors...)
	}
	if c.Fallback != nil {
		for _, it := range c.Fallback.Items {
			if it.Ref != NoRef {
				out = append(out, it.Ref)
			}
		}
	}
	return out
}

func (c *Call) RemapOperands(f func(Ref) Ref) {
	c.Callee.Ref = f(c.Callee.Ref)
	for i := range c.Args {
		c.Args[i].Value = f(c.Args[i].Value)
	}
	for i := range c.Blocks {
		c.Blocks[i] = f(c.Blocks[i])
	}
	if c.Mut != nil {
		for i := range c.Mut.RecvAccessors {
			c.Mut.RecvAccessors[i] = f(c.Mut.RecvAccessors[i])
		}
	}
	if c.Fallback != nil {
		for i := range c.Fallback.Items {
			if c.Fallback.Items[i].Ref != NoRef {
				c.Fallback.Items[i].Ref = f(c.Fallback.Items[i].Ref)
			}
		}
	}
}

// CallSet invokes a function whose return is an lvalue (the called function
// registers a setter via [value.FunctionCallContext.Setter]); the runtime then
// invokes the setter with the supplied new value. For compound assignments
// (`+=`, etc.) Op is the binary op (StripAssign applied); the setter receives
// `op(callResult, NewVal)`. For plain assignment Op == [syntax.Assign] and
// NewVal is written directly. Side-effect-only — no SSA result.
type CallSet struct {
	voidInstr
	Callee Callee
	Args   []CallArg
	Blocks []Ref
	NewVal Ref
	Op     syntax.BinaryOp
}

func (c *CallSet) Operands(dst []Ref) []Ref {
	dst = append(dst, c.Callee.Ref)
	for _, a := range c.Args {
		dst = append(dst, a.Value)
	}
	dst = append(dst, c.Blocks...)
	return append(dst, c.NewVal)
}

func (c *CallSet) RemapOperands(f func(Ref) Ref) {
	c.Callee.Ref = f(c.Callee.Ref)
	for i := range c.Args {
		c.Args[i].Value = f(c.Args[i].Value)
	}
	for i := range c.Blocks {
		c.Blocks[i] = f(c.Blocks[i])
	}
	c.NewVal = f(c.NewVal)
}

// FieldWrite writes a value into a named field of a target (a dictionary entry,
// content field, etc.). For compound assignment, Op carries the stripped binary
// op so the runtime can compute `old op NewVal` before writing. Side-effect-
// only — no SSA result.
type FieldWrite struct {
	voidInstr
	Target Ref
	Field  name.Name
	NewVal Ref
	Op     syntax.BinaryOp
}

func (f *FieldWrite) Operands(dst []Ref) []Ref { return append(dst, f.Target, f.NewVal) }
func (f *FieldWrite) RemapOperands(rn func(Ref) Ref) {
	f.Target = rn(f.Target)
	f.NewVal = rn(f.NewVal)
}

// DiscardCheck warns, at eval time, when an explicit `return value` throws away
// the joined content a bare return would have produced instead. Value is the
// discarded join; the runtime emits the warning only when it is actually
// content (non-content prefixes — arrays, strings, none — never warn).
// Side-effect-only: it records a warning on the session and has no SSA result.
type DiscardCheck struct {
	voidInstr
	Value Ref
}

// Warn records a non-fatal diagnostic at eval time (e.g. linebreaks ignored in
// a math cell). It is emitted statically by the analyzer but fires only when
// its block executes, so it never warns for code that isn't evaluated.
// Side-effect-only: no SSA result and no operands.
type Warn struct {
	voidInstr
	Msg   string
	Hints []string
}

func (w *Warn) Operands(dst []Ref) []Ref    { return dst }
func (w *Warn) RemapOperands(func(Ref) Ref) {}

func (d *DiscardCheck) Operands(dst []Ref) []Ref       { return append(dst, d.Value) }
func (d *DiscardCheck) RemapOperands(rn func(Ref) Ref) { d.Value = rn(d.Value) }

// DestructArray validates Source for a destructuring pattern with positional
// items (possibly with a sink).
//
// Before is the number of fixed positions before the sink (or before the
// end if no sink); After is the number after the sink. HasSink reports
// whether the pattern includes a `..rest` sink.
//
// Hybrid is true when every positional item is a simple identifier (so the
// pattern can also destructure a dict via shorthand semantics, where each
// ident names a key).
//
// On success, the result carries the validated source (array or, in
// Hybrid mode, dict) so downstream readers can dispatch by type. On
// failure the result is a *value.Error.
type DestructArray struct {
	instr
	Source  Ref
	Before  int
	After   int
	HasSink bool
	Hybrid  bool
}

func (d *DestructArray) Operands(dst []Ref) []Ref      { return append(dst, d.Source) }
func (d *DestructArray) RemapOperands(f func(Ref) Ref) { d.Source = f(d.Source) }

// ArrayElem reads a single element from Source. When FromEnd is false the
// element is Index counted from the front; when FromEnd is true it is Index
// counted from the back (0 = last element). When Source is a dict and Key is
// non-zero, the lookup uses Key instead (hybrid dict shorthand).
type ArrayElem struct {
	instr
	Source  Ref
	Index   int
	FromEnd bool
	Key     name.Name
}

func (a *ArrayElem) Operands(dst []Ref) []Ref      { return append(dst, a.Source) }
func (a *ArrayElem) RemapOperands(f func(Ref) Ref) { a.Source = f(a.Source) }

// ArraySlice produces a sub-array Source[Before:len-After] — i.e. the
// "rest" of a destructuring pattern with a sink. When Source is a dict
// and Hybrid is true, the result is a dict containing every entry whose
// key is not in ExcludeKeys.
type ArraySlice struct {
	instr
	Source      Ref
	Before      int
	After       int
	Hybrid      bool
	ExcludeKeys []name.Name
}

func (a *ArraySlice) Operands(dst []Ref) []Ref      { return append(dst, a.Source) }
func (a *ArraySlice) RemapOperands(f func(Ref) Ref) { a.Source = f(a.Source) }

// DestructDict validates Source for a dict-shape destructuring pattern.
// FirstNamedSpan is the span of the first named pair in the pattern; it
// is used as the error span when Source turns out to be an array
// ("cannot destructure named pattern from an array"). Consumed lists the
// keys consumed by named/shorthand items.
type DestructDict struct {
	instr
	Source         Ref
	Consumed       []name.Name
	HasSink        bool
	FirstNamedSpan syntax.Span
}

func (d *DestructDict) Operands(dst []Ref) []Ref      { return append(dst, d.Source) }
func (d *DestructDict) RemapOperands(f func(Ref) Ref) { d.Source = f(d.Source) }

// DictField reads a key from Source for a destructuring pattern. Unlike
// [FieldRead], the error message uses the destructure phrasing
// "dictionary does not contain key %q".
type DictField struct {
	instr
	Source    Ref
	Field     name.Name
	FieldSpan syntax.Span
}

func (d *DictField) Operands(dst []Ref) []Ref      { return append(dst, d.Source) }
func (d *DictField) RemapOperands(f func(Ref) Ref) { d.Source = f(d.Source) }

// DictRest produces a dictionary containing all entries of Source whose
// keys are not in Consumed.
type DictRest struct {
	instr
	Source   Ref
	Consumed []name.Name
}

func (d *DictRest) Operands(dst []Ref) []Ref      { return append(dst, d.Source) }
func (d *DictRest) RemapOperands(f func(Ref) Ref) { d.Source = f(d.Source) }

// IterOpen produces an opaque iterator over an array/dict/string.
//
// Destructuring is true when the enclosing for-loop's pattern is a
// destructuring pattern (e.g. `for (x, y) in iterable`). It only affects
// the runtime check for string iterables: with Destructuring set, opening
// an iterator over a string raises "cannot destructure values of string"
// at PatternSpan instead of producing characters.
type IterOpen struct {
	instr
	Iterable      Ref
	Destructuring bool
	PatternSpan   syntax.Span
}

func (i *IterOpen) Operands(dst []Ref) []Ref      { return append(dst, i.Iterable) }
func (i *IterOpen) RemapOperands(f func(Ref) Ref) { i.Iterable = f(i.Iterable) }

// IterHasNext returns a boolean indicating whether the iterator has more
// elements.
type IterHasNext struct {
	instr
	Iter Ref
}

func (i *IterHasNext) Operands(dst []Ref) []Ref      { return append(dst, i.Iter) }
func (i *IterHasNext) RemapOperands(f func(Ref) Ref) { i.Iter = f(i.Iter) }

// IterAdvance advances the iterator one step and yields the next element. Must
// only be evaluated when [IterHasNext] just returned true.
type IterAdvance struct {
	instr
	Iter Ref
}

func (i *IterAdvance) Operands(dst []Ref) []Ref      { return append(dst, i.Iter) }
func (i *IterAdvance) RemapOperands(f func(Ref) Ref) { i.Iter = f(i.Iter) }

// MakeClosure constructs a closure value pointing at the [Function] with the
// given FuncID, capturing one outer SSA value per entry in the function's
// [Function.Captures] list.
type MakeClosure struct {
	instr
	Func     FuncID
	Captures []Ref
}

func (m *MakeClosure) Operands(dst []Ref) []Ref { return append(dst, m.Captures...) }
func (m *MakeClosure) RemapOperands(f func(Ref) Ref) {
	for i := range m.Captures {
		m.Captures[i] = f(m.Captures[i])
	}
}

// Markup instructions ////////////////////////////////////////////////////////

// ContentResult joins a sequence of value refs into a content sequence,
// applying the content joiner. Used at the end of markup bodies / content
// blocks to materialise the document/content fragment.
//
// Math marks a join over the contents of an equation, where symbols, strings
// and numbers become math text rather than the upright text they are in markup
// (see value.ToMathContent) — a single-item math body bypasses the joiner and
// is coerced by the element it lands in, so both paths have to agree.
type ContentResult struct {
	instr
	Items []Ref
	Math  bool
}

func (c *ContentResult) Operands(dst []Ref) []Ref { return append(dst, c.Items...) }
func (c *ContentResult) RemapOperands(f func(Ref) Ref) {
	for i := range c.Items {
		c.Items[i] = f(c.Items[i])
	}
}

// Error raises a value error at eval time with the stored message. Used to
// surface deferred analyzer errors (e.g. "cannot mutate a temporary value").
//
// If From is set, the instruction propagates from that Ref's value when it
// is already a [*value.Error]: the propagating error is yielded as the
// instruction's result and Msg is suppressed. This keeps the diagnostic
// from cascading when an upstream computation already failed. The cascade
// is implemented by the generic operand-error propagation in evalInst,
// driven by [Error.Operands] returning From — no special-case logic in the
// Error case body.
type Error struct {
	instr
	Msg   string
	Hints []string
	From  Ref // optional; NoRef when this Error is unconditional
	// Reported marks an error already surfaced via [Module.ParseErrors]
	// (a scanner/parser diagnostic on the source, reported whether or not
	// this code path runs). The runtime evaluates it to a poison
	// [value.Error] without recording it again.
	Reported bool
}

func (r *Error) Operands(dst []Ref) []Ref {
	if r.From == NoRef {
		return dst
	}
	return append(dst, r.From)
}
func (r *Error) RemapOperands(f func(Ref) Ref) {
	if r.From != NoRef {
		r.From = f(r.From)
	}
}

// AttachLabel puts a label on the content Content produces, and records it in
// the document's label set so a [RefMarkup] can resolve it. Labeling content
// that already has a label warns at eval time and keeps the newer label.
type AttachLabel struct {
	instr
	Content Ref
	Label   name.Name
}

func (a *AttachLabel) Operands(dst []Ref) []Ref      { return append(dst, a.Content) }
func (a *AttachLabel) RemapOperands(f func(Ref) Ref) { a.Content = f(a.Content) }

// CodeJoin joins a sequence of value refs using the code-mode joiner. The
// joiner selects an output type based on the input types (strings concat, ints
// sum, content sequence, etc.). Used as the final value of code blocks.
type CodeJoin struct {
	instr
	Items     []Ref
	ItemSpans []syntax.Span // parallel to Items; used for per-item error reporting
}

func (c *CodeJoin) Operands(dst []Ref) []Ref { return append(dst, c.Items...) }
func (c *CodeJoin) RemapOperands(f func(Ref) Ref) {
	for i := range c.Items {
		c.Items[i] = f(c.Items[i])
	}
}

// JoinBegin produces an initial join-accumulator value (an empty array
// internally; treated as opaque accumulator state). Each iteration's value is
// added via [JoinAdd], and the final joined value is obtained via [JoinResult].
// This is the dynamic-arity counterpart to the pure n-ary
// [CodeJoin]/[ContentResult]: it exists for the one join whose item count is
// not known at lowering time, a loop's cross-iteration accumulator. Every
// straight-line join (block and function bodies) is built statically instead.
type JoinBegin struct {
	instr
}

func (*JoinBegin) Operands(dst []Ref) []Ref      { return dst }
func (*JoinBegin) RemapOperands(_ func(Ref) Ref) {}

// JoinAdd appends item to a loop accumulator and returns the same accumulator
// (mutated in place). The mutation is safe under SSA because the previous
// accumulator value is read exactly once per iteration before being overwritten
// by the WriteVar that follows.
type JoinAdd struct {
	instr
	Acc  Ref
	Item Ref
}

func (a *JoinAdd) Operands(dst []Ref) []Ref { return append(dst, a.Acc, a.Item) }
func (a *JoinAdd) RemapOperands(f func(Ref) Ref) {
	a.Acc = f(a.Acc)
	a.Item = f(a.Item)
}

// JoinResult joins the accumulated items into a single value using the
// code-mode joiner (same semantics as [CodeJoin]). It finalizes a loop's
// cross-iteration accumulator; straight-line joins are built statically with
// [CodeJoin]/[ContentResult] instead.
type JoinResult struct {
	instr
	Acc Ref
}

func (a *JoinResult) Operands(dst []Ref) []Ref      { return append(dst, a.Acc) }
func (a *JoinResult) RemapOperands(f func(Ref) Ref) { a.Acc = f(a.Acc) }

// Heading is a `= Title` heading at the given level.
type Heading struct {
	instr
	Level int
	Body  Ref
}

func (h *Heading) Operands(dst []Ref) []Ref      { return append(dst, h.Body) }
func (h *Heading) RemapOperands(f func(Ref) Ref) { h.Body = f(h.Body) }

// Strong wraps content in bold formatting.
type Strong struct {
	instr
	Body Ref
}

func (s *Strong) Operands(dst []Ref) []Ref      { return append(dst, s.Body) }
func (s *Strong) RemapOperands(f func(Ref) Ref) { s.Body = f(s.Body) }

// Emph wraps content in italic formatting.
type Emph struct {
	instr
	Body Ref
}

func (e *Emph) Operands(dst []Ref) []Ref      { return append(dst, e.Body) }
func (e *Emph) RemapOperands(f func(Ref) Ref) { e.Body = f(e.Body) }

// Link is a hyperlink with a destination URL and body content.
type Link struct {
	instr
	Dest string
	Body Ref
}

func (l *Link) Operands(dst []Ref) []Ref      { return append(dst, l.Body) }
func (l *Link) RemapOperands(f func(Ref) Ref) { l.Body = f(l.Body) }

// RefMarkup is the `@label` reference construct (named with the `Markup`
// suffix to disambiguate from the SSA-value type [Ref]).
type RefMarkup struct {
	instr
	Target     name.Name
	Supplement Ref // NoRef when no supplement was provided
}

func (r *RefMarkup) Operands(dst []Ref) []Ref {
	if r.Supplement == NoRef {
		return dst
	}
	return append(dst, r.Supplement)
}

func (r *RefMarkup) RemapOperands(f func(Ref) Ref) {
	if r.Supplement != NoRef {
		r.Supplement = f(r.Supplement)
	}
}

// ListItem is one item of a bulleted list.
type ListItem struct {
	instr
	Body Ref
}

func (l *ListItem) Operands(dst []Ref) []Ref      { return append(dst, l.Body) }
func (l *ListItem) RemapOperands(f func(Ref) Ref) { l.Body = f(l.Body) }

// EnumItem is one item of a numbered list. Number is -1 when the item was
// written with `+` and takes its number from its position.
type EnumItem struct {
	instr
	Number int
	Body   Ref
}

func (e *EnumItem) Operands(dst []Ref) []Ref      { return append(dst, e.Body) }
func (e *EnumItem) RemapOperands(f func(Ref) Ref) { e.Body = f(e.Body) }

// TermItem is one entry of a term list: a term and its description.
type TermItem struct {
	instr
	Term        Ref
	Description Ref
}

func (t *TermItem) Operands(dst []Ref) []Ref { return append(dst, t.Term, t.Description) }
func (t *TermItem) RemapOperands(f func(Ref) Ref) {
	t.Term = f(t.Term)
	t.Description = f(t.Description)
}

// Math instructions ///////////////////////////////////////////////////////////

// Equation is a math equation, `$...$`. Block reports whether it stands on a
// line of its own rather than in the surrounding text.
type Equation struct {
	instr
	Block bool
	Body  Ref
}

func (e *Equation) Operands(dst []Ref) []Ref      { return append(dst, e.Body) }
func (e *Equation) RemapOperands(f func(Ref) Ref) { e.Body = f(e.Body) }

// MathAttach is a base with optional sub-/superscripts: a_1^2. Top and Bottom
// are NoRef when absent.
type MathAttach struct {
	instr
	Base   Ref
	Top    Ref
	Bottom Ref
}

func (m *MathAttach) Operands(dst []Ref) []Ref {
	dst = append(dst, m.Base)
	if m.Top != NoRef {
		dst = append(dst, m.Top)
	}
	if m.Bottom != NoRef {
		dst = append(dst, m.Bottom)
	}
	return dst
}

func (m *MathAttach) RemapOperands(f func(Ref) Ref) {
	m.Base = f(m.Base)
	if m.Top != NoRef {
		m.Top = f(m.Top)
	}
	if m.Bottom != NoRef {
		m.Bottom = f(m.Bottom)
	}
}

// MathFrac is a fraction: x/2.
type MathFrac struct {
	instr
	Num   Ref
	Denom Ref
}

func (m *MathFrac) Operands(dst []Ref) []Ref { return append(dst, m.Num, m.Denom) }
func (m *MathFrac) RemapOperands(f func(Ref) Ref) {
	m.Num = f(m.Num)
	m.Denom = f(m.Denom)
}

// MathRoot is a root: √x or root(3, x). Index is NoRef for a square root.
type MathRoot struct {
	instr
	Index    Ref
	Radicand Ref
}

func (m *MathRoot) Operands(dst []Ref) []Ref {
	if m.Index == NoRef {
		return []Ref{m.Radicand}
	}
	return []Ref{m.Index, m.Radicand}
}

func (m *MathRoot) RemapOperands(f func(Ref) Ref) {
	if m.Index != NoRef {
		m.Index = f(m.Index)
	}
	m.Radicand = f(m.Radicand)
}

// MathPrimes attaches Count prime marks to a base: a”'.
type MathPrimes struct {
	instr
	Base  Ref
	Count int
}

func (m *MathPrimes) Operands(dst []Ref) []Ref      { return append(dst, m.Base) }
func (m *MathPrimes) RemapOperands(f func(Ref) Ref) { m.Base = f(m.Base) }

// MathDelimited is a delimited group in math: [x + y]. Open and Close hold the
// delimiter content.
type MathDelimited struct {
	instr
	Open  Ref
	Body  Ref
	Close Ref
}

func (m *MathDelimited) Operands(dst []Ref) []Ref { return append(dst, m.Open, m.Body, m.Close) }
func (m *MathDelimited) RemapOperands(f func(Ref) Ref) {
	m.Open = f(m.Open)
	m.Body = f(m.Body)
	m.Close = f(m.Close)
}

// Rules and other top-level constructs /////////////////////////////////////////

// SetRule is a `set target(args) [if cond]` rule.
type SetRule struct {
	instr
	Target    Ref
	Args      []CallArg
	Condition Ref // NoRef when no `if` clause
}

func (s *SetRule) Operands(dst []Ref) []Ref {
	dst = append(dst, s.Target)
	for _, a := range s.Args {
		dst = append(dst, a.Value)
	}
	if s.Condition != NoRef {
		dst = append(dst, s.Condition)
	}
	return dst
}

func (s *SetRule) RemapOperands(f func(Ref) Ref) {
	s.Target = f(s.Target)
	for i := range s.Args {
		s.Args[i].Value = f(s.Args[i].Value)
	}
	if s.Condition != NoRef {
		s.Condition = f(s.Condition)
	}
}

// ShowRule is a `show selector: transform` rule.
type ShowRule struct {
	instr
	Selector  Ref // NoRef for bare `show: transform`
	Transform Ref
}

func (s *ShowRule) Operands(dst []Ref) []Ref {
	if s.Selector == NoRef {
		return []Ref{s.Transform}
	}
	return []Ref{s.Selector, s.Transform}
}

func (s *ShowRule) RemapOperands(f func(Ref) Ref) {
	if s.Selector != NoRef {
		s.Selector = f(s.Selector)
	}
	s.Transform = f(s.Transform)
}

// Contextual is a `context expr` construct.
type Contextual struct {
	instr
	Body Ref
}

func (c *Contextual) Operands(dst []Ref) []Ref      { return append(dst, c.Body) }
func (c *Contextual) RemapOperands(f func(Ref) Ref) { c.Body = f(c.Body) }

// ModuleInclude is `include "path"`.
type ModuleInclude struct {
	instr
	Source Ref
}

func (m *ModuleInclude) Operands(dst []Ref) []Ref      { return append(dst, m.Source) }
func (m *ModuleInclude) RemapOperands(f func(Ref) Ref) { m.Source = f(m.Source) }
