package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/writst/syntax"
)

func TestFormat(t *testing.T) {
	errNode := syntax.Error("something went wrong", "literal")
	errNode.AsError().AddHint("try again")

	node := syntax.Inner(syntax.KindMarkup, []syntax.Node{
		syntax.Leaf(syntax.KindText, "hello"),
		syntax.Inner(syntax.KindStrong, []syntax.Node{
			syntax.Leaf(syntax.KindStar, "*"),
			syntax.Leaf(syntax.KindText, "world"),
			syntax.Leaf(syntax.KindStar, "*"),
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
