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

package value

import (
	"fmt"

	"github.com/woodsbury/decimal128"
)

// ToContent shows v as content the way writing `#v` in markup does. Every value
// has such a form: one that has no element of its own shows as the source that
// would produce it. A [None] shows as nothing at all, which is a nil Content
// rather than an empty one.
//
// In math, use [ToMathContent] instead: it shows a value with no element of
// its own differently.
func ToContent(v Value) Content {
	switch v := v.(type) {
	case Content:
		return v
	case Str:
		return &Text{Text: string(v)}
	case *Symbol:
		return &Text{Text: v.String()}
	case None:
		return nil
	case Int:
		return &Raw{Text: fmt.Sprintf("%d", v)}
	case Float:
		return &Raw{Text: floatText(v)}
	case Length:
		return &Raw{Text: v.String()}
	case Relative:
		return &Raw{Text: v.String()}
	case Decimal:
		d := decimal128.Decimal(v)
		var s string
		if d.IsInf(1) {
			s = "inf"
		} else if d.IsInf(-1) {
			s = "-inf"
		} else if d.IsNaN() {
			s = "nan"
		} else {
			s = d.String()
		}
		return &Raw{Text: s}
	case Datetime:
		return &Raw{Text: v.String()}
	case Duration:
		return &Raw{Text: v.String()}
	}
	// Anything else shows as the source that would produce it, the way Typst's
	// `Value::display` falls back to a raw of the value's repr.
	return &Raw{Text: Repr(v)}
}

// ToMathContent is [ToContent] in math. Symbols, strings and numbers become
// [MathText] rather than the upright [Text] or [Raw] ToContent gives, so a
// character renders the same however it was written: `$sym.alpha + 1$`,
// `$alpha + 1$` and `#math.frac(alpha, 1)` all agree. Anything else goes to
// ToContent.
func ToMathContent(v Value) Content {
	switch v := v.(type) {
	case *Symbol:
		return &MathText{Text: v.String()}
	case Str:
		return &MathText{Text: string(v)}
	case Int:
		return &MathText{Text: fmt.Sprintf("%d", v)}
	case Float:
		return &MathText{Text: floatText(v)}
	}
	return ToContent(v)
}

// floatText renders a float the way content displays it: NaN is spelled the way
// it is written in source rather than as the repr `float.nan`.
func floatText(v Float) string {
	if s := v.String(); s != "float.nan" {
		return s
	}
	return "nan"
}

// CastMathContent is [ToMathContent] for a place that accepts only real
// content, such as a parameter typed `content`. Strings and symbols still
// convert, being text; anything else is an error rather than a rendering of the
// value's source.
func CastMathContent(v Value) (Content, error) {
	switch v.(type) {
	case Content, Str, *Symbol, None:
		return ToMathContent(v), nil
	}
	return nil, fmt.Errorf("expected content, found %s", v.Type())
}
