// Copyright 2017 The go-ethereum Authors
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
	"fmt"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// stateFn is used through the lifetime of the
// lexer to parse the different values at the
// current state.
type stateFn func(*lexer) stateFn

// token is emitted when the lexer has discovered
// a new parsable token. These are delivered over
// the tokens channels of the lexer
type token struct {
	text string
	line int
	typ  tokenType
}

func (t *token) is(types ...tokenType) bool { return slices.Contains(types, t.typ) }
func (t *token) String() string             { return fmt.Sprintf("%v %s (line %d)", t.typ, t.text, t.line) }

// tokenType are the different types the lexer
// is able to parse and return.
type tokenType byte

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type tokenType

const (
	eof                tokenType = iota // end of file
	lineStart                           // emitted when a line starts
	lineEnd                             // emitted when a line ends
	invalidToken                        // any invalid statement
	identifier                          // something
	dottedIdentifier                    // .something
	variableIdentifier                  // $something
	labelRef                            // @label
	dottedLabelRef                      // @.label
	label                               // label:
	dottedLabel                         // .label:
	numberLiteral                       // number is emitted when a number is found
	stringLiteral                       // stringValue is emitted when a string has been found
	openParen                           // (
	closeParen                          // )
	comma                               // ,
	directive                           // #define, #include, ...
	instMacroIdent                      // %macro
	openBrace                           // {
	closeBrace                          // }
	equals                              // =
	arith                               // +, -, *, /, ... (see arith.go)
)

const (
	decimalNumbers = "1234567890"                                           // characters representing any decimal number
	hexNumbers     = decimalNumbers + "aAbBcCdDeEfF"                        // characters representing any hexadecimal
	alpha          = "abcdefghijklmnopqrstuwvxyzABCDEFGHIJKLMNOPQRSTUWVXYZ" // characters representing alphanumeric
	identChars     = alpha + "_" + decimalNumbers
)

// lexer is the basic construct for parsing
// source code and turning them in to tokens.
// Tokens are interpreted by the compiler.
type lexer struct {
	input string // input contains the source code of the program

	tokens chan token // tokens is used to deliver tokens to the listener
	state  stateFn    // the current state function

	lineno            int // current line number in the source file
	start, pos, width int // positions for lexing and returning value

	debug bool // flag for triggering debug output
}

// runLexer lexes the program by name with the given source. It returns a
// channel on which the tokens are delivered.
func runLexer(source []byte, debug bool) <-chan token {
	ch := make(chan token)
	l := &lexer{
		input:  string(source),
		tokens: ch,
		state:  lexNext,
		debug:  debug,
		lineno: 1,
	}
	go func() {
		l.emit(lineStart)
		for l.state != nil {
			l.state = l.state(l)
		}
		l.emit(eof)
		close(l.tokens)
	}()

	return ch
}

// next returns the next rune in the program's source.
func (l *lexer) next() (rune rune) {
	if l.pos >= len(l.input) {
		l.width = 0
		return 0
	}
	rune, l.width = utf8.DecodeRuneInString(l.input[l.pos:])
	l.pos += l.width
	return rune
}

// backup backsup the last parsed element (multi-character)
func (l *lexer) backup() {
	l.pos -= l.width
}

// peek returns the next rune but does not advance the seeker
func (l *lexer) peek() rune {
	r := l.next()
	l.backup()
	return r
}

// ignore advances the seeker and ignores the value
func (l *lexer) ignore() {
	l.start = l.pos
}

// Accepts checks whether the given input matches the next rune
func (l *lexer) accept(valid string) bool {
	if strings.ContainsRune(valid, l.next()) {
		return true
	}
	l.backup()
	return false
}

// acceptRun will continue to advance the seeker until valid
// can no longer be met.
func (l *lexer) acceptRun(fn func(rune) bool) {
	for fn(l.next()) {
	}
	l.backup()
}

// acceptRunUntil is the inverse of acceptRun and will continue
// to advance the seeker until the rune has been found.
func (l *lexer) acceptRunUntil(until rune) bool {
	for {
		i := l.next()
		if i == until {
			l.pos--
			return true
		}
		if i == 0 {
			return false // eof
		}
	}
}

// emit creates a new token and sends it to token channel for processing.
func (l *lexer) emit(t tokenType) {
	token := token{line: l.lineno, text: l.input[l.start:l.pos], typ: t}

	if l.debug {
		fmt.Fprintf(os.Stderr, "%04d: (%-20v) %s\n", token.line, token.typ, token.text)
	}

	l.tokens <- token
	l.start = l.pos
}

