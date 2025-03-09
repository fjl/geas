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
	ecLabelInBytes
	ecUnknownOpcode
	ecUndefinedVariable
	ecUndefinedMacro
	ecUndefinedInstrMacro
	ecUndefinedBuiltinMacro
	ecRecursiveCall
	ecInvalidArgumentCount
	ecNegativeResult
	ecOddLengthBytesLiteral
	ecIncludeNoFS
	ecIncludeDepthLimit
	ecUnknownPragma
	ecPragmaTargetInIncludeFile
	ecPragmaTargetConflict
	ecPragmaTargetUnknown
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
	case ecLabelInBytes:
		return "labels can't be used in #bytes"
	case ecUnknownOpcode:
		return "unknown op"
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
		return "expression result is negative number"
	case ecOddLengthBytesLiteral:
		return "odd-length hex in bytes context"
	case ecIncludeNoFS:
		return "#include not allowed"
	case ecIncludeDepthLimit:
		return "#include depth limit reached"
	case ecUnknownPragma:
		return "unknown #pragma"
	case ecPragmaTargetInIncludeFile:
		return "#pragma target cannot be used in #include'd files"
	case ecPragmaTargetConflict:
		return "duplicate '#pragma target ...' directive"
	case ecPragmaTargetUnknown:
		return "unknown #pragma target"
	default:
		return fmt.Sprintf("invalid error %d", e)
	}
}

// statementError is an error related to an assembler instruction.
type statementError struct {
	inst ast.Statement
	err  error
}

func (e *statementError) Position() ast.Position {
	return e.inst.Position()
}

func (e *statementError) Unwrap() error {
	return e.err
}

func (e *statementError) Error() string {
	return fmt.Sprintf("%v: %s", e.inst.Position(), e.err.Error())
}

// simpleWarning is a warning issued by the compiler.
type simpleWarning struct {
	pos ast.Position
	str string
}

func (e *simpleWarning) Error() string {
	return fmt.Sprintf("%v: warning: %s", e.pos, e.str)
}

func (e *simpleWarning) IsWarning() bool {
	return true
}

// unassignedLabelError signals use of a label that doesn't have a valid PC.
type unassignedLabelError struct {
	lref *ast.LabelRefExpr
}

func (e unassignedLabelError) Error() string {
	return fmt.Sprintf("%v not instantiated in program", e.lref)
}

// Warning is implemented by errors that could also be just a warning.
type Warning interface {
	error
	IsWarning() bool
}

// IsWarning reports whether an error is a warning.
func IsWarning(err error) bool {
	var w Warning
	return errors.As(err, &w) && w.IsWarning()
}

// errorList maintains a list of errors and warnings. It also implements the mechanism
// that aborts compilation when too many errors have accumulated.
type errorList struct {
	list        []error
	numErrors   int
	numWarnings int
	maxErrors   int
}

// catchAbort traps the panic condition that gets thrown when too many errors have accumulated.
// A call to catchAbort must be deferred around any code that uses [errorList.add].
func (e *errorList) catchAbort() {
	ok := recover()
	if ok != nil && ok != errCancelCompilation {
		panic(ok)
	}
}

// add puts errors into the list.
// This returns true if there were any actual errors in the arguments.
func (e *errorList) add(errs ...error) (anyRealError bool) {
	for _, err := range errs {
		if err == nil {
			continue
		}
		e.list = append(e.list, err)
		if IsWarning(err) {
			e.numWarnings++
		} else {
			e.numErrors++
			anyRealError = true
		}
		if e.numErrors > e.maxErrors {
			panic(errCancelCompilation)
		}
	}
	return
}

// addParseErrors is like add, but for errors from the parser.
func (e *errorList) addParseErrors(errs []*ast.ParseError) bool {
	conv := make([]error, len(errs))
	for i := range errs {
		conv[i] = errs[i]
	}
	return e.add(conv...)
}

// warnings returns the current warning list.
func (e *errorList) warnings() []error {
	s := make([]error, 0, e.numWarnings)
	for _, err := range e.list {
		if IsWarning(err) {
			s = append(s, err)
		}
	}
	return s
}

// warnings returns the current error list.
func (e *errorList) errors() []error {
	s := make([]error, 0, e.numErrors)
	for _, err := range e.list {
		if !IsWarning(err) {
			s = append(s, err)
		}
	}
	return s
}

// hasError reports whether there were any actual errors.
func (e *errorList) hasError() bool {
	return e.numErrors > 0
}
