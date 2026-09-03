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

// Package testfile reads and updates golden test files used by the parser,
// analyzer, and evaluator tests.
//
// Test files use this format:
//
//	--- test-name ---
//	input here
//	│ expected output here
//	│ (each line prefixed with "│ ")
//
// A test case consists of a header line (--- name ---), input lines, and
// optional expected output lines prefixed with "│ ". Test names must match
// [a-z][a-z0-9-]* and may include a skip annotation: --- name (skip: reason) ---.
package testfile

import (
	"bufio"
	"bytes"
	"io"
	"iter"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Test is one case from a golden test file.
type Test struct {
	Name  string // Test name from the header line.
	Skip  string // Skip reason, if any.
	Input string // Input source text.
	Want  string // Expected output (empty for error-only tests).
}

var testcase = regexp.MustCompile(`^--- (.+) ---$`)
var header = regexp.MustCompile(`^([a-z][a-z0-9-]*)(?: +\(skip: (.*)\))?$`)

// Read returns the test cases in the golden file at path. It fails the test on
// an unreadable file or a malformed one.
func Read(t testing.TB, path string) []Test {
	t.Helper()
	var tests []Test
	var name string
	var skip string
	var input []string
	var want []string
	commit := func() {
		if name == "" {
			return
		}
		if len(want) == 0 {
			for len(input) > 0 && input[len(input)-1] == "" {
				input = input[:len(input)-1]
			}
		}
		test := Test{
			Name:  name,
			Skip:  skip,
			Input: strings.Join(input, "\n"),
			Want:  strings.Join(want, "\n"),
		}
		if len(input) > 0 {
			test.Input += "\n"
		}
		if len(want) > 0 {
			test.Want += "\n"
		}
		tests = append(tests, test)
		input = input[:0]
		want = want[:0]
		skip = ""
	}
	for token := range scan(t, path) {
		switch token.kind {
		case kindSpace, kindComment:
			continue
		case kindHeader:
			commit()
			m := header.FindStringSubmatch(token.text)
			if m == nil {
				t.Fatalf("invalid test file format in %s:%d: test case name must match %s, got %q", path, token.lineno, header, token.text)
			}
			name = m[1]
			if len(m) > 2 {
				skip = m[2]
			}
		case kindInput:
			input = append(input, token.text)
		case kindWant:
			want = append(want, token.text)
		}
	}
	if name != "" {
		commit()
	}
	return tests
}

// Update writes the expected output of tests back into the golden file at
// path, leaving the inputs and the preamble as they were. The test names must
// be the ones already in the file.
func Update(t testing.TB, path string, tests []Test) {
	t.Helper()
	var buf bytes.Buffer
	cur := 0
	seenTest := false
	for token := range scan(t, path) {
		switch token.kind {
		case kindSpace, kindComment:
			if !seenTest {
				buf.WriteString(token.text)
				buf.WriteString("\n")
			}
		case kindHeader:
			header := tests[cur].Name
			if tests[cur].Skip != "" {
				header += " (skip: " + tests[cur].Skip + ")"
			}
			if token.text != header {
				t.Fatalf("test case name mismatch when updating %s: expected %q, got %q", path, header, token.text)
			}
			if seenTest {
				buf.WriteString("\n")
			}
			seenTest = true
			writeTest(&buf, tests[cur])
			cur++
		}
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeTest(buf *bytes.Buffer, test Test) {
	var skip string
	if test.Skip != "" {
		skip = " (skip: " + test.Skip + ")"
	}
	buf.WriteString("--- " + test.Name + skip + " ---\n")
	buf.WriteString(test.Input)
	if !strings.HasSuffix(test.Input, "\n") {
		buf.WriteString("\n")
	}
	for line := range strings.Lines(test.Want) {
		buf.WriteString("│ " + line)
	}
}

type kind int

const (
	kindSpace kind = iota
	kindComment
	kindHeader
	kindInput
	kindWant
)

type token struct {
	kind   kind
	text   string
	lineno int
}

func scan(t testing.TB, path string) iter.Seq[token] {
	t.Helper()
	return func(yield func(token) bool) {
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()

		r := bufio.NewReader(file)

		const (
			preamble = iota
			body
			expectation
		)
		state := preamble
		lineno := 0
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if line == "" {
						break
					}
				} else {
					t.Fatal(err)
				}
			}
			lineno++
			line = strings.TrimRight(line, "\r\n")

		Start:
			switch state {
			case preamble:
				if line == "" {
					if !yield(token{kind: kindSpace, text: line}) {
						return
					}
					continue
				}
				if strings.HasPrefix(line, "//") {
					if !yield(token{kind: kindComment, text: line}) {
						return
					}
					continue
				}
				m := testcase.FindStringSubmatch(line)
				if m == nil {
					t.Fatalf("invalid test file format in %s:%d: expected test case header matching %s, got %q", path, lineno, testcase, line)
				}
				if !yield(token{kind: kindHeader, text: m[1], lineno: lineno}) {
					return
				}
				state = body
			case body:
				if strings.HasPrefix(line, "│ ") {
					state = expectation
					goto Start
				}
				if testcase.MatchString(line) {
					state = preamble
					goto Start
				}
				if !yield(token{kind: kindInput, text: line, lineno: lineno}) {
					return
				}
			case expectation:
				if line == "" {
					state = preamble
					goto Start
				}
				after, ok := strings.CutPrefix(line, "│ ")
				if !ok {
					t.Fatalf("invalid test file format in %s:%d: expected expectation line starting with `│ `, got %q", path, lineno, line)
				}
				if !yield(token{kind: kindWant, text: after, lineno: lineno}) {
					return
				}
			}
		}
	}
}
