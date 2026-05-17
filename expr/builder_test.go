package expr

import (
	"testing"

	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

func TestBuilderLinear(t *testing.T) {
	// Trivial: write a var in the entry block, read it back. No phi expected.
	b := NewModuleBuilder().NewBuilder()
	x := Var{Name: name.Make("x"), Version: 1}
	v0 := b.Const(syntax.Span{}, value.Int(1))
	b.WriteVar(x, b.CurrentBlock(), v0)

	got := b.ReadVar(x, b.CurrentBlock())
	if got != v0 {
		t.Fatalf("ReadVar after WriteVar in same block: got %d, want %d", got, v0)
	}
	if phis := b.Function().Blocks[0].Phis; len(phis) != 0 {
		t.Fatalf("entry block should have no phis, got %d", len(phis))
	}
}

func TestBuilderIfElseJoin(t *testing.T) {
	// Build:
	//   b0:  v0 = const 1
	//        write x = v0
	//        branch cond, b1, b2
	//   b1:  v1 = const 10
	//        write x = v1
	//        jump b3
	//   b2:  v2 = const 20
	//        write x = v2
	//        jump b3
	//   b3:  read x  // expect phi [b1 v1, b2 v2]
	b := NewModuleBuilder().NewBuilder()
	x := Var{Name: name.Make("x"), Version: 1}
	span := syntax.Span{}

	entry := b.CurrentBlock()
	v0 := b.Const(span, value.Int(1))
	b.WriteVar(x, entry, v0)
	cond := b.Const(span, value.Bool(true))

	thenBlk := b.NewBlock()
	elseBlk := b.NewBlock()
	join := b.NewBlock()
	b.Branch(span, cond, thenBlk, elseBlk)

	b.SetBlock(thenBlk)
	v1 := b.Const(span, value.Int(10))
	b.WriteVar(x, thenBlk, v1)
	b.Jump(span, join)

	b.SetBlock(elseBlk)
	v2 := b.Const(span, value.Int(20))
	b.WriteVar(x, elseBlk, v2)
	b.Jump(span, join)

	// Branch-join blocks are sealed as soon as both predecessors' terminators
	// are in (i.e. right now).
	b.SealBlock(thenBlk)
	b.SealBlock(elseBlk)
	b.SealBlock(join)

	b.SetBlock(join)
	got := b.ReadVar(x, join)

	phis := b.Function().Blocks[join].Phis
	if len(phis) != 1 {
		t.Fatalf("join block should have 1 phi, got %d", len(phis))
	}
	if phis[0].Result() != got {
		t.Fatalf("ReadVar returned %d, but phi result is %d", got, phis[0].Result())
	}
	ops := phis[0].Operands()
	if len(ops) != 2 {
		t.Fatalf("phi should have 2 operands, got %d", len(ops))
	}
	// Check operands by predecessor
	gotByPred := map[BlockID]Ref{}
	for _, op := range ops {
		gotByPred[op.Pred] = op.Value
	}
	if gotByPred[thenBlk] != v1 {
		t.Errorf("phi operand for then-block: got %d, want %d", gotByPred[thenBlk], v1)
	}
	if gotByPred[elseBlk] != v2 {
		t.Errorf("phi operand for else-block: got %d, want %d", gotByPred[elseBlk], v2)
	}
}

func TestBuilderIfOneArmWrites(t *testing.T) {
	// Only one arm writes x; the other arm's operand should resolve to the
	// entry's definition.
	//   b0:  v0 = const 1
	//        write x = v0
	//        branch cond, b1, b2
	//   b1:  v1 = const 10
	//        write x = v1
	//        jump b3
	//   b2:  jump b3   // no write to x
	//   b3:  read x  // expect phi [b1 v1, b2 v0]
	b := NewModuleBuilder().NewBuilder()
	x := Var{Name: name.Make("x"), Version: 1}
	span := syntax.Span{}

	v0 := b.Const(span, value.Int(1))
	b.WriteVar(x, b.CurrentBlock(), v0)
	cond := b.Const(span, value.Bool(true))

	thenBlk := b.NewBlock()
	elseBlk := b.NewBlock()
	join := b.NewBlock()
	b.Branch(span, cond, thenBlk, elseBlk)

	b.SetBlock(thenBlk)
	v1 := b.Const(span, value.Int(10))
	b.WriteVar(x, thenBlk, v1)
	b.Jump(span, join)

	b.SetBlock(elseBlk)
	b.Jump(span, join)

	b.SealBlock(thenBlk)
	b.SealBlock(elseBlk)
	b.SealBlock(join)

	b.SetBlock(join)
	got := b.ReadVar(x, join)

	phis := b.Function().Blocks[join].Phis
	if len(phis) != 1 {
		t.Fatalf("join should have exactly 1 phi, got %d", len(phis))
	}
	if phis[0].Result() != got {
		t.Fatalf("ReadVar returned %d, phi result %d", got, phis[0].Result())
	}
	byPred := map[BlockID]Ref{}
	for _, op := range phis[0].Operands() {
		byPred[op.Pred] = op.Value
	}
	if byPred[thenBlk] != v1 {
		t.Errorf("then-arm operand: got %d, want %d", byPred[thenBlk], v1)
	}
	if byPred[elseBlk] != v0 {
		t.Errorf("else-arm operand should be entry-def %d, got %d", v0, byPred[elseBlk])
	}
}

func TestBuilderLoopHeaderPhi(t *testing.T) {
	// Loop with back-edge — exercises unsealed phi insertion.
	//   b0:  v0 = const 0
	//        write i = v0
	//        jump b1
	//   b1 (header, unsealed):
	//        read i  // -> incomplete phi
	//        branch cond, b2, b3
	//   b2 (body):
	//        v1 = const 1
	//        write i = v1
	//        jump b1  // back-edge
	//   b3 (exit)
	// SealBlock(b1) after back-edge is added.
	// Expect: header phi with operands [b0 v0, b2 v1].
	b := NewModuleBuilder().NewBuilder()
	i := Var{Name: name.Make("i"), Version: 1}
	span := syntax.Span{}

	entry := b.CurrentBlock()
	v0 := b.Const(span, value.Int(0))
	b.WriteVar(i, entry, v0)

	header := b.NewBlock()
	body := b.NewBlock()
	exit := b.NewBlock()
	b.Jump(span, header)

	b.SetBlock(header)
	headerVal := b.ReadVar(i, header) // creates incomplete phi
	cond := b.Const(span, value.Bool(true))
	b.Branch(span, cond, body, exit)

	b.SetBlock(body)
	v1 := b.Const(span, value.Int(1))
	b.WriteVar(i, body, v1)
	b.Jump(span, header) // back-edge

	// Now seal the header (predecessor list is complete).
	b.SealBlock(header)
	b.SealBlock(body) // body's only predecessor is the header
	b.SealBlock(exit)

	phis := b.Function().Blocks[header].Phis
	if len(phis) != 1 {
		t.Fatalf("header should have 1 phi, got %d", len(phis))
	}
	phi := phis[0]
	if phi.Result() != headerVal {
		t.Fatalf("headerVal %d, phi result %d", headerVal, phi.Result())
	}
	byPred := map[BlockID]Ref{}
	for _, op := range phi.Operands() {
		byPred[op.Pred] = op.Value
	}
	if byPred[entry] != v0 {
		t.Errorf("loop entry operand: got %d, want %d", byPred[entry], v0)
	}
	if byPred[body] != v1 {
		t.Errorf("loop body operand: got %d, want %d", byPred[body], v1)
	}
}

func TestBuilderTrivialPhiElim(t *testing.T) {
	// Both arms write the SAME ref to x. The phi the builder inserts at the
	// join must be trivial and get eliminated.
	//   b0: v0 = const 7
	//       write x = v0
	//       branch cond, b1, b2
	//   b1: jump b3   // x not rewritten (still v0)
	//   b2: jump b3   // x not rewritten (still v0)
	//   b3: read x  -> expect v0 directly, no phi
	b := NewModuleBuilder().NewBuilder()
	x := Var{Name: name.Make("x"), Version: 1}
	span := syntax.Span{}

	v0 := b.Const(span, value.Int(7))
	b.WriteVar(x, b.CurrentBlock(), v0)
	cond := b.Const(span, value.Bool(true))

	then := b.NewBlock()
	els := b.NewBlock()
	join := b.NewBlock()
	b.Branch(span, cond, then, els)

	b.SetBlock(then)
	b.Jump(span, join)
	b.SetBlock(els)
	b.Jump(span, join)

	b.SealBlock(then)
	b.SealBlock(els)
	b.SealBlock(join)

	b.SetBlock(join)
	got := b.ReadVar(x, join)
	if got != v0 {
		t.Fatalf("trivial-phi elimination failed: ReadVar returned %d, want %d", got, v0)
	}
	if phis := b.Function().Blocks[join].Phis; len(phis) != 0 {
		t.Fatalf("join should have no phis after trivial elimination, got %d", len(phis))
	}
}
