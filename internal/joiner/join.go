package joiner

import (
	"fmt"
	"strings"

	"znkr.io/writst/internal/ordered"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// joinResultType defines which type combinations can be joined with the +
// operator and what type the result has. For example, str + str = str,
// content + content = content.
var joinResultType = map[[2]types.Type]types.Type{
	{types.Str, types.Str}:             types.Str,
	{types.Bytes, types.Bytes}:         types.Bytes,
	{types.Array, types.Array}:         types.Array,
	{types.Dict, types.Dict}:           types.Dict,
	{types.Str, types.Content}:         types.Content,
	{types.Content, types.Content}:     types.Content,
	{types.Arguments, types.Arguments}: types.Arguments,
}

type Joiner interface {
	Add(v value.Value) error
	Result() value.Value
}

type Selector struct {
	rtyp types.Type
}

func (s *Selector) Add(t types.Type) error {
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

func (s *Selector) Joiner() Joiner {
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
	v value.Value
}

func (j *unjoinableJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	if j.v != nil {
		panic("unjoinable: joiner selector should have errored before this call")
	}
	j.v = v
	return nil
}
func (j *unjoinableJoiner) Result() value.Value {
	if j.v == nil {
		return value.None{}
	}
	return j.v
}

type strJoiner struct {
	sb strings.Builder
}

func (j *strJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	j.sb.WriteString(string(v.(value.Str)))
	return nil
}
func (j *strJoiner) Result() value.Value { return value.Str(j.sb.String()) }

type bytesJoiner struct {
	sb strings.Builder
}

func (j *bytesJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	j.sb.WriteString(string(v.(value.Bytes)))
	return nil
}
func (j *bytesJoiner) Result() value.Value { return value.Bytes(j.sb.String()) }

type contentJoiner struct {
	content []value.Content
}

func (j *contentJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	c, err := value.ToContent(v)
	if err != nil {
		return err
	}
	j.content = append(j.content, c)
	return nil
}
func (j *contentJoiner) Result() value.Value { return &value.Sequence{Children: j.content} }

type arrayJoiner struct {
	values []value.Value
}

func (j *arrayJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	j.values = append(j.values, v)
	return nil
}
func (j *arrayJoiner) Result() value.Value { return &value.Array{Elems: j.values} }

type dictJoiner struct {
	dict ordered.Map[value.Str, value.Value]
}

func (j *dictJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	for k, v := range v.(*value.Dict).Elems.All() {
		j.dict.Put(k, v)
	}
	return nil
}
func (j *dictJoiner) Result() value.Value {
	return &value.Dict{Elems: j.dict}
}

type argumentJoiner struct {
	args *value.Arguments
}

func (j *argumentJoiner) Add(v value.Value) error {
	if v == (value.None{}) {
		return nil
	}
	j.args = j.args.Merge(v.(*value.Arguments))
	return nil
}
func (j *argumentJoiner) Result() value.Value { return j.args }
