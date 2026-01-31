// Copyright 2024 The go-ethereum Authors
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
	"bytes"
	"fmt"
	"regexp"
	"strconv"

	"github.com/fjl/geas/internal/lzint"
)

// Parser performs parsing of the token stream.
type Parser struct {
	in     <-chan token
	buffer []token
	doc    *Document
	errors []*ParseError
}

// NewParser creates a parser.
func NewParser(file string, content []byte) *Parser {
	return &Parser{
		in:  runLexer(content),
		doc: newDocument(file, nil),
	}
}

func newDocument(file string, parent *Document) *Document {
	return &Document{
		File:        file,
		labels:      make(map[string]*LabelDef),
		exprMacros:  make(map[string]*ExpressionMacroDef),
		instrMacros: make(map[string]*InstructionMacroDef),
		Parent:      parent,
	}
}

// next reads the next token from the lexer.
func (p *Parser) next() token {
	if len(p.buffer) > 0 {
		t := p.buffer[len(p.buffer)-1]
		p.buffer = p.buffer[:len(p.buffer)-1]
		return t
	}
	t := <-p.in
	return t
}

// unread puts a token back into the queue for reading.
func (p *Parser) unread(t token) {
	p.buffer = append(p.buffer, t)
}

// drainLexer runs the lexer to completion.
func (p *Parser) drainLexer() {
	for p.next().typ != eof {
	}
}

// throwError adds a new error to the error list.
// The parser is returned to the toplevel and will continue parsing
// at the next line.
func (p *Parser) throwError(tok token, format string, args ...any) {
	err := &ParseError{tok: tok, file: p.doc.File, err: fmt.Errorf(format, args...)}
	p.errors = append(p.errors, err)
	// resync to start of next line
	for {
		switch tok.typ {
		case lineEnd, eof:
			panic(err)
		}
		tok = p.next()
	}
}

// unexpected signals that an unexpected token occurred in the input.
func (p *Parser) unexpected(tok token) {
	p.throwError(tok, "unexpected %v %s", tok.typ, tok.text)
}

// Parse runs the parser, outputting a document.
func (p *Parser) Parse() (*Document, []*ParseError) {
	defer p.drainLexer()
	for {
		if p.parseOne() {
			return p.doc, p.errors
		}
	}
}

func (p *Parser) parseOne() bool {
	defer func() {
		err := recover()
		if _, ok := err.(*ParseError); !ok && err != nil {
			panic(err)
		}
	}()
	return parseStatement(p)
}

// ParseExpression parses the input as a single expression.
// This is used in evaluator tests.
func (p *Parser) ParseExpression() (expr Expr, err error) {
	defer p.drainLexer()
	defer func() {
		e := recover()
		if pe, ok := e.(*ParseError); ok {
			err = pe
		} else if e != nil {
			panic(e)
		}
	}()

	// skip lineStart
	switch tok := p.next(); tok.typ {
	case lineStart:
		expr = parseExpr(p, p.next())
		return expr, nil
	case lineEnd, eof:
		p.unexpected(tok)
	}
	return nil, nil
}

// atDocumentTop reports whether the parser is at the toplevel.
// This returns false while parsing an instruction macro definition.
func (p *Parser) atDocumentTop() bool {
	return p.doc.Parent == nil
}

// makeComment creates a comment node.
func (p *Parser) makeComment(tok token) *Comment {
	return &Comment{
		stbase: stbase{src: p.doc, line: tok.line},
		Text:   tok.text,
	}
}

// ------------- start parser functions -------------

