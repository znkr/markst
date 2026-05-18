package expr

import (
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// Instruction is a single SSA operation. Instructions live inside
// [BasicBlock.Instrs] in program order; each produces a single SSA value (its
// [Result]) or [NoRef] for void operations.
//
// Per-Ref source spans for value-producing kinds live in [Function.RefSpans]
// keyed by [Result]; the [instr] base therefore carries no span field.
// Void-instruction kinds (which have no Result) keep their span on the
// [voidInstr] base.
type Instruction interface {
	Result() Ref
	Operands() []Ref
	// RemapOperands substitutes every Ref-typed operand through rename.
	// Used by [Builder.Finalize] to inline trivial-param removals.
	RemapOperands(rename func(Ref) Ref)
	aInstruction()
}

// IsPure reports whether inst can be safely removed when its result Ref has
// no uses. Pure instructions have no observable effect beyond producing
// their result; impure ones (calls, error-recording, iterator mutation,
// stub TODO instructions) must be kept regardless of use count.
//
// Block params are always pure; this predicate is only consulted for
// instructions.
func IsPure(instr Instruction) bool {
	switch instr.(type) {
	case *Const, *Unary, *Binary,
		*MakeArray, *MakeDict, *FieldRead, *Extract,
		*MakeClosure,
		*Heading, *Strong, *Emph, *Link, *RefMarkup,
		*ListItem, *EnumItem, *TermItem,
		*ContentResult, *CodeJoin,
		*LoopAccBegin, *LoopAccResult:
		return true
	}
	return false
}

// instr is the base for value-producing instruction kinds: those that
// publish their result as an SSA value via [Result]. Concrete kinds embed
// *instr to inherit Result/aInstruction (and setResult, which is used by
// [Builder.Finalize]'s Ref-compaction pass). The source span lives in
// [Function.RefSpans] indexed by [Result], not on the instruction.
type instr struct {
	result Ref
}

func (i *instr) Result() Ref     { return i.result }
func (i *instr) aInstruction()   {}
func (i *instr) setResult(r Ref) { i.result = r }

// voidInstr is the base for side-effect-only instruction kinds: those that
// have no SSA result. Embedding voidInstr instead of [instr] is what makes
// a kind structurally void — there is no result field to misuse, no
// setResult method, and [Result] is hardcoded to [NoRef]. Compaction and
// other Ref-rewriting passes dispatch on the absence of setResult to skip
// these kinds.
type voidInstr struct {
	span syntax.Span
}

func (i *voidInstr) Result() Ref       { return NoRef }
func (i *voidInstr) Span() syntax.Span { return i.span }
func (i *voidInstr) aInstruction()     {}

// Terminator ends a [BasicBlock]. Every block has exactly one.
type Terminator interface {
	Successors() []BlockID
	// Operands returns the Ref-typed operands of the terminator, including
	// every arg flowing to a successor's [BlockParam] slot.
	Operands() []Ref
	// RemapOperands substitutes every Ref-typed operand through rename,
	// including args.
	RemapOperands(rename func(Ref) Ref)
	Span() syntax.Span
	aTerminator()
}

type term struct {
	span syntax.Span
}

func (t *term) Span() syntax.Span { return t.span }
func (t *term) aTerminator()      {}

// Jump unconditionally transfers control to Target, carrying Args into
// Target's [BlockParam]s in declaration order. `len(Args)` always equals
// `len(Blocks[Target].Params)`.
type Jump struct {
	term
	Target BlockID
	Args   []Ref
}

func (t *Jump) Successors() []BlockID { return []BlockID{t.Target} }
func (t *Jump) Operands() []Ref       { return t.Args }
func (t *Jump) RemapOperands(f func(Ref) Ref) {
	for i := range t.Args {
		t.Args[i] = f(t.Args[i])
	}
}

// Branch is a two-way branch on a boolean SSA value. ThenArgs flow to
// Blocks[Then].Params on the true edge; ElseArgs flow to
// Blocks[Else].Params on the false edge. Then and Else are distinct
// blocks (analyzer-enforced).
type Branch struct {
	term
	Cond     Ref
	Then     BlockID
	ThenArgs []Ref
	Else     BlockID
	ElseArgs []Ref
}

func (t *Branch) Successors() []BlockID { return []BlockID{t.Then, t.Else} }
func (t *Branch) Operands() []Ref {
	out := make([]Ref, 0, 1+len(t.ThenArgs)+len(t.ElseArgs))
	out = append(out, t.Cond)
	out = append(out, t.ThenArgs...)
	out = append(out, t.ElseArgs...)
	return out
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
func (t *Return) Operands() []Ref {
	if t.Value == NoRef {
		return nil
	}
	return []Ref{t.Value}
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
func (t *Unreachable) Operands() []Ref               { return nil }
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
// Concrete instruction types live here. Each embeds [inst] for span/result.

// Const materializes a constant runtime value.
type Const struct {
	instr
	Value value.Value
}

func (c *Const) Operands() []Ref               { return nil }
func (c *Const) RemapOperands(_ func(Ref) Ref) {}

// Unary applies a unary operator to a single operand.
type Unary struct {
	instr
	Op syntax.UnaryOp
	X  Ref
}

func (u *Unary) Operands() []Ref               { return []Ref{u.X} }
func (u *Unary) RemapOperands(f func(Ref) Ref) { u.X = f(u.X) }

// Binary applies a non-assignment binary operator. Assignment forms
// (`=`, `+=`, etc.) lower to a write of the LHS, not to Binary.
type Binary struct {
	instr
	Op syntax.BinaryOp
	L  Ref
	R  Ref
}

func (b *Binary) Operands() []Ref               { return []Ref{b.L, b.R} }
func (b *Binary) RemapOperands(f func(Ref) Ref) { b.L = f(b.L); b.R = f(b.R) }

// MakeArray constructs an array. Items may be plain values or spread sources;
// the [ArrayItem.Spread] flag distinguishes them.
type MakeArray struct {
	instr
	Items []ArrayItem
}

func (m *MakeArray) Operands() []Ref {
	out := make([]Ref, len(m.Items))
	for i, it := range m.Items {
		out[i] = it.Value
	}
	return out
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

func (m *MakeDict) Operands() []Ref {
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

// FieldRead reads a named field from a value (dictionary, content, or method
// receiver). Method dispatch lives in the runtime, not the IR.
//
// FieldSpan covers just the `.field` portion of the source (used by errors
// pointing at the field name, e.g. "content does not have field X");
// [inst.span] covers the whole `target.field` expression (used by errors that
// report against the access as a whole, e.g. "type T has no method").
type FieldRead struct {
	instr
	Target    Ref
	Field     name.Name
	FieldSpan syntax.Span
}

func (f *FieldRead) Operands() []Ref                { return []Ref{f.Target} }
func (f *FieldRead) RemapOperands(rn func(Ref) Ref) { f.Target = rn(f.Target) }

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

// Call invokes a function value. Args carry the positional, named, and spread
// arguments in source order; Blocks holds the Refs of any trailing content
// blocks (markup form: `f[...]`). AllowSetter is true when this call is the LHS
// of an assignment, signalling that the callee's runtime should surface a
// setter via FunctionCallContext.
type Call struct {
	instr
	Callee      Ref
	Args        []CallArg
	Blocks      []Ref
	AllowSetter bool
}

func (c *Call) Operands() []Ref {
	out := make([]Ref, 0, 1+len(c.Args)+len(c.Blocks))
	out = append(out, c.Callee)
	for _, a := range c.Args {
		out = append(out, a.Value)
	}
	out = append(out, c.Blocks...)
	return out
}

func (c *Call) RemapOperands(f func(Ref) Ref) {
	c.Callee = f(c.Callee)
	for i := range c.Args {
		c.Args[i].Value = f(c.Args[i].Value)
	}
	for i := range c.Blocks {
		c.Blocks[i] = f(c.Blocks[i])
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
	Callee Ref
	Args   []CallArg
	Blocks []Ref
	NewVal Ref
	Op     syntax.BinaryOp
}

func (c *CallSet) Operands() []Ref {
	out := make([]Ref, 0, 2+len(c.Args)+len(c.Blocks))
	out = append(out, c.Callee)
	for _, a := range c.Args {
		out = append(out, a.Value)
	}
	out = append(out, c.Blocks...)
	out = append(out, c.NewVal)
	return out
}

func (c *CallSet) RemapOperands(f func(Ref) Ref) {
	c.Callee = f(c.Callee)
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

func (f *FieldWrite) Operands() []Ref { return []Ref{f.Target, f.NewVal} }
func (f *FieldWrite) RemapOperands(rn func(Ref) Ref) {
	f.Target = rn(f.Target)
	f.NewVal = rn(f.NewVal)
}

// Extract reads a positional element from an indexed source (array or
// tuple-like value). Used to lower destructuring patterns.
type Extract struct {
	instr
	Source Ref
	Index  int
}

func (e *Extract) Operands() []Ref               { return []Ref{e.Source} }
func (e *Extract) RemapOperands(f func(Ref) Ref) { e.Source = f(e.Source) }

// LengthCheck asserts that Source has at least Want elements (or exactly Want
// if HasSink is false). Used to lower destructuring patterns; raises a runtime
// error if the assertion fails. Side-effect-only — no SSA result.
type LengthCheck struct {
	voidInstr
	Source  Ref
	Want    int
	HasSink bool
}

func (l *LengthCheck) Operands() []Ref               { return []Ref{l.Source} }
func (l *LengthCheck) RemapOperands(f func(Ref) Ref) { l.Source = f(l.Source) }

// IterOpen produces an opaque iterator over an array/dict/string.
type IterOpen struct {
	instr
	Iterable Ref
}

func (i *IterOpen) Operands() []Ref               { return []Ref{i.Iterable} }
func (i *IterOpen) RemapOperands(f func(Ref) Ref) { i.Iterable = f(i.Iterable) }

// IterHasNext returns a boolean indicating whether the iterator has more
// elements.
type IterHasNext struct {
	instr
	Iter Ref
}

func (i *IterHasNext) Operands() []Ref               { return []Ref{i.Iter} }
func (i *IterHasNext) RemapOperands(f func(Ref) Ref) { i.Iter = f(i.Iter) }

// IterAdvance advances the iterator one step and yields the next element. Must
// only be evaluated when [IterHasNext] just returned true.
type IterAdvance struct {
	instr
	Iter Ref
}

func (i *IterAdvance) Operands() []Ref               { return []Ref{i.Iter} }
func (i *IterAdvance) RemapOperands(f func(Ref) Ref) { i.Iter = f(i.Iter) }

// MakeClosure constructs a closure value pointing at the [Function] with the
// given FuncID, capturing one outer SSA value per entry in the function's
// [Function.Captures] list.
type MakeClosure struct {
	instr
	Func     FuncID
	Captures []Ref
}

func (m *MakeClosure) Operands() []Ref { return m.Captures }
func (m *MakeClosure) RemapOperands(f func(Ref) Ref) {
	for i := range m.Captures {
		m.Captures[i] = f(m.Captures[i])
	}
}

// Markup instructions ////////////////////////////////////////////////////////

// ContentResult joins a sequence of value refs into a content sequence,
// applying the content joiner. Used at the end of markup bodies / content
// blocks to materialise the document/content fragment.
type ContentResult struct {
	instr
	Items []Ref
}

func (c *ContentResult) Operands() []Ref { return c.Items }
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
}

func (r *Error) Operands() []Ref {
	if r.From == NoRef {
		return nil
	}
	return []Ref{r.From}
}
func (r *Error) RemapOperands(f func(Ref) Ref) {
	if r.From != NoRef {
		r.From = f(r.From)
	}
}

// AttachLabel binds a label name to the content produced by Content. It also
// registers the label in the evaluator's label set so subsequent [RefMarkup]
// instructions can resolve it. Re-labelling produces a runtime warning and
// discards the older label, matching legacy semantics.
type AttachLabel struct {
	instr
	Content Ref
	Label   name.Name
}

func (a *AttachLabel) Operands() []Ref               { return []Ref{a.Content} }
func (a *AttachLabel) RemapOperands(f func(Ref) Ref) { a.Content = f(a.Content) }

// CodeJoin joins a sequence of value refs using the code-mode joiner. The
// joiner selects an output type based on the input types (strings concat, ints
// sum, content sequence, etc.). Used as the final value of code blocks.
type CodeJoin struct {
	instr
	Items     []Ref
	ItemSpans []syntax.Span // parallel to Items; used for per-item error reporting
}

func (c *CodeJoin) Operands() []Ref { return c.Items }
func (c *CodeJoin) RemapOperands(f func(Ref) Ref) {
	for i := range c.Items {
		c.Items[i] = f(c.Items[i])
	}
}

// LoopAccBegin produces an initial loop-accumulator value (an empty array
// internally; treated as opaque accumulator state). Each iteration's body
// result is added via [LoopAccAdd], and the final joined value is obtained via
// [LoopAccResult].
type LoopAccBegin struct {
	instr
}

func (*LoopAccBegin) Operands() []Ref               { return nil }
func (*LoopAccBegin) RemapOperands(_ func(Ref) Ref) {}

// LoopAccAdd appends item to the accumulator and returns the same accumulator
// (mutated in place). The mutation is safe under SSA because the previous
// accumulator value is read exactly once per iteration before being overwritten
// by the WriteVar that follows.
type LoopAccAdd struct {
	instr
	Acc  Ref
	Item Ref
}

func (a *LoopAccAdd) Operands() []Ref { return []Ref{a.Acc, a.Item} }
func (a *LoopAccAdd) RemapOperands(f func(Ref) Ref) {
	a.Acc = f(a.Acc)
	a.Item = f(a.Item)
}

// LoopAccResult joins the accumulated items into a single value using the
// code-mode joiner (same semantics as [CodeJoin]).
type LoopAccResult struct {
	instr
	Acc Ref
}

func (a *LoopAccResult) Operands() []Ref               { return []Ref{a.Acc} }
func (a *LoopAccResult) RemapOperands(f func(Ref) Ref) { a.Acc = f(a.Acc) }

// Heading represents a `= Title`-style heading at the given level.
type Heading struct {
	instr
	Level int
	Body  Ref
}

func (h *Heading) Operands() []Ref               { return []Ref{h.Body} }
func (h *Heading) RemapOperands(f func(Ref) Ref) { h.Body = f(h.Body) }

// Strong wraps content in bold formatting.
type Strong struct {
	instr
	Body Ref
}

func (s *Strong) Operands() []Ref               { return []Ref{s.Body} }
func (s *Strong) RemapOperands(f func(Ref) Ref) { s.Body = f(s.Body) }

// Emph wraps content in italic formatting.
type Emph struct {
	instr
	Body Ref
}

func (e *Emph) Operands() []Ref               { return []Ref{e.Body} }
func (e *Emph) RemapOperands(f func(Ref) Ref) { e.Body = f(e.Body) }

// Link is a hyperlink with a destination URL and body content.
type Link struct {
	instr
	Dest string
	Body Ref
}

func (l *Link) Operands() []Ref               { return []Ref{l.Body} }
func (l *Link) RemapOperands(f func(Ref) Ref) { l.Body = f(l.Body) }

// RefMarkup is the `@label` reference construct (named with the `Markup`
// suffix to disambiguate from the SSA-value type [Ref]).
type RefMarkup struct {
	instr
	Target     name.Name
	Supplement Ref // NoRef when no supplement was provided
}

func (r *RefMarkup) Operands() []Ref {
	if r.Supplement == NoRef {
		return nil
	}
	return []Ref{r.Supplement}
}

func (r *RefMarkup) RemapOperands(f func(Ref) Ref) {
	if r.Supplement != NoRef {
		r.Supplement = f(r.Supplement)
	}
}

// ListItem represents a bulleted list item.
type ListItem struct {
	instr
	Body Ref
}

func (l *ListItem) Operands() []Ref               { return []Ref{l.Body} }
func (l *ListItem) RemapOperands(f func(Ref) Ref) { l.Body = f(l.Body) }

// EnumItem represents a numbered list item. Number is -1 for "+" markers.
type EnumItem struct {
	instr
	Number int
	Body   Ref
}

func (e *EnumItem) Operands() []Ref               { return []Ref{e.Body} }
func (e *EnumItem) RemapOperands(f func(Ref) Ref) { e.Body = f(e.Body) }

// TermItem represents a definition-list entry: term / description.
type TermItem struct {
	instr
	Term        Ref
	Description Ref
}

func (t *TermItem) Operands() []Ref { return []Ref{t.Term, t.Description} }
func (t *TermItem) RemapOperands(f func(Ref) Ref) {
	t.Term = f(t.Term)
	t.Description = f(t.Description)
}

// Stub instructions for currently-unimplemented constructs ////////////////////
//
// Set/show rules, contextual blocks, and module includes have IR placeholders
// so the analyser can produce well-formed modules; the evaluator panics on
// these (matching today's behaviour for the legacy tree IR).

// SetRule is a `set target(args) [if cond]` rule.
type SetRule struct {
	instr
	Target    Ref
	Args      []CallArg
	Condition Ref // NoRef when no `if` clause
}

func (s *SetRule) Operands() []Ref {
	out := []Ref{s.Target}
	for _, a := range s.Args {
		out = append(out, a.Value)
	}
	if s.Condition != NoRef {
		out = append(out, s.Condition)
	}
	return out
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

func (s *ShowRule) Operands() []Ref {
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

func (c *Contextual) Operands() []Ref               { return []Ref{c.Body} }
func (c *Contextual) RemapOperands(f func(Ref) Ref) { c.Body = f(c.Body) }

// ModuleInclude is `include "path"`.
type ModuleInclude struct {
	instr
	Source Ref
}

func (m *ModuleInclude) Operands() []Ref               { return []Ref{m.Source} }
func (m *ModuleInclude) RemapOperands(f func(Ref) Ref) { m.Source = f(m.Source) }
