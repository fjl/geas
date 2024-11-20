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

package asm

import (
	"errors"
	"fmt"

	"github.com/fjl/geas/internal/ast"
)

// panic sentinel value:
var errCancelCompilation = errors.New("end compilation")

// PositionError is an error containing a file position.
type PositionError interface {
	error
	Position() ast.Position
}

// compilerErrorCode represents an error detected by the compiler.
type compilerError int

const (
	ecPushOverflow256 compilerError = iota
	ecPushzeroWithArgument
	ecFixedSizePushOverflow
	ecVariablePushOverflow
	ecPushWithoutArgument
	ecUnexpectedArgument
	ecJumpNeedsLiteralLabel
	ecJumpToDottedLabel
	ecJumpToUndefinedLabel
	ecUnknownOpcode
	ecUndefinedVariable
	ecUndefinedMacro
	ecUndefinedInstrMacro
	ecUndefinedBuiltinMacro
	ecRecursiveCall
	ecInvalidArgumentCount
	ecNegativeResult
	ecIncludeNoFS
	ecIncludeDepthLimit
)

func (e compilerError) Error() string {
	switch e {
	case ecPushOverflow256:
		return "instruction argument > 256 bits"
	case ecPushzeroWithArgument:
		return "PUSH0 can't have argument"
	case ecFixedSizePushOverflow:
		return "instruction argument overflows explicitly given PUSH<n> size"
	case ecVariablePushOverflow:
		return "instruction argument overflows push"
	case ecUnexpectedArgument:
		return "only JUMP* and PUSH* support immediate arguments"
	case ecPushWithoutArgument:
		return "PUSH requires an immediate argument"
	case ecJumpNeedsLiteralLabel:
		return "JUMP argument must be literal label"
	case ecJumpToDottedLabel:
		return "JUMP to dotted label"
	case ecJumpToUndefinedLabel:
		return "JUMP to undefined label"
	case ecUnknownOpcode:
		return "unknown opcode"
	case ecUndefinedVariable:
		return "undefined macro parameter"
	case ecUndefinedMacro:
		return "undefined macro"
	case ecUndefinedBuiltinMacro:
		return "undefined builtin macro"
	case ecUndefinedInstrMacro:
		return "undefined instruction macro"
	case ecRecursiveCall:
		return "recursive call of macro"
	case ecInvalidArgumentCount:
		return "invalid number of arguments"
	case ecNegativeResult:
		return "negative PUSH argument not supported"
	case ecIncludeNoFS:
		return "#include not allowed"
	case ecIncludeDepthLimit:
		return "#include depth limit reached"
	default:
		return fmt.Sprintf("invalid error %d", e)
	}
}

// astError is an error related to an assembler instruction.
type astError struct {
	inst ast.Statement
	err  error
}

func (e *astError) Position() ast.Position {
	return e.inst.Position()
}

func (e *astError) Unwrap() error {
	return e.err
}

func (e *astError) Error() string {
	return fmt.Sprintf("%v: %s", e.inst.Position(), e.err.Error())
}

// unassignedLabelError signals use of a label that doesn't have a valid PC.
type unassignedLabelError struct {
	lref *ast.LabelRefExpr
}

func (e unassignedLabelError) Error() string {
	return fmt.Sprintf("%v not instantiated in program", e.lref)
}