// parseStatement reads a single statement and adds it to the parser document.
func parseStatement(p *Parser) (done bool) {
	var st Statement
	var lineCount int

	// Parse up to statement.
	for st == nil {
		switch tok := p.next(); tok.typ {
		// Handle end of document.
		case eof, closeBrace:
			if p.atDocumentTop() != (tok.typ == eof) {
				p.unexpected(tok)
			}
			return true

		// Process line endings.
		case lineStart:
			lineCount++
		case lineEnd:

		// Statements:
		case comment:
			// The line has just a comment and nothing else,
			// so the comment becomes its own statement.
			st = p.makeComment(tok)
		case label, dottedLabel:
			st = parseLabelDef(p, tok)
		case directive:
			st = parseDirective(p, tok)
		case identifier:
			st = parseOpcode(p, tok)
		case instMacroIdent:
			st = parseInstructionMacroCall(p, tok)
		default:
			p.unexpected(tok)
		}
	}

	// Check what's left on this line after the statement.
	// Note we skip this for instruction macro definitions because they
	// usually end on a separate line with just the closing brace.
	if _, ok := st.(*InstructionMacroDef); !ok {
		switch tok := p.next(); tok.typ {
		case lineEnd:
		// Consume line ending.
		case comment:
			// Check if there is a comment that should be attached to the statement.
			st.base().comment = p.makeComment(tok)
		default:
			// There's another statement on the same line, the next call will handle it.
			p.unread(tok)
		}
	}

	st.base().startsBlock = lineCount > 1
	p.doc.Statements = append(p.doc.Statements, st)
	return false
}

func parseLabelDef(p *Parser, tok token) *LabelDef {
	name := tok.text
	li := &LabelDef{
		stbase: stbase{src: p.doc, line: tok.line},
		Ident:  name,
		Dotted: tok.typ == dottedLabel,
	}
	if firstDef, ok := p.doc.labels[name]; ok {
		p.throwError(tok, "%w", ErrLabelAlreadyDef(firstDef, li))
		return li
	}
	p.doc.labels[name] = li
	return li
}

func parseDirective(p *Parser, tok token) Statement {
	switch tok.text {
	case "#define":
		if !p.atDocumentTop() {
			p.throwError(tok, "nested macro definitions are not allowed")
		}
		return parseMacroDef(p)
	case "#include":
		return parseInclude(p, tok)
	case "#assemble":
		return parseAssemble(p, tok)
	case "#pragma":
		return parsePragma(p, tok)
	case "#bytes":
		return parseBytes(p, tok)
	default:
		p.throwError(tok, "unknown compiler directive %q", tok.text)
		return nil
	}
}

func parseMacroDef(p *Parser) Statement {
	name := p.next()
	switch name.typ {
	case dottedIdentifier:
		p.throwError(name, "attempt to redefine builtin macro .%s", name.text)
	case instMacroIdent:
		return parseInstructionMacroDef(p, name)
	case identifier:
	default:
		p.unexpected(name)
	}

	// Parse parameters and body.
	var (
		base         = stbase{src: p.doc, line: name.line}
		def          = &ExpressionMacroDef{stbase: base, Ident: name.text}
		bodyTok      token
		didParams    bool
		legacySyntax bool
	)
loop:
	for {
		switch tok := p.next(); tok.typ {
		case lineEnd, eof, comment:
			p.throwError(tok, "incomplete macro definition")

		case openBrace:
			p.throwError(tok, "unexpected { in expression macro definition")

		case openParen:
			if didParams {
				bodyTok, legacySyntax = tok, true
				break loop
			} else {
				def.Params = parseParameterList(p)
				didParams = true
			}

		case equals:
			bodyTok = p.next()
			break loop

		default:
			bodyTok, legacySyntax = tok, true
			break loop
		}
	}

	if legacySyntax {
		p.errors = append(p.errors, &ParseError{
			tok:     bodyTok,
			file:    p.doc.File,
			err:     fmt.Errorf("legacy definition syntax, missing '=' before expression"),
			warning: true,
		})
	}
	def.Body = parseExpr(p, bodyTok)

	// Register the macro.
	checkDuplicateMacro(p, name)
	p.doc.exprMacros[name.text] = def
	return def
}

func parseInstructionMacroDef(p *Parser, nameTok token) *InstructionMacroDef {
	var params []string
	var didParams bool
paramLoop:
	for {
		switch tok := p.next(); tok.typ {
		case lineEnd, eof, comment:
			p.throwError(tok, "incomplete macro definition")
		case openBrace:
			break paramLoop // start of body
		case openParen:
			if !didParams {
				params = parseParameterList(p)
				didParams = true
				continue paramLoop
			}
		default:
			p.unexpected(tok)
		}
	}

	// Check for comment after the opening brace. As a side effect, this skips over
	// blank lines at the start of the macro body.
	var startComment *Comment
commentLoop:
	for {
		switch tok := p.next(); tok.typ {
		case lineEnd, lineStart:
		case comment:
			startComment = p.makeComment(tok)
			break commentLoop
		default:
			p.unread(tok)
			break commentLoop
		}
	}

	// Set definition context in parser.
	topdoc := p.doc
	doc := newDocument(p.doc.File, p.doc)
	p.doc = doc
	defer func() { p.doc = topdoc }()

	// Parse macro body.
	for !parseStatement(p) {
	}

	// Register definition.
	checkDuplicateMacro(p, nameTok)
	def := &InstructionMacroDef{
		stbase:       stbase{src: p.doc, line: nameTok.line},
		Ident:        nameTok.text,
		Params:       params,
		Body:         doc,
		StartComment: startComment,
	}
	doc.Creation = def
	topdoc.instrMacros[nameTok.text] = def
	return def
}

