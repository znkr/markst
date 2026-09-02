package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/markst/syntax"
)

func TestFormat(t *testing.T) {
	node := syntax.NewInner(syntax.KindMarkup, []syntax.Node{
		syntax.NewLeaf(syntax.KindText, syntax.Span{Start: 0, End: 5}, []byte("hello")),
		syntax.NewInner(syntax.KindStrong, []syntax.Node{
			syntax.NewLeaf(syntax.KindStar, syntax.Span{Start: 5, End: 6}, []byte("*")),
			syntax.NewLeaf(syntax.KindText, syntax.Span{Start: 6, End: 11}, []byte("world")),
			syntax.NewLeaf(syntax.KindStar, syntax.Span{Start: 11, End: 12}, []byte("*")),
		}),
		syntax.NewError(
			syntax.Span{Start: 12, End: 19},
			"something went wrong",
			[]byte("literal"),
			"try again",
		),
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
