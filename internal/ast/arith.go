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

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type ArithOp

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

var precedence = [ArithMax + 1]int{
	ArithPlus:   4,
	ArithMinus:  4,
	ArithMul:    5,
	ArithDiv:    5,
	ArithMod:    5,
	ArithLshift: 3,
	ArithRshift: 3,
	ArithAnd:    2,
	ArithOr:     0,
	ArithXor:    1,
}
