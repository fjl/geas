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
	"fmt"
	"unicode/utf8"
)

// ParseComment parses a stack comment at the start of the given input string.
// It checks for basic syntax errors, such as invalid parenthesis nesting.
// Stack items are canonicalized, i.e. all whitespace in items is removed.
func ParseComment(text string) ([]string, error) {
	in := inStream(text)
	switch in.skipSpace() {
	case -1:
		return nil, ErrEmptyComment
	case '[':
		in.next()
		// Start of stack comment.
	default:
		return nil, ErrNotStackComment
	}

	// The starting square bracket has been read.
	items := []string{}
	nest := new(paramNesting)
	nest.enter('[')
	for {
		in.skipSpace()
		item, err := parseElem(&in, nest)
		if err != nil {
			return items, err
		}
		if len(item) > 0 {
			items = append(items, item)
		}
		switch in.skipSpace() {
		case ',':
			// continue parsing next item
			in.next()
		case ']':
			// end of stack comment
			return items, nil
		}
	}
}

func parseElem(in *inStream, nest *paramNesting) (elem string, err error) {
	var chars []rune
loop:
	for {
		c := in.peek()
		switch c {
		case -1:
			return "", errIncompleteComment
		case ' ', '\t':
			in.next()
			continue loop // skip all space
		case '[', '(', '{':
			nest.enter(c)
		case ']', ')', '}':
			err := nest.leave(c)
			if err != nil {
				return "", err
			}
			if len(*nest) == 0 {
				return string(chars), nil
			}
		case ',':
			if !nest.inExpr() {
				if len(chars) == 0 {
					return "", errEmptyItem
				}
				return string(chars), nil
			}
		case '"':
			return "", errDoubleQuote
		}
		chars = append(chars, c)
		in.next()
	}
}

// inStream reads characters from a string.
type inStream string

func (in *inStream) peek() rune {
	if len(*in) == 0 {
		return -1
	}
	c, _ := utf8.DecodeRuneInString(string(*in))
	return c
}

func (in *inStream) next() {
	if len(*in) > 0 {
		_, size := utf8.DecodeRuneInString(string(*in))
		*in = (*in)[size:]
	}
}

// skipSpace forwards the input to the next non-space character.
func (in *inStream) skipSpace() rune {
	for {
		if len(*in) == 0 {
			return -1
		}
		c, size := utf8.DecodeRuneInString(string(*in))
		switch c {
		case ' ', '\t':
			*in = (*in)[size:]
		default:
			return c
		}
	}
}

// paramNesting tracks nesting of parentheses.
type paramNesting []rune

// inExpr reports whether the parser is currently inside of a parenthesized expression.
func (n *paramNesting) inExpr() bool {
	return len(*n) > 1
}

func (n *paramNesting) enter(c rune) {
	*n = append(*n, c)
}

func (n *paramNesting) leave(c rune) error {
	if len(*n) == 0 {
		panic("BUG: empty nest")
	}
	opening := (*n)[len(*n)-1]
	var expected rune
	switch opening {
	case '(':
		expected = ')'
	case '{':
		expected = '}'
	case '[':
		expected = ']'
	default:
		panic(fmt.Sprintf("BUG: invalid nest char %c", c))
	}
	if c != expected {
		return nestingError{opening, expected, c}
	}
	*n = (*n)[:len(*n)-1]
	return nil
}