func checkDuplicateMacro(p *Parser, nameTok token) {
	name := nameTok.text
	if _, ok := p.doc.instrMacros[name]; ok {
		p.throwError(nameTok, "instruction macro %s already defined", name)
	}
	if _, ok := p.doc.exprMacros[name]; ok {
		p.throwError(nameTok, "expression macro %s already defined", name)
	}
}

func parseInclude(p *Parser, d token) *Include {
	st := &Include{stbase: stbase{src: p.doc, line: d.line}}
	switch tok := p.next(); tok.typ {
	case stringLiteral:
		st.Filename = tok.text
	default:
		p.throwError(tok, "expected filename following #include")
	}
	return st
}

func parseAssemble(p *Parser, d token) *Assemble {
	st := &Assemble{stbase: stbase{src: p.doc, line: d.line}}
	switch tok := p.next(); tok.typ {
	case stringLiteral:
		st.Filename = tok.text
	default:
		p.throwError(tok, "expected filename following #assemble")
	}
	return st
}

func parsePragma(p *Parser, d token) *Pragma {
	st := &Pragma{stbase: stbase{src: p.doc, line: d.line}}
	switch tok := p.next(); tok.typ {
	case identifier:
		st.Option = tok.text
		switch v := p.next(); v.typ {
		case stringLiteral, numberLiteral:
			st.Value = v.text
		case equals:
			p.throwError(tok, "unexpected = after #pragma %s", st.Option)
		default:
			p.throwError(tok, "#pragma option value must be string or number literal")
		}
	default:
		p.throwError(tok, "expected option name following #pragma")
	}
	return st
}

func parseBytes(p *Parser, d token) *Bytes {
	st := &Bytes{stbase: stbase{src: p.doc, line: d.line}}
	for {
		switch tok := p.next(); tok.typ {
		case lineEnd, eof:
			p.throwError(d, "expected expression following #bytes")

		case label:
			// "named bytes"
			if st.Label != nil {
				p.throwError(d, "extra label on #bytes")
			}
			st.Label = &LabelDef{
				stbase: st.stbase,
				Dotted: true, // always dotted
				Ident:  tok.text,
			}

		default:
			st.Value = parseExpr(p, tok)

			// For named bytes, register them as both a macro and label.
			if st.Label != nil {
				p.doc.labels[st.Label.Ident] = st.Label
				p.doc.exprMacros[st.Label.Ident] = &ExpressionMacroDef{
					Ident: st.Label.Ident,
					Body:  st.Value,
				}
			}
			return st
		}
	}
}

func parseOpcode(p *Parser, tok token) *Opcode {
	st := &Opcode{
		stbase: stbase{src: p.doc, line: tok.line},
		Op:     tok.text,
	}
	size, isPush := parsePushSize(tok.text)
	if isPush {
		st.PushSize = byte(size + 1)
	}

	// Parse optional immediates in brackets.
	argToken := p.next()
	if argToken.typ == openBracket {
		st.Immediates = parseImmediates(p)
		argToken = p.next()
	}
	// Parse optional argument.
	switch argToken.typ {
	case lineEnd, eof, comment:
		p.unread(argToken)
	default:
		st.Arg = parseExpr(p, argToken)
	}
	return st
}

