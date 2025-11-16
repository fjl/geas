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

package printer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/fjl/geas/internal/ast"
)

const (
	defaultIndent     = "    "
	defaultCommentCol = 32

	// If the detected comment
	autoCommentColMin = 22
	autoCommentColMax = 48
)

// Printer is used to configure AST printing.
type Printer struct {
	out           writer
	bufferWrapped bool // true if `out` above was wrapped in bufio.Writer
	lineLength    int  // length of current line

	lenCache map[ast.Statement]lenCacheEntry

	// settings
	indent        string
	indentSet     bool
	commentCol    int
	commentColSet bool
}

type writer interface {
	WriteString(string) (int, error)
	WriteByte(byte) error
}

type lenCacheEntry struct {
	str string
}

// SetIndent configures the indentation prefix.
func (p *Printer) SetIndent(s string) {
	p.indent = s
	p.indentSet = true
}

// SetCommentColumn configures the indentation prefix.
func (p *Printer) SetCommentColumn(col int) {
	p.commentCol = col
	p.commentColSet = true
}

func (p *Printer) reset(w io.Writer) {
	p.out = bufio.NewWriter(w)
	p.bufferWrapped = true
	p.lenCache = nil

	if !p.indentSet {
		p.indent = defaultIndent
	}
	if !p.commentColSet {
		p.commentCol = defaultCommentCol
	}
}

// Document writes a document to the given writer.
func (p *Printer) Document(w io.Writer, doc *ast.Document) (err error) {
	defer p.finishToplevel(&err)
	p.reset(w)

	// First figure out the column that line comments will be indented to.
	// To do this, we format all opcode argument expressions and store their
	// text into a cache.
	p.lenCache = make(map[ast.Statement]lenCacheEntry)
	p.preFormat(doc)
	if !p.commentColSet {
		p.commentCol = p.computeCommentColumn()
	}

	// Print all statements.
	p.document(doc)
	return
}

// Expr writes an expression to the given writer.
func (p *Printer) Expr(w io.Writer, e ast.Expr) (err error) {
	defer p.finishToplevel(&err)
	p.reset(w)
	p.expr(e, nil)
	return
}

type printError struct {
	e error
}

func (p *Printer) finishToplevel(err *error) {
	panicErr := recover()
	if panicErr == nil {
		// Regular exit, flush output.
		if p.bufferWrapped {
			*err = p.out.(*bufio.Writer).Flush()
		}
	} else if pe, ok := panicErr.(printError); ok {
		// Internal error thrown.
		*err = pe.e
	} else {
		panic(err)
	}
}

// byte outputs a single byte.
func (p *Printer) byte(b byte) {
	p.lineLength += 1
	err := p.out.WriteByte(b)
	if err != nil {
		panic(printError{err})
	}
}

// newline outputs a newline.
func (p *Printer) newline() {
	p.byte('\n')
	p.lineLength = 0
}

// string outputs a string.
func (p *Printer) string(s string) {
	p.lineLength += len(s)
	_, err := p.out.WriteString(s)
	if err != nil {
		panic(printError{err})
	}
}

// expr writes an expression to the output.
func (p *Printer) expr(e ast.Expr, parent ast.Expr) {
	switch e := e.(type) {
	case fmt.Stringer:
		// literals, etc.
		p.string(e.String())
		return

	case *ast.UnaryArithExpr:
		p.string(e.Op.Sign())
		p.expr(e.Arg, e)

	case *ast.ArithExpr:
		// Add parens if the parent is unary or it has higher precedence.
		var paren bool
		var dense bool
		switch pe := parent.(type) {
		case *ast.UnaryArithExpr:
			paren = true
		case *ast.ArithExpr:
			paren = pe.Op.Precedence() > e.Op.Precedence()
			dense = pe.Op.Precedence() < e.Op.Precedence()
		}

		if paren {
			p.byte('(')
		}
		p.expr(e.Left, e)
		if !dense {
			p.byte(' ')
		}
		p.string(e.Op.Sign())
		if !dense {
			p.byte(' ')
		}
		p.expr(e.Right, e)
		if paren {
			p.byte(')')
		}

	case *ast.MacroCallExpr:
		if e.Builtin {
			p.byte('.')
		}
		p.string(e.Ident)
		p.argumentList(e.Args)

	default:
		panic(fmt.Errorf("BUG: unhandled expr type %T", e))
	}
}

