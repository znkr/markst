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

package markst_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"znkr.io/markst"
)

// TestCompileStopsOnError covers the documents that used to run forever. An
// error is an ordinary value in markst, propagated along data flow, so a loop
// whose condition does not read the failing value has nothing to stop it. Each
// source here is one shape of that: the loop condition never becomes false, and
// what ends the compile is the diagnostic rather than the condition.
//
// The deadline is the test's own failure mode. A source that runs away hits it
// and reports the cancellation instead of the diagnostic, which is the failure
// this test is here to catch.
func TestCompileStopsOnError(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want string
	}{{
		// `i++` is not markst syntax: it parses as `i + (+‹error›)`, so `i` is
		// never incremented and `ps` stops growing once 3 is found composite.
		name: "syntax error in a loop body",
		src: `#let primes(n) = {
  let ps = (2,)
  let i = 3
  while ps.len() < n {
    let prime = true
    for p in ps {
      if calc.rem(i, p) == 0 {
        prime = false
        break
      }
    }
    if prime {
      ps.push(i)
    }
    i++
  }
  ps.map(str).join(", ")
}
#primes(5)
`,
		want: "expected expression",
	}, {
		name: "runtime error in a loop body",
		src:  "#{\n  let i = 0\n  while i < 100000000 {\n    (1, 2).at(5)\n  }\n}\n",
		want: "array index out of bounds",
	}, {
		// A continue reaches the loop header without passing the ordinary back
		// edge, so it carries the check of its own.
		name: "runtime error before a continue",
		src:  "#while true {\n  unknown\n  continue\n}\n",
		want: "unknown variable: unknown",
	}} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, _, err := markst.Compile(ctx, []byte(tt.src))
			if err == nil {
				t.Fatal("Compile() = nil, want an error")
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Compile() ran until the deadline: %v", err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Compile() = %q, want it to mention %q", err, tt.want)
			}
		})
	}
}

// TestCompileStopsAtSyntaxError covers a source that does not parse: it is not
// evaluated at all, so the only diagnostics are the parse errors themselves.
func TestCompileStopsAtSyntaxError(t *testing.T) {
	_, _, err := markst.Compile(t.Context(), []byte("#let x = 1\n#(\n#nope\n"))
	var diags markst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T, want markst.DiagnosticList", err)
	}
	for _, d := range diags {
		if strings.Contains(d.Msg, "unknown variable") {
			t.Errorf("Compile() reported %q, want parse errors only", d.Msg)
		}
	}
}

// TestCompileCancel covers a loop nothing in the document can stop. Bounding it
// is the caller's job, and the error carries the context's own so errors.Is
// reaches it.
func TestCompileCancel(t *testing.T) {
	t.Run("deadline", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		_, _, err := markst.Compile(ctx, []byte("#while true { }\n"))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Compile() = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, _, err := markst.Compile(ctx, []byte("= Heading\n\nSome text.\n"))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Compile() = %v, want context.Canceled", err)
		}
	})

	t.Run("recursion", func(t *testing.T) {
		// Recursion is capped by the call-depth limit rather than by the
		// context, but the entry check has to unwind the frames already
		// running.
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		src := "#let f() = { while true { } }\n#f()\n"
		_, _, err := markst.Compile(ctx, []byte(src))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Compile() = %v, want context.DeadlineExceeded", err)
		}
	})

	t.Run("not canceled", func(t *testing.T) {
		// A context cancelled after the compile finished is not an abort.
		ctx, cancel := context.WithCancel(t.Context())
		doc, _, err := markst.Compile(ctx, []byte("= Heading\n\nSome text.\n"))
		cancel()
		if err != nil {
			t.Fatalf("Compile() = %v", err)
		}
		if doc == nil {
			t.Error("Compile() document is nil")
		}
	})
}
