package parser_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/writst/syntax/internal/format"
	"znkr.io/writst/syntax/parser"
)

var update = flag.Bool("update", false, "update golden files")

func TestParse(t *testing.T) {
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		runTestFile(t, file)
	}
}

type testCase struct {
	name  string
	input string
	want  string
}

type state int

const (
	statePreamble state = iota
	stateBody
	stateExpectation
)

func runTestFile(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var cases []*testCase
	lineno := 0
	state := statePreamble
	var name string
	var input strings.Builder
	var want strings.Builder
	commit := func() {
		if name == "" {
			return
		}
		cases = append(cases, &testCase{
			name:  name,
			input: input.String(),
			want:  want.String(),
		})
		name = ""
		input.Reset()
		want.Reset()
	}
	for line := range strings.Lines(string(content)) {
		lineno++
	Start:
		switch state {
		case statePreamble:
			if line == "" {
				continue
			}
			n, ok := strings.CutPrefix(line, "§ ")
			if !ok {
				t.Fatalf("invalid test file format in %s:%d: expected test case header starting with `§ `, got %q", path, lineno, line)
			}
			name = strings.TrimSpace(n)
			state = stateBody
			continue
		case stateBody:
			if strings.HasPrefix(line, "│ ") {
				state = stateExpectation
				goto Start
			}
			if strings.HasPrefix(line, "§ ") {
				commit()
				state = statePreamble
				goto Start
			}
			input.WriteString(line)
		case stateExpectation:
			if strings.TrimSpace(line) == "" {
				commit()
				state = statePreamble
				continue
			}
			after, ok := strings.CutPrefix(line, "│ ")
			if !ok {
				t.Fatalf("invalid test file format in %s:%d: expected expectation line starting with `│ `, got %q", path, lineno, line)
			}
			want.WriteString(after)
		}
	}
	if name != "" {
		commit()
	}

	for _, tc := range cases {
		t.Run(filepath.Base(path)+"/"+tc.name, func(t *testing.T) {
			node := parser.Parse(tc.input)
			got := format.Format(node)

			if diff := textdiff.Unified(tc.want, got); diff != "" {
				if *update {
					tc.want = got
					return
				}
				t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
			}
		})
	}

	if *update {
		if err := writeTestFile(path, cases); err != nil {
			t.Fatal(err)
		}
	}
}

func writeTestFile(path string, cases []*testCase) error {
	var sb strings.Builder
	for i, tc := range cases {
		if i > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "§ %s\n", tc.name)
		sb.WriteString(tc.input)
		for line := range strings.Lines(tc.want) {
			sb.WriteString("│ " + line)
		}
	}
	return os.WriteFile(path, []byte(sb.String()), 0644)
}
