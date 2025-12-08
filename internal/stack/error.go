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
)

type nestingError struct {
	opening, expected, found rune
}

func (e nestingError) Error() string {
	return fmt.Sprintf("expected %c to close %c, found %c", e.opening, e.found, e.expected)
}

// Analysis errors.

// ErrOpUnderflows is reported when an operation uses more items than are currently
// available on the stack.
type ErrOpUnderflows struct {
	Want int // how many slots the op consumes
	Have int // current stack depth
}

func (e ErrOpUnderflows) Error() string {
	return fmt.Sprintf("stack underflow: op requires %d items, stack has %d", e.Want, e.Have)
}

// ErrCommentUnderflows is reported when a stack comment declares more items than
// are currently available on the stack.
type ErrCommentUnderflows struct {
	Items []string // computed stack
	Want  int      // how many slots the comment declares
}

func (e ErrCommentUnderflows) Error() string {
	return fmt.Sprintf("stack has %d items, comment declares %d", len(e.Items), e.Want)
}

// ErrMismatch is reported when a stack comment declares a specific item should be
// contained in a stack slot, but the stack is known to contain a different one at the
// same position.
type ErrMismatch struct {
	Items []string // computed stack
	Slot  int      // stack slot index
	Want  string   // what the comment has at that index
}

func (e ErrMismatch) Error() string {
	return fmt.Sprintf("stack item %d differs (expected %q, have %q) in %s", e.Slot, e.Want, e.Items[e.Slot], render(e.Items))
}

// ErrCommentRenamesItem is raised when the stack comment changes the name of an existing
// item, i.e. one that wasn't produced by the current operation.
type ErrCommentRenamesItem struct {
	Item    string
	NewName string
}

func (e ErrCommentRenamesItem) Error() string {
	return fmt.Sprintf("comment introduces new name %s for existing stack item %s", e.NewName, e.Item)
}
