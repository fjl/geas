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
	"fmt"
	"regexp"
	"strconv"
)

// Parser performs parsing of the token stream.
type Parser struct {
	in     <-chan token
	buffer []token
	doc    *Document
	errors []*ParseError
}

// NewParser creates a parser.
func NewParser(file string, content []byte, debug bool) *Parser {
	return &Parser{
		in:  runLexer(content, debug),
		doc: newDocument(file, nil),
	}
}

func newDocument(file string, parent *Document) *Document {
	return &Document{
		File:        file,
		labels:      make(map[string]*LabelDefSt),
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
	p.throwError(tok, "unexpected %s %s", tok.typ.String(), tok.text)
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

// ------------- start parser functions -------------

func parseStatement(p *Parser) (done bool) {
	switch tok := p.next(); tok.typ {
	case eof, closeBrace:
		if p.atDocumentTop() != (tok.typ == eof) {
			p.unexpected(tok)
		}
		return true
	case label, dottedLabel:
		parseLabelDef(p, tok)
	case directive:
		parseDirective(p, tok)
	case identifier:
		parseInstruction(p, tok)
	case instMacroIdent:
		parseInstructionMacroCall(p, tok)
	case lineStart, lineEnd:
		return false
	default:
		p.unexpected(tok)
	}
	return false
}

func parseLabelDef(p *Parser, tok token) {
	name := tok.text
	li := &LabelDefSt{
		tok:    tok,
		Src:    p.doc,
		Dotted: tok.typ == dottedLabel,
		Global: IsGlobal(name),
	}
	p.doc.Statements = append(p.doc.Statements, li)
	if firstDef, ok := p.doc.labels[name]; ok {
		p.throwError(tok, "%w", ErrLabelAlreadyDef(firstDef, li))
		return
	}
	p.doc.labels[name] = li
}

func parseDirective(p *Parser, tok token) {
	switch tok.text {
	case "#define":
		if !p.atDocumentTop() {
			p.throwError(tok, "nested macro definitions are not allowed")
		}
		parseMacroDef(p)

	case "#include":
		parseInclude(p, tok)

	case "#assemble":
		parseAssemble(p, tok)

	default:
		p.throwError(tok, "unknown compiler directive %q", tok.text)
	}
}

func parseMacroDef(p *Parser) {
	name := p.next()
	switch name.typ {
	case identifier:
	case instMacroIdent:
		parseInstructionMacroDef(p, name)
		return
	default:
		p.unexpected(name)
	}

	// Parse parameters and body.
	pos := Position{File: p.doc.File, Line: name.line}
	def := &ExpressionMacroDef{Name: name.text, pos: pos}
	var didParams bool
loop:
	for {
		tok := p.next()
		switch tok.typ {
		case lineEnd, eof:
			p.throwError(tok, "incomplete macro definition")

		case openBrace:
			p.throwError(tok, "unexpected { in expression macro definition")

		case openParen:
			if !didParams {
				def.Params = parseParameterList(p)
				didParams = true
				continue
			}
			fallthrough
		default:
			def.Body = parseExpr(p, tok)
			break loop
		}
	}

	// Register the macro.
	checkDuplicateMacro(p, name)
	p.doc.exprMacros[name.text] = def
}

func parseInstructionMacroDef(p *Parser, nameTok token) {
	var params []string
	var didParams bool
paramLoop:
	for {
		switch tok := p.next(); tok.typ {
		case lineEnd, eof:
			p.throwError(tok, "incomplete macro definition")

		case openBrace:
			// start of body
			break paramLoop

		case openParen:
			if !didParams {
				params = parseParameterList(p)
				didParams = true
				continue paramLoop
			}
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
	pos := Position{File: p.doc.File, Line: nameTok.line}
	def := &InstructionMacroDef{Name: nameTok.text, pos: pos, Params: params, Body: doc}
	doc.Creation = def
	topdoc.instrMacros[nameTok.text] = def
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

func parseInclude(p *Parser, d token) {
	instr := &IncludeSt{Src: p.doc, tok: d}
	switch tok := p.next(); tok.typ {
	case stringLiteral:
		instr.Filename = tok.text
		p.doc.Statements = append(p.doc.Statements, instr)
	default:
		p.throwError(tok, "expected filename following #include")
	}
}

func parseAssemble(p *Parser, d token) {
	instr := &AssembleSt{Src: p.doc, tok: d}
	switch tok := p.next(); tok.typ {
	case stringLiteral:
		instr.Filename = tok.text
		p.doc.Statements = append(p.doc.Statements, instr)
	default:
		p.throwError(tok, "expected filename following #assemble")
	}
}

func parseInstruction(p *Parser, tok token) {
	opcode := &OpcodeSt{Op: tok.text, Src: p.doc, tok: tok}
	size, isPush := parsePushSize(tok.text)
	if isPush {
		opcode.PushSize = byte(size + 1)
	}

	// Register in document.
	p.doc.Statements = append(p.doc.Statements, opcode)

	// Parse optional argument.
	argToken := p.next()
	switch argToken.typ {
	case lineEnd, eof:
		return
	default:
		opcode.Arg = parseExpr(p, argToken)
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

func parseInstructionMacroCall(p *Parser, nameTok token) {
	call := &MacroCallSt{Src: p.doc, Ident: nameTok.text, tok: nameTok}
	p.doc.Statements = append(p.doc.Statements, call)

	switch tok := p.next(); tok.typ {
	case lineEnd, eof:
		return
	case openParen:
		call.Args = parseCallArguments(p)
	default:
		p.unexpected(tok)
	}
}

func parseExpr(p *Parser, tok token) Expr {
	switch tok.typ {
	case identifier, dottedIdentifier:
		arg := &MacroCallExpr{Ident: tok.text, Builtin: tok.typ == dottedIdentifier}
		return parseExprTail(p, arg)

	case variableIdentifier:
		arg := &VariableExpr{Ident: tok.text}
		return parseExprTail(p, arg)

	case labelRef, dottedLabelRef:
		arg := &LabelRefExpr{
			Ident:  tok.text,
			Dotted: tok.typ == dottedLabelRef,
			Global: IsGlobal(tok.text),
		}
		return parseExprTail(p, arg)

	case numberLiteral, stringLiteral:
		arg := &LiteralExpr{tok: tok}
		return parseExprTail(p, arg)

	case openParen:
		e := parseParenExpr(p)
		return parseExprTail(p, e)

	default:
		p.unexpected(tok)
		return nil
	}
}

func parseParenExpr(p *Parser) Expr {
	var expr Expr
	switch tok := p.next(); tok.typ {
	case closeParen:
		p.throwError(tok, "empty parenthesized expression")
		return nil
	default:
		expr = parseExpr(p, tok)
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

// parseExprTail parses the end of an expression. Here we check whether the expression
// is a binary arithmetic operation.
func parseExprTail(p *Parser, arg Expr) Expr {
	for {
		tok := p.next()
		switch {
		case tok.is(closeParen, lineEnd, comma, closeBrace, eof):
			p.unread(tok)
			return arg

		case tok.is(openParen):
			call, ok := arg.(*MacroCallExpr)
			if !ok {
				p.unexpected(tok)
			}
			call.Args = parseCallArguments(p)
			arg = call // continue parsing for arith binop after call

		case tok.typ == arith:
			return parseArith(p, arg, tok)

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

func parseArith(p *Parser, arg Expr, opToken token) Expr {
	expr := &ArithExpr{Op: tokenArithOp(opToken), Left: arg}
	tok := p.next()
	switch tok.typ {
	case lineEnd, eof, closeParen:
		p.throwError(tok, "expected right operand in arithmetic expression")
	default:
		expr.Right = parseExpr(p, tok)
	}
	return expr
}
