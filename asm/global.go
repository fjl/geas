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

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/lzint"
)

// globalScope holds definitions across files.
type globalScope struct {
	label      map[string]*ast.LabelDefSt
	labelInstr map[string]*instruction
	labelDoc   map[string]*ast.Document
	instrMacro map[string]globalDef[*ast.InstructionMacroDef]
	exprMacro  map[string]globalDef[*ast.ExpressionMacroDef]
}

type globalDef[M any] struct {
	def M
	doc *ast.Document
}

func newGlobalScope() *globalScope {
	return &globalScope{
		label:      make(map[string]*ast.LabelDefSt),
		labelInstr: make(map[string]*instruction),
		labelDoc:   make(map[string]*ast.Document),
		instrMacro: make(map[string]globalDef[*ast.InstructionMacroDef]),
		exprMacro:  make(map[string]globalDef[*ast.ExpressionMacroDef]),
	}
}

// registerDefinitions processes a document and registers the globals contained in it.
func (gs *globalScope) registerDefinitions(doc *ast.Document) (errs []error) {
	for _, li := range doc.GlobalLabels() {
		gs.registerLabel(li)
	}
	for _, mac := range doc.GlobalExprMacros() {
		def := globalDef[*ast.ExpressionMacroDef]{mac, doc}
		if err := gs.registerExprMacro(mac.Name, def); err != nil {
			errs = append(errs, err)
		}
	}
	for _, mac := range doc.GlobalInstrMacros() {
		def := globalDef[*ast.InstructionMacroDef]{mac, doc}
		if err := gs.registerInstrMacro(mac.Name, def); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// registerLabel registers a label as known.
func (gs *globalScope) registerLabel(def *ast.LabelDefSt) {
	_, found := gs.label[def.Name()]
	if !found {
		gs.label[def.Name()] = def
	}
}

// registerInstrMacro registers the first definition of an instruction macro.
func (gs *globalScope) registerInstrMacro(name string, def globalDef[*ast.InstructionMacroDef]) error {
	firstDef, found := gs.instrMacro[name]
	if found {
		return &statementError{
			inst: def.def,
			err:  fmt.Errorf("macro %%%s already defined%s", name, firstDef.doc.CreationString()),
		}
	}
	gs.instrMacro[name] = def
	return nil
}

// registerExprMacro registers the first definition of an expression macro.
func (gs *globalScope) registerExprMacro(name string, def globalDef[*ast.ExpressionMacroDef]) error {
	firstDef, found := gs.exprMacro[name]
	if found {
		return &statementError{
			inst: def.def,
			err:  fmt.Errorf("macro %s already defined%s", name, firstDef.doc.CreationString()),
		}
	}
	gs.exprMacro[name] = def
	return nil
}

// overrideExprMacroValue sets a macro to the given value, overriding its definition.
func (gs *globalScope) overrideExprMacroValue(name string, val *lzint.Value) {
	gs.exprMacro[name] = globalDef[*ast.ExpressionMacroDef]{
		doc: nil,
		def: &ast.ExpressionMacroDef{
			Name: name,
			Body: ast.MakeNumber(val),
		},
	}
}

func (gs *globalScope) lookupInstrMacro(name string) (*ast.InstructionMacroDef, *ast.Document) {
	gdef := gs.instrMacro[name]
	return gdef.def, gdef.doc
}

func (gs *globalScope) lookupExprMacro(name string) (*ast.ExpressionMacroDef, *ast.Document) {
	gdef := gs.exprMacro[name]
	return gdef.def, gdef.doc
}

// setLabelDocument registers the document that a label was created in. This is subtly
// different from the source document of the labelDefInstruction. The distinction matters
// for labels created by macros, because macros create a new document on expansion.
//
// These documents need to be tracked here in order to report the first macro invocation
// or #include statement that created a label.
func (gs *globalScope) setLabelDocument(li *ast.LabelDefSt, doc *ast.Document) error {
	name := li.Name()
	firstDefDoc := gs.labelDoc[name]
	if firstDefDoc == nil {
		gs.labelDoc[name] = doc
		return nil
	}
	firstDef := gs.label[name]
	err := ast.ErrLabelAlreadyDef(firstDef, li)
	if loc := firstDefDoc.CreationString(); loc != "" {
		err = fmt.Errorf("%w%s", err, loc)
	}
	return err
}

// setLabelInstr sets the next instruction after a label.
func (gs *globalScope) setLabelInstr(name string, instr *instruction) {
	gs.labelInstr[name] = instr
}

// lookupLabel returns the PC value of a label, and also reports whether the label was found at all.
func (gs *globalScope) lookupLabel(lref *ast.LabelRefExpr) (instr *instruction, def *ast.LabelDefSt) {
	li, ok := gs.label[lref.Ident]
	if !ok {
		return nil, nil
	}
	instr = gs.labelInstr[lref.Ident]
	return instr, li
}
