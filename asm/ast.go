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

package asm

import (
	"fmt"
	"math/big"
	"regexp"
	"strconv"
)

// document is the toplevel of the AST. It represents a list of abstract instructions and
// macro definitions.
type document struct {
	file         string
	instructions []astInstruction
	labels       map[string]*labelDefInstruction
	exprMacros   map[string]*expressionMacroDef
	instrMacros  map[string]*instructionMacroDef
	parent       *document

	// for compiler
	includes map[*includeInstruction]*document // filled by compiler
	creation astStatement
}

func (doc *document) lookupLabel(lref *labelRefExpr) (*labelDefInstruction, *document) {
	for doc != nil {
		li, ok := doc.labels[lref.ident]
		if ok && li.dotted == lref.dotted {
			return li, doc
		}
		doc = doc.parent
	}
	return nil, nil
}

func (doc *document) lookupInstrMacro(name string) (*instructionMacroDef, *document) {
	for doc != nil {
		if def, ok := doc.instrMacros[name]; ok {
			return def, doc
		}
		doc = doc.parent
	}
	return nil, nil
}

type astStatement interface {
	position() Position
	description() string
}

// toplevel statement types
type (
	opcodeInstruction struct {
		tok      token
		src      *document
		arg      astExpr // Immediate argument for PUSH* / JUMP*.
		pushSize byte    // For PUSH<n>, this is n+1.
	}

	labelDefInstruction struct {
		tok    token
		src    *document
		dotted bool
		global bool
	}

	macroCallInstruction struct {
		ident token
		src   *document
		args  []astExpr
	}

	includeInstruction struct {
		tok      token
		src      *document
		filename string
	}
)

// definitions
type (
	expressionMacroDef struct {
		name   string
		pos    Position
		params []string
		body   astExpr
	}

	instructionMacroDef struct {
		name   string
		pos    Position
		params []string
		body   *document
	}
)

// expression types
type (
	astExpr interface {
		eval(e *evaluator, env *evalEnvironment) (*big.Int, error)
	}

	literalExpr struct {
		tok   token
		value *big.Int
	}

	labelRefExpr struct {
		ident  string
		dotted bool
		global bool
	}

	variableExpr struct {
		ident   string
		builtin bool
	}

	macroCallExpr struct {
		ident   string
		builtin bool
		args    []astExpr
	}

	arithExpr struct {
		op    token
		left  astExpr
		right astExpr
	}
)

func (inst *macroCallInstruction) position() Position {
	return Position{File: inst.src.file, Line: inst.ident.line}
}

func (inst *macroCallInstruction) description() string {
	return fmt.Sprintf("invocation of %%%s", inst.ident.text)
}

func (inst *includeInstruction) position() Position {
	return Position{File: inst.src.file, Line: inst.tok.line}
}

func (inst *includeInstruction) description() string {
	return fmt.Sprintf("#include %q", inst.filename)
}

func (inst *opcodeInstruction) position() Position {
	return Position{File: inst.src.file, Line: inst.tok.line}
}

func (inst *opcodeInstruction) description() string {
	return fmt.Sprintf("opcode %s", inst.tok.text)
}

func (inst *labelDefInstruction) position() Position {
	return Position{File: inst.src.file, Line: inst.tok.line}
}

func (inst *labelDefInstruction) description() string {
	return fmt.Sprintf("definition of %s", inst.String())
}

func (def *instructionMacroDef) position() Position {
	return def.pos
}

func (def *instructionMacroDef) description() string {
	return fmt.Sprintf("definition of %%%s", def.name)
}

func (def *expressionMacroDef) position() Position {
	return def.pos
}

func (def *expressionMacroDef) description() string {
	return fmt.Sprintf("definition of %s", def.name)
}

func (l *labelRefExpr) String() string {
	dot := ""
	if l.dotted {
		dot = "."
	}
	return "@" + dot + l.ident
}

func (l *labelDefInstruction) String() string {
	r := labelRefExpr{dotted: l.dotted, ident: l.tok.text}
	return r.String()
}

// parser performs parsing of the token stream.
type parser struct {
	in     <-chan token
	buffer []token
	doc    *document
	errors []*parseError
}

