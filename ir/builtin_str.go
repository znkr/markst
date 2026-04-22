package ir

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var (
	builtinStr = &Function{
		Name: "str",
		Positional: []Param{
			{Name: "value", Type: types.Any},
		},
		F: builtinStrImpl,
	}

	builtinStrAt = &Function{
		Name: "str.at",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Str)},
			{Name: "index", Type: types.SetOf(types.Int)},
		},
		Named: NamedParams{
			names.Default: Param{Name: "default", Type: types.Any},
		},
		F: builtinStrAtImpl,
	}

	builtinStrSplit = &Function{
		Name: "str.split",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Str)},
			{Name: "pattern", Type: types.SetOf(types.Str, types.None), Default: none},
		},
		F: builtinStrSplitImpl,
	}
)

func builtinStrSplitImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	s := string(args[0].(Str))
	switch pat := args[1].(type) {
	case None:
		// Split on whitespace.
		parts := strings.Fields(s)
		elems := make([]Value, len(parts))
		for i, p := range parts {
			elems[i] = Str(p)
		}
		return &Array{Elems: elems}, nil
	case Str:
		if pat == "" {
			// Empty separator: split into code points with empty strings at boundaries.
			elems := make([]Value, 0, utf8.RuneCountInString(s)+2)
			elems = append(elems, Str(""))
			for _, r := range s {
				elems = append(elems, Str(string(r)))
			}
			elems = append(elems, Str(""))
			return &Array{Elems: elems}, nil
		}
		parts := strings.Split(s, string(pat))
		elems := make([]Value, len(parts))
		for i, p := range parts {
			elems[i] = Str(p)
		}
		return &Array{Elems: elems}, nil
	default:
		panic("unexpected type: " + pat.Type().String())
	}
}

func builtinStrAtImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	s := string(args[0].(Str))
	index := int(args[1].(Int))
	for i, r := range s {
		if i == index {
			return Str(string(r)), nil
		}
	}
	return named.Get(names.Default), nil
}

func builtinStrImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Bytes:
		return Str(v), nil
	case Int:
		r := strconv.FormatInt(int64(v), 10)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return Str(r), nil
	case Float:
		r := strconv.FormatFloat(float64(v), 'f', -1, 64)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return Str(r), nil
	case *Label:
		return Str(v.Name.Value()), nil
	case *Type:
		return Str(v.Reflected.String()), nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}
