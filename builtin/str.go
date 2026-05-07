package builtin

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Str = &value.Function{
		Name: "str",
		Positional: []value.Param{
			{Name: "value", Type: types.Any},
		},
		F: strImpl,
	}

	StrAt = &value.Function{
		Name: "str.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Str)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: StrAtImpl,
	}

	StrLen = &value.Function{
		Name: "str.len",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Str)},
		},
		F: strLenImpl,
	}

	StrSplit = &value.Function{
		Name: "str.split",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Str)},
			{Name: "pattern", Type: types.SetOf(types.Str, types.None), Default: value.None{}},
		},
		F: StrSplitImpl,
	}
)

func strImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Bytes:
		return value.Str(v), nil
	case value.Int:
		r := strconv.FormatInt(int64(v), 10)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return value.Str(r), nil
	case value.Float:
		r := strconv.FormatFloat(float64(v), 'f', -1, 64)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return value.Str(r), nil
	case value.Decimal:
		r := v.String()
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return value.Str(r), nil
	case *value.Label:
		return value.Str(v.Name.Value()), nil
	case *value.Type:
		return value.Str(v.Reflected.String()), nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}

func StrAtImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	s := string(args[0].(value.Str))
	index := int(args[1].(value.Int))
	for i, r := range s {
		if i == index {
			return value.Str(string(r)), nil
		}
	}
	return named.Get(names.Default), nil
}

func strLenImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	s := string(args[0].(value.Str))
	return value.Int(utf8.RuneCountInString(s)), nil
}

func StrSplitImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	s := string(args[0].(value.Str))
	switch pat := args[1].(type) {
	case value.None:
		// Split on whitespace.
		parts := strings.Fields(s)
		elems := make([]value.Value, len(parts))
		for i, p := range parts {
			elems[i] = value.Str(p)
		}
		return &value.Array{Elems: elems}, nil
	case value.Str:
		if pat == "" {
			// Empty separator: split into code points with empty strings at boundaries.
			elems := make([]value.Value, 0, utf8.RuneCountInString(s)+2)
			elems = append(elems, value.Str(""))
			for _, r := range s {
				elems = append(elems, value.Str(string(r)))
			}
			elems = append(elems, value.Str(""))
			return &value.Array{Elems: elems}, nil
		}
		parts := strings.Split(s, string(pat))
		elems := make([]value.Value, len(parts))
		for i, p := range parts {
			elems[i] = value.Str(p)
		}
		return &value.Array{Elems: elems}, nil
	default:
		panic("unexpected type: " + pat.Type().String())
	}
}
