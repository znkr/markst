package errcmp

import (
	gocmp "cmp"
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"znkr.io/writst/syntax"
)

type Error struct {
	Span    syntax.Span
	Type    string // "Error" or "Warning"
	Message string
	Hints   []string
}

func Diff(root syntax.RootNode, got []Error) string {
	want := collectErrors(root.Source, root.Children())
	return cmp.Diff(want, got, errcmpopts)
}

var errcmpopts = cmp.Options{
	cmpopts.SortSlices(func(a, b Error) int {
		if n := gocmp.Compare(a.Span.Start, b.Span.Start); n != 0 {
			return n
		}
		if n := gocmp.Compare(a.Span.End, b.Span.End); n != 0 {
			return n
		}
		return gocmp.Compare(a.Message, b.Message)
	}),
}

var errorExpectationRe = regexp.MustCompile(`^// (Error|Warning|Hint): (\d+)(?:-(\d+))?\s+(.+)$`)

func collectErrors(source syntax.Source, ns []syntax.Node) []Error {
	type pendingExpectation struct {
		startCol int
		endCol   int
		typ      string
		message  string
		hints    []string
	}

	var result []Error
	var pending []pendingExpectation

	for _, n := range ns {
		// Read all error expectations from line comments. The format is:
		//   // Error: <start>-<end> <message>
		//   // Error: <start> <message>

		if n.Kind() == syntax.KindLineComment {
			lit := n.Text()
			if match := errorExpectationRe.FindStringSubmatch(lit); match != nil {
				typ := match[1]
				startCol, _ := strconv.Atoi(match[2])
				endCol := startCol
				if match[3] != "" {
					endCol, _ = strconv.Atoi(match[3])
				}
				switch typ {
				case "Error", "Warning":
					pending = append(pending, pendingExpectation{
						typ:      typ,
						message:  match[4],
						startCol: startCol,
						endCol:   endCol,
					})
				case "Hint":
					if len(pending) == 0 {
						panic("no pending error to attach hint to")
					}
					err := &pending[len(pending)-1]
					if err.startCol != startCol || err.endCol != endCol {
						panic(fmt.Sprintf("pending error expected at %d-%d, but hint is at %d-%d", err.startCol, err.endCol, startCol, endCol))
					}
					err.hints = append(err.hints, match[4])
				}
				continue
			}
		}

		// Continue until the first non-trivia node is found. The non-trivia node is the one that
		// the pending expectations refer to (or at least to one node on the same line).
		if syntax.Trivia.Contains(n.Kind()) {
			continue
		}

		// Resolve pending expectations to the current node. The line number is determined by the
		// node's start position.
		line := source.Position(n.Span().Start).Line
		for _, p := range pending {
			result = append(result, Error{
				Span: syntax.Span{
					Start: source.Offset(syntax.Position{Line: line, Column: uint32(p.startCol)}),
					End:   source.Offset(syntax.Position{Line: line, Column: uint32(p.endCol)}),
				},
				Type:    p.typ,
				Message: p.message,
				Hints:   p.hints,
			})
		}
		pending = nil

		if inner, ok := n.(*syntax.Inner); ok {
			result = append(result, collectErrors(source, inner.Children())...)
		}
	}
	if len(pending) > 0 {
		panic(fmt.Sprintf("pending error expectations without corresponding nodes: %+v", pending))
	}
	return result
}