// document writes a document to the output.
func (p *Printer) document(doc *ast.Document) {
	// Now print all statements.
	for _, st := range doc.Statements {
		// Add blank line before a block of statements.
		if st.StartsBlock() {
			p.newline()
		}

		// Write the statement itself.
		p.statement(st)

		// Print line comment.
		if st.Comment() != nil {
			// Indent up to comment column.
			for p.lineLength < p.commentCol+1 {
				p.byte(' ')
			}
			p.string("; ") // force one semicolon
			p.string(st.Comment().InnerText())
		}
		p.newline()
	}
}

// preFormat caches the formatted output for statements with an attached line comment.
// This is the first pass of outputting a document, and the cached outputs are used in a
// later stage to determine the comment column.
func (p *Printer) preFormat(doc *ast.Document) {
	prevWriter, prevWrapped := p.out, p.bufferWrapped
	defer func() {
		p.out, p.bufferWrapped = prevWriter, prevWrapped
	}()

	var b bytes.Buffer
	p.out, p.bufferWrapped = &b, false

	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.Comment, *ast.ExpressionMacroDef:
			continue

		case *ast.InstructionMacroDef:
			p.preFormat(st.Body)

		default:
			if st.Comment() == nil {
				continue
			}
			b.Reset()
			p.statement(st)
			p.lenCache[st] = lenCacheEntry{str: b.String()}
		}
	}
}

// computeCommentColumn computes a column to which line comments will be indented.
func (p *Printer) computeCommentColumn() int {
	autocol := autoCommentColMin
	for _, entry := range p.lenCache {
		col := len(entry.str) + 1
		if col > autocol && col < autoCommentColMax {
			autocol = col
		}
	}
	return autocol
}

// statement outputs a statement.
func (p *Printer) statement(st ast.Statement) {
	if cached, ok := p.lenCache[st]; ok {
		p.string(cached.str)
		return
	}

	switch st := st.(type) {
	case *ast.Opcode:
		// TODO: add option to set lowercase/uppercase
		p.string(p.indent)
		p.string(st.Op)
		if st.Arg != nil {
			p.byte(' ')
			p.expr(st.Arg, nil)
		}

	case *ast.Bytes:
		p.string("#bytes")
		if st.Label != nil {
			p.byte(' ')
			p.string(st.Label.String())
		}
		if st.Value != nil {
			p.byte(' ')
			p.expr(st.Value, nil)
		}

	case *ast.LabelDef:
		p.string(st.String())

	case *ast.Assemble:
		p.string("#assemble ")
		p.quotedString(st.Filename)

	case *ast.Include:
		p.string("#include ")
		p.quotedString(st.Filename)

	case *ast.ExpressionMacroDef:
		p.string("#define ")
		p.string(st.Ident)
		p.parameterList(st.Params)
		p.string(" = ")
		p.expr(st.Body, nil)

	case *ast.InstructionMacroDef:
		p.string("#define %")
		p.string(st.Ident)
		p.parameterList(st.Params)
		p.string(" {")
		if len(st.Body.Statements) > 0 {
			p.newline()
			p.document(st.Body)
		}
		p.byte('}')

	case *ast.InstrMacroCall:
		p.string(p.indent)
		p.byte('%')
		p.string(st.Ident)
		p.argumentList(st.Args)

	case *ast.Pragma:
		p.string("#pragma ")
		p.string(st.Option)
		p.byte(' ')
		p.string(st.Value)

	case *ast.Comment:
		switch st.Level() {
		case 1:
			p.string(p.indent)
			for p.lineLength < p.commentCol+1 {
				p.byte(' ')
			}
		case 2:
			p.string(p.indent)
		}
		p.string(st.Text)
	}
}

func (p *Printer) quotedString(s string) {
	p.byte('"')
	for _, c := range s {
		switch c {
		case '\\':
			p.string("\\\\")
		case '"':
			p.string("\\\"")
		default:
			p.string(string(c))
		}
	}
	p.byte('"')
}

func (p *Printer) parameterList(params []string) {
	if len(params) == 0 {
		return
	}
	p.byte('(')
	for i, param := range params {
		if i > 0 {
			p.string(", ")
		}
		p.string(param)
	}
	p.byte(')')
}

func (p *Printer) argumentList(args []ast.Expr) {
	if len(args) == 0 {
		return
	}
	p.byte('(')
	for i, arg := range args {
		if i > 0 {
			p.string(", ")
		}
		p.expr(arg, nil)
	}
	p.byte(')')
}
