package syntax_test

import (
	"testing"

	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/parser"
)

// locSource is the document Locate is exercised against. Line 2 holds a
// multibyte rune so column counting is visibly in runes, not bytes.
const locSource = "hello\nwörld\nbye\n"

func TestLocate(t *testing.T) {
	src := parser.Parse([]byte(locSource)).Source

	tests := []struct {
		name string
		span syntax.Span
		want syntax.Location
	}{
		{
			name: "empty_span_at_start",
			span: syntax.Span{Start: 0, End: 0},
			want: syntax.Location{
				Span:  syntax.Span{Start: 0, End: 0},
				Start: syntax.Position{Line: 1, Column: 1},
				End:   syntax.Position{Line: 1, Column: 1},
			},
		},
		{
			name: "within_one_line",
			span: syntax.Span{Start: 0, End: 5}, // "hello"
			want: syntax.Location{
				Span:  syntax.Span{Start: 0, End: 5},
				Start: syntax.Position{Line: 1, Column: 1},
				End:   syntax.Position{Line: 1, Column: 6},
			},
		},
		{
			name: "columns_count_runes",
			span: syntax.Span{Start: 6, End: 12}, // "wörld", ö is two bytes
			want: syntax.Location{
				Span:  syntax.Span{Start: 6, End: 12},
				Start: syntax.Position{Line: 2, Column: 1},
				End:   syntax.Position{Line: 2, Column: 6},
			},
		},
		{
			name: "spans_several_lines",
			span: syntax.Span{Start: 0, End: 16}, // "hello\nwörld\nbye"
			want: syntax.Location{
				Span:  syntax.Span{Start: 0, End: 16},
				Start: syntax.Position{Line: 1, Column: 1},
				End:   syntax.Position{Line: 3, Column: 4},
			},
		},
		{
			name: "end_of_input",
			span: syntax.Span{Start: 17, End: 17}, // len(locSource)
			want: syntax.Location{
				Span:  syntax.Span{Start: 17, End: 17},
				Start: syntax.Position{Line: 4, Column: 1}, // the source ends in a newline
				End:   syntax.Position{Line: 4, Column: 1},
			},
		},
		{
			name: "no_span",
			span: syntax.NoSpan,
			want: syntax.Location{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := syntax.Locate(src, tt.span)
			if got != tt.want {
				t.Errorf("Locate(%+v) = %+v, want %+v", tt.span, got, tt.want)
			}
			if want := tt.span != syntax.NoSpan; got.IsValid() != want {
				t.Errorf("Locate(%+v).IsValid() = %v, want %v", tt.span, got.IsValid(), want)
			}
		})
	}
}

// TestLocateNilSource checks the defensive path: a caller that has no Source
// gets an unlocated Location rather than a panic.
func TestLocateNilSource(t *testing.T) {
	got := syntax.Locate(nil, syntax.Span{Start: 1, End: 2})
	if got != (syntax.Location{}) {
		t.Errorf("Locate(nil, …) = %+v, want the zero Location", got)
	}
	if got.IsValid() {
		t.Error("Locate(nil, …).IsValid() = true, want false")
	}
}

func TestLocationString(t *testing.T) {
	tests := []struct {
		name string
		loc  syntax.Location
		want string
	}{
		{
			name: "no_location",
			loc:  syntax.Location{},
			want: "<no location>",
		},
		{
			name: "point",
			loc: syntax.Location{
				Start: syntax.Position{Line: 2, Column: 3},
				End:   syntax.Position{Line: 2, Column: 3},
			},
			want: "2:3",
		},
		{
			name: "range",
			loc: syntax.Location{
				Start: syntax.Position{Line: 2, Column: 3},
				End:   syntax.Position{Line: 4, Column: 1},
			},
			want: "2:3-4:1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.loc.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
