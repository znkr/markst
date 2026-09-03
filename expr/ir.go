// Package expr defines the SSA intermediate representation: what the analyzer
// produces and the evaluator runs.
//
// A [Module] is the document body as a [Function], plus one [Function] per
// closure. A function is a control-flow graph of [BasicBlock]s, each ending in
// a [Terminator]. SSA values are [Ref]s, handles into the value table a call
// runs against.
//
// Where two paths join, the joined value is a [BlockParam] rather than a phi
// node: the block declares the parameter, and each predecessor's terminator
// carries the value flowing into it ([Jump.Args], [Branch.ThenArgs],
// [Branch.ElseArgs]).
//
// The analyzer builds a module by driving a [Builder], following Braun,
// Buchwald & Hack (2013), "Simple and Efficient Construction of Static Single
// Assignment Form". [FormatModule] dumps one as text.
package expr

import (
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

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

// FuncID indexes [Module.Functions]. Instructions name a function by ID
// rather than by pointer, so closures can refer to one another and so a
// printed module reads the same every time.
type FuncID int32

// Function is one document body or closure, as a CFG in SSA form.
type Function struct {
	Name     string
	Params   []Param
	Captures []Ref         // one Ref per free variable; values arrive at MakeClosure
	SelfRef  Ref           // the Ref bound to the running closure; NoRef if unused
	Blocks   []*BasicBlock // Blocks[0] is the entry block
	RefSpans []syntax.Span // RefSpans[r] is the source r was produced from
}

// NumRefs returns how many slots a call to fn needs in its value table, one
// per [Ref].
func (fn *Function) NumRefs() int { return len(fn.RefSpans) }

// Param is one formal parameter of a [Function].
type Param struct {
	Name    name.Name
	Kind    ParamKind
	Default Ref // for a named param, its default; NoRef when it has none
	Ref     Ref // the Ref this parameter is bound to inside the body
}

// ParamKind says whether a [Param] is positional, named, or a `..rest` sink.
type ParamKind uint8

const (
	ParamPositional ParamKind = iota
	ParamNamed
	ParamSink
)

// BlockID indexes [Function.Blocks].
type BlockID int32

// BasicBlock is a run of instructions with no branches in or out except at the
// ends, finished by a [Terminator].
//
// Params are bound on entry from the args the incoming edge carried, one for
// one in order. Preds lists one entry per incoming edge; since a [Branch]
// always has two different targets, which of its two arg lists an edge used is
// never in doubt.
type BasicBlock struct {
	ID     BlockID
	Preds  []BlockID
	Params []*BlockParam
	Instrs []Instruction
	Term   Terminator
}

// Ref names an SSA value. Three ranges of Ref mean three different things:
//
//   - r >= 0 is a slot in the value table of the running call, and its source
//     is [Function.RefSpans] at the same index. Such a Ref only means anything
//     within the function that allocated it.
//   - r == [NoRef] is "no value".
//   - r <= -2 is a constant in [Module.Constants], at index [Ref.ModConstID].
//     These are valid in every function of the module.
type Ref int32

// NoRef stands for a reference that is not there: an argument with no default,
// or the value of a bare return.
const NoRef Ref = -1

// ModConstRef returns the [Ref] for the module constant at index id.
func ModConstRef(id int32) Ref { return Ref(-2 - id) }

// IsModConst reports whether r refers to a module-level constant.
func (r Ref) IsModConst() bool { return r < NoRef }

// IsLocal reports whether r is a function-local Ref.
func (r Ref) IsLocal() bool { return r >= 0 }

// ModConstID returns r's index into [Module.Constants]. The result is
// meaningless unless [Ref.IsModConst] reports true.
func (r Ref) ModConstID() int32 { return -2 - int32(r) }

// BlockParam is a value declared at the head of a [BasicBlock], standing in
// for a phi node. It is bound on entry from the arg in the matching slot of
// whichever predecessor's terminator branched here.
type BlockParam struct {
	result Ref
	block  BlockID
}

// Result returns the SSA value this parameter produces.
func (p *BlockParam) Result() Ref { return p.result }

// Block returns the block this parameter belongs to.
func (p *BlockParam) Block() BlockID { return p.block }

func (p *BlockParam) setResult(r Ref) { p.result = r }
