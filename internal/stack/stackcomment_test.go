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
	"slices"
	"testing"
)

var parseCommentTests = []struct {
	input   string
	output  []string
	wantErr error
}{
	// valid cases
	{
		input:  "[]",
		output: []string{},
	},
	{
		input:  "[ ]",
		output: []string{},
	},
	{
		input:  "[a, b]",
		output: []string{"a", "b"},
	},
	// whitespace removal, nesting
	{
		input:  "[a == b, d', (x*y) + 1 - 2, arr[1:2], fn(a, b)]",
		output: []string{"a==b", "d'", "(x*y)+1-2", "arr[1:2]", "fn(a,b)"},
	},

	// some errors
	{
		input:   "",
		wantErr: ErrEmptyComment,
	},
	{
		input:   "text",
		wantErr: ErrNotStackComment,
	},
	{
		input:   "[a",
		wantErr: errIncompleteComment,
	},
	{
		input:   "[a,",
		wantErr: errIncompleteComment,
	},
	{
		input:   "[a,,]",
		wantErr: errEmptyItem,
	},
	{
		input:   `[a, "b"]`,
		wantErr: errDoubleQuote,
	},
	{
		input:   `[a, func(])]`,
		wantErr: nestingError{opening: '(', expected: ')', found: ']'},
	},
}

func TestParseComment(t *testing.T) {
	for _, test := range parseCommentTests {
		slice, err := ParseComment(test.input)
		if err != nil {
			if test.wantErr == nil {
				t.Errorf("test(%q): unexpected error: %q", test.input, err)
			} else if err != test.wantErr {
				t.Errorf("test(%q): wrong error: %q, want %q", test.input, err, test.wantErr)
			}
		} else {
			if test.wantErr != nil {
				t.Errorf("test(%q): expected error, got none (result %v)", test.input, slice)
			} else if !slices.Equal(slice, test.output) {
				t.Errorf("test(%q): wrong result %#v", test.input, slice)
			}
		}
	}
}
