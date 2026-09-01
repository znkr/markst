package expr

import (
	"testing"

	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/value"
)

func TestFormatFunctionIfElseJoin(t *testing.T) {
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
	v2 := b.Const(span, value.Int(20))
	b.WriteVar(x, elseBlk, v2)
	b.Jump(span, join)

	b.SealBlock(thenBlk)
	b.SealBlock(elseBlk)
	b.SealBlock(join)

	b.SetBlock(join)
	r := b.ReadVar(x, join)
	b.Return(span, r)

	want := `fn $top:
  b0:
    branch c1, b1, b2
  b1:
    jump b3(c2)
  b2:
    jump b3(c3)
  b3(v0):
    return v0
`
	got := FormatFunction("$top", b.Function())
	if got != want {
		t.Errorf("FormatFunction mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
