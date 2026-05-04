package builtin

import (
	"errors"
	"fmt"
	"slices"

	"znkr.io/writst/internal/joiner"
	"znkr.io/writst/internal/names"
	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Array = &value.Function{
		Name: "array",
		Positional: []value.Param{
			{Name: "value", Type: types.SetOf(types.Array, types.Bytes)},
		},
		F: arrayImpl,
	}

	Range = &value.Function{
		Name: "range",
		Positional: []value.Param{
			{Name: "start", Type: types.SetOf(types.Int), Default: value.Int(0)},
			{Name: "end", Type: types.SetOf(types.Int)}},
		Named: value.NamedParams{
			names.Step: value.Param{Name: "step", Type: types.SetOf(types.Int), Default: value.Int(1)},
		},
		F: rangeImpl,
	}

	ArrayAt = &value.Function{
		Name: "array.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: arrayAtImpl,
	}

	ArrayFirst = &value.Function{
		Name: "array.first",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: arrayFirstImpl,
	}

	ArrayLast = &value.Function{
		Name: "array.last",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: arrayLastImpl,
	}

	ArrayLen = &value.Function{
		Name: "array.len",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: arrayLenImpl,
	}

	ArrayInsert = &value.Function{
		Name: "array.insert",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
			{Name: "value", Type: types.Any},
		},
		F: arrayInsertImpl,
	}

	ArrayPush = &value.Function{
		Name: "array.push",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "value", Type: types.Any},
		},
		F: arrayPushImpl,
	}

	ArrayRemove = &value.Function{
		Name: "array.remove",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: arrayRemoveImpl,
	}

	ArrayPop = &value.Function{
		Name: "array.pop",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: arrayPopImpl,
	}

	ArrayJoin = &value.Function{
		Name: "array.join",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "separator", Type: types.Any, Default: value.None{}},
		},
		Named: value.NamedParams{
			names.Last:    value.Param{Name: "last", Type: types.Any},
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: arrayJoinImpl,
	}

	ArraySlice = &value.Function{
		Name: "array.slice",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "start", Type: types.SetOf(types.Int)},
			{Name: "end", Type: types.SetOf(types.Int, types.None), Default: value.None{}},
		},
		Named: value.NamedParams{
			names.Count: value.Param{Name: "count", Type: types.SetOf(types.Int)},
		},
		F: arraySliceImpl,
	}

	ArrayProduct = &value.Function{
		Name: "array.product",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: value.NamedParams{
			names.Default: {Name: "default", Type: types.Any},
		},
		F: arrayProductImpl,
	}

	ArraySum = &value.Function{
		Name: "array.sum",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: value.NamedParams{
			names.Default: {Name: "default", Type: types.Any},
		},
		F: arraySumImpl,
	}

	ArrayRev = &value.Function{
		Name: "array.rev",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: arrayRevImpl,
	}

	ArrayIntersperse = &value.Function{
		Name: "array.intersperse",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "separator", Type: types.Any},
		},
		F: arrayIntersperseImpl,
	}

	ArrayWindows = &value.Function{
		Name: "array.windows",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "window-size", Type: types.SetOf(types.Int)},
		},
		F: arrayWindowsImpl,
	}

	ArrayZip = &value.Function{
		Name: "array.zip",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Variadic: &value.Param{Name: "others", Type: types.SetOf(types.Array)},
		Named: value.NamedParams{
			names.Exact: value.Param{Name: "exact", Type: types.SetOf(types.Bool), Default: value.Bool(false)},
		},
		F: arrayZipImpl,
	}

	ArrayEnumerate = &value.Function{
		Name: "array.enumerate",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: value.NamedParams{
			names.Start: value.Param{Name: "start", Type: types.SetOf(types.Int), Default: value.Int(0)},
		},
		F: arrayEnumerateImpl,
	}

	ArrayToDict = &value.Function{
		Name: "array.to-dict",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: arrayToDictImpl,
	}

	ArrayChunks = &value.Function{
		Name: "array.chunks",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "chunk-size", Type: types.SetOf(types.Int)},
		},
		Named: value.NamedParams{
			names.Exact: value.Param{Name: "exact", Type: types.SetOf(types.Bool), Default: value.Bool(false)},
		},
		F: arrayChunksImpl,
	}

	ArraySorted = &value.Function{
		Name: "array.sorted",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: arraySortedImpl,
	}
)

func arrayImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case *value.Array:
		return v, nil
	case value.Bytes:
		var result []value.Value
		for _, b := range []byte(v) {
			result = append(result, value.Int(b))
		}
		return &value.Array{Elems: result}, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}

func rangeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	start := args[0].(value.Int)
	end, hasEnd := args[1].(value.Int)
	if !hasEnd {
		end = start
		start = 0
	}
	step := named.Get(names.Step).(value.Int)

	var result []value.Value
	switch {
	case step == 0:
		return nil, value.ArgErrorNamedf(names.Step, "number must not be zero")
	case step > 0:
		for i := start; i < end; i += step {
			result = append(result, value.Int(i))
		}
	case step < 0:
		for i := start; i > end; i += step {
			result = append(result, value.Int(i))
		}
	}
	return &value.Array{Elems: result}, nil
}

func arrayAtImpl(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	index := args[1].(value.Int)
	origIndex := index
	if index < 0 {
		index += value.Int(len(arr.Elems))
	}
	if index < 0 || index >= value.Int(len(arr.Elems)) {
		def := named.Get(names.Default)
		var suffix string
		if call.Setter == nil {
			if def != (value.None{}) {
				return def, nil
			}
			suffix = " and no default value was specified"
		}
		return nil, value.ArgErrorPosf(1, "array index out of bounds (index: %d, len: %d)%s", origIndex, len(arr.Elems), suffix)
	}
	if call.Setter != nil {
		*call.Setter = func(v value.Value) {
			arr.Elems[index] = v
		}
	}
	return arr.Elems[index], nil
}

func arrayFirstImpl(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != (value.None{}) {
			return v, nil
		}
		return nil, value.ArgErrorPosf(0, "array is empty")
	}
	if call.Setter != nil {
		*call.Setter = func(v value.Value) {
			arr.Elems[0] = v
		}
	}
	return arr.Elems[0], nil
}

func arrayLastImpl(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != (value.None{}) {
			return v, nil
		}
		return nil, value.ArgErrorPosf(0, "array is empty")
	}
	if call.Setter != nil {
		*call.Setter = func(v value.Value) {
			arr.Elems[len(arr.Elems)-1] = v
		}
	}
	return arr.Elems[len(arr.Elems)-1], nil
}

func arrayLenImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	return value.Int(len(arr.Elems)), nil
}

func arrayInsertImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	index := args[1].(value.Int)
	val := args[2]
	if index < 0 {
		index += value.Int(len(arr.Elems))
	}
	if index < 0 || index > value.Int(len(arr.Elems)) {
		return nil, value.ArgErrorPosf(1, "index out of range: %d", index)
	}
	arr.Elems = slices.Insert(arr.Elems, int(index), val)
	return value.None{}, nil
}

func arrayPushImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	val := args[1]
	arr.Elems = append(arr.Elems, val)
	return value.None{}, nil
}

func arrayRemoveImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	index := args[1].(value.Int)
	if index < 0 {
		index += value.Int(len(arr.Elems))
	}
	if index < 0 || index >= value.Int(len(arr.Elems)) {
		if v := named.Get(names.Default); v != (value.None{}) {
			return v, nil
		}
		return nil, value.ArgErrorPosf(1, "index out of range: %d", index)
	}
	rem := arr.Elems[int(index)]
	arr.Elems = slices.Delete(arr.Elems, int(index), int(index+1))
	return rem, nil
}

func arrayPopImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		return nil, value.ArgErrorPosf(0, "cannot pop from an empty array")
	}
	lastIndex := len(arr.Elems) - 1
	last := arr.Elems[lastIndex]
	arr.Elems = arr.Elems[:lastIndex]
	return last, nil
}

func arrayJoinImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != (value.None{}) {
			return v, nil
		}
		return value.None{}, nil
	}

	sep := args[1]
	last := named.Get(names.Last)

	var joinSel joiner.Selector
	for _, v := range arr.Elems {
		if err := joinSel.Add(v.Type()); err != nil {
			return nil, value.ArgErrorPosf(0, "%s", err.Error())
		}
	}
	if sep != (value.None{}) {
		if err := joinSel.Add(sep.Type()); err != nil {
			return nil, value.ArgErrorPosf(1, "%s", err.Error())
		}
	}
	if last != (value.None{}) {
		if err := joinSel.Add(last.Type()); err != nil {
			return nil, value.ArgErrorNamedf(names.Last, "%s", err.Error())
		}
	}

	joiner := joinSel.Joiner()
	for i, v := range arr.Elems {
		if i > 0 && sep != (value.None{}) {
			if err := joiner.Add(sep); err != nil {
				return nil, value.ArgErrorPosf(1, "%s", err.Error())
			}
		}
		if err := joiner.Add(v); err != nil {
			return nil, value.ArgErrorPosf(0, "%s", err.Error())
		}
	}
	if last != (value.None{}) {
		if err := joiner.Add(last); err != nil {
			return nil, value.ArgErrorNamedf(names.Last, "%s", err.Error())
		}
	}
	return joiner.Result(), nil
}

func arraySliceImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	length := value.Int(len(arr.Elems))

	start := args[1].(value.Int)
	if start < 0 {
		start += length
	}

	var end value.Int
	switch v := args[2].(type) {
	case value.Int:
		if named.IsSet(names.Count) {
			return nil, fmt.Errorf("`end` and `count` are mutually exclusive")
		}
		end = v
		if end < 0 {
			end += length
		}
		if end < 0 {
			return nil, fmt.Errorf("array index out of bounds (index: %d, len: %d)", v, length)
		}
	case value.None:
		if count := named.Get(names.Count); count != (value.None{}) {
			count := count.(value.Int)
			if count < 0 {
				return nil, value.ArgErrorNamedf(names.Count, "count must be non-negative")
			}
			end = start + count
		} else {
			end = length
		}
	default:
		panic("should never happen")
	}

	if end > length {
		return nil, fmt.Errorf("array index out of bounds (index: %d, len: %d)", end, length)
	}
	end = max(start, end)
	return &value.Array{Elems: slices.Clone(arr.Elems[start:end])}, nil
}

func arrayProductImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		if !named.IsSet(names.Default) {
			return nil, value.ArgErrorPosf(0, "cannot calculate product of empty array with no default")
		}
		return named.Get(names.Default), nil
	}
	product := arr.Elems[0]
	for _, v := range arr.Elems[1:] {
		var err error
		product, err = value.BinaryOp(syntax.Mul, product, v)
		if err != nil {
			return nil, fmt.Errorf("error calculating the product: %v", err)
		}
	}
	return product, nil
}

func arraySumImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	if len(arr.Elems) == 0 {
		if !named.IsSet(names.Default) {
			return nil, value.ArgErrorPosf(0, "cannot calculate sum of empty array with no default")
		}
		return named.Get(names.Default), nil
	}
	sum := arr.Elems[0]
	for _, v := range arr.Elems[1:] {
		var err error
		sum, err = value.BinaryOp(syntax.Add, sum, v)
		if err != nil {
			return nil, fmt.Errorf("error calculating the sum: %v", err)
		}
	}
	return sum, nil
}

func arrayRevImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	res := slices.Clone(arr.Elems)
	slices.Reverse(res)
	return &value.Array{Elems: res}, nil
}

func arrayEnumerateImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	start := named.Get(names.Start).(value.Int)
	result := make([]value.Value, len(arr.Elems))
	for i, v := range arr.Elems {
		result[i] = &value.Array{Elems: []value.Value{start + value.Int(i), v}}
	}
	return &value.Array{Elems: result}, nil
}

func arrayToDictImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	result := new(value.Dict)
	for _, elem := range arr.Elems {
		pair, ok := elem.(*value.Array)
		if !ok {
			return nil, value.ArgErrorPosf(0, "expected (str, any) pairs, found %s", elem.Type())
		}
		if len(pair.Elems) != 2 {
			return nil, value.ArgErrorPosf(0, "expected pairs of length 2, found length %d", len(pair.Elems))
		}
		key, ok := pair.Elems[0].(value.Str)
		if !ok {
			return nil, value.ArgErrorPosf(0, "expected key of type str, found %s", pair.Elems[0].Type())
		}
		result.Elems.Put(key, pair.Elems[1])
	}
	return result, nil
}

func arrayIntersperseImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	sep := args[1]
	if len(arr.Elems) <= 1 {
		return &value.Array{Elems: slices.Clone(arr.Elems)}, nil
	}
	result := make([]value.Value, 0, len(arr.Elems)*2-1)
	for i, v := range arr.Elems {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, v)
	}
	return &value.Array{Elems: result}, nil
}

func arrayWindowsImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	size := args[1].(value.Int)
	if size <= 0 {
		return nil, value.ArgErrorPosf(1, "number must be positive")
	}
	var windows []value.Value
	for i := 0; i <= len(arr.Elems)-int(size); i++ {
		windows = append(windows, &value.Array{Elems: slices.Clone(arr.Elems[i : i+int(size)])})
	}
	return &value.Array{Elems: windows}, nil
}

func arrayZipImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	self := args[0].(*value.Array)
	others := args[1].(*value.Array)
	exact := named.Get(names.Exact).(value.Bool)

	// Collect all arrays: self + others.
	arrays := make([]*value.Array, 1+len(others.Elems))
	arrays[0] = self
	for i, v := range others.Elems {
		arrays[i+1] = v.(*value.Array)
	}

	// Find minimum length.
	minLen := len(self.Elems)
	for _, a := range arrays[1:] {
		if len(a.Elems) < minLen {
			minLen = len(a.Elems)
		}
	}

	// Check exact mode.
	if exact {
		var errs []error
		for i, a := range arrays[1:] {
			if len(a.Elems) != len(self.Elems) {
				var prefix string
				if len(arrays) == 2 {
					prefix = "second "
				}
				errs = append(errs, value.ArgErrorPosf(i+1, "%sarray has different length (%d) from first array (%d)", prefix, len(a.Elems), len(self.Elems)))
			}
		}
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
	}

	// Build result.
	result := make([]value.Value, minLen)
	for i := 0; i < minLen; i++ {
		tuple := make([]value.Value, len(arrays))
		for j, a := range arrays {
			tuple[j] = a.Elems[i]
		}
		result[i] = &value.Array{Elems: tuple}
	}
	return &value.Array{Elems: result}, nil
}

func arrayChunksImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	size := args[1].(value.Int)
	exact := named.Get(names.Exact).(value.Bool)
	if size <= 0 {
		return nil, value.ArgErrorPosf(1, "number must be positive")
	}
	var chunks []value.Value
	for i := 0; i < len(arr.Elems); i += int(size) {
		end := i + int(size)
		if end > len(arr.Elems) {
			if exact {
				break
			}
			end = len(arr.Elems)
		}
		chunks = append(chunks, &value.Array{Elems: slices.Clone(arr.Elems[i:end])})
	}
	return &value.Array{Elems: chunks}, nil
}

func arraySortedImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	arr := args[0].(*value.Array)
	sortedElems := slices.Clone(arr.Elems)
	var err error
	slices.SortFunc(sortedElems, func(a, b value.Value) int {
		cmp, err0 := value.Compare(a, b)
		if err0 != nil && err == nil {
			err = err0
		}
		return cmp
	})
	if err != nil {
		return nil, &value.FunctionCallError{
			Msg:   err.Error(),
			Hints: []string{"consider choosing a `key` or defining the comparison with `by`"},
		}
	}
	return &value.Array{Elems: sortedElems}, nil
}
