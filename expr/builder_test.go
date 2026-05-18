package expr

import (
	"testing"

	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

func TestBuilderLinear(t *testing.T) {
	// Trivial: write a var in the entry block, read it back. No block params
	// expected.
	b := NewModuleBuilder().NewBuilder()
	x := Var{Name: name.Make("x"), Version: 1}
	v0 := b.Const(syntax.Span{}, value.Int(1))
	b.WriteVar(x, b.CurrentBlock(), v0)

	got := b.ReadVar(x, b.CurrentBlock())
	if got != v0 {
		t.Fatalf("ReadVar after WriteVar in same block: got %d, want %d", got, v0)
	}
	if params := b.Function().Blocks[0].Params; len(params) != 0 {
		t.Fatalf("entry block should have no params, got %d", len(params))
	}
}

func TestBuilderIfElseJoin(t *testing.T) {
	// Build:
	//   b0:  v0 = const 1
	//        write x = v0
	//        branch cond, b1, b2
	//   b1:  v1 = const 10
	//        write x = v1
	//        jump b3(v1)
	//   b2:  v2 = const 20
	//        write x = v2
	//        jump b3(v2)
	//   b3(v3): read x  // v3 is the block param
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

	params := b.Function().Blocks[join].Params
	if len(params) != 1 {
		t.Fatalf("join block should have 1 param, got %d", len(params))
	}
	if params[0].Result() != got {
		t.Fatalf("ReadVar returned %d, but param result is %d", got, params[0].Result())
	}
	// Check the arg flowing in from each pred terminator.
	thenJump, ok := b.Function().Blocks[thenBlk].Term.(*Jump)
	if !ok {
		t.Fatalf("then-block terminator should be Jump")
	}
	if len(thenJump.Args) != 1 || thenJump.Args[0] != v1 {
		t.Errorf("then-block Jump.Args: got %v, want [%d]", thenJump.Args, v1)
	}
	elseJump, ok := b.Function().Blocks[elseBlk].Term.(*Jump)
	if !ok {
		t.Fatalf("else-block terminator should be Jump")
	}
	if len(elseJump.Args) != 1 || elseJump.Args[0] != v2 {
		t.Errorf("else-block Jump.Args: got %v, want [%d]", elseJump.Args, v2)
	}
}

func TestBuilderIfOneArmWrites(t *testing.T) {
	// Only one arm writes x; the other arm's incoming arg should resolve to
	// the entry's definition.
	//   b0:  v0 = const 1
	//        write x = v0
	//        branch cond, b1, b2
	//   b1:  v1 = const 10
	//        write x = v1
	//        jump b3(v1)
	//   b2:  jump b3(v0)   // no write to x; passes the entry value
	//   b3(v_join): read x
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

	params := b.Function().Blocks[join].Params
	if len(params) != 1 {
		t.Fatalf("join should have exactly 1 param, got %d", len(params))
	}
	if params[0].Result() != got {
		t.Fatalf("ReadVar returned %d, param result %d", got, params[0].Result())
	}
	thenJump := b.Function().Blocks[thenBlk].Term.(*Jump)
	if len(thenJump.Args) != 1 || thenJump.Args[0] != v1 {
		t.Errorf("then-arm arg: got %v, want [%d]", thenJump.Args, v1)
	}
	elseJump := b.Function().Blocks[elseBlk].Term.(*Jump)
	if len(elseJump.Args) != 1 || elseJump.Args[0] != v0 {
		t.Errorf("else-arm arg should be entry-def %d, got %v", v0, elseJump.Args)
	}
}

func TestBuilderLoopHeaderPhi(t *testing.T) {
	// Loop with back-edge — exercises unsealed param insertion.
	//   b0:  v0 = const 0
	//        write i = v0
	//        jump b1(v0)
	//   b1(v_i, unsealed at construction):
	//        read i  // -> param v_i created
	//        branch cond, b2, b3
	//   b2 (body):
	//        v1 = const 1
	//        write i = v1
	//        jump b1(v1)  // back-edge
	//   b3 (exit)
	// SealBlock(b1) after back-edge is added.
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
	headerVal := b.ReadVar(i, header) // creates incomplete param
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

	params := b.Function().Blocks[header].Params
	if len(params) != 1 {
		t.Fatalf("header should have 1 param, got %d", len(params))
	}
	p := params[0]
	if p.Result() != headerVal {
		t.Fatalf("headerVal %d, param result %d", headerVal, p.Result())
	}
	entryJump := b.Function().Blocks[entry].Term.(*Jump)
	if len(entryJump.Args) != 1 || entryJump.Args[0] != v0 {
		t.Errorf("loop-entry arg: got %v, want [%d]", entryJump.Args, v0)
	}
	bodyJump := b.Function().Blocks[body].Term.(*Jump)
	if len(bodyJump.Args) != 1 || bodyJump.Args[0] != v1 {
		t.Errorf("loop-body back-edge arg: got %v, want [%d]", bodyJump.Args, v1)
	}
}

func TestBuilderTrivialPhiElim(t *testing.T) {
	// Both arms write the SAME ref to x. The param the builder inserts at the
	// join must be trivial and get flagged for elimination; ReadVar returns
	// the underlying value directly.
	//   b0: v0 = const 7
	//       write x = v0
	//       branch cond, b1, b2
	//   b1: jump b3(v0)   // x not rewritten (still v0)
	//   b2: jump b3(v0)   // x not rewritten (still v0)
	//   b3: read x  -> expect v0 directly, param marked dead
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
		t.Fatalf("trivial-param elimination failed: ReadVar returned %d, want %d", got, v0)
	}
	// The param has been recorded for redirect but not yet spliced (splicing
	// happens in Finalize). Verify the redirect entry.
	params := b.Function().Blocks[join].Params
	if len(params) != 1 {
		t.Fatalf("join should have exactly 1 (pending-redirect) param pre-Finalize, got %d", len(params))
	}
	if t2, ok := b.redirects[params[0].Result()]; !ok || t2 != v0 {
		t.Errorf("trivial param should redirect to %d, got %v (ok=%v)", v0, t2, ok)
	}
	// After Finalize, the dead param and its arg slots are spliced out.
	b.Finalize()
	if params := b.Function().Blocks[join].Params; len(params) != 0 {
		t.Errorf("join should have no params after Finalize, got %d", len(params))
	}
}

