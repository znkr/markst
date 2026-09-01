package expr

import (
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

// This file contains the SSA-form intermediate representation produced by
// the analyzer and consumed by the evaluator.
//
// Structure: a [Module] is a top-level [Function] plus all nested closures
// (also [Function]s). Each function is a CFG of [BasicBlock]s in SSA form,
// terminated by a [Terminator]. SSA values are referenced by [Ref], a
// function-local handle that indexes the runtime value table. Producers
// carry their own Ref: instructions via embedded [instr], block parameters
// via [BlockParam.Result], parameters via [Param.Ref], captures via
// [Function.Captures], and the closure self-reference via [Function.SelfRef].
//
// Join values are encoded as block parameters: each [BasicBlock] carries
// zero or more [BlockParam]s and the value flowing into each parameter is
// carried by the predecessor terminator's args (see [Jump.Args],
// [Branch.ThenArgs], [Branch.ElseArgs]). The evaluator binds the args
// directly when dispatching the terminator, so block entry has no extra
// per-edge bookkeeping.
//
// Trivial-param elimination is handled inside the builder: when a param's
// incoming args are all the same value, its Ref is recorded in a
// builder-local rename map. [Builder.Finalize] transitively closes the
// map, rewrites every Ref-typed operand through it, and physically splices
// the dead parameter slot from both the block's [BasicBlock.Params] and
// every incoming terminator's args list.
//
// The construction algorithm is Braun, Buchwald & Hack (2013), "Simple and
// Efficient Construction of Static Single Assignment Form".

// Module is a fully analyzed markst program: the document body plus every
// closure hoisted out into a top-level [Function].
type Module struct {
	Top       *Function     // the document body
	Functions []*Function   // closures, indexed by FuncID
	Constants []value.Value // module-wide constant pool, indexed by ModConstID

	// Origin is the source every [syntax.Span] in this module indexes into.
	// It travels with the module because a [Function] outlives the evaluation
	// that built it: a closure from one module can be called during another
	// module's evaluation, and its spans must still resolve against the source
	// it was written in.
	Origin syntax.Origin

	// ParseErrors are syntax errors lowered from the tree, collected so they
	// surface regardless of whether their containing function is ever executed
	// (parse errors are diagnostics on the source, not on a code path). Eval
	// records them up front; any that also fire at runtime are deduplicated.
	ParseErrors []*value.Error
}

// FuncID indexes [Module.Functions]. It is used in MakeClosure-style
// instructions instead of an inline pointer so that closures can refer to
// each other and so the printed form is well-defined.
type FuncID int32

// Function is a CFG in SSA form.
type Function struct {
	Name     string
	Params   []Param
	Captures []Ref         // one Ref per free variable; runtime values arrive at MakeClosure
	SelfRef  Ref           // the Ref bound to the running closure value; NoRef if unused
	Blocks   []*BasicBlock // Blocks[0] is the entry block
	RefSpans []syntax.Span // RefSpans[r] is the source span attached to the producer of r; len is the size of the per-frame value table
}

// NumRefs returns the size of the per-frame value table — one slot per
// allocated [Ref].
func (fn *Function) NumRefs() int { return len(fn.RefSpans) }

// Param describes a formal parameter of a [Function].
type Param struct {
	Name    name.Name
	Kind    ParamKind
	Default Ref // for Named: the SSA value of the default; NoRef when none
	Ref     Ref // the SSA Ref bound to this parameter inside the body
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
// by a [Terminator]. Block parameters at the head receive their values from
// the predecessor terminator's args ([Jump.Args], [Branch.ThenArgs],
// [Branch.ElseArgs]); the i-th param is bound from the i-th arg of the
// incoming edge.
//
// Preds lists one entry per incoming edge. The analyzer guarantees that
// every [Branch] has distinct Then and Else targets, so a predecessor
// appears in Preds at most once per (predecessor, slot) pair and edge
// slots are unambiguous given the predecessor's terminator type.
type BasicBlock struct {
	ID     BlockID
	Preds  []BlockID
	Params []*BlockParam
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

// IsLocal reports whether r is a function-local Ref.
func (r Ref) IsLocal() bool { return r >= 0 }

// ModConstID returns the [Module.Constants] index for a module-constant ref.
// Result is undefined if [Ref.IsModConst] returns false.
func (r Ref) ModConstID() int32 { return -2 - int32(r) }

// BlockParam is a value defined at the head of a [BasicBlock]. The SSA Ref
// it produces is bound at runtime from the corresponding arg slot of the
// predecessor terminator: [Jump.Args]`[i]`, [Branch.ThenArgs]`[i]`, or
// [Branch.ElseArgs]`[i]`, depending on which edge fired.
//
// The struct contains only IR-facing state. The [Var] each param joins
// during Braun construction and the trivial-elim "already processed" flag
// live in builder-side sidecar maps so they don't leak past [Builder.Finalize].
// Per-Ref source spans live in [Function.RefSpans] keyed by Result, so the
// param doesn't carry its own span.
type BlockParam struct {
	result Ref
	block  BlockID
}

func (p *BlockParam) Result() Ref     { return p.result }
func (p *BlockParam) Block() BlockID  { return p.block }
func (p *BlockParam) setResult(r Ref) { p.result = r }
