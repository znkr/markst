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
	fn := args[0].(*value.Function)
	return fn.With(args[1].(*value.Arguments))
}
