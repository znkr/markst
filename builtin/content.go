package builtin

import (
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var Emph = &value.Function{
	Name: "emph",
	Positional: []value.Param{
		{Name: "body", Type: types.SetOf(types.Content)},
	},
	F: emphImpl,
}

func emphImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	content := args[0].(value.Content)
	return &value.Emph{Body: content}, nil
}
