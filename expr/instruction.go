package expr

import (
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// Instruction is a single SSA operation. Instructions live inside
// [BasicBlock.Insts] in program order; each produces a single SSA value
// (its [Result]) or [NoRef] for void operations.
type Instruction interface {
	Result() Ref
	Operands() []Ref
	Span() syntax.Span
	aInstruction()
}

// instr is the common base for instruction types; concrete instructions
// embed it to inherit Result/Span and to satisfy aInstruction().
type instr struct {
	result Ref
	span   syntax.Span
}

func (i *instr) Result() Ref       { return i.result }
func (i *instr) Span() syntax.Span { return i.span }
func (i *instr) aInstruction()     {}

// Terminator ends a [BasicBlock]. Every block has exactly one.
type Terminator interface {
	Successors() []BlockID
	Span() syntax.Span
	aTerminator()
}

type term struct {
	span syntax.Span
}

func (t *term) Span() syntax.Span { return t.span }
func (t *term) aTerminator()      {}

// Jump unconditionally transfers control to Target.
type Jump struct {
	term
	Target BlockID
}

func (t *Jump) Successors() []BlockID { return []BlockID{t.Target} }

// Branch is a two-way branch on a boolean SSA value.
type Branch struct {
	term
	Cond Ref
	Then BlockID
	Else BlockID
}

func (t *Branch) Successors() []BlockID { return []BlockID{t.Then, t.Else} }

// Return exits the enclosing [Function]. Value is the returned SSA value, or
// [NoRef] for a bare return (which yields none at runtime).
type Return struct {
	term
	Value Ref
}

func (t *Return) Successors() []BlockID { return nil }

// Unreachable marks a control-flow position that the analyser has proved
// cannot be reached (e.g. straight-line code after a Return). Evaluating it
// is a bug.
type Unreachable struct {
	term
}

func (t *Unreachable) Successors() []BlockID { return nil }

// Instructions ////////////////////////////////////////////////////////////////
//
// Concrete instruction types live here. Each embeds [inst] for span/result.

// Const materialises a constant runtime value.
type Const struct {
	instr
	Value value.Value
}

func (c *Const) Operands() []Ref { return nil }

// Unary applies a unary operator to a single operand.
type Unary struct {
	instr
	Op syntax.UnaryOp
	X  Ref
}

func (u *Unary) Operands() []Ref { return []Ref{u.X} }

// Binary applies a non-assignment binary operator. Assignment forms
// (`=`, `+=`, etc.) lower to a write of the LHS, not to Binary.
type Binary struct {
	instr
	Op syntax.BinaryOp
	L  Ref
	R  Ref
}

func (b *Binary) Operands() []Ref { return []Ref{b.L, b.R} }

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
// [inst.span] covers the whole `target.field` expression (used by errors
// that report against the access as a whole, e.g. "type T has no method").
type FieldRead struct {
	instr
	Target    Ref
	Field     name.Name
	FieldSpan syntax.Span
}

func (f *FieldRead) Operands() []Ref { return []Ref{f.Target} }

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

// Call invokes a function value. Args carry the positional, named, and
// spread arguments in source order; Blocks holds the Refs of any trailing
// content blocks (markup form: `f[...]`). AllowSetter is true when this call
// is the LHS of an assignment, signalling that the callee's runtime should
// surface a setter via FunctionCallContext.
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

// CallSet invokes a function whose return is an lvalue (the called
// function registers a setter via [value.FunctionCallContext.Setter]); the
// runtime then invokes the setter with the supplied new value. For
// compound assignments (`+=`, etc.) Op is the binary op (StripAssign
// applied); the setter receives `op(callResult, NewVal)`. For plain
// assignment Op == [syntax.Assign] and NewVal is written directly.
type CallSet struct {
	instr
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

// FieldWrite writes a value into a named field of a target (a dictionary
// entry, content field, etc.). For compound assignment, Op carries the
// stripped binary op so the runtime can compute `old op NewVal` before
// writing.
type FieldWrite struct {
	instr
	Target Ref
	Field  name.Name
	NewVal Ref
	Op     syntax.BinaryOp
}

func (f *FieldWrite) Operands() []Ref { return []Ref{f.Target, f.NewVal} }

// Extract reads a positional element from an indexed source (array or
// tuple-like value). Used to lower destructuring patterns.
type Extract struct {
	instr
	Source Ref
	Index  int
}

func (e *Extract) Operands() []Ref { return []Ref{e.Source} }

// LengthCheck asserts that Source has at least Want elements (or exactly
// Want if HasSink is false). Used to lower destructuring patterns; raises
// a runtime error if the assertion fails.
type LengthCheck struct {
	instr
	Source  Ref
	Want    int
	HasSink bool
}

func (l *LengthCheck) Result() Ref     { return NoRef }
func (l *LengthCheck) Operands() []Ref { return []Ref{l.Source} }

// IterOpen produces an opaque iterator over an array/dict/string.
type IterOpen struct {
	instr
	Iterable Ref
}

func (i *IterOpen) Operands() []Ref { return []Ref{i.Iterable} }

// IterHasNext returns a boolean indicating whether the iterator has more
// elements.
type IterHasNext struct {
	instr
	Iter Ref
}

func (i *IterHasNext) Operands() []Ref { return []Ref{i.Iter} }

// IterAdvance advances the iterator one step and yields the next element.
// Must only be evaluated when [IterHasNext] just returned true.
type IterAdvance struct {
	instr
	Iter Ref
}

func (i *IterAdvance) Operands() []Ref { return []Ref{i.Iter} }

// MakeClosure constructs a closure value pointing at the [Function] with
// the given FuncID, capturing one outer SSA value per entry in the
// function's [Function.Captures] list.
type MakeClosure struct {
	instr
	Func     FuncID
	Captures []Ref
}

func (m *MakeClosure) Operands() []Ref { return m.Captures }

// Markup instructions ////////////////////////////////////////////////////////

// ContentResult joins a sequence of value refs into a content sequence,
// applying the content joiner. Used at the end of markup bodies / content
// blocks to materialise the document/content fragment.
type ContentResult struct {
	instr
	Items []Ref
}

func (c *ContentResult) Operands() []Ref { return c.Items }

// RaiseError unconditionally raises a value error at eval time with the
// stored message. Used to surface deferred analyzer errors (e.g.
// "cannot mutate a temporary value") that should fire only if preceding
// instructions don't error first.
type RaiseError struct {
	instr
	Msg string
}

func (r *RaiseError) Operands() []Ref { return nil }

// AttachLabel binds a label name to the content produced by Content. It
// also registers the label in the evaluator's label set so subsequent
// [RefMarkup] instructions can resolve it. Re-labelling produces a runtime
// warning and discards the older label, matching legacy semantics.
type AttachLabel struct {
	instr
	Content Ref
	Label   name.Name
}

func (a *AttachLabel) Operands() []Ref { return []Ref{a.Content} }

// CodeJoin joins a sequence of value refs using the code-mode joiner. The
// joiner selects an output type based on the input types (strings concat,
// ints sum, content sequence, etc.). Used as the final value of code blocks.
type CodeJoin struct {
	instr
	Items     []Ref
	ItemSpans []syntax.Span // parallel to Items; used for per-item error reporting
}

func (c *CodeJoin) Operands() []Ref { return c.Items }

// LoopAccBegin produces an initial loop-accumulator value (an empty array
// internally; treated as opaque accumulator state). Each iteration's body
// result is added via [LoopAccAdd], and the final joined value is obtained
// via [LoopAccResult].
type LoopAccBegin struct {
	instr
}

func (*LoopAccBegin) Operands() []Ref { return nil }

// LoopAccAdd appends item to the accumulator and returns the same
// accumulator (mutated in place). The mutation is safe under SSA because
// the previous accumulator value is read exactly once per iteration before
// being overwritten by the WriteVar that follows.
type LoopAccAdd struct {
	instr
	Acc  Ref
	Item Ref
}

func (a *LoopAccAdd) Operands() []Ref { return []Ref{a.Acc, a.Item} }

// LoopAccResult joins the accumulated items into a single value using the
// code-mode joiner (same semantics as [CodeJoin]).
type LoopAccResult struct {
	instr
	Acc Ref
}

func (a *LoopAccResult) Operands() []Ref { return []Ref{a.Acc} }

// Heading represents a `= Title`-style heading at the given level.
type Heading struct {
	instr
	Level int
	Body  Ref
}

func (h *Heading) Operands() []Ref { return []Ref{h.Body} }

// Strong wraps content in bold formatting.
type Strong struct {
	instr
	Body Ref
}

func (s *Strong) Operands() []Ref { return []Ref{s.Body} }

// Emph wraps content in italic formatting.
type Emph struct {
	instr
	Body Ref
}

func (e *Emph) Operands() []Ref { return []Ref{e.Body} }

// Link is a hyperlink with a destination URL and body content.
type Link struct {
	instr
	Dest string
	Body Ref
}

func (l *Link) Operands() []Ref { return []Ref{l.Body} }

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

// ListItem represents a bulleted list item.
type ListItem struct {
	instr
	Body Ref
}

func (l *ListItem) Operands() []Ref { return []Ref{l.Body} }

// EnumItem represents a numbered list item. Number is -1 for "+" markers.
type EnumItem struct {
	instr
	Number int
	Body   Ref
}

func (e *EnumItem) Operands() []Ref { return []Ref{e.Body} }

// TermItem represents a definition-list entry: term / description.
type TermItem struct {
	instr
	Term        Ref
	Description Ref
}

func (t *TermItem) Operands() []Ref { return []Ref{t.Term, t.Description} }

// Stub instructions for currently-unimplemented constructs ///////////////////
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

// Contextual is a `context expr` construct.
type Contextual struct {
	instr
	Body Ref
}

func (c *Contextual) Operands() []Ref { return []Ref{c.Body} }

// ModuleInclude is `include "path"`.
type ModuleInclude struct {
	instr
	Source Ref
}

func (m *ModuleInclude) Operands() []Ref { return []Ref{m.Source} }
