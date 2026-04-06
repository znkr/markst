package ir

import (
	"slices"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

type unaryopKey struct {
	op  syntax.UnaryOp
	typ types.Type
}

var unaryops = map[unaryopKey]func(x Value) Value{
	{syntax.Not, types.Bool}: func(x Value) Value {
		return !x.(Bool)
	},
	{syntax.Neg, types.Int}: func(x Value) Value {
		return -x.(Int)
	},
	{syntax.Neg, types.Float}: func(x Value) Value {
		return -x.(Float)
	},
}

type binopKey struct {
	op        syntax.BinaryOp
	leftType  types.Type
	rightType types.Type
}

var binops = map[binopKey]func(x, y Value) Value{
	// Int operations
	{syntax.Add, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) + y.(Int)
	},
	{syntax.Sub, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) - y.(Int)
	},
	{syntax.Mul, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) * y.(Int)
	},
	{syntax.Div, types.Int, types.Int}: func(x, y Value) Value {
		return x.(Int) / y.(Int)
	},
	{syntax.Lt, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) < y.(Int))
	},
	{syntax.Gt, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) > y.(Int))
	},
	{syntax.Leq, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) <= y.(Int))
	},
	{syntax.Geq, types.Int, types.Int}: func(x, y Value) Value {
		return Bool(x.(Int) >= y.(Int))
	},

	// Float operations
	{syntax.Add, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) + y.(Float)
	},
	{syntax.Sub, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) - y.(Float)
	},
	{syntax.Mul, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) * y.(Float)
	},
	{syntax.Div, types.Float, types.Float}: func(x, y Value) Value {
		return x.(Float) / y.(Float)
	},
	{syntax.Lt, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) < y.(Float))
	},
	{syntax.Gt, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) > y.(Float))
	},
	{syntax.Leq, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) <= y.(Float))
	},
	{syntax.Geq, types.Float, types.Float}: func(x, y Value) Value {
		return Bool(x.(Float) >= y.(Float))
	},

	// Decimal operations
	{syntax.Add, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Add(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Sub, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Sub(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Mul, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Mul(decimal128.Decimal(y.(Decimal))))
	},
	{syntax.Div, types.Decimal, types.Decimal}: func(x, y Value) Value {
		return Decimal(decimal128.Decimal(x.(Decimal)).Quo(decimal128.Decimal(y.(Decimal))))
	},

	// Ratio operations
	{syntax.Mul, types.Ratio, types.Ratio}: func(x, y Value) Value {
		r := x.(Numeric).Value * y.(Numeric).Value
		return Numeric{Value: r, Unit: UnitPercent}
	},

	// String operations
	{syntax.Add, types.Str, types.Str}: func(x, y Value) Value {
		return Str(string(x.(Str)) + string(y.(Str)))
	},

	// Bytes operations
	{syntax.Add, types.Bytes, types.Bytes}: func(x, y Value) Value {
		return Bytes(string(x.(Bytes)) + string(y.(Bytes)))
	},

	// Array operations
	{syntax.Add, types.Array, types.Array}: func(x, y Value) Value {
		arr1, arr2 := x.(*Array), y.(*Array)
		return &Array{Elems: slices.Concat(arr1.Elems, arr2.Elems)}
	},
	{syntax.Mul, types.Array, types.Int}: func(x, y Value) Value {
		arr, times := x.(*Array), y.(Int)
		if times < 0 {
			panic("cannot multiply array by negative integer")
		}
		result := slices.Repeat(arr.Elems, int(times))
		return &Array{result}
	},

	// Arguments operations
	{syntax.Add, types.Arguments, types.Arguments}: func(x, y Value) Value {
		a, b := x.(*Arguments), y.(*Arguments)
		return a.merge(b)
	},

	// Content operations
	{syntax.Mul, types.Content, types.Int}: func(x, y Value) Value {
		c, times := x.(Content), y.(Int)
		if times < 0 {
			panic("cannot multiply content by negative integer")
		}
		result := slices.Repeat([]Content{c}, int(times))
		return &Sequence{Children: result}
	},
}
