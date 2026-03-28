package ir

import (
	"fmt"
	"maps"
	"strings"

	"znkr.io/writst/ir/types"
)

var joinResultType = map[[2]types.Type]types.Type{
	{types.Str, types.Str}:             types.Str,
	{types.Bytes, types.Bytes}:         types.Bytes,
	{types.Array, types.Array}:         types.Array,
	{types.Dict, types.Dict}:           types.Dict,
	{types.Str, types.Content}:         types.Content,
	{types.Content, types.Content}:     types.Content,
	{types.Arguments, types.Arguments}: types.Arguments,
}

func join(values []Value) (Value, error) {
	rtype := types.None
	for i, v := range values {
		if i == 0 {
			rtype = v.Type()
		} else if v.Type() != rtype {
			var at, bt types.Type
			at = rtype
			bt = v.Type()
			if at > bt {
				at, bt = bt, at
			}
			rtyp, ok := joinResultType[[2]types.Type{at, bt}]
			if !ok {
				return nil, &indexError{fmt.Errorf("cannot join %s with %s", rtype, v.Type()), i}
			}
			rtype = rtyp
		}
	}
	switch rtype {
	case types.None:
	case types.Int, types.Float:
		return values[0], nil
	case types.Str:
		var sb strings.Builder
		for _, v := range values {
			sb.WriteString(string(v.(Str)))
		}
		return Str(sb.String()), nil
	case types.Bytes:
		var sb strings.Builder
		for _, v := range values {
			sb.WriteString(string(v.(Bytes)))
		}
		return Bytes(sb.String()), nil
	case types.Content:
		children := make([]Content, 0, len(values))
		for i, v := range values {
			c, err := toContent(v)
			if err != nil {
				return nil, &indexError{err, i}
			}
			children = append(children, c)
		}
		return &Sequence{Children: children}, nil
	case types.Array:
		var arr Array
		for _, v := range values {
			arr = append(arr, v.(Array)...)
		}
		return arr, nil
	case types.Dict:
		dict := make(Dict)
		for _, v := range values {
			maps.Copy(dict, v.(Dict))
		}
		return dict, nil
	case types.Arguments:
		var args Arguments
		for _, v := range values {
			args = *args.merge(v.(*Arguments))
		}
		return &args, nil
	}
	panic("unsupported result type: " + rtype.String())
}
