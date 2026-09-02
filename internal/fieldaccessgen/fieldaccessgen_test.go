package main

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerate(t *testing.T) {
	got, err := generate("../../value/content.go", "../../value/style.go", "../../value/state.go", "../../value/custom.go")
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("../../value/content_gen.go")
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("differences between generated file and checked in file detected:\n%s\nForgot to run go generate?", diff)
	}
}
