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

package stack

import (
	"errors"
	"fmt"
)

// Parse errors.

var (
	ErrNotStackComment   = errors.New("not a stack comment, missing [")
	ErrEmptyComment      = errors.New("empty comment")
	errIncompleteComment = errors.New("incomplete stack comment")
	errEmptyItem         = errors.New("empty item in stack comment")
	errDoubleQuote       = errors.New("double-quote not allowed in stack comment")
	errWildcardNotLast   = errors.New("wildcard (..) must be last item in stack comment")
)

type nestingError struct {
	opening, expected, found rune
}

func (e nestingError) Error() string {
	return fmt.Sprintf("expected %c to close %c, found %c", e.expected, e.opening, e.found)
}

// Analysis errors raised by [Stack.Apply] and [Stack.CheckComment].
var (
	ErrOpUnderflow      = errors.New("stack underflow")
	ErrCommentUnderflow = errors.New("stack comment underflow")
	ErrCommentDepth     = errors.New("stack comment depth mismatch")
	ErrMismatch         = errors.New("stack comment item mismatch")
	ErrRename           = errors.New("stack comment renames item")
)

// Checker errors raised by the stackcheck package.
var (
	ErrMergeDepth     = errors.New("stack depth mismatch at merge point")
	ErrPushLiteral    = errors.New("push literal mismatch in stack comment")
	ErrLoopUnbalanced = errors.New("loop unbalanced")
)
