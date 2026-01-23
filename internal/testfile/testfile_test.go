package testfile

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func TestRead(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		tests []Test
	}{
		{
			name: "sample",
			file: "testdata/sample.test",
			tests: []Test{
				{Name: "simple", Input: "hello world\n", Want: "HELLO WORLD\n"},
				{Name: "multi-line", Input: "line one\nline two\nline three\n", Want: "LINE ONE\nLINE TWO\nLINE THREE\n"},
				{Name: "empty-input", Input: "\n", Want: "empty\n"},
				{Name: "with-spaces", Input: "  indented input\n", Want: "  INDENTED OUTPUT\n"},
			},
		},
		{
			name: "no-want",
			file: "testdata/no-want.test",
			tests: []Test{
				{Name: "has-want", Input: "input one\n", Want: "expected one\n"},
				{Name: "no-want", Input: "input two\n", Want: ""},
				{Name: "also-no-want", Input: "line one\nline two\n", Want: ""},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Read(t, tc.file)

			if len(got) != len(tc.tests) {
				t.Fatalf("expected %d tests, got %d", len(tc.tests), len(got))
			}

			for i, want := range tc.tests {
				if got[i].Name != want.Name {
					t.Errorf("test %d: expected name %q, got %q", i, want.Name, got[i].Name)
				}
				if got[i].Input != want.Input {
					t.Errorf("test %d (%s): expected input %q, got %q", i, want.Name, want.Input, got[i].Input)
				}
				if got[i].Want != want.Want {
					t.Errorf("test %d (%s): expected want %q, got %q", i, want.Name, want.Want, got[i].Want)
				}
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	cases := []struct {
		name   string
		file   string
		golden string // if empty, compare against source (no-op test)
		modify func([]Test)
	}{
		{
			name:   "modify-want",
			file:   "testdata/sample.test",
			golden: "testdata/sample.golden",
			modify: func(tests []Test) {
				tests[0].Want = "MODIFIED OUTPUT\n"
			},
		},
		{
			name:   "add-want",
			file:   "testdata/no-want.test",
			golden: "testdata/no-want.golden",
			modify: func(tests []Test) {
				tests[1].Want = "new expected two\n"
				tests[2].Want = "new line one\nnew line two\n"
			},
		},
		{
			name:   "idempotent",
			file:   "testdata/sample.test",
			golden: "", // test idempotency: run Update twice
			modify: func(tests []Test) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}

			tmpFile := filepath.Join(t.TempDir(), "test.file")
			if err := os.WriteFile(tmpFile, src, 0644); err != nil {
				t.Fatal(err)
			}

			tests := Read(t, tmpFile)
			tc.modify(tests)
			Update(t, tmpFile, tests)

			got, err := os.ReadFile(tmpFile)
			if err != nil {
				t.Fatal(err)
			}

			if tc.golden == "" {
				// Idempotency test: run Update again, output should be identical
				tests2 := Read(t, tmpFile)
				Update(t, tmpFile, tests2)
				got2, err := os.ReadFile(tmpFile)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != string(got2) {
					t.Errorf("Update not idempotent:\nfirst:\n%s\nsecond:\n%s", got, got2)
				}
				return
			}

			if *update {
				if err := os.WriteFile(tc.golden, got, 0644); err != nil {
					t.Fatal(err)
				}
			}

			want, err := os.ReadFile(tc.golden)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != string(want) {
				t.Errorf("output mismatch:\nwant:\n%s\ngot:\n%s", want, got)
			}
		})
	}
}
