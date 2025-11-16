// Copyright 2023 The go-ethereum Authors
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

import "github.com/fjl/geas/internal/lzint"

type Expr interface {
	Position() Position
}

// expression types
type (
	LiteralExpr struct {
		text   string
		pos    Position
		value  *lzint.Value
		string bool
	}

	LabelRefExpr struct {
		Ident  string
		Dotted bool
		pos    Position
	}

	VariableExpr struct {
		Ident string
		pos   Position
	}

	MacroCallExpr struct {
		Ident   string
		Builtin bool
		Args    []Expr
		pos     Position
	}

	BinaryExpr struct {
		Op    ArithOp
		Left  Expr
		Right Expr
		pos   Position
	}

	UnaryExpr struct {
		Op  ArithOp
		Arg Expr
		pos Position
	}
)

// MakeNumber creates a number literal with the given value.
func MakeNumber(v *lzint.Value) *LiteralExpr {
	return &LiteralExpr{
		text:  v.String(),
		value: v,
	}
}

// MakeString creates a string literal with the given value.
func MakeString(v string) *LiteralExpr {
	return &LiteralExpr{
		text:   v,
		string: true,
		value:  lzint.FromBytes([]byte(v)),
	}
}

// Value returns the parsed value of the literal.
func (e *LiteralExpr) Value() *lzint.Value {
	return e.value
}

// Text returns the text content of the literal as-written.
// Note this does not include the quotes for string literals.
func (e *LiteralExpr) Text() string {
	return e.text
}

// Sting returns the literal as-written.
func (e *LiteralExpr) String() string {
	if e.string {
		return `"` + e.text + `"`
	}
	return e.text
}

func (l *LiteralExpr) Position() Position {
	return l.pos
}

func (l *LabelRefExpr) Position() Position {
	return l.pos
}

func (l *LabelRefExpr) String() string {
	dot := ""
	if l.Dotted {
		dot = "."
	}
	return "@" + dot + l.Ident
}

func (e *VariableExpr) Position() Position {
	return e.pos
}

func (e *VariableExpr) String() string {
	return "$" + e.Ident
}

func (e *MacroCallExpr) Position() Position {
	return e.pos
}

func (e *BinaryExpr) Position() Position {
	return e.pos
}

func (e *UnaryExpr) Position() Position {
	return e.pos
}

func (e *UnaryExpr) Position() Position {
	return e.pos
}
