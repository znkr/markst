// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtin

import (
	"fmt"

	"znkr.io/markst/value"
)

// applyCallback calls fn with args and distinguishes the two ways a user
// callback can fail.
//
// poison is a [*value.Error] the callback evaluated to. Its diagnostic is
// already recorded on the session, and a builtin that receives one must return
// it as its own result rather than continue with some other value. An Error
// flows through evaluation without being recorded again, so a failed predicate
// is reported once; continuing would give later code a value that looks valid
// and produce a second, misleading diagnostic.
//
// err is the other kind: a failure not recorded anywhere yet, which the builtin
// wraps and returns as usual.
//
// call is the calling builtin's own context, passed through rather than
// replaced. It supplies the evaluator state the callback runs against
// ([value.FunctionCallContext.Runtime]) and the instant the document is
// rendered at, both of which a callback can observe: a closure records its
// diagnostics on the running session, and `datetime.today()` reads Now. It also
// means the callback reports against the builtin's call site, the nearest span
// available.
func applyCallback(call *value.FunctionCallContext, fn *value.Function, args ...value.Value) (res value.Value, poison *value.Error, err error) {
	fcc := *call
	fcc.Setter = nil // the callback's result is not a place; see value.Function.Accessor
	res, err = fn.Apply(&fcc, &value.Arguments{Positional: args})
	if err != nil {
		return nil, nil, err
	}
	if e, ok := value.IsError(res); ok {
		return nil, e, nil
	}
	return res, nil, nil
}

// applyPredicate calls fn with v as its sole positional argument and returns
// the resulting boolean. It is used by filter-like builtins. See
// [applyCallback] for poison.
func applyPredicate(call *value.FunctionCallContext, fn *value.Function, v value.Value) (include bool, poison *value.Error, err error) {
	res, poison, err := applyCallback(call, fn, v)
	if err != nil || poison != nil {
		return false, poison, err
	}
	b, ok := res.(value.Bool)
	if !ok {
		return false, nil, fmt.Errorf("test function must return a boolean")
	}
	return bool(b), nil, nil
}

// applyMapper calls fn with v as its sole positional argument and returns the
// result. It is used by map-like builtins. See [applyCallback] for poison.
func applyMapper(call *value.FunctionCallContext, fn *value.Function, v value.Value) (res value.Value, poison *value.Error, err error) {
	return applyCallback(call, fn, v)
}

// rejectSinkNamed reports the first named argument captured by an element's
// positional sink as an error. An element with a sink and a fixed set of named
// properties has no use for any other named argument: bind routes the
// recognized ones to the declared params, so whatever reached the sink is a
// typo.
func rejectSinkNamed(sink *value.Arguments) error {
	for n := range sink.Named.All() {
		return value.ArgErrorNamedPairf(n, "unexpected argument: %s", n)
	}
	return nil
}
