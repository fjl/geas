// Copyright 2025 The go-ethereum Authors
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

package loader

import (
	"errors"
	"fmt"

	"github.com/fjl/geas/internal/ast"
)

// panic sentinel value:
var errCancelCompilation = errors.New("end compilation")

// ErrorList maintains a list of errors and warnings. It also implements the mechanism
// that aborts compilation when too many errors have accumulated.
type ErrorList struct {
	list        []error
	numErrors   int
	numWarnings int
	maxErrors   int
}

// NewErrorList creates an error list.
func NewErrorList(maxErrors int) *ErrorList {
	return &ErrorList{maxErrors: maxErrors}
}

// catchAbort traps the panic condition that gets thrown when too many errors have accumulated.
// A call to catchAbort must be deferred around any code that uses [errorList.add].
func (e *ErrorList) CatchAbort() {
	ok := recover()
	if ok != nil && ok != errCancelCompilation {
		panic(ok)
	}
}

func (e *ErrorList) Clear() {
	e.list = nil
	e.numErrors = 0
	e.numWarnings = 0
}

// Add puts errors into the list.
// This returns true if there were any actual errors in the arguments.
func (e *ErrorList) Add(errs ...error) (anyRealError bool) {
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

// AddErrorAt adds an error related to a specific statement.
func (e *ErrorList) AddAt(st ast.Statement, err error) {
	if err == nil {
		panic("BUG: errorAt(st, nil)")
	}
	e.Add(&statementError{st, err})
}

// addParseErrors is like add, but for errors from the parser.
func (e *ErrorList) addParseErrors(errs []*ast.ParseError) bool {
	conv := make([]error, len(errs))
	for i := range errs {
		conv[i] = errs[i]
	}
	return e.Add(conv...)
}

// Warnings returns the current warning list.
func (e *ErrorList) Warnings() []error {
	s := make([]error, 0, e.numWarnings)
	for _, err := range e.list {
		if IsWarning(err) {
			s = append(s, err)
		}
	}
	return s
}

// Errors returns the current error list.
func (e *ErrorList) Errors() []error {
	s := make([]error, 0, e.numErrors)
	for _, err := range e.list {
		if !IsWarning(err) {
			s = append(s, err)
		}
	}
	return s
}

// ErrorsAndWarnings returns all accumulated errors.
func (e *ErrorList) ErrorsAndWarnings() []error {
	return e.list
}

// HasError reports whether there were any actual errors.
func (e *ErrorList) HasError() bool {
	return e.numErrors > 0
}

// statementError is an error related to an assembler instruction.
type statementError struct {
	st  ast.Statement
	err error
}

func (e *statementError) Position() ast.Position {
	return e.st.Position()
}

func (e *statementError) Unwrap() error {
	return e.err
}

func (e *statementError) Error() string {
	return fmt.Sprintf("%v: %s", e.st.Position(), e.err.Error())
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
