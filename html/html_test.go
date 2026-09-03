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
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/diff/textdiff"
	"znkr.io/markst"
	"znkr.io/markst/html"
	"znkr.io/markst/internal/testfile"
)

var update = flag.Bool("update", false, "update golden files")

// render compiles src and renders it, failing the test on any diagnostic: a
// golden file records what correct source renders as, so anything the compiler
// has to say about the source is a mistake in the test, not an expectation.
func render(t *testing.T, src string, opts ...html.Option) string {
	t.Helper()
	doc, diags, err := markst.Compile(t.Context(), []byte(src), markst.WithName("test.mst"))
	if err != nil {
		t.Fatalf("compiling:\n%s", formatDiags(err.(markst.DiagnosticList)))
	}
	if len(diags) > 0 {
		t.Fatalf("unexpected diagnostics:\n%s", formatDiags(diags))
	}
	var b strings.Builder
	if err := html.Render(&b, doc, opts...); err != nil {
		t.Fatalf("rendering: %v", err)
	}
	return b.String()
}

func formatDiags(diags []markst.Diagnostic) string {
	var b strings.Builder
	markst.FormatDiagnostics(&b, diags)
	return b.String()
}

func TestRender(t *testing.T) {
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no golden files found")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			tests := testfile.Read(t, file)
			for i, tc := range tests {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}
					got := render(t, tc.Input)
					if *update {
						tests[i].Want = got
						return
					}
					if diff := textdiff.Unified(tc.Want, got); diff != "" {
						t.Errorf("Render() mismatch (-want +got):\n%s", diff)
					}
				})
			}
			if *update {
				testfile.Update(t, file, tests)
			}
		})
	}
}
