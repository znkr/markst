package value

import (
	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// Custom is a content element the host defines. Markst does not know what it
// means: it carries [Custom.Value] through the pipeline untouched, and the
// program presenting the document recognizes it again and renders it.
//
//	&value.Function{
//		Name:       "include-diff",
//		Positional: []value.Param{{Name: "path", Type: types.SetOf(types.Str)}},
//		F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
//			edits, err := computeDiff(string(args[0].(value.Str)))
//			if err != nil {
//				return nil, value.ArgErrorPosf(0, "%v", err)
//			}
//			return &value.Custom{Elem: "include-diff", Block: true, Value: edits}, nil
//		},
//	}
type Custom struct {
	// Elem names the element — in diagnostics ("element include-diff has no
	// method `x`") and in the document's textual form. By convention it is the
	// name of the binding that constructs it.
	Elem string

	// Block reports whether the element is block-level; see [Content.IsBlock].
	Block bool

	// Value is the host's payload. Markst never inspects it, never copies it,
	// and hands back the identical value it was given.
	Value any

	Label *Label
}

func (*Custom) aValue()          {}
func (*Custom) aContent()        {}
func (*Custom) Type() types.Type { return types.Content }

// Equal reports whether other is this same element. The payload is opaque, so
// markst cannot compare two of them; identity is the only answer it can give.
func (n *Custom) Equal(other Value) bool {
	o, ok := other.(*Custom)
	return ok && n == o
}

func (n *Custom) Format(f *formatter.Formatter) {
	f.FuncCall("custom", []formatter.Arg{formatter.PositionalArg(n.Elem)})
	if n.Label != nil {
		f.Str(" <")
		f.Str(n.Label.Name.String())
		f.Str(">")
	}
}

func (n *Custom) Field(name.Name) Value   { return nil }
func (n *Custom) HasField(name.Name) bool { return false }
func (n *Custom) Fields() *Dict           { return new(Dict) }
func (n *Custom) Name() string            { return n.Elem }
func (n *Custom) IsBlock() bool           { return n.Block }
func (n *Custom) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}
func (n *Custom) GetLabel() *Label {
	return n.Label
}