func parseImmediates(p *Parser) []int {
	const limit = 2 // how many immediates allowed

	var args []int
	for {
		tok := p.next()
		switch tok.typ {
		case numberLiteral:
			n, _ := lzint.ParseNumberLiteral(tok.text)
			if n.IntegerBitLen() > 8 {
				p.throwError(tok, "immediate value > 8 bits")
			}
			args = append(args, int(n.Int().Int64()))
		case lineEnd, eof, comment:
			p.throwError(tok, "unexpected end of immediates")
		default:
			p.throwError(tok, "expected number in immediates list")
		}
		tok = p.next()
		switch tok.typ {
		case closeBracket:
			return args
		case comma:
			if len(args) >= limit {
				p.throwError(tok, "too many immediates")
			}
		case lineEnd, eof, comment:
			p.throwError(tok, "unexpected end of immediates")
		default:
			p.throwError(tok, "expected ',' or ']'")
		}
	}
}

var sizedPushRE = regexp.MustCompile("(?i)^PUSH([0-9]*)$")

func parsePushSize(name string) (int, bool) {
	m := sizedPushRE.FindStringSubmatch(name)
	if len(m) == 0 {
		return 0, false
	}
	if len(m[1]) > 0 {
		sz, _ := strconv.Atoi(m[1])
		return sz, true
	}
	return -1, true
}

func parseInstructionMacroCall(p *Parser, nameTok token) *InstructionMacroCall {
	st := &InstructionMacroCall{
		stbase: stbase{src: p.doc, line: nameTok.line},
		Ident:  nameTok.text,
	}
	switch tok := p.next(); tok.typ {
	case lineEnd, eof, comment:
		p.unread(tok)
	case openParen:
		st.Args = parseCallArguments(p)
	default:
		p.unexpected(tok)
	}
	return st
}

// parseExpr parses an expression.
func parseExpr(p *Parser, tok token) Expr {
	left := parsePrimaryExpr(p, tok)
	return parseArith(p, left, p.next(), 0)
}

// parseArith parses an arithmetic expression.
func parseArith(p *Parser, left Expr, tok token, minPrecedence int) Expr {
	for ; ; tok = p.next() {
		// Check for (another) arithmetic op.
		var op ArithOp
		switch tok.typ {
		case arith:
			op = tokenArithOp(tok)
			if op.Precedence() < minPrecedence {
				p.unread(tok)
				return left
			}
		default:
			// End of binary expression.
			p.unread(tok)
			return left
		}

		// Parse right operand.
		var right Expr
		switch tok = p.next(); tok.typ {
		case comma, closeParen, closeBrace, lineEnd, eof:
			p.throwError(tok, "expected right operand in arithmetic expression")
		default:
			right = parsePrimaryExpr(p, tok)
		}

		// Check for next op of higher precedence.
		right = parseArithInner(p, right, op.Precedence())

		// Combine into binary expression.
		left = &BinaryExpr{
			Op:    op,
			Left:  left,
			Right: right,
			pos:   Position{p.doc.File, tok.line},
		}
	}
}

func parseArithInner(p *Parser, right Expr, curPrecedence int) Expr {
	for {
		switch tok := p.next(); tok.typ {
		case arith:
			nextop := tokenArithOp(tok)
			if nextop.Precedence() <= curPrecedence {
				p.unread(tok)
				return right
			}
			right = parseArith(p, right, tok, curPrecedence+1)

		default:
			p.unread(tok)
			return right
		}
	}
}

func parsePrimaryExpr(p *Parser, tok token) Expr {
	switch tok.typ {
	case identifier, dottedIdentifier:
		call := &MacroCallExpr{
			Ident:   tok.text,
			Builtin: tok.typ == dottedIdentifier,
			pos:     Position{p.doc.File, tok.line},
		}
		switch tok := p.next(); tok.typ {
		case openParen:
			call.Args = parseCallArguments(p)
		default:
			p.unread(tok)
		}
		return call

	case variableIdentifier:
		return &VariableExpr{
			Ident: tok.text,
			pos:   Position{p.doc.File, tok.line},
		}

	case labelRef, dottedLabelRef:
		return &LabelRefExpr{
			Ident:  tok.text,
			Dotted: tok.typ == dottedLabelRef,
			pos:    Position{p.doc.File, tok.line},
		}

	case numberLiteral:
		v, err := lzint.ParseNumberLiteral(tok.text)
		if err != nil {
			p.throwError(tok, "invalid number literal: %v", err)
			return nil
		}
		return &LiteralExpr{
			text:  tok.text,
			value: v,
			pos:   Position{p.doc.File, tok.line},
		}

	case stringLiteral:
		bytes, err := parseStringText(tok.text)
		if err != nil {
			p.throwError(tok, "%w", err)
			return nil
		}
		return &LiteralExpr{
			text:   tok.text,
			value:  lzint.FromBytes(bytes),
			string: true,
			pos:    Position{p.doc.File, tok.line},
		}

	case arith:
		return parseUnaryExpr(p, tok)

	case openParen:
		return parseParenExpr(p, tok)

	default:
		p.unexpected(tok)
		return nil
	}
}