func TestBuilderCallPurityDCE(t *testing.T) {
	// A Call whose Callee is a ModConst *value.Function with Impure=false is
	// droppable when its result has no uses; an Impure callee is kept.
	pure := &value.Function{Name: "pure_test"}
	impure := &value.Function{Name: "impure_test", Impure: true}

	mb := NewModuleBuilder()
	b := mb.NewBuilder()
	span := syntax.Span{}

	pureCallee := Callee{Ref: b.Const(span, pure)}
	impureCallee := Callee{Ref: b.Const(span, impure)}
	b.Call(span, pureCallee, nil, nil, false)              // result unused
	keptRef := b.Call(span, impureCallee, nil, nil, false) // result also unused, but impure
	// Pin the impure call's result to confirm it's not getting dropped by the
	// (unrelated) regular DCE path — the impurity check is what should save it.
	_ = keptRef
	b.Return(span, NoRef)

	b.Finalize()

	var got []string
	for _, inst := range b.Function().Blocks[0].Instrs {
		if c, ok := inst.(*Call); ok {
			callee := mb.mod.Constants[c.Callee.Ref.ModConstID()].(*value.Function)
			got = append(got, callee.Name)
		}
	}
	if len(got) != 1 || got[0] != "impure_test" {
		t.Fatalf("after Finalize: got Calls=%v, want only [impure_test]", got)
	}
}
