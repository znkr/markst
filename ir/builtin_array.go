package ir

import (
	"fmt"
	"slices"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

var (
	builtinArray = &Function{
		Name: "array",
		Positional: []Param{
			{Name: "value", Type: types.SetOf(types.Array, types.Bytes)},
		},
		F: builtinArrayImpl,
	}

	builtinRange = &Function{
		Name: "range",
		Positional: []Param{
			{Name: "start", Type: types.SetOf(types.Int), Default: Int(0)},
			{Name: "end", Type: types.SetOf(types.Int)}},
		Named: NamedParams{
			names.Step: Param{Name: "step", Type: types.SetOf(types.Int), Default: Int(1)},
		},
		F: builtinRangeImpl,
	}

	builtinArrayAt = &Function{
		Name: "array.at",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: NamedParams{
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinArrayAtImpl,
	}

	builtinArrayFirst = &Function{
		Name: "array.first",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: NamedParams{
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinArrayFirstImpl,
	}

	builtinArrayLast = &Function{
		Name: "array.last",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: NamedParams{
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinArrayLastImpl,
	}

	builtinArrayLen = &Function{
		Name: "array.len",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: builtinArrayLenImpl,
	}

	builtinArrayInsert = &Function{
		Name: "array.insert",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
			{Name: "value", Type: types.Any},
		},
		F: builtinArrayInsertImpl,
	}

	builtinArrayPush = &Function{
		Name: "array.push",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "value", Type: types.Any},
		},
		F: builtinArrayPushImpl,
	}

	builtinArrayRemove = &Function{
		Name: "array.remove",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: NamedParams{
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinArrayRemoveImpl,
	}

	builtinArrayPop = &Function{
		Name: "array.pop",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: builtinArrayPopImpl,
	}

	builtinArrayJoin = &Function{
		Name: "array.join",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "separator", Type: types.Any, Default: none},
		},
		Named: NamedParams{
			names.Last:    Param{Name: "last", Type: types.Any},
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinArrayJoinImpl,
	}

	builtinArraySlice = &Function{
		Name: "array.slice",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "start", Type: types.SetOf(types.Int)},
			{Name: "end", Type: types.SetOf(types.Int, types.None), Default: none},
		},
		Named: NamedParams{
			names.Count: Param{Name: "count", Type: types.SetOf(types.Int)},
		},
		F: builtinArraySliceImpl,
	}

	builtinArrayProduct = &Function{
		Name: "array.product",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: NamedParams{
			names.Default: {Name: "default", Type: types.Any},
		},
		F: builtinArrayProductImpl,
	}

	builtinArraySum = &Function{
		Name: "array.sum",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: NamedParams{
			names.Default: {Name: "default", Type: types.Any},
		},
		F: builtinArraySumImpl,
	}

	builtinArrayRev = &Function{
		Name: "array.rev",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: builtinArrayRevImpl,
	}

	builtinArrayIntersperse = &Function{
		Name: "array.intersperse",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "separator", Type: types.Any},
		},
		F: builtinArrayIntersperseImpl,
	}

	builtinArrayWindows = &Function{
		Name: "array.windows",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "window-size", Type: types.SetOf(types.Int)},
		},
		F: builtinArrayWindowsImpl,
	}

	builtinArrayZip = &Function{
		Name: "array.zip",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Variadic: &Param{Name: "others", Type: types.SetOf(types.Array)},
		Named: NamedParams{
			names.Exact: Param{Name: "exact", Type: types.SetOf(types.Bool), Default: Bool(false)},
		},
		F: builtinArrayZipImpl,
	}

	builtinArrayEnumerate = &Function{
		Name: "array.enumerate",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		Named: NamedParams{
			names.Start: Param{Name: "start", Type: types.SetOf(types.Int), Default: Int(0)},
		},
		F: builtinArrayEnumerateImpl,
	}

	builtinArrayToDict = &Function{
		Name: "array.to-dict",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: builtinArrayToDictImpl,
	}

	builtinArrayChunks = &Function{
		Name: "array.chunks",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
			{Name: "chunk-size", Type: types.SetOf(types.Int)},
		},
		Named: NamedParams{
			names.Exact: Param{Name: "exact", Type: types.SetOf(types.Bool), Default: Bool(false)},
		},
		F: builtinArrayChunksImpl,
	}

	builtinArraySorted = &Function{
		Name: "array.sorted",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Array)},
		},
		F: builtinArraySortedImpl,
	}
)

func builtinArrayImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case *Array:
		return v, nil
	case Bytes:
		var result []Value
		for _, b := range []byte(v) {
			result = append(result, Int(b))
		}
		return &Array{Elems: result}, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}

func builtinRangeImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	start := args[0].(Int)
	end, hasEnd := args[1].(Int)
	if !hasEnd {
		end = start
		start = 0
	}
	step := named.Get(names.Step).(Int)

	var result []Value
	switch {
	case step == 0:
		return nil, ArgErrorNamedf(names.Step, "number must not be zero")
	case step > 0:
		for i := start; i < end; i += step {
			result = append(result, Int(i))
		}
	case step < 0:
		for i := start; i > end; i += step {
			result = append(result, Int(i))
		}
	}
	return &Array{Elems: result}, nil
}

