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

import "fmt"

//go:generate go run golang.org/x/tools/cmd/stringer -type ArithOp

// ArithOp is an arithmetic operation.
type ArithOp byte

const (
	ArithPlus   = ArithOp(iota + 1) // +
	ArithMinus                      // -
	ArithMul                        // *
	ArithDiv                        // /
	ArithMod                        // %
	ArithLshift                     // <<
	ArithRshift                     // >>
	ArithAnd                        // &
	ArithOr                         // |
	ArithXor                        // ^
	ArithMax    = ArithXor
)

// arithChars contains all the single-character arithmetic operations.
// note that '%' is also absent from this list since it has a dual purpose.
var arithChars = map[rune]ArithOp{
	'+': ArithPlus,
	'-': ArithMinus,
	'*': ArithMul,
	'/': ArithDiv,
	'&': ArithAnd,
	'|': ArithOr,
	'^': ArithXor,
}

// Sign returns the sign of the operation.
func (op ArithOp) Sign() string {
	switch op {
	case ArithPlus:
		return "+"
	case ArithMinus:
		return "-"
	case ArithMul:
		return "*"
	case ArithDiv:
		return "/"
	case ArithMod:
		return "%"
	case ArithLshift:
		return "<<"
	case ArithRshift:
		return ">>"
	case ArithAnd:
		return "&"
	case ArithOr:
		return "|"
	case ArithXor:
		return "^"
	default:
		panic(fmt.Errorf("invalid ArithOp %d", op))
	}
}

var precedenceTable = [ArithMax + 1]int{
	ArithMul:    2,
	ArithDiv:    2,
	ArithMod:    2,
	ArithLshift: 2,
	ArithRshift: 2,
	ArithAnd:    2,
	ArithPlus:   1,
	ArithMinus:  1,
	ArithOr:     1,
	ArithXor:    1,
}

// Precedence returns the precedence level of the operation.
func (op ArithOp) Precedence() int {
	return precedenceTable[op]
}

// tokenArithOp returns the arithmetic operation represented by an operator token.
func tokenArithOp(tok token) ArithOp {
	if tok.typ != arith {
		panic("token is not arith")
	}
	switch {
	case tok.text == "<<":
		return ArithLshift
	case tok.text == ">>":
		return ArithRshift
	case tok.text == "%":
		return ArithMod
	default:
		op, ok := arithChars[[]rune(tok.text)[0]]
		if !ok {
			panic("invalid arith op")
		}
		return op
	}
}
