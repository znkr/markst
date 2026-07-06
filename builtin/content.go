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

// Table constructs a table from its content children.
var Table = &value.Function{
	Name: "table",
	Positional: []value.Param{
		{Name: "children", Type: types.SetOf(types.Content)},
	},
	Sink: new(0),
	F:    tableImpl,
}

func tableImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	children := make([]value.Content, len(sink.Positional))
	for i, v := range sink.Positional {
		c, err := value.ToContent(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		children[i] = c
	}
	return &value.Table{Children: children}, nil
}
