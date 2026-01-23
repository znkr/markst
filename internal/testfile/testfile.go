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

type Test struct {
	Name  string
	Input string
	Want  string
}

var testcase = regexp.MustCompile(`^--- (.+) ---$`)
var testname = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func Read(t *testing.T, path string) []Test {
	t.Helper()
	var tests []Test
	var name string
	var input strings.Builder
	var want strings.Builder
	commit := func() {
		if name == "" {
			return
		}
		tests = append(tests, Test{
			Name:  name,
			Input: input.String(),
			Want:  want.String(),
		})
		input.Reset()
		want.Reset()
	}
	for token := range scan(t, path) {
		switch token.kind {
		case kindSpace, kindComment:
			continue
		case kindName:
			commit()
			name = token.text
		case kindInput:
			input.WriteString(token.text)
			input.WriteString("\n")
		case kindWant:
			want.WriteString(token.text)
			want.WriteString("\n")
		}
	}
	if name != "" {
		commit()
	}
	return tests
}

func Update(t *testing.T, path string, tests []Test) {
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
		case kindName:
			if tests[cur].Name != token.text {
				t.Fatalf("test case name mismatch when updating %s: expected %q, got %q", path, tests[cur].Name, token.text)
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
	buf.WriteString("--- " + test.Name + " ---\n")
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
	kindName
	kindInput
	kindWant
)

type token struct {
	kind kind
	text string
}

func scan(t *testing.T, path string) iter.Seq[token] {
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
					break
				}
				t.Fatal(err)
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
				if !testname.MatchString(m[1]) {
					t.Fatalf("invalid test file format in %s:%d: test case name must match %s, got %q", path, lineno, testname, m[1])
				}
				if !yield(token{kind: kindName, text: m[1]}) {
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
				if !yield(token{kind: kindInput, text: line}) {
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
				if !yield(token{kind: kindWant, text: after}) {
					return
				}
			}
		}
	}
}