func builtinArrayAtImpl(call *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	index := args[1].(Int)
	origIndex := index
	if index < 0 {
		index += Int(len(arr.Elems))
	}
	if index < 0 || index >= Int(len(arr.Elems)) {
		def := named.Get(names.Default)
		var suffix string
		if call.setter == nil {
			if def != none {
				return def, nil
			}
			suffix = " and no default value was specified"
		}
		return nil, ArgErrorPosf(1, "array index out of bounds (index: %d, len: %d)%s", origIndex, len(arr.Elems), suffix)
	}
	if call.setter != nil {
		*call.setter = func(v Value) {
			arr.Elems[index] = v
		}
	}
	return arr.Elems[index], nil
}

func builtinArrayFirstImpl(call *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != none {
			return v, nil
		}
		return nil, ArgErrorPosf(0, "array is empty")
	}
	if call.setter != nil {
		*call.setter = func(v Value) {
			arr.Elems[0] = v
		}
	}
	return arr.Elems[0], nil
}

func builtinArrayLastImpl(call *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != none {
			return v, nil
		}
		return nil, ArgErrorPosf(0, "array is empty")
	}
	if call.setter != nil {
		*call.setter = func(v Value) {
			arr.Elems[len(arr.Elems)-1] = v
		}
	}
	return arr.Elems[len(arr.Elems)-1], nil
}

func builtinArrayLenImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	return Int(len(arr.Elems)), nil
}

func builtinArrayInsertImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	index := args[1].(Int)
	value := args[2]
	if index < 0 {
		index += Int(len(arr.Elems))
	}
	if index < 0 || index > Int(len(arr.Elems)) {
		return nil, ArgErrorPosf(1, "index out of range: %d", index)
	}
	arr.Elems = slices.Insert(arr.Elems, int(index), value)
	return none, nil
}

func builtinArrayPushImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	value := args[1]
	arr.Elems = append(arr.Elems, value)
	return none, nil
}

func builtinArrayRemoveImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	index := args[1].(Int)
	if index < 0 {
		index += Int(len(arr.Elems))
	}
	if index < 0 || index >= Int(len(arr.Elems)) {
		if v := named.Get(names.Default); v != none {
			return v, nil
		}
		return nil, ArgErrorPosf(1, "index out of range: %d", index)
	}
	rem := arr.Elems[int(index)]
	arr.Elems = slices.Delete(arr.Elems, int(index), int(index+1))
	return rem, nil
}

func builtinArrayPopImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		return nil, ArgErrorPosf(0, "cannot pop from an empty array")
	}
	lastIndex := len(arr.Elems) - 1
	last := arr.Elems[lastIndex]
	arr.Elems = arr.Elems[:lastIndex]
	return last, nil
}

func builtinArrayJoinImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		if v := named.Get(names.Default); v != none {
			return v, nil
		}
		return none, nil
	}

	sep := args[1]
	last := named.Get(names.Last)

	var joinSel joinerSelector
	for _, v := range arr.Elems {
		if err := joinSel.add(v.Type()); err != nil {
			return nil, ArgErrorPosf(0, "%s", err.Error())
		}
	}
	if sep != none {
		if err := joinSel.add(sep.Type()); err != nil {
			return nil, ArgErrorPosf(1, "%s", err.Error())
		}
	}
	if last != none {
		if err := joinSel.add(last.Type()); err != nil {
			return nil, ArgErrorNamedf(names.Last, "%s", err.Error())
		}
	}

	joiner := joinSel.joiner()
	for i, v := range arr.Elems {
		if i > 0 && sep != none {
			if err := joiner.add(sep); err != nil {
				return nil, ArgErrorPosf(1, "%s", err.Error())
			}
		}
		if err := joiner.add(v); err != nil {
			return nil, ArgErrorPosf(0, "%s", err.Error())
		}
	}
	if last != none {
		if err := joiner.add(last); err != nil {
			return nil, ArgErrorNamedf(names.Last, "%s", err.Error())
		}
	}
	return joiner.result(), nil
}

func builtinArraySliceImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	length := Int(len(arr.Elems))

	start := args[1].(Int)
	if start < 0 {
		start += length
	}

	var end Int
	switch v := args[2].(type) {
	case Int:
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
	case None:
		if count := named.Get(names.Count); count != none {
			count := count.(Int)
			if count < 0 {
				return nil, ArgErrorNamedf(names.Count, "count must be non-negative")
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
	return &Array{Elems: slices.Clone(arr.Elems[start:end])}, nil
}

func builtinArrayProductImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		if !named.IsSet(names.Default) {
			return nil, ArgErrorPosf(0, "cannot calculate product of empty array with no default")
		}
		return named.Get(names.Default), nil
	}
	product := arr.Elems[0]
	for _, v := range arr.Elems[1:] {
		op := binops[binopKey{syntax.Mul, product.Type(), v.Type()}]
		if op == nil {
			return nil, ArgErrorPosf(0, "unsupported type for array.product: %s", product.Type())
		}
		var err error
		product, err = op(product, v)
		if err != nil {
			return nil, fmt.Errorf("error calculating the product: %v", err)
		}
	}
	return product, nil
}

func builtinArraySumImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	if len(arr.Elems) == 0 {
		if !named.IsSet(names.Default) {
			return nil, ArgErrorPosf(0, "cannot calculate sum of empty array with no default")
		}
		return named.Get(names.Default), nil
	}
	sum := arr.Elems[0]
	for _, v := range arr.Elems[1:] {
		op := binops[binopKey{syntax.Add, sum.Type(), v.Type()}]
		if op == nil {
			return nil, ArgErrorPosf(0, "unsupported type for array.sum: %s", sum.Type())
		}
		var err error
		sum, err = op(sum, v)
		if err != nil {
			return nil, fmt.Errorf("error calculating the sum: %v", err)
		}
	}
	return sum, nil
}

func builtinArrayRevImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	res := slices.Clone(arr.Elems)
	slices.Reverse(res)
	return &Array{Elems: res}, nil
}

func builtinArrayEnumerateImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	start := named.Get(names.Start).(Int)
	result := make([]Value, len(arr.Elems))
	for i, v := range arr.Elems {
		result[i] = &Array{Elems: []Value{start + Int(i), v}}
	}
	return &Array{Elems: result}, nil
}

func builtinArrayToDictImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	result := new(Dict)
	for _, elem := range arr.Elems {
		pair, ok := elem.(*Array)
		if !ok {
			return nil, ArgErrorPosf(0, "expected (str, any) pairs, found %s", elem.Type())
		}
		if len(pair.Elems) != 2 {
			return nil, ArgErrorPosf(0, "expected pairs of length 2, found length %d", len(pair.Elems))
		}
		key, ok := pair.Elems[0].(Str)
		if !ok {
			return nil, ArgErrorPosf(0, "expected key of type str, found %s", pair.Elems[0].Type())
		}
		result.Elems.Put(key, pair.Elems[1])
	}
	return result, nil
}

func builtinArrayIntersperseImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	sep := args[1]
	if len(arr.Elems) <= 1 {
		return &Array{Elems: slices.Clone(arr.Elems)}, nil
	}
	result := make([]Value, 0, len(arr.Elems)*2-1)
	for i, v := range arr.Elems {
		if i > 0 {
			result = append(result, sep)
		}
		result = append(result, v)
	}
	return &Array{Elems: result}, nil
}

func builtinArrayWindowsImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	size := args[1].(Int)
	if size <= 0 {
		return nil, ArgErrorPosf(1, "number must be positive")
	}
	var windows []Value
	for i := 0; i <= len(arr.Elems)-int(size); i++ {
		windows = append(windows, &Array{Elems: slices.Clone(arr.Elems[i : i+int(size)])})
	}
	return &Array{Elems: windows}, nil
}

func builtinArrayZipImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	self := args[0].(*Array)
	others := args[1].(*Array)
	exact := named.Get(names.Exact).(Bool)

	// Collect all arrays: self + others.
	arrays := make([]*Array, 1+len(others.Elems))
	arrays[0] = self
	for i, v := range others.Elems {
		arrays[i+1] = v.(*Array)
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
		var errs ArgErrors
		for i, a := range arrays[1:] {
			if len(a.Elems) != len(self.Elems) {
				var prefix string
				if len(arrays) == 2 {
					prefix = "second "
				}
				errs = append(errs, ArgErrorPosf(i+1, "%sarray has different length (%d) from first array (%d)", prefix, len(a.Elems), len(self.Elems)))
			}
		}
		if len(errs) == 1 {
			return nil, errs[0]
		}
		if len(errs) > 1 {
			return nil, errs
		}
	}

	// Build result.
	result := make([]Value, minLen)
	for i := 0; i < minLen; i++ {
		tuple := make([]Value, len(arrays))
		for j, a := range arrays {
			tuple[j] = a.Elems[i]
		}
		result[i] = &Array{Elems: tuple}
	}
	return &Array{Elems: result}, nil
}

func builtinArrayChunksImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	size := args[1].(Int)
	exact := named.Get(names.Exact).(Bool)
	if size <= 0 {
		return nil, ArgErrorPosf(1, "number must be positive")
	}
	var chunks []Value
	for i := 0; i < len(arr.Elems); i += int(size) {
		end := i + int(size)
		if end > len(arr.Elems) {
			if exact {
				break
			}
			end = len(arr.Elems)
		}
		chunks = append(chunks, &Array{Elems: slices.Clone(arr.Elems[i:end])})
	}
	return &Array{Elems: chunks}, nil
}

func builtinArraySortedImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(*Array)
	sortedElems := slices.Clone(arr.Elems)
	var err error
	slices.SortFunc(sortedElems, func(a, b Value) int {
		cmp, err0 := cmpValues(a, b)
		if err0 != nil && err == nil {
			err = err0
		}
		return cmp
	})
	if err != nil {
		return nil, &ValueError{
			msg:   err.Error(),
			hints: []string{"consider choosing a `key` or defining the comparison with `by`"},
		}
	}
	return &Array{Elems: sortedElems}, nil
}
