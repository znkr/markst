// Package joiner combines a run of values into one, the way the + operator and
// a block's trailing expressions do.
//
// Which types can be combined and what comes out is decided first, by handing
// each value's type to a [Selector]; [Selector.Joiner] then returns the
// [Joiner] that does the work.
package joiner

import (
	"fmt"
	"strings"

	"znkr.io/markst/internal/ordered"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// joinResultType maps a pair of types, in ascending order, to the type joining
// them produces: str + str is str, str + content is content.
var joinResultType = map[[2]types.Type]types.Type{
	{types.Str, types.Str}:             types.Str,
	{types.Bytes, types.Bytes}:         types.Bytes,
	{types.Array, types.Array}:         types.Array,
	{types.Dict, types.Dict}:           types.Dict,
	{types.Str, types.Content}:         types.Content,
	{types.Content, types.Content}:     types.Content,
	{types.Arguments, types.Arguments}: types.Arguments,
}

// Joiner accumulates values of one result type into a single joined value.
type Joiner interface {
	// Add accumulates v, and returns an error if v cannot be joined with what
	// came before.
	Add(v value.Value) error

	// Result returns the joined value.
	Result() value.Value
}

// Selector determines the type that joining a run of values will produce. Add
// each value's type to it, then call [Selector.Joiner] for the joiner to use.
type Selector struct {
	rtyp types.Type
}

// Add folds t into the result type so far, and returns an error if t cannot be
// joined with it. A `none` is ignored, since joining with none is the identity.
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

// Joiner returns a joiner for the result type worked out so far. Call it only
// after every value's type has gone through [Selector.Add] without error.
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

// unjoinableJoiner handles a run whose result type has no joiner: a scalar, or
// nothing at all. [Selector.Add] has already rejected a run with two such
// values in it, so at most one ever arrives here.
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
	j.content = append(j.content, value.ToContent(v))
	return nil
}
func (j *contentJoiner) Result() value.Value {
	// Joining folds with `+` starting from `none`, so `none + x == x`: a single
	// content element joins to itself, not a one-element sequence. This keeps a
	// joined single content equal to the bare content (matching Typst).
	if len(j.content) == 1 {
		return j.content[0]
	}
	return &value.Sequence{Children: j.content}
}

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