func parseUnaryExpr(p *Parser, tok token) Expr {
	switch op := tokenArithOp(tok); op {
	case ArithMinus:
		arg := parsePrimaryExpr(p, p.next())
		return &UnaryExpr{
			Op:  op,
			Arg: arg,
			pos: Position{p.doc.File, tok.line},
		}
	default:
		p.throwError(tok, "unexpected arithmetic op %v", op)
		return nil
	}
}

func parseParenExpr(p *Parser, openParen token) Expr {
	var expr *GroupExpr
	switch tok := p.next(); tok.typ {
	case closeParen:
		p.throwError(tok, "empty parenthesized expression")
		return nil
	default:
		expr = &GroupExpr{
			Inner: parseExpr(p, tok),
			pos:   Position{File: p.doc.File, Line: openParen.line},
		}
	}
	// Ensure closing paren is there.
	for {
		switch tok := p.next(); tok.typ {
		case closeParen:
			return expr
		case lineStart, lineEnd:
			continue
		default:
			p.unexpected(tok)
		}
	}
}

// parseParameterList parses a comma-separated list of names.
func parseParameterList(p *Parser) (names []string) {
	for {
		tok := p.next()
		switch tok.typ {
		case closeParen:
			return names
		case identifier:
			names = append(names, tok.text)
		default:
			p.unexpected(tok)
		}
		if parseListEnd(p) {
			return names
		}
	}
}

// parseCallArguments parses the argument list of a macro call.
func parseCallArguments(p *Parser) (args []Expr) {
	for {
		tok := p.next()
		switch tok.typ {
		case closeParen:
			return args
		default:
			if arg := parseExpr(p, tok); arg != nil {
				args = append(args, arg)
			}
		}
		if parseListEnd(p) {
			return args
		}
	}
}

func parseListEnd(p *Parser) bool {
	for {
		tok := p.next()
		switch tok.typ {
		case comma:
			return false
		case lineStart, lineEnd:
			continue
		case closeParen:
			return true
		default:
			p.unexpected(tok)
		}
	}
}

// parseStringText parses characters and escape sequences in a string literal.
func parseStringText(s string) ([]byte, error) {
	var result bytes.Buffer
	result.Grow(len(s))

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if r := runes[i]; r != '\\' {
			result.WriteRune(r)
			continue
		}

		// Escape sequence.
		if i+1 >= len(runes) {
			return nil, fmt.Errorf("incomplete escape sequence at end of string")
		}
		next := runes[i+1]
		if next == 'x' {
			// \xXX hex sequences are for specifying arbitrary bytes.
			if i+3 >= len(runes) {
				return nil, fmt.Errorf("incomplete hex escape sequence in string")
			}
			hex1 := runes[i+2]
			hex2 := runes[i+3]
			if !isHex(hex1) || !isHex(hex2) {
				return nil, fmt.Errorf("invalid hex escape sequence \\x%c%c in string", hex1, hex2)
			}
			val := hexToByte(hex1)*16 + hexToByte(hex2)
			result.WriteByte(val)
			i += 3 // Skip the 'x' and two hex digits.
		} else {
			var val byte
			switch next {
			case '\\', '"':
				val = byte(next)
			case 'n':
				val = '\n'
			case 'r':
				val = '\r'
			case 't':
				val = '\t'
			default:
				return nil, fmt.Errorf("invalid escape sequence \\%c in string", next)
			}
			result.WriteByte(val)
			i++ // Skip the escaped character.
		}
	}
	return result.Bytes(), nil
}

// hexToByte converts a hex character to its byte value.
func hexToByte(r rune) byte {
	switch {
	case r >= '0' && r <= '9':
		return byte(r - '0')
	case r >= 'a' && r <= 'f':
		return byte(r - 'a' + 10)
	case r >= 'A' && r <= 'F':
		return byte(r - 'A' + 10)
	}
	panic("BUG: invalid hex in hexToByte")
}
