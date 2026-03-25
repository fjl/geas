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

import (
	"slices"
	"testing"
)

func lexAll(src string) []token {
	ch := runLexer([]byte(src))

	var tokens []token
	for i := range ch {
		tokens = append(tokens, i)
	}
	return tokens
}

func TestLexer(t *testing.T) {
	tests := []struct {
		input  string
		tokens []token
	}{
		{
			input:  ";; this is a comment",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: comment, text: ";; this is a comment", line: 1, column: 0}, {typ: eof, line: 1, column: 20}},
		},
		{
			input:  "0x12345678",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "0x12345678", line: 1, column: 0}, {typ: eof, line: 1, column: 10}},
		},
		{
			input:  "0x123ggg",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "0x123", line: 1, column: 0}, {typ: identifier, text: "ggg", line: 1, column: 5}, {typ: eof, line: 1, column: 8}},
		},
		{
			input:  "12345678",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "12345678", line: 1, column: 0}, {typ: eof, line: 1, column: 8}},
		},
		{
			input:  "123abc",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "123", line: 1, column: 0}, {typ: identifier, text: "abc", line: 1, column: 3}, {typ: eof, line: 1, column: 6}},
		},
		{
			input:  "0123abc",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "0123", line: 1, column: 0}, {typ: identifier, text: "abc", line: 1, column: 4}, {typ: eof, line: 1, column: 7}},
		},
		{
			input:  "00123abc",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: numberLiteral, text: "00123", line: 1, column: 0}, {typ: identifier, text: "abc", line: 1, column: 5}, {typ: eof, line: 1, column: 8}},
		},
		{
			input:  "@foo",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: labelRef, text: "foo", line: 1, column: 1}, {typ: eof, line: 1, column: 4}},
		},
		{
			input:  "@label123",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: labelRef, text: "label123", line: 1, column: 1}, {typ: eof, line: 1, column: 9}},
		},
		{
			input:  "@.label .label: .ident",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: dottedLabelRef, text: "label", line: 1, column: 2}, {typ: dottedLabel, text: "label", line: 1, column: 9}, {typ: dottedIdentifier, text: "ident", line: 1, column: 17}, {typ: eof, line: 1, column: 22}},
		},
		// comment after label
		{
			input:  "@label123 ;; comment",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: labelRef, text: "label123", line: 1, column: 1}, {typ: comment, text: ";; comment", line: 1, column: 10}, {typ: eof, line: 1, column: 20}},
		},
		// comment after instruction
		{
			input:  "push 3 ;; comment\nadd",
			tokens: []token{{typ: lineStart, line: 1, column: 0}, {typ: identifier, text: "push", line: 1, column: 0}, {typ: numberLiteral, text: "3", line: 1, column: 5}, {typ: comment, text: ";; comment", line: 1, column: 7}, {typ: lineEnd, text: "\n", line: 1, column: 17}, {typ: lineStart, line: 2, column: 0}, {typ: identifier, line: 2, column: 0, text: "add"}, {typ: eof, line: 2, column: 3}},
		},
	}

	for _, test := range tests {
		tokens := lexAll(test.input)
		if !slices.Equal(tokens, test.tokens) {
			t.Errorf("input %q\ngot:  %v\nwant: %v", test.input, tokens, test.tokens)
		}
	}
}
