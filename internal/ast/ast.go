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
	"math/big"
	"slices"
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

	labels      map[string]*LabelDefSt
	exprMacros  map[string]*ExpressionMacroDef
	instrMacros map[string]*InstructionMacroDef
}

// LookupLabel finds the definition of a label.
func (doc *Document) LookupLabel(lref *LabelRefExpr) (*LabelDefSt, *Document) {
	for doc != nil {
		li, ok := doc.labels[lref.Ident]
		if ok && li.Dotted == lref.Dotted {
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
func (doc *Document) GlobalLabels() []*LabelDefSt {
	result := make([]*LabelDefSt, 0)
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

func (doc *Document) CreationString() string {
	if doc.Creation == nil {
		if doc.File == "" {
			return ""
		}
		return " in " + doc.File
	}
	return fmt.Sprintf(" by %s at %v", doc.Creation.Description(), doc.Creation.Position())
}

type Statement interface {
	Position() Position
	Description() string
}

// toplevel statement types
type (
	OpcodeSt struct {
		Op       string
		Src      *Document
		Arg      Expr // Immediate argument for PUSH* / JUMP*.
		PushSize byte // For PUSH<n>, this is n+1.
		tok      token
	}

	LabelDefSt struct {
		Src    *Document
		Dotted bool
		Global bool
		tok    token
	}

	MacroCallSt struct {
		Ident string
		Src   *Document
		Args  []Expr
		tok   token
	}

	IncludeSt struct {
		tok      token
		Src      *Document
		Filename string
	}

	AssembleSt struct {
		tok      token
		Src      *Document
		Filename string
	}
)

// definitions
type (
	ExpressionMacroDef struct {
		Name   string
		Params []string
		Body   Expr
		pos    Position
	}

	InstructionMacroDef struct {
		Name   string
		Params []string
		Body   *Document
		pos    Position
	}
)

// expression types
type (
	Expr interface{}

	LiteralExpr struct {
		tok   token
		Value *big.Int
	}

	LabelRefExpr struct {
		Ident  string
		Dotted bool
		Global bool
	}

	VariableExpr struct {
		Ident string
	}

	MacroCallExpr struct {
		Ident   string
		Builtin bool
		Args    []Expr
	}

	ArithExpr struct {
		Op    ArithOp
		Left  Expr
		Right Expr
	}
)

func (inst *MacroCallSt) Position() Position {
	return Position{File: inst.Src.File, Line: inst.tok.line}
}

func (inst *MacroCallSt) Description() string {
	return fmt.Sprintf("invocation of %%%s", inst.Ident)
}

func (inst *IncludeSt) Position() Position {
	return Position{File: inst.Src.File, Line: inst.tok.line}
}

func (inst *IncludeSt) Description() string {
	return fmt.Sprintf("#include %q", inst.Filename)
}

func (inst *AssembleSt) Position() Position {
	return Position{File: inst.Src.File, Line: inst.tok.line}
}

func (inst *AssembleSt) Description() string {
	return fmt.Sprintf("#assemble %q", inst.Filename)
}

func (inst *OpcodeSt) Position() Position {
	return Position{File: inst.Src.File, Line: inst.tok.line}
}

func (inst *OpcodeSt) Description() string {
	return fmt.Sprintf("opcode %s", inst.tok.text)
}

func (inst *LabelDefSt) Position() Position {
	return Position{File: inst.Src.File, Line: inst.tok.line}
}

func (inst *LabelDefSt) Description() string {
	return fmt.Sprintf("definition of %s", inst.String())
}

func (def *InstructionMacroDef) Position() Position {
	return def.pos
}

func (def *InstructionMacroDef) Description() string {
	return fmt.Sprintf("definition of %%%s", def.Name)
}

func (def *ExpressionMacroDef) Position() Position {
	return def.pos
}

func (def *ExpressionMacroDef) Description() string {
	return fmt.Sprintf("definition of %s", def.Name)
}

func (l *LabelRefExpr) String() string {
	dot := ""
	if l.Dotted {
		dot = "."
	}
	return "@" + dot + l.Ident
}

func (l *LabelDefSt) String() string {
	r := LabelRefExpr{Dotted: l.Dotted, Ident: l.tok.text}
	return r.String()
}

func (l *LabelDefSt) Name() string {
	return l.tok.text
}

func (e *LiteralExpr) IsString() bool {
	return e.tok.is(stringLiteral)
}

func (e *LiteralExpr) IsNumber() bool {
	return e.tok.is(numberLiteral)
}

func (e *LiteralExpr) Text() string {
	return e.tok.text
}
