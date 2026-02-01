package convert

import (
	"fmt"
	"strconv"
)

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
