// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
