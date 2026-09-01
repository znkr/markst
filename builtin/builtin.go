package builtin

import (
	"znkr.io/markst/internal/lorem"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Repr = &value.Function{
		Name:       "repr",
		Positional: []value.Param{{Type: types.Any}},
		F:          reprImpl,
	}

	Type = &value.Function{
		Name:       "type",
		Positional: []value.Param{{Type: types.Any}},
		// F is set in init() to avoid an initialization cycle.
	}

	Lorem = &value.Function{
		Name: "lorem",
		Positional: []value.Param{
			{Name: "words", Type: types.SetOf(types.Int)},
		},
		F: loremImpl,
	}
)

func init() {
	Type.F = typeImpl
}

func reprImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	return value.Str(value.Repr(args[0])), nil
}

func typeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return reflectedTypes[args[0].Type()], nil
}

func loremImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	words := int(args[0].(value.Int))
	lg := lorem.NewGenerator()
	return value.Str(lg.Words(words)), nil
}
