package syntax

import "testing"

func TestNumKinds(t *testing.T) {
	if got, max := int(numKinds), 3*64; got > max {
		t.Errorf("number of kinds is %d, which exceeds the maximum of %d", got, max)
	}
}

func TestSet(t *testing.T) {
	tests := []struct {
		name     string
		elements []Kind
		check    []Kind
		missing  []Kind
	}{
		{
			name:     "empty",
			elements: nil,
			missing:  []Kind{KindEnd, KindSpace, KindDestructAssignment},
		},
		{
			name:     "single",
			elements: []Kind{KindSpace},
			check:    []Kind{KindSpace},
			missing:  []Kind{KindEnd, KindLineComment},
		},
		{
			name:     "multiple",
			elements: []Kind{KindSpace, KindParbreak, KindLineComment},
			check:    []Kind{KindSpace, KindParbreak, KindLineComment},
			missing:  []Kind{KindEnd, KindBlockComment},
		},
		{
			name:     "duplicate",
			elements: []Kind{KindSpace, KindSpace},
			check:    []Kind{KindSpace},
			missing:  []Kind{KindEnd, KindLineComment},
		},
		{
			name:     "boundary-low",
			elements: []Kind{0},
			check:    []Kind{0},
			missing:  []Kind{1},
		},
		{
			name:     "boundary-first-chunk",
			elements: []Kind{63},
			check:    []Kind{63},
			missing:  []Kind{64},
		},
		{
			name:     "boundary-second-chunk",
			elements: []Kind{64},
			check:    []Kind{64},
			missing:  []Kind{63},
		},
		// Check the highest defined kind
		{
			name:     "max-kind",
			elements: []Kind{numKinds - 1},
			check:    []Kind{numKinds - 1},
			missing:  []Kind{numKinds - 2},
		},
		{
			name:     "spread",
			elements: []Kind{0, 64, 128},
			check:    []Kind{0, 64, 128},
			missing:  []Kind{1, 63, 65, 127, 129},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SetOf(tt.elements...)
			for _, k := range tt.check {
				if !s.Contains(k) {
					t.Errorf("set should contain %v", k)
				}
			}
			for _, k := range tt.missing {
				if s.Contains(k) {
					t.Errorf("set should not contain %v", k)
				}
			}
		})
	}
}
