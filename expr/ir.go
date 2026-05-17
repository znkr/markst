package expr

import (
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// This file contains the SSA-form intermediate representation produced by
// the analyzer and consumed by the evaluator.
//
// Structure: a [Module] is a top-level [Function] plus all nested closures
// (also [Function]s). Each function is a CFG of [BasicBlock]s in SSA form,
// terminated by a [Terminator]. SSA values are referenced by [Ref], a
// function-local handle that indexes the runtime value table. Producers
// carry their own Ref: instructions via embedded [instr], phis via [Phi.Result],
// parameters via [Param.Ref], captures via [Function.CaptureRefs], and the
// closure self-reference via [Function.SelfRef].
//
// Phi nodes live on the block, separately from straight-line instructions,
// because they are conceptually evaluated as a parallel copy on block entry
// rather than sequentially.
//
// Trivial-phi elimination is handled inside the builder: when a phi is
// found to be trivial, its Ref is recorded in a builder-local rename map.
// [Builder.Finalize] transitively closes the map and then rewrites every
// Ref-typed operand in the IR through it, so by the time the function
// reaches the evaluator, no live Ref refers to a removed phi.
//
// The construction algorithm is Braun, Buchwald & Hack (2013), "Simple and
// Efficient Construction of Static Single Assignment Form".

// Module is a fully analyzed writst program: the document body plus every
// closure hoisted out into a top-level [Function].
type Module struct {
	Top       *Function     // the document body
	Functions []*Function   // closures, indexed by FuncID
	Constants []value.Value // module-wide constant pool, indexed by ModConstID
}

// FuncID indexes [Module.Functions]. It is used in MakeClosure-style
// instructions instead of an inline pointer so that closures can refer to
// each other and so the printed form is well-defined.
type FuncID int32

// Function is a CFG in SSA form.
type Function struct {
	Name        string
	Params      []Param
	Captures    []Var         // free-variable identifiers; values arrive at MakeClosure
	CaptureRefs []Ref         // parallel to Captures; the Ref bound at frame entry
	SelfRef     Ref           // the Ref bound to the running closure value; NoRef if unused
	Blocks      []*BasicBlock // Blocks[0] is the entry block
	NumRefs     int32         // size of the per-frame value table; one slot per allocated Ref
	RefSpans    []syntax.Span // RefSpans[r] is the source span attached to the producer of r
	Span        syntax.Span
}

// Param describes a formal parameter of a [Function].
type Param struct {
	Name    name.Name
	Kind    ParamKind
	Default Ref // for Named: the SSA value of the default; NoRef when none
	Ref     Ref // the SSA Ref bound to this parameter inside the body
	Span    syntax.Span
}

// ParamKind distinguishes positional, named, and sink (..rest) parameters.
type ParamKind uint8

const (
	ParamPositional ParamKind = iota
	ParamNamed
	ParamSink
)

// BlockID indexes [Function.Blocks].
type BlockID int32

// BasicBlock is a maximal straight-line sequence of instructions terminated
// by a [Terminator]. Phi nodes at the top are evaluated in parallel on
// block entry; their operands are looked up by predecessor block ID.
type BasicBlock struct {
	ID     BlockID
	Preds  []BlockID
	Phis   []*Phi
	Instrs []Instruction
	Term   Terminator
}

// Ref is an SSA value handle. Non-negative refs are function-local handles
// into the runtime value table (sized by [Function.NumRefs]); their source
// spans are in [Function.RefSpans] at the same index. [NoRef] is the
// missing-reference sentinel. Refs strictly below [NoRef] (i.e. r <= -2)
// encode a module-level constant: the [ModConstID] is `-2 - r`, indexing
// [Module.Constants]. Module-const refs are global to a [Module] — the same
// ref is valid in any function within that module.
type Ref int32

// NoRef is the sentinel value for a missing reference (e.g. an absent default
// argument or the value operand of a bare return).
const NoRef Ref = -1

// ModConstRef constructs a module-constant [Ref] from a pool index.
func ModConstRef(id int32) Ref { return Ref(-2 - id) }

// IsModConst reports whether r refers to a module-level constant.
func (r Ref) IsModConst() bool { return r < NoRef }

// ModConstID returns the [Module.Constants] index for a module-constant ref.
// Result is undefined if [Ref.IsModConst] returns false.
func (r Ref) ModConstID() int32 { return -2 - int32(r) }

// Phi is a phi node sitting at the head of a [BasicBlock]. Its [Result] is
// the SSA value produced; Operands map each predecessor block to the Ref
// flowing in along that edge.
type Phi struct {
	result   Ref
	block    BlockID
	operands []PhiOperand
	span     syntax.Span
}

func (p *Phi) Result() Ref            { return p.result }
func (p *Phi) Block() BlockID         { return p.block }
func (p *Phi) Operands() []PhiOperand { return p.operands }
func (p *Phi) Span() syntax.Span      { return p.span }

// RemapOperands substitutes every operand Ref through rename. Used by
// [Builder.Finalize] to inline trivial-phi removals.
func (p *Phi) RemapOperands(rename func(Ref) Ref) {
	for i := range p.operands {
		p.operands[i].Value = rename(p.operands[i].Value)
	}
}

// PhiOperand pairs a predecessor block with the SSA value that flows from it.
type PhiOperand struct {
	Pred  BlockID
	Value Ref
}
