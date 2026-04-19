// Package errcmp compares expected errors (from inline comments in test
// input) against actual errors produced by the analyzer or evaluator.
//
// The [Diff] function extracts these expectations from the syntax tree and
// compares them against the provided actual errors, rendering mismatches
// as a unified diff of the annotated source file.
package errcmp

import (
	"fmt"
	"regexp"
	"strings"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/syntax"
)

// Error represents an error expectation or actual error, with its source
// span, type ("Error" or "Warning"), message, and optional hints.
type Error struct {
	Span    syntax.Span
	Type    string // "Error" or "Warning"
	Message string
	Hints   []string
}

// Diff extracts error expectations from inline comments in root's syntax tree
// and compares them against got. It returns a human-readable diff string
// (empty if they match). Mismatches are shown as a unified diff of the source
// with error annotations as inline comments.
func Diff(root syntax.RootNode, got []Error) string {
	src := root.Text()
	gotAnnotated := annotateSource(root.Source, src, got)
	return textdiff.Unified(src, gotAnnotated)
}

var errorExpectationRe = regexp.MustCompile(`^// (Error|Warning|Hint): (\d+)(?:-(\d+))?\s+(.+)$`)

// annotateSource strips existing error comments and inserts new ones based on errs.
func annotateSource(source syntax.Source, src string, errs []Error) string {
	lines := strings.Split(src, "\n")

	// Keep non-comment lines with their original line numbers.
	type keptLine struct {
		origLine uint32
		text     string
	}
	var kept []keptLine
	for i, line := range lines {
		if !errorExpectationRe.MatchString(strings.TrimSpace(line)) {
			kept = append(kept, keptLine{uint32(i + 1), line})
		}
	}

	// Attach each error to a kept line (first kept line >= error's line, or past-EOF).
	attachedErrs := make([][]Error, len(kept)+1)
	for _, e := range errs {
		errLine := source.Position(e.Span.Start).Line
		target := len(kept)
		for i, kl := range kept {
			if kl.origLine >= errLine {
				target = i
				break
			}
		}
		attachedErrs[target] = append(attachedErrs[target], e)
	}

	// Build output.
	var buf strings.Builder
	for i, kl := range kept {
		indent := leadingWhitespace(kl.text)
		for _, e := range attachedErrs[i] {
			writeError(&buf, source, e, indent)
		}
		buf.WriteString(kl.text)
		if i < len(kept)-1 {
			buf.WriteByte('\n')
		}
	}

	// Errors past EOF.
	if len(attachedErrs[len(kept)]) > 0 {
		buf.WriteByte('\n')
		for _, e := range attachedErrs[len(kept)] {
			writeError(&buf, source, e, "")
		}
	}

	return buf.String()
}

func leadingWhitespace(s string) string {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return s[:i]
		}
	}
	return ""
}

func writeError(buf *strings.Builder, source syntax.Source, e Error, indent string) {
	pos := source.Position(e.Span.Start)
	endPos := source.Position(e.Span.End)

	startCol, endCol := pos.Column, endPos.Column
	if e.Span.End > e.Span.Start && endPos.Line > pos.Line && endPos.Column == 1 {
		endCol = startCol + (e.Span.End - e.Span.Start)
	}

	writeComment(buf, indent, e.Type, startCol, endCol, e.Message)
	for _, h := range e.Hints {
		writeComment(buf, indent, "Hint", startCol, endCol, h)
	}
}

func writeComment(buf *strings.Builder, indent, typ string, startCol, endCol uint32, msg string) {
	if startCol == endCol {
		fmt.Fprintf(buf, "%s// %s: %d %s\n", indent, typ, startCol, msg)
	} else {
		fmt.Fprintf(buf, "%s// %s: %d-%d %s\n", indent, typ, startCol, endCol, msg)
	}
}
