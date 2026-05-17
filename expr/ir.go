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
// function-local handle into [Function.Defs]; each [Def] records how to
// materialize the value (constant, parameter, capture, phi result, or
// instruction result).
//
// Phi nodes live on the block, separately from straight-line instructions,
// because they are conceptually evaluated as a parallel copy on block entry
// rather than sequentially.
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
	Name     string
	Params   []Param
	Captures []Var         // free-variable identifiers; values arrive at MakeClosure
	Blocks   []*BasicBlock // Blocks[0] is the entry block
	Defs     []Def         // Defs[ref] describes how to materialize that SSA value
	Span     syntax.Span
}

// Param describes a formal parameter of a [Function].
type Param struct {
	Name    name.Name
	Kind    ParamKind
	Default Ref // for Named: the SSA value of the default; NoRef when none
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

// Ref is an SSA value handle. Non-negative refs index a function's [Defs]
// slice (function-local). [NoRef] is the missing-reference sentinel. Refs
// strictly below [NoRef] (i.e. r <= -2) encode a module-level constant: the
// [ModConstID] is `-2 - r`, indexing [Module.Constants]. Module-const refs are
// global to a [Module] — the same ref is valid in any function within that
// module — and never appear in [Function.Defs].
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

// Def records how to materialize the SSA value with a given [Ref]. There is
// one Def per Ref; storing them in a flat slice makes the runtime value
// table a simple `[]value.Value` indexed by Ref.
type Def interface {
	Block() BlockID
	Span() syntax.Span
	aDef()
}

// def is the common base embedded by all concrete Def types.
type def struct {
	block BlockID
	span  syntax.Span
}

func (d *def) Block() BlockID    { return d.block }
func (d *def) Span() syntax.Span { return d.span }
func (d *def) aDef()             {}

// DefInstr is a Def produced by an [Instruction].
type DefInstr struct {
	def
	Instr Instruction
}

// DefPhi is a Def produced by a [Phi] node.
type DefPhi struct {
	def
	Phi *Phi
}

// DefParam is a Def for a function parameter; Idx selects [Function.Params].
type DefParam struct {
	def
	Idx int
}

// DefCapture is a Def for a captured variable; Idx selects [Function.Captures].
type DefCapture struct {
	def
	Idx int
}

// DefSelf is a Def for the currently-executing closure value. It lets a
// closure body refer to itself without an explicit capture, enabling direct
// recursion. The runtime materializes it from the call-time `self` slot.
type DefSelf struct {
	def
}

// DefRedirect is a Def that aliases another Ref; reads should follow Redirect.
type DefRedirect struct {
	def
	Redirect Ref
}

// Resolve follows [DefRedirect] chains and returns the underlying Ref.
// Module-constant refs and [NoRef] are returned unchanged.
func (f *Function) Resolve(r Ref) Ref {
	for r >= 0 && int(r) < len(f.Defs) {
		d, ok := f.Defs[r].(*DefRedirect)
		if !ok {
			break
		}
		r = d.Redirect
	}
	return r
}

// Phi is a phi node sitting at the head of a [BasicBlock]. Its [Result] is
// the SSA value produced; Operands map each predecessor block to the Ref
// flowing in along that edge.
type Phi struct {
	result   Ref
	operands []PhiOperand
	span     syntax.Span
}

func (p *Phi) Result() Ref            { return p.result }
func (p *Phi) Operands() []PhiOperand { return p.operands }
func (p *Phi) Span() syntax.Span      { return p.span }

// PhiOperand pairs a predecessor block with the SSA value that flows from it.
type PhiOperand struct {
	Pred  BlockID
	Value Ref
}
