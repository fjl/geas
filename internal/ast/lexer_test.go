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
	"reflect"
	"testing"
)

func lexAll(src string) []token {
	ch := runLexer([]byte(src), false)

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
			tokens: []token{{typ: lineStart, line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "0x12345678",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "0x12345678", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "0x123ggg",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "0x123", line: 1}, {typ: identifier, text: "ggg", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "12345678",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "12345678", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "123abc",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "123", line: 1}, {typ: identifier, text: "abc", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "0123abc",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "0123", line: 1}, {typ: identifier, text: "abc", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "00123abc",
			tokens: []token{{typ: lineStart, line: 1}, {typ: numberLiteral, text: "00123", line: 1}, {typ: identifier, text: "abc", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "@foo",
			tokens: []token{{typ: lineStart, line: 1}, {typ: labelRef, text: "foo", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "@label123",
			tokens: []token{{typ: lineStart, line: 1}, {typ: labelRef, text: "label123", line: 1}, {typ: eof, line: 1}},
		},
		{
			input:  "@.label .label: .ident",
			tokens: []token{{typ: lineStart, line: 1}, {typ: dottedLabelRef, text: "label", line: 1}, {typ: dottedLabel, text: "label", line: 1}, {typ: dottedIdentifier, text: "ident", line: 1}, {typ: eof, line: 1}},
		},
		// comment after label
		{
			input:  "@label123 ;; comment",
			tokens: []token{{typ: lineStart, line: 1}, {typ: labelRef, text: "label123", line: 1}, {typ: eof, line: 1}},
		},
		// comment after instruction
		{
			input:  "push 3 ;; comment\nadd",
			tokens: []token{{typ: lineStart, line: 1}, {typ: identifier, text: "push", line: 1}, {typ: numberLiteral, text: "3", line: 1}, {typ: lineEnd, text: "\n", line: 1}, {typ: lineStart, line: 2}, {typ: identifier, line: 2, text: "add"}, {typ: eof, line: 2}},
		},
	}

	for _, test := range tests {
		tokens := lexAll(test.input)
		if !reflect.DeepEqual(tokens, test.tokens) {
			t.Errorf("input %q\ngot:  %+v\nwant: %+v", test.input, tokens, test.tokens)
		}
	}
}
