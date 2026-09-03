package builtin

import (
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// State is `state(key, init: none)`, which names a piece of document state.
// `state.update(...)` on the handle produces content that changes the value
// where that content lands. The handle itself is not content but a value of its
// own type.
var State = &value.Function{
	Name: "state",
	Positional: []value.Param{
		{Name: "key", Type: types.SetOf(types.Str)},
		{Name: "init", Type: types.Any, Default: value.None{}},
	},
	F: stateImpl,
}

func stateImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.State{Key: string(args[0].(value.Str)), Init: args[1]}, nil
}

// StateUpdate implements state.update. Unlike the state() handle, its result is
// content: it updates the state's value when it reaches the document.
var StateUpdate = &value.Function{
	Name: "update",
	Positional: []value.Param{
		{Name: "self", Type: types.SetOf(types.State)},
		{Name: "update", Type: types.Any},
	},
	F: stateUpdateImpl,
}

func stateUpdateImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	self := args[0].(*value.State)
	return &value.StateUpdate{Key: self.Key, Update: args[1]}, nil
}
