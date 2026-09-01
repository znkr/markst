package smartquote_test

import (
	"strings"
	"testing"

	"znkr.io/markst/name"
	"znkr.io/markst/smartquote"
	"znkr.io/markst/value"
)

// present renders content the way a document presenter would: every element is
// handed to the quoter, and the presenter writes its own output for the ones
// the quoter has nothing to substitute.
func present(items ...value.Content) string {
	var q smartquote.Quoter
	var sb strings.Builder
	for _, it := range items {
		sb.WriteString(q.Advance(it))
		switch c := it.(type) {
		case *value.Text:
			sb.WriteString(c.Text)
		case *value.Raw:
			sb.WriteString(c.Text)
		case *value.MathText:
			sb.WriteString(c.Text)
		case *value.Ref:
			sb.WriteString(c.Target.String())
		case *value.Linebreak:
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// quoted parses a shorthand where ' and " stand for smart quotes and every
// other run is text, so a test case reads like the markup it comes from.
func quoted(s string) []value.Content {
	var items []value.Content
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			items = append(items, &value.Text{Text: text.String()})
			text.Reset()
		}
	}
	for _, r := range s {
		switch r {
		case '\'', '"':
			flush()
			items = append(items, &value.SmartQuote{Double: r == '"'})
		default:
			text.WriteRune(r)
		}
	}
	flush()
	return items
}

func TestQuoter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "sentence",
			in:   `"The horse eats no cucumber salad" was uttered on the 'telephone.'`,
			want: `“The horse eats no cucumber salad” was uttered on the ‘telephone.’`,
		},
		{
			name: "apostrophe",
			in:   `The 5'11" 'quick' brown fox jumps over the "lazy" dog's ear.`,
			want: `The 5′11″ ‘quick’ brown fox jumps over the “lazy” dog’s ear.`,
		},
		{
			name: "apostrophe-mid-word",
			in:   `I'm sure it's fine.`,
			want: `I’m sure it’s fine.`,
		},
		{
			name: "nesting",
			in:   `"'test statement'"`,
			want: `“‘test statement’”`,
		},
		{
			name: "nesting-leading",
			in:   `"'test' statement"`,
			want: `“‘test’ statement”`,
		},
		{
			name: "nesting-trailing",
			in:   `"statement 'test'"`,
			want: `“statement ‘test’”`,
		},
		{
			name: "prime",
			in:   `A 2" nail.`,
			want: `A 2″ nail.`,
		},
		{
			// A quotation of the same kind is already open, so this closes it
			// rather than priming — a prime only wins where a quote can't.
			name: "prime-loses-to-a-matching-close",
			in:   `"A 2" nail."`,
			want: `“A 2” nail.“`,
		},
		{
			name: "prime-inside-single-quotation",
			in:   `'A 2" nail.'`,
			want: `‘A 2″ nail.’`,
		},
		{
			name: "bracket-opens",
			in:   `"a ["b"] c"`,
			want: `“a [“b”] c”`,
		},
		{
			name: "slash-opens",
			in:   `"Hello"/"World"`,
			want: `“Hello”/“World”`,
		},
		{
			name: "start-of-block-opens",
			in:   `"quoted"`,
			want: `“quoted”`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := present(quoted(tt.in)...); got != tt.want {
				t.Errorf("present(%q):\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestQuoterBlockResets checks that a quotation left open by one block does not
// leak into the next one, and that the quoter derives that from the content
// rather than from the presenter telling it.
func TestQuoterBlockResets(t *testing.T) {
	dquo := &value.SmartQuote{Double: true}
	for _, block := range []value.Content{
		&value.Par{Body: &value.Text{Text: "x"}},
		&value.Heading{Depth: 1, Body: &value.Text{Text: "x"}},
		&value.ListItem{Body: &value.Text{Text: "x"}},
		&value.Parbreak{},
	} {
		t.Run(block.Name(), func(t *testing.T) {
			var q smartquote.Quoter
			if got, want := q.Advance(dquo), smartquote.DoubleOpen; got != want {
				t.Fatalf("Advance(quote) = %q, want %q", got, want)
			}
			q.Advance(block)
			q.Advance(&value.Text{Text: "word"})
			if got, want := q.Advance(dquo), smartquote.DoubleOpen; got != want {
				t.Errorf("Advance(quote) after %s = %q, want %q (the open quotation should not carry over)", block.Name(), got, want)
			}
		})
	}
}

// TestQuoterInlineDoesNotReset is the other half: an inline wrapper or a line
// break inside a paragraph leaves the open quotation alone.
func TestQuoterInlineDoesNotReset(t *testing.T) {
	for _, inline := range []value.Content{
		&value.Strong{Body: &value.Text{Text: "x"}},
		&value.Linebreak{},
	} {
		t.Run(inline.Name(), func(t *testing.T) {
			var q smartquote.Quoter
			q.Advance(&value.SmartQuote{Double: true})
			q.Advance(inline)
			q.Advance(&value.Text{Text: "word"})
			got := q.Advance(&value.SmartQuote{Double: true})
			if want := smartquote.DoubleClose; got != want {
				t.Errorf("Advance(quote) after %s = %q, want %q (the quotation is still open)", inline.Name(), got, want)
			}
		})
	}
}

// TestQuoterFollowsVisibleText checks the inline elements that render as text
// without being one: a quote right behind them reads them, not the space in
// front of them.
func TestQuoterFollowsVisibleText(t *testing.T) {
	mathX := &value.MathText{Text: "x"}
	tests := []struct {
		name string
		in   []value.Content
		want string
	}{
		{
			name: "inline-raw",
			in: []value.Content{
				&value.Text{Text: "A "},
				&value.Raw{Text: "dog"},
				&value.SmartQuote{},
				&value.Text{Text: "s ear."},
			},
			want: "A dog\u2019s ear.",
		},
		{
			// A walker hands over the equation and then descends into it, so the
			// math text arrives as its own element.
			name: "math",
			in: []value.Content{
				&value.Text{Text: "The "},
				&value.Equation{Body: mathX},
				mathX,
				&value.SmartQuote{},
				&value.Text{Text: "s value."},
			},
			want: "The x\u2019s value.",
		},
		{
			name: "ref",
			in: []value.Content{
				&value.Text{Text: "See "},
				&value.Ref{Target: name.Make("fig")},
				&value.SmartQuote{},
				&value.Text{Text: "s caption."},
			},
			want: "See fig\u2019s caption.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := present(tt.in...); got != tt.want {
				t.Errorf("present() = %q, want %q", got, tt.want)
			}
		})
	}
}
