// Copyright 2024 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package ast

import (
	"strings"
	"unicode"
)

// IsGlobal returns true when 'name' is a global identifier.
func IsGlobal(name string) bool {
	return len(name) > 0 && unicode.IsUpper([]rune(name)[0])
}

// IsPush reports whether an op is a push.
func IsPush(op string) bool {
	return strings.HasPrefix(op, "PUSH")
}

// IsPush0 reports whether an op is the PUSH0 instruction.
func IsPush0(op string) bool {
	return strings.EqualFold(op, "PUSH0")
}

// IsJump reports whether an op is a jump.
func IsJump(op string) bool {
	return strings.HasPrefix(op, "JUMP")
}
