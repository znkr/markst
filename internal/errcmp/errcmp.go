// Package errcmp compares expected errors (from inline comments in test
// input) against actual errors produced by the analyzer or evaluator.
//
// The [Diff] function extracts these expectations from the syntax tree and
// compares them against the provided actual errors, rendering mismatches
// as a unified diff of the annotated source file.
package errcmp

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"znkr.io/diff/textdiff"
	"znkr.io/markst/syntax"
)

// Error represents an error expectation or actual error, with its source
// span, type ("Error" or "Warning"), message, and optional hints.
type Error struct {
	Span    syntax.Span
	Type    string // "Error" or "Warning"
	Message string
	Hints   []Hint
}

// Hint is one hint on an [Error]. Span is where the hint points, or
// [syntax.NoSpan] to point where the error itself does.
type Hint struct {
	Span syntax.Span
	Msg  string
}

// Diff extracts error expectations from inline comments in root's syntax tree
// and compares them against got. It returns a human-readable diff string
// (empty if they match). Mismatches are shown as a unified diff of the source
// with error annotations as inline comments.
func Diff(root syntax.RootNode, got []Error) string {
	src := string(root.Src)
	gotAnnotated := annotateSource(root.Source, src, got)
	return textdiff.Unified(src, gotAnnotated)
}

var errorExpectationRe = regexp.MustCompile(`^// (Error|Warning|Hint): (\d+(?::\d+)?)(?:-(\d+(?::\d+)?))?\s+(.+)$`)

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
		// A diagnostic with no location (one reported during realization,
		// which has no spans to point at) goes past EOF: there is no line it
		// belongs to.
		target := len(kept)
		if loc := syntax.Locate(source, e.Span); loc.IsValid() {
			for i, kl := range kept {
				if kl.origLine >= loc.Start.Line {
					target = i
					break
				}
			}
		}
		attachedErrs[target] = append(attachedErrs[target], e)
	}

	// Build output.
	var buf strings.Builder
	for i, kl := range kept {
		indent := leadingWhitespace(kl.text)
		// Multi-line spans use line numbers relative to the attached content line.
		baseLine := kl.origLine - 1
		for _, e := range attachedErrs[i] {
			writeError(&buf, source, src, e, indent, baseLine)
		}
		buf.WriteString(kl.text)
		if i < len(kept)-1 {
			buf.WriteByte('\n')
		}
	}

	// Errors past EOF.
	if len(attachedErrs[len(kept)]) > 0 {
		buf.WriteByte('\n')
		var eof uint32
		if len(kept) > 0 {
			eof = kept[len(kept)-1].origLine - 1
		}
		for _, e := range attachedErrs[len(kept)] {
			writeError(&buf, source, src, e, "", eof)
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

// writeError renders an error and its hints as annotation comments, ordered by
// the span each points at. A hint that points before the error it belongs to —
// one naming the callee of a failing call, say — is written above it, which is
// where a reader looking at the source expects to find it.
func writeError(buf *strings.Builder, source syntax.Source, src string, e Error, indent string, baseLine uint32) {
	type annotation struct {
		span syntax.Span
		typ  string
		msg  string
	}
	anns := []annotation{{e.Span, e.Type, e.Message}}
	for _, h := range e.Hints {
		span := e.Span
		if h.Span != syntax.NoSpan {
			span = h.Span
		}
		anns = append(anns, annotation{span, "Hint", h.Msg})
	}
	slices.SortStableFunc(anns, func(a, b annotation) int { return cmp.Compare(a.span.Start, b.span.Start) })
	for _, a := range anns {
		writeComment(buf, indent, a.typ, formatSpan(syntax.Locate(source, a.span), src, baseLine), a.msg)
	}
}

// formatSpan renders loc as the column range golden annotations use, with
// lines counted relative to the line the annotation is attached to. A
// locationless diagnostic renders as column 0, which no real column can be.
func formatSpan(loc syntax.Location, src string, baseLine uint32) string {
	if !loc.IsValid() {
		return "0"
	}
	pos, endPos, span := loc.Start, loc.End, loc.Span

	// A span ending at column 1 of the next line logically belongs to the
	// start line. Only use line:col-line:col for truly multi-line spans.
	crossesLines := span.End > span.Start && endPos.Line > pos.Line
	if crossesLines && (endPos.Line != pos.Line+1 || endPos.Column != 1) {
		return fmt.Sprintf("%d:%d-%d:%d", pos.Line-baseLine, pos.Column, endPos.Line-baseLine, endPos.Column)
	}

	startCol, endCol := pos.Column, endPos.Column
	if crossesLines {
		// Fold the end back onto the start line. Columns count runes, so it
		// is the span's rune length that gets added, not its byte length.
		endCol = startCol + uint32(utf8.RuneCountInString(src[span.Start:span.End]))
	}
	if startCol == endCol {
		return fmt.Sprintf("%d", startCol)
	}
	return fmt.Sprintf("%d-%d", startCol, endCol)
}

func writeComment(buf *strings.Builder, indent, typ, span, msg string) {
	fmt.Fprintf(buf, "%s// %s: %s %s\n", indent, typ, span, msg)
}
