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

package loader

import (
	"fmt"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
	"github.com/fjl/geas/internal/set"
)

// Program represents a source code file and its associated include files.
type Program struct {
	Toplevel    *ast.Document
	Fork        *evm.InstructionSet
	forkDefined bool

	includes     map[*ast.Include]*ast.Document
	defs         map[*ast.Document]definitions
	global       definitions
	macroGLabels set.Set[string] // global labels defined in macro bodies
}

type definitions struct {
	label      map[string]*ast.LabelDef
	instrMacro map[string]*ast.InstructionMacroDef
	exprMacro  map[string]*ast.ExpressionMacroDef
}

func newDefinitions() definitions {
	return definitions{
		label:      make(map[string]*ast.LabelDef),
		instrMacro: make(map[string]*ast.InstructionMacroDef),
		exprMacro:  make(map[string]*ast.ExpressionMacroDef),
	}
}

// NewProgram creates a Program with the given top-level document.
// It registers all definitions (labels, macros) found in the document's statements.
func NewProgram(top *ast.Document) *Program {
	p := newProgram(top)
	for _, st := range top.Statements {
		switch st := st.(type) {
		case *ast.LabelDef:
			p.registerLabel(top, st)
		case *ast.ExpressionMacroDef:
			p.registerExprMacro(top, st)
		case *ast.InstructionMacroDef:
			p.registerInstrMacro(top, st)
		}
	}
	return p
}

func newProgram(top *ast.Document) *Program {
	return &Program{
		Toplevel:     top,
		global:       newDefinitions(),
		macroGLabels: make(set.Set[string]),
		includes:     make(map[*ast.Include]*ast.Document),
		defs:         make(map[*ast.Document]definitions),
	}
}

// LookupExprMacro finds the definition of a expression macro.
func (p *Program) LookupExprMacro(name string, in *ast.Document) *ast.ExpressionMacroDef {
	if ast.IsGlobal(name) {
		return p.global.exprMacro[name]
	}
	for doc := in; doc != nil; doc = doc.Parent {
		m := p.defs[doc].exprMacro[name]
		if m != nil {
			return m
		}
	}
	return nil
}

func (p *Program) registerExprMacro(doc *ast.Document, def *ast.ExpressionMacroDef) error {
	var d definitions
	if ast.IsGlobal(def.Ident) {
		d = p.global
	} else {
		p.initDefinitions(doc)
		d = p.defs[doc]
	}
	if existing := d.exprMacro[def.Ident]; existing != nil {
		loc := existing.Document().CreationString()
		return fmt.Errorf("macro %s already defined%s", def.Ident, loc)
	}
	d.exprMacro[def.Ident] = def
	return nil
}

func (p *Program) LookupInstrMacro(name string, in *ast.Document) *ast.InstructionMacroDef {
	if ast.IsGlobal(name) {
		mac := p.global.instrMacro[name]
		if mac != nil {
			return mac
		}
	}
	for doc := in; doc != nil; doc = doc.Parent {
		m := p.defs[doc].instrMacro[name]
		if m != nil {
			return m
		}
	}
	return nil
}

func (p *Program) registerInstrMacro(doc *ast.Document, def *ast.InstructionMacroDef) error {
	var d definitions
	if ast.IsGlobal(def.Ident) {
		d = p.global
	} else {
		p.initDefinitions(doc)
		d = p.defs[doc]
	}
	if existing := d.instrMacro[def.Ident]; existing != nil {
		loc := existing.Body.Parent.CreationString()
		return fmt.Errorf("macro %%%s already defined%s", def.Ident, loc)
	}
	d.instrMacro[def.Ident] = def
	return nil
}

// LookupLabel finds the definition of a label. For global labels defined in macro bodies,
// inMacro is true, indicating the label may not be instantiated in the program.
func (p *Program) LookupLabel(name string, in *ast.Document) (def *ast.LabelDef, inMacro bool) {
	if ast.IsGlobal(name) {
		return p.global.label[name], p.macroGLabels.Includes(name)
	}
	for doc := in; doc != nil; doc = doc.Parent {
		if label := p.defs[doc].label[name]; label != nil {
			return label, false
		}
	}
	return nil, false
}

func (p *Program) registerLabel(doc *ast.Document, def *ast.LabelDef) error {
	if ast.IsGlobal(def.Ident) {
		// The loader registers the first definition it sees and ignores duplicates.
		// All duplicate detection for global labels is handled by the compiler
		// during expansion.
		if p.global.label[def.Ident] == nil {
			p.global.label[def.Ident] = def
		}
		if doc.IsMacro() {
			p.macroGLabels.Add(def.Ident)
		}
	} else {
		p.initDefinitions(doc)
		d := p.defs[doc]
		if existing := d.label[def.Ident]; existing != nil {
			return ast.ErrLabelAlreadyDef(existing, def)
		}
		d.label[def.Ident] = def
	}
	return nil
}

// IncludeDoc returns the parsed document for an include statement.
func (p *Program) IncludeDoc(inc *ast.Include) *ast.Document {
	return p.includes[inc]
}

func (p *Program) initDefinitions(doc *ast.Document) {
	if _, ok := p.defs[doc]; !ok {
		p.defs[doc] = newDefinitions()
	}
}
