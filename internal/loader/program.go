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
)

// Program represents a source code file and its associated include files.
type Program struct {
	Toplevel    *ast.Document
	Fork        *evm.InstructionSet
	forkDefined bool

	includes map[*ast.Include]*ast.Document
	defs     map[*ast.Document]definitions
	global   definitions
	dupes    []ast.Statement
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

func newProgram(top *ast.Document) *Program {
	return &Program{
		Toplevel: top,
		global:   newDefinitions(),
		includes: make(map[*ast.Include]*ast.Document),
		defs:     make(map[*ast.Document]definitions),
	}
}

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

func (p *Program) registerExprMacro(def *ast.ExpressionMacroDef) error {
	var d definitions
	if ast.IsGlobal(def.Ident) {
		d = p.global
	} else {
		p.initDefinitions(def.Document())
		d = p.defs[def.Document()]
	}
	if d.exprMacro[def.Ident] != nil {
		return fmt.Errorf("expression macro %s already defined", def.Ident)
	}
	d.exprMacro[def.Ident] = def
	return nil
}

func (p *Program) LookupInstrMacro(name string, in *ast.Document) *ast.InstructionMacroDef {
	if ast.IsGlobal(name) {
		return p.global.instrMacro[name]
	}
	for doc := in; doc != nil; doc = doc.Parent {
		m := p.defs[doc].instrMacro[name]
		if m != nil {
			return m
		}
	}
	return nil
}

func (p *Program) registerInstrMacro(def *ast.InstructionMacroDef) error {
	var d definitions
	if ast.IsGlobal(def.Ident) {
		d = p.global
	} else {
		p.initDefinitions(def.Document())
		d = p.defs[def.Document()]
	}
	if d.instrMacro[def.Ident] != nil {
		return fmt.Errorf("instruction macro %s already defined", def.Ident)
	}
	d.instrMacro[def.Ident] = def
	return nil
}

func (p *Program) LookupLabel(name string, in *ast.Document) (*ast.LabelDef, *ast.Document) {
	if ast.IsGlobal(name) {
		return p.global.label[name], nil
	}
	for doc := in; doc != nil; doc = doc.Parent {
		label := p.defs[doc].label[name]
		if label != nil {
			return label, doc
		}
	}
	return nil
}

func (p *Program) registerLabel(def *ast.LabelDef) error {
	var d definitions
	if ast.IsGlobal(def.Ident) {
		d = p.global
	} else {
		p.initDefinitions(def.Document())
		d = p.defs[def.Document()]
	}
	if d.label[def.Ident] != nil {
		return fmt.Errorf("label %s already defined", def.Ident)
	}
	d.label[def.Ident] = def
	return nil
}

func (p *Program) initDefinitions(doc *ast.Document) {
	if _, ok := p.defs[doc]; !ok {
		p.defs[doc] = newDefinitions()
	}
}
