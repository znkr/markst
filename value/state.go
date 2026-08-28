package value

import (
	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// State is a document state handle created by the state() function. It carries
// a key that identifies the state across the document and an initial value.
type State struct {
	Key  string
	Init Value
}

func (*State) aValue()          {}
func (*State) Type() types.Type { return types.State }

func (n *State) Equal(other Value) bool {
	o, ok := other.(*State)
	return ok && n.Key == o.Key && n.Init.Equal(o.Init)
}

func (n *State) Format(f *formatter.Formatter) {
	args := []formatter.Arg{formatter.PositionalArg(n.Key)}
	if _, isNone := n.Init.(None); !isNone {
		args = append(args, formatter.PositionalArg(n.Init))
	}
	f.FuncCall("state", args)
}

// StateUpdate is content produced by state.update. It represents a pending
// change to a state's value and must reach the document to take effect.
type StateUpdate struct {
	Key    string
	Update Value
	Label  *Label
}

func (*StateUpdate) aValue()          {}
func (*StateUpdate) aContent()        {}
func (*StateUpdate) Type() types.Type { return types.Content }

func (n *StateUpdate) Equal(other Value) bool {
	o, ok := other.(*StateUpdate)
	return ok && n.Key == o.Key && n.Update.Equal(o.Update) && labelEqual(n.Label, o.Label)
}

func (n *StateUpdate) Format(f *formatter.Formatter) {
	f.FuncCall("state", []formatter.Arg{formatter.PositionalArg(n.Key)})
	f.Str(".update(")
	n.Update.Format(f)
	f.Str(")")
}

func (n *StateUpdate) Field(name.Name) Value   { return nil }
func (n *StateUpdate) HasField(name.Name) bool { return false }
func (n *StateUpdate) Fields() *Dict           { return new(Dict) }
func (n *StateUpdate) Name() string            { return "state" }
func (n *StateUpdate) IsBlock() bool           { return false }
func (n *StateUpdate) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}
func (n *StateUpdate) GetLabel() *Label {
	return n.Label
}
