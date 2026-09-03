// Package convert reads the values Markst literals stand for.
package convert

import (
	"fmt"
	"strconv"
)

// ParseInt returns the value of a Markst integer literal, written in decimal
// or with a 0b, 0o, or 0x prefix.
func ParseInt(s string) (int, error) {
	base := 10
	if len(s) >= 2 && s[0] == '0' {
		switch s[1] {
		case 'b':
			base = 2
			s = s[2:]
		case 'o':
			base = 8
			s = s[2:]
		case 'x':
			base = 16
			s = s[2:]
		}
	}
	iv, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid int literal: %s", s)
	}
	return int(iv), nil
}
