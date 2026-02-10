package errcmp

import (
	gocmp "cmp"
	"fmt"
	"regexp"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"znkr.io/writst/ir"
	"znkr.io/writst/syntax"
)

func Diff(root syntax.RootNode, err error) string {
	want := collectErrors(root.Source, root.Children())
	var got []cmpError
	switch err := err.(type) {
	case nil:
		// no error
	case syntax.ErrorList:
		for _, err := range err {
			got = append(got, cmpError{
				Span:    err.Span(),
				Message: err.Error(),
				Hints:   err.Hints(),
			})
		}
	case *syntax.Error:
		got = append(got, cmpError{
			Span:    err.Span(),
			Message: err.Error(),
			Hints:   err.Hints(),
		})
	case ir.Error:
		got = append(got, cmpError{
			Span:    err.Span(),
			Message: err.Error(),
			Hints:   err.Hints(),
		})
	default:
		panic(fmt.Sprintf("unexpected error type: %T", err))
	}
	return cmp.Diff(want, got, errcmpopts)
}

var errcmpopts = cmp.Options{
	cmpopts.SortSlices(func(a, b cmpError) int {
		if n := gocmp.Compare(a.Span.Start, b.Span.Start); n != 0 {
			return n
		}
		if n := gocmp.Compare(a.Span.End, b.Span.End); n != 0 {
			return n
		}
		return gocmp.Compare(a.Message, b.Message)
	}),
}

type cmpError struct {
	Span    syntax.Span
	Message string
	Hints   []string
}

var errorExpectationRe = regexp.MustCompile(`^// (Error|Hint): (\d+)-(\d+)\s+(.+)$`)

func collectErrors(source syntax.Source, ns []syntax.Node) []cmpError {
	type pendingExpectation struct {
		startCol int
		endCol   int
		message  string
		hints    []string
	}

	var result []cmpError
	var pending []pendingExpectation

	for _, n := range ns {
		// Read all error expectations from line comments. The format is:
		//   // Error: <start>-<end> <message>
		if n.Kind() == syntax.KindLineComment {
			lit := n.Text()
			if match := errorExpectationRe.FindStringSubmatch(lit); match != nil {
				typ := match[1]
				startCol, _ := strconv.Atoi(match[2])
				endCol, _ := strconv.Atoi(match[3])
				switch typ {
				case "Error":
					pending = append(pending, pendingExpectation{
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
			result = append(result, cmpError{
				Span: syntax.Span{
					Start: source.Offset(syntax.Position{Line: line, Column: uint32(p.startCol)}),
					End:   source.Offset(syntax.Position{Line: line, Column: uint32(p.endCol)}),
				},
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
