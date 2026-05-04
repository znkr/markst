package builtin

import (
	"unique"

	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Label = &value.Function{
		Name: "label",
		Positional: []value.Param{
			{Name: "name", Type: types.SetOf(types.Str)},
		},
		F: labelImpl,
	}
)

func labelImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Str:
		if v == "" {
			return nil, value.ArgErrorPosf(0, "label name must not be empty")
		}
		return &value.Label{Name: unique.Make(string(v))}, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}