// lexNext is state function for lexing lines
func lexNext(l *lexer) stateFn {
	for {
		switch r := l.next(); {
		// known symbols:

		case r == ';':
			return lexComment

		case r == '@':
			l.ignore()
			return lexLabel

		case r == '$':
			l.ignore()
			return lexVariable

		case r == '"':
			return lexInsideString

		case r == '(':
			l.emit(openParen)
			return lexNext

		case r == ')':
			l.emit(closeParen)
			return lexNext

		case r == '{':
			l.emit(openBrace)
			return lexNext

		case r == '}':
			l.emit(closeBrace)
			return lexNext

		case r == ',':
			l.emit(comma)
			return lexNext

		case r == '#':
			return lexPreprocessor

		case r == '=':
			l.emit(equals)
			return lexNext

		// numbers and identifiers:

		case unicode.IsDigit(r):
			return lexNumber

		case r == '.' || isIdentBegin(r):
			return lexIdentifier

		// arithmetic:

		case r == '<':
			return lexLshift

		case r == '>':
			return lexRshift

		case r == '%':
			return lexPercent

		case arithChars[r] != 0:
			l.emit(arith)
			return lexNext

		// whitespace, etc.

		case r == '\n':
			l.emit(lineEnd)
			l.ignore()
			l.lineno++
			l.emit(lineStart)

		case isSpace(r):
			l.ignore()

		case r == 0:
			return nil // eof

		default:
			l.emit(invalidToken)
		}
	}
}

// lexComment parses the current position until the end
// of the line and discards the text.
func lexComment(l *lexer) stateFn {
	l.acceptRunUntil('\n')
	l.ignore()
	return lexNext
}

// lexLabel parses a label reference.
func lexLabel(l *lexer) stateFn {
	typ := labelRef
	if l.peek() == '.' {
		typ = dottedLabelRef
		l.next() // consume optional .
		l.ignore()
	}
	l.acceptRun(isIdent)
	l.emit(typ)
	return lexNext
}

func lexPercent(l *lexer) stateFn {
	r := l.peek()
	if isIdentBegin(r) {
		l.ignore()
		l.acceptRun(isIdent)
		l.emit(instMacroIdent)
	} else {
		l.emit(arith)
	}
	return lexNext
}

// lexInsideString lexes the inside of a string until
// the state function finds the closing quote.
// It returns the lex text state function.
func lexInsideString(l *lexer) stateFn {
	// TODO: allow escaping quotes
	if l.acceptRunUntil('"') {
		l.start += 1 // remove beginning quote
		l.emit(stringLiteral)
		l.next() // consume "
	}
	return lexNext
}

func lexNumber(l *lexer) stateFn {
	acceptance := unicode.IsDigit
	if l.accept("xX") {
		acceptance = isHex
	}
	l.acceptRun(acceptance)
	l.emit(numberLiteral)
	return lexNext
}

func lexLshift(l *lexer) stateFn {
	if !l.accept("<") {
		l.emit(invalidToken)
	} else {
		l.emit(arith)
	}
	return lexNext
}

func lexRshift(l *lexer) stateFn {
	if !l.accept(">") {
		l.emit(invalidToken)
	} else {
		l.emit(arith)
	}
	return lexNext
}

func lexPreprocessor(l *lexer) stateFn {
	l.acceptRun(isIdent)
	l.emit(directive)
	return lexNext
}

func lexVariable(l *lexer) stateFn {
	l.acceptRun(isIdent)
	l.emit(variableIdentifier)
	return lexNext
}

func lexIdentifier(l *lexer) stateFn {
	firstIsDot := l.input[l.start] == '.'
	if firstIsDot {
		l.ignore()
	}
	l.acceptRun(isIdent)

	if l.peek() == ':' {
		if firstIsDot {
			l.emit(dottedLabel)
		} else {
			l.emit(label)
		}
		l.accept(":")
		l.ignore()
	} else {
		if firstIsDot {
			l.emit(dottedIdentifier)
		} else {
			l.emit(identifier)
		}
	}
	return lexNext
}

func isLetter(t rune) bool {
	return unicode.IsLetter(t)
}

func isSpace(t rune) bool {
	return unicode.IsSpace(t)
}

func isHex(t rune) bool {
	return unicode.IsDigit(t) || (t >= 'a' && t <= 'f') || (t >= 'A' && t <= 'F')
}

func isIdentBegin(t rune) bool {
	return t == '_' || unicode.IsLetter(t)
}

func isIdent(t rune) bool {
	return t == '_' || unicode.IsLetter(t) || unicode.IsNumber(t)
}
