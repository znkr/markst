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

package html_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/markst"
	"znkr.io/markst/html"
	"znkr.io/markst/internal/testfile"
	"znkr.io/markst/value"
)

// FuzzRender renders arbitrary documents. Rendering is total by contract:
// every content element has an HTML form, so the only failures Render is
// allowed to report come from a hook or from the writer, and there are neither
// here. Any panic, any error, and any output that does not balance is a bug.
//
// A document that does not compile is still rendered: Compile returns what it
// managed to build alongside the diagnostics, and that partial document is
// exactly the kind a presenter must not choke on.
//
// A *panic* out of Compile is somebody else's bug and is skipped rather than
// failed. There is no fuzz target over the evaluator yet — znkr.io/markst's
// FuzzAnalyze stops at the analyzer — so this one reaches code no fuzzer has
// covered before, and failing here would report those crashes as renderer
// failures. The parser's missing recursion-depth guard (IDEAS.md) is not
// skippable that way — it is a fatal stack overflow — and shows up here as a
// worker that dies during minimization, exactly as it does for FuzzAnalyze.
func FuzzRender(f *testing.F) {
	for _, glob := range []string{"testdata/*.test", "../testdata/*/*.test"} {
		files, err := filepath.Glob(glob)
		if err != nil {
			f.Fatal(err)
		}
		for _, file := range files {
			for _, tc := range testfile.Read(f, file) {
				if tc.Skip == "" {
					f.Add(tc.Input)
				}
			}
		}
	}

	f.Fuzz(func(t *testing.T, src string) {
		doc := compileForFuzz(t, src)
		if doc == nil {
			return
		}
		var b strings.Builder
		if err := html.Render(&b, doc); err != nil {
			t.Fatalf("Render() = %v, want no error", err)
		}
		if err := checkBalanced(b.String()); err != nil {
			t.Fatalf("%v\nsource:\n%s\noutput:\n%s", err, src, b.String())
		}
	})
}

// compileForFuzz compiles src, skipping the case when the compiler panics.
func compileForFuzz(t *testing.T, src string) (doc *value.Document) {
	defer func() {
		if r := recover(); r != nil {
			doc = nil
			t.Skipf("compiling panicked, which is not this package's bug: %v", r)
		}
	}()
	doc, _, _ = markst.Compile(t.Context(), []byte(src))
	return doc
}

func TestCheckBalanced(t *testing.T) {
	for _, tc := range []struct {
		in string
		ok bool
	}{
		{"<p>a</p>", true},
		{"<p>a<br>b</p>", true},
		{"<p>a", false},
		{"a</p>", false},
		{"<p><em>a</p></em>", false},
		{"plain text", true},
	} {
		err := checkBalanced(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("checkBalanced(%q) = %v, want ok=%v", tc.in, err, tc.ok)
		}
	}
}

// checkBalanced reports whether every tag in s is closed, in order. It can scan
// naively because the renderer escapes < and > everywhere they are not markup,
// attribute values included, so the first > after a < always ends that tag.
func checkBalanced(s string) error {
	var stack []string
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			break
		}
		s = s[i+1:]
		j := strings.IndexByte(s, '>')
		if j < 0 {
			return fmt.Errorf("unterminated tag at %q", trunc(s))
		}
		tag := s[:j]
		s = s[j+1:]

		closing := strings.HasPrefix(tag, "/")
		tag = strings.TrimPrefix(tag, "/")
		if k := strings.IndexAny(tag, " \t\n"); k >= 0 {
			if closing {
				return fmt.Errorf("closing tag with attributes: </%s>", tag)
			}
			tag = tag[:k]
		}
		if tag == "" || strings.ContainsAny(tag, `"'=/`) {
			return fmt.Errorf("malformed tag <%s>", tag)
		}

		switch {
		case closing:
			if len(stack) == 0 {
				return fmt.Errorf("closing tag </%s> with nothing open", tag)
			}
			if top := stack[len(stack)-1]; top != tag {
				return fmt.Errorf("closing tag </%s> where </%s> was due", tag, top)
			}
			stack = stack[:len(stack)-1]
		case value.HtmlTagVoid(tag):
			// No body and no closing tag.
		default:
			stack = append(stack, tag)
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("unclosed tags: %s", strings.Join(stack, ", "))
	}
	return nil
}

func trunc(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
