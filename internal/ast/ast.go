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
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Document is the toplevel of the AST. It represents a list of abstract instructions and
// macro definitions.
type Document struct {
	File       string
	Statements []Statement

	// The document that contains/encloses this document.
	Parent *Document

	// The statement that created this document.
	// This is filled in for instruction macros, #include/#assemble, etc.
	Creation Statement

	labels      map[string]*LabelDef
	exprMacros  map[string]*ExpressionMacroDef
	instrMacros map[string]*InstructionMacroDef
}

// LookupLabel finds the definition of a label.
func (doc *Document) LookupLabel(lref *LabelRefExpr) (*LabelDef, *Document) {
	for doc != nil {
		li, ok := doc.labels[lref.Ident]
		if ok {
			return li, doc
		}
		doc = doc.Parent
	}
	return nil, nil
}

// LookupInstrMacro finds the definition of an instruction macro.
func (doc *Document) LookupInstrMacro(name string) (*InstructionMacroDef, *Document) {
	for doc != nil {
		if def, ok := doc.instrMacros[name]; ok {
			return def, doc
		}
		doc = doc.Parent
	}
	return nil, nil
}

// LookupExprMacro finds the definition of an expression macro.
func (doc *Document) LookupExprMacro(name string) (*ExpressionMacroDef, *Document) {
	for doc != nil {
		if def, ok := doc.exprMacros[name]; ok {
			return def, doc
		}
		doc = doc.Parent
	}
	return nil, nil
}

// GlobalLabels returns the list of global label definitions in the docment.
func (doc *Document) GlobalLabels() []*LabelDef {
	result := make([]*LabelDef, 0)
	for _, name := range slices.Sorted(maps.Keys(doc.labels)) {
		if IsGlobal(name) {
			result = append(result, doc.labels[name])
		}
	}
	return result
}

// GlobalExprMacros returns the list of global expression macro definitions in the docment.
func (doc *Document) GlobalExprMacros() []*ExpressionMacroDef {
	result := make([]*ExpressionMacroDef, 0)
	for _, name := range slices.Sorted(maps.Keys(doc.exprMacros)) {
		if IsGlobal(name) {
			result = append(result, doc.exprMacros[name])
		}
	}
	return result
}

// GlobalInstrMacros returns the list of global instruction macro definitions in the docment.
func (doc *Document) GlobalInstrMacros() []*InstructionMacroDef {
	result := make([]*InstructionMacroDef, 0)
	for _, name := range slices.Sorted(maps.Keys(doc.instrMacros)) {
		if IsGlobal(name) {
			result = append(result, doc.instrMacros[name])
		}
	}
	return result
}

// InstrMacros returns the list of all instruction macro definitions in the docment.
func (doc *Document) InstrMacros() []*InstructionMacroDef {
	result := make([]*InstructionMacroDef, 0)
	for _, name := range slices.Sorted(maps.Keys(doc.instrMacros)) {
		result = append(result, doc.instrMacros[name])
	}
	return result
}

// CreationString explains how the document got into the program.
func (doc *Document) CreationString() string {
	if doc.Creation == nil {
		if doc.File == "" {
			return ""
		}
		return " in " + doc.File
	}
	return fmt.Sprintf(" by %s at %v", doc.Creation.Description(), doc.Creation.Position())
}

// Statement represents a statement in a source file.
type Statement interface {
	base() *stbase

	// Description is displayed in some error messages referring the statement.
	Description() string

	// Position tells where the statement is in the source.
	Position() Position

	// Comment returns the comment that's on the same line as the statement, if any.
	Comment() *Comment

	// StartsBlock returns true when the source code contains one or more blank lines
	// before the statement.
	StartsBlock() bool
}

// stbase is embedded into all statement types.
type stbase struct {
	src         *Document
	line        int
	comment     *Comment
	startsBlock bool
}

func (st *stbase) base() *stbase {
	return st
}

func (st *stbase) Position() Position {
	return Position{st.src.File, st.line}
}

func (st *stbase) Document() *Document {
	return st.src
}

func (st *stbase) Comment() *Comment {
	return st.comment
}

func (st *stbase) StartsBlock() bool {
	return st.startsBlock
}

// toplevel statement types
type (
	Opcode struct {
		stbase
		Op       string
		Arg      Expr // Immediate argument for PUSH* / JUMP*.
		PushSize byte // For PUSH<n>, this is n+1.
	}

	LabelDef struct {
		stbase
		Ident  string // label name
		Dotted bool   // whether definition is dotted
	}

	InstrMacroCall struct {
		stbase
		Ident string
		Args  []Expr
	}

	Include struct {
		stbase
		Filename string
	}

	Assemble struct {
		stbase
		Filename string
	}

	Pragma struct {
		stbase
		Option string
		Value  string
	}

	Bytes struct {
		stbase
		Value Expr
		Label *LabelDef
	}

	Comment struct {
		stbase
		Text string
	}

	// definitions

	ExpressionMacroDef struct {
		stbase
		Ident  string
		Params []string
		Body   Expr
	}

	InstructionMacroDef struct {
		stbase
		Ident  string
		Params []string
		Body   *Document

		// This is the comment attached to the line where the opening brace { is. It's
		// tracked separately because this is typically where the input stack of the macro
		// will be documented.
		StartComment *Comment
	}
)

func (st *InstrMacroCall) Description() string {
	return fmt.Sprintf("invocation of %%%s", st.Ident)
}

func (st *Include) Description() string {
	return fmt.Sprintf("#include %q", st.Filename)
}

func (st *Assemble) Description() string {
	return fmt.Sprintf("#assemble %q", st.Filename)
}

func (st *Pragma) Description() string {
	return fmt.Sprintf("#pragma %s %q", st.Option, st.Value)
}

func (st *Bytes) Description() string {
	return "#bytes"
}

func (st *Opcode) Description() string {
	return fmt.Sprintf("opcode %s", st.Op)
}

func (st *Comment) Description() string {
	return "comment"
}

func (st *InstructionMacroDef) Description() string {
	return fmt.Sprintf("definition of %%%s", st.Ident)
}

func (st *ExpressionMacroDef) Description() string {
	return fmt.Sprintf("definition of %s", st.Ident)
}

func (st *LabelDef) Description() string {
	return fmt.Sprintf("definition of %v", st.Ref())
}

// Ref returns a label reference for the definition.
func (st *LabelDef) Ref() *LabelRefExpr {
	return &LabelRefExpr{Dotted: st.Dotted, Ident: st.Ident}
}

// String returns the text of the definition.
func (st *LabelDef) String() string {
	var s strings.Builder
	s.Grow(len(st.Ident) + 2)
	if st.Dotted {
		s.WriteByte('.')
	}
	s.WriteString(st.Ident)
	s.WriteByte(':')
	return s.String()
}

// Level returns the number of semicolons in the comment.
func (st *Comment) Level() int {
	var count int
	for _, c := range st.Text {
		if c != ';' {
			break
		}
		count++
	}
	return count
}

// IsStackComment reports whether the comment is a conventional
// stack effect docmentation.
func (st *Comment) IsStackComment() bool {
	t := st.InnerText()
	return len(t) > 0 && t[0] == '['
}

// InnerText returns the actual text of the comment.
func (st *Comment) InnerText() string {
	return strings.TrimSpace(strings.TrimLeft(st.Text, ";"))
}
