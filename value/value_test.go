package value

import (
	"testing"

	"znkr.io/writst/types"
)

func TestFunctionWith(t *testing.T) {
	nopF := func(_ *FunctionCallContext, _ []Value, _ NamedArgsWithDefaults) (Value, error) {
		return None{}, nil
	}

	rangeFunc := &Function{
		Name: "range",
		Positional: []Param{
			{Type: types.SetOf(types.Int), Default: Int(0)},
			{Type: types.SetOf(types.Int)},
		},
		F: nopF,
	}

	customBindFunc := &Function{
		Name:       "anything",
		Positional: []Param{{Type: types.SetOf(types.Int)}},
		Bind: func(_ *Function, args *Arguments) (*Arguments, []int, error) {
			return args, nil, nil
		},
		F: nopF,
	}

	tests := []struct {
		name    string
		fn      *Function
		args    *Arguments
		wantErr bool
	}{
		{"valid_arg", rangeFunc, &Arguments{Positional: []Value{Int(5)}}, false},
		{"two_valid_args", rangeFunc, &Arguments{Positional: []Value{Int(1), Int(2)}}, false},
		{"bad_type_rejected", rangeFunc, &Arguments{Positional: []Value{Str("string")}}, true},
		{"too_many_args", rangeFunc, &Arguments{Positional: []Value{Int(1), Int(2), Int(3)}}, true},
		{"custom_bind_skips_type_check", customBindFunc, &Arguments{Positional: []Value{Str("ok")}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.fn.With(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("With() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}

	t.Run("chained_with", func(t *testing.T) {
		fn, err := rangeFunc.With(&Arguments{Positional: []Value{Int(1)}})
		if err != nil {
			t.Fatalf("first With: %v", err)
		}
		_, err = fn.With(&Arguments{Positional: []Value{Int(2)}})
		if err != nil {
			t.Fatalf("second With: %v", err)
		}
	})
}
