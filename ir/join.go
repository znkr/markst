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

type joiner interface {
	add(v Value) error
	result() Value
}

type joinerSelector struct {
	rtyp types.Type
}

func (s *joinerSelector) add(t types.Type) error {
	if t == types.None {
		return nil
	}
	if s.rtyp == types.None {
		s.rtyp = t
		return nil
	}
	var at, bt types.Type
	at = s.rtyp
	bt = t
	if at > bt {
		at, bt = bt, at
	}
	rtyp, ok := joinResultType[[2]types.Type{at, bt}]
	if !ok {
		return fmt.Errorf("cannot join %s with %s", s.rtyp, t)
	}
	s.rtyp = rtyp
	return nil
}

func (s *joinerSelector) joiner() joiner {
	switch s.rtyp {
	default:
		return new(unjoinableJoiner)
	case types.Str:
		return new(strJoiner)
	case types.Bytes:
		return new(bytesJoiner)
	case types.Content:
		return new(contentJoiner)
	case types.Array:
		return new(arrayJoiner)
	case types.Dict:
		return new(dictJoiner)
	case types.Arguments:
		return new(argumentJoiner)
	}
}

type unjoinableJoiner struct {
	v Value
}

func (j *unjoinableJoiner) add(v Value) error {
	if v == none {
		return nil
	}
	if j.v != nil {
		panic("unjoinable: joiner selector should have errored before this call")
	}
	j.v = v
	return nil
}
func (j *unjoinableJoiner) result() Value {
	if j.v == nil {
		return none
	}
	return j.v
}

type strJoiner struct {
	sb strings.Builder
}

func (j *strJoiner) add(v Value) error {
	j.sb.WriteString(string(v.(Str)))
	return nil
}
func (j *strJoiner) result() Value { return Str(j.sb.String()) }

type bytesJoiner struct {
	sb strings.Builder
}

func (j *bytesJoiner) add(v Value) error {
	j.sb.WriteString(string(v.(Bytes)))
	return nil
}
func (j *bytesJoiner) result() Value { return Bytes(j.sb.String()) }

type contentJoiner struct {
	content []Content
}

func (j *contentJoiner) add(v Value) error {
	c, err := toContent(v)
	if err != nil {
		return err
	}
	j.content = append(j.content, c)
	return nil
}
func (j *contentJoiner) result() Value { return &Sequence{Children: j.content} }

type arrayJoiner struct {
	values []Value
}

func (j *arrayJoiner) add(v Value) error {
	j.values = append(j.values, v)
	return nil
}
func (j *arrayJoiner) result() Value { return &Array{Elems: j.values} }

type dictJoiner struct {
	dict Dict
}

func (j *dictJoiner) add(v Value) error {
	if j.dict == nil {
		j.dict = make(Dict)
	}
	maps.Copy(j.dict, v.(Dict))
	return nil
}
func (j *dictJoiner) result() Value { return j.dict }

type argumentJoiner struct {
	args *Arguments
}

func (j *argumentJoiner) add(v Value) error {
	j.args = j.args.merge(v.(*Arguments))
	return nil
}
func (j *argumentJoiner) result() Value { return j.args }
