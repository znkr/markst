package builtin

import (
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var FunctionWith = &value.Function{
	Name: "function.with",
	Positional: []value.Param{
		{Name: "self", Type: types.SetOf(types.Function)},
		{Name: "args", Type: types.SetOf(types.Arguments)},
	},
	Sink: new(1),
	F:    functionWithImpl,
}

func functionWithImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	// An element is a function too, but pre-binding drops the element identity:
	// the result is a plain function and cannot be used as a selector.
	var fn *value.Function
	switch f := args[0].(type) {
	case *value.Function:
		fn = f
	case *value.Element:
		fn = &f.Function
	}
	return fn.With(args[1].(*value.Arguments))
}