func newParser(file string, content []byte, debug bool) *parser {
	return &parser{
		in:  runLexer(content, debug),
		doc: newDocument(file, nil),
	}
}

func newDocument(file string, parent *document) *document {
	return &document{
		file:        file,
		labels:      make(map[string]*labelDefInstruction),
		exprMacros:  make(map[string]*expressionMacroDef),
		instrMacros: make(map[string]*instructionMacroDef),
		includes:    make(map[*includeInstruction]*document),
		parent:      parent,
	}
}

// next reads the next token from the lexer.
func (p *parser) next() token {
	if len(p.buffer) > 0 {
		t := p.buffer[len(p.buffer)-1]
		p.buffer = p.buffer[:len(p.buffer)-1]
		return t
	}
	t, _ := <-p.in
	return t
}

// unread puts a token back into the queue for reading.
func (p *parser) unread(t token) {
	p.buffer = append(p.buffer, t)
}

// drainLexer runs the lexer to completion.
func (p *parser) drainLexer() {
	for p.next().typ != eof {
	}
}

// throwError adds a new error to the error list.
// The parser is returned to the toplevel and will continue parsing
// at the next line.
func (p *parser) throwError(tok token, format string, args ...any) {
	err := &parseError{tok: tok, file: p.doc.file, err: fmt.Errorf(format, args...)}
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
func (p *parser) unexpected(tok token) {
	p.throwError(tok, "unexpected %s %s", tok.typ.String(), tok.text)
}

// parse runs the parser, outputting a document.
func (p *parser) parse() (*document, []*parseError) {
	defer p.drainLexer()
	for {
		if p.parseOne() {
			return p.doc, p.errors
		}
	}
}

func (p *parser) parseOne() bool {
	defer func() {
		err := recover()
		if _, ok := err.(*parseError); !ok && err != nil {
			panic(err)
		}
	}()
	return parseStatement(p)
}

// parseExpression parses the input as a single expression.
// This is used in evaluator tests.
func (p *parser) parseExpression() (expr astExpr, err error) {
	defer p.drainLexer()
	defer func() {
		e := recover()
		if pe, ok := e.(*parseError); ok {
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
func (p *parser) atDocumentTop() bool {
	return p.doc.parent == nil
}

// ------------- start parser functions -------------

func parseStatement(p *parser) (done bool) {
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

func parseLabelDef(p *parser, tok token) {
	name := tok.text
	li := &labelDefInstruction{
		tok:    tok,
		src:    p.doc,
		dotted: tok.typ == dottedLabel,
		global: isGlobal(name),
	}
	p.doc.instructions = append(p.doc.instructions, li)
	if firstDef, ok := p.doc.labels[name]; ok {
		p.throwError(tok, "%w", errLabelAlreadyDef(firstDef, li))
		return
	}
	p.doc.labels[name] = li
}

func parseDirective(p *parser, tok token) {
	switch tok.text {
	case "#define":
		if !p.atDocumentTop() {
			p.throwError(tok, "nested macro definitions are not allowed")
		}
		parseMacroDef(p)

	case "#include":
		parseInclude(p, tok)

	default:
		p.throwError(tok, "unknown compiler directive %q", tok.text)
	}
}

func parseMacroDef(p *parser) {
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
	pos := Position{File: p.doc.file, Line: name.line}
	def := &expressionMacroDef{name: name.text, pos: pos}
	var didParams bool
loop:
	for {
		tok := p.next()
		switch tok.typ {
		case lineEnd, eof:
			p.throwError(tok, "incomplete macro definition")

		case openBrace:
			p.unexpected(tok)

		case openParen:
			if !didParams {
				def.params = parseParameterList(p)
				didParams = true
				continue
			}
			fallthrough
		default:
			def.body = parseExpr(p, tok)
			break loop
		}
	}

	// Register the macro.
	checkDuplicateMacro(p, name)
	p.doc.exprMacros[name.text] = def
}

func parseInstructionMacroDef(p *parser, nameTok token) {
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
	doc := newDocument(p.doc.file, p.doc)
	p.doc = doc
	defer func() { p.doc = topdoc }()

	// Parse macro body.
	for !parseStatement(p) {
	}

	// Register definition.
	checkDuplicateMacro(p, nameTok)
	pos := Position{File: p.doc.file, Line: nameTok.line}
	def := &instructionMacroDef{name: nameTok.text, pos: pos, params: params, body: doc}
	doc.creation = def
	topdoc.instrMacros[nameTok.text] = def
}

func checkDuplicateMacro(p *parser, nameTok token) {
	name := nameTok.text
	if _, ok := builtinMacros[name]; ok {
		p.throwError(nameTok, "attempt to redefine builtin macro %s", name)
	}
	if _, ok := p.doc.instrMacros[name]; ok {
		p.throwError(nameTok, "instruction macro %s already defined", name)
	}
	if _, ok := p.doc.exprMacros[name]; ok {
		p.throwError(nameTok, "expression macro %s already defined", name)
	}
}

func parseInclude(p *parser, d token) {
	instr := &includeInstruction{src: p.doc, tok: d}
	switch tok := p.next(); tok.typ {
	case stringLiteral:
		instr.filename = tok.text
		p.doc.instructions = append(p.doc.instructions, instr)
	default:
		p.throwError(tok, "expected filename following #include")
	}
}

func parseInstruction(p *parser, tok token) {
	opcode := &opcodeInstruction{src: p.doc, tok: tok}
	size, isPush := parsePushSize(tok.text)
	if isPush {
		opcode.pushSize = byte(size + 1)
	}

	// Register in document.
	p.doc.instructions = append(p.doc.instructions, opcode)

	// Parse optional argument.
	argToken := p.next()
	switch argToken.typ {
	case lineEnd, eof:
		return
	default:
		opcode.arg = parseExpr(p, argToken)
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

func parseInstructionMacroCall(p *parser, nameTok token) {
	call := &macroCallInstruction{src: p.doc, ident: nameTok}
	p.doc.instructions = append(p.doc.instructions, call)

	switch tok := p.next(); tok.typ {
	case lineEnd, eof:
		return
	case openParen:
		call.args = parseCallArguments(p)
	default:
		p.unexpected(tok)
	}
}

func parseExpr(p *parser, tok token) astExpr {
	switch tok.typ {
	case identifier, dottedIdentifier:
		arg := &variableExpr{ident: tok.text, builtin: tok.typ == dottedIdentifier}
		return parseExprTail(p, arg)
	case numberLiteral, stringLiteral:
		arg := &literalExpr{tok: tok}
		return parseExprTail(p, arg)
	case labelRef, dottedLabelRef:
		arg := &labelRefExpr{
			ident:  tok.text,
			dotted: tok.typ == dottedLabelRef,
			global: isGlobal(tok.text),
		}
		return parseExprTail(p, arg)
	case openParen:
		e := parseParenExpr(p)
		return parseExprTail(p, e)
	default:
		p.unexpected(tok)
		return nil
	}
}

func parseParenExpr(p *parser) astExpr {
	var expr astExpr
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
func parseExprTail(p *parser, arg astExpr) astExpr {
	for {
		tok := p.next()
		switch {
		case tok.is(closeParen, lineEnd, comma, closeBrace, eof):
			p.unread(tok)
			return arg
		case tok.is(openParen):
			varExpr, ok := arg.(*variableExpr)
			if !ok {
				p.unexpected(tok)
			}
			call := &macroCallExpr{ident: varExpr.ident, builtin: varExpr.builtin}
			call.args = parseCallArguments(p)
			arg = call // continue parsing for arith binop after call
		case tok.isArith():
			return parseArith(p, arg, tok)
		default:
			p.unexpected(tok)
		}
	}
}

// parseParameterList parses a comma-separated list of names.
func parseParameterList(p *parser) (names []string) {
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
func parseCallArguments(p *parser) (args []astExpr) {
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

func parseListEnd(p *parser) bool {
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

func parseArith(p *parser, arg astExpr, opToken token) astExpr {
	expr := &arithExpr{op: opToken, left: arg}
	tok := p.next()
	switch tok.typ {
	case lineEnd, eof, closeParen:
		p.throwError(tok, "expected right operand in arithmetic expression")
	default:
		expr.right = parseExpr(p, tok)
	}
	return expr
}
