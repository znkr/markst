package builtin

import (
	"fmt"

	"znkr.io/writst/value"
)

var FunctionWith = &value.Function{
	Name: "function.with",
	Bind: func(_ *value.Function, args *value.Arguments) (*value.Arguments, []int, error) {
		if len(args.Positional) < 1 {
			return nil, nil, fmt.Errorf("function with requires at least 1 positional argument")
		}
		if _, ok := args.Positional[0].(*value.Function); !ok {
			return nil, nil, fmt.Errorf("first argument to function with must be a function")
		}
		return args, nil, nil
	},
	F: functionWithImpl,
}

func functionWithImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	fn := args[0].(*value.Function)
	fnargs := &value.Arguments{Positional: args[1:], Named: named.Args}
	return fn.With(fnargs)
}
