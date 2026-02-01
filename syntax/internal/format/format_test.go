package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/writst/syntax"
)

func TestFormat(t *testing.T) {
	errNode := syntax.Error("something went wrong", syntax.Span{Start: 12, End: 19}, "literal")
	errNode.AsError().AddHint("try again")

	node := syntax.Inner(syntax.KindMarkup, syntax.Span{Start: 0, End: 19}, []syntax.Node{
		syntax.Leaf(syntax.KindText, syntax.Span{Start: 0, End: 5}, "hello"),
		syntax.Inner(syntax.KindStrong, syntax.Span{Start: 5, End: 12}, []syntax.Node{
			syntax.Leaf(syntax.KindStar, syntax.Span{Start: 5, End: 6}, "*"),
			syntax.Leaf(syntax.KindText, syntax.Span{Start: 6, End: 11}, "world"),
			syntax.Leaf(syntax.KindStar, syntax.Span{Start: 11, End: 12}, "*"),
		}),
		errNode,
	})

	expected := `Markup
  Text: "hello"
  Strong
    Star: "*"
    Text: "world"
    Star: "*"
  Error: "literal"
    Msg: "something went wrong"
    Hint: "try again"
`

	got := Format(node)
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}
