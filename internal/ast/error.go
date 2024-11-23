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

// Position represents a line in a file.
type Position struct {
	File string
	Line int
}

func (p Position) String() string {
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// ParseError is an error that happened during parsing.
type ParseError struct {
	tok     token
	file    string
	err     error
	warning bool
}

func (e *ParseError) Error() string {
	warn := ""
	if e.warning {
		warn = "warning: "
	}
	return fmt.Sprintf("%s:%d: %s%v", e.file, e.tok.line, warn, e.err)
}

func (e *ParseError) Position() Position {
	return Position{File: e.file, Line: e.tok.line}
}

func (e *ParseError) IsWarning() bool {
	return e.warning
}

func (e *ParseError) Unwrap() error {
	return e.err
}

func ErrLabelAlreadyDef(firstDef, secondDef *LabelDefSt) error {
	dotInfo := ""
	if firstDef.Dotted && !secondDef.Dotted {
		dotInfo = " (as dotted label)"
	}
	if !firstDef.Dotted && secondDef.Dotted {
		dotInfo = " (as jumpdest)"
	}
	return fmt.Errorf("%v already defined%s", secondDef, dotInfo)
}
