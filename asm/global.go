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
	"unicode"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

// isGlobal returns true when 'name' is a global identifier.
func isGlobal(name string) bool {
	return len(name) > 0 && unicode.IsUpper([]rune(name)[0])
}

// globalScope holds definitions across files.
type globalScope struct {
	labelPC    map[string]int
	label      map[string]*labelDefInstruction
	labelDoc   map[string]*document
	instrMacro map[string]globalDef[*instructionMacroDef]
	exprMacro  map[string]globalDef[*expressionMacroDef]
}

type globalDef[M any] struct {
	def M
	doc *document
}

func newGlobalScope() *globalScope {
	return &globalScope{
		label:      make(map[string]*labelDefInstruction),
		labelPC:    make(map[string]int),
		labelDoc:   make(map[string]*document),
		instrMacro: make(map[string]globalDef[*instructionMacroDef]),
		exprMacro:  make(map[string]globalDef[*expressionMacroDef]),
	}
}

// registerDefinitions processes a document and registers the globals contained in it.
func (gs *globalScope) registerDefinitions(doc *document) (errs []error) {
	for _, li := range doc.labels {
		if li.global {
			gs.registerLabel(li, doc)
		}
	}
	for _, name := range sortedKeys(doc.exprMacros) {
		if isGlobal(name) {
			m := doc.exprMacros[name]
			def := globalDef[*expressionMacroDef]{m, doc}
			if err := gs.registerExprMacro(name, def); err != nil {
				errs = append(errs, err)
			}
		}
	}
	for _, name := range sortedKeys(doc.instrMacros) {
		if isGlobal(name) {
			m := doc.instrMacros[name]
			def := globalDef[*instructionMacroDef]{m, doc}
			if err := gs.registerInstrMacro(name, def); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

func sortedKeys[K constraints.Ordered, V any](m map[K]V) []K {
	keys := maps.Keys(m)
	slices.Sort(keys)
	return keys
}

// registerLabel registers a label as known.
func (gs *globalScope) registerLabel(def *labelDefInstruction, doc *document) {
	name := def.tok.text
	_, found := gs.label[name]
	if !found {
		gs.label[name] = def
	}
}

// registerInstrMacro registers the first definition of an instruction macro.
func (gs *globalScope) registerInstrMacro(name string, def globalDef[*instructionMacroDef]) error {
	firstDef, found := gs.instrMacro[name]
	if found {
		return &astError{
			inst: def.def,
			err:  fmt.Errorf("macro %%%s already defined%s", name, documentCreationString(firstDef.doc)),
		}
	}
	gs.instrMacro[name] = def
	return nil
}

// registerExprMacro registers the first definition of an expression macro.
func (gs *globalScope) registerExprMacro(name string, def globalDef[*expressionMacroDef]) error {
	firstDef, found := gs.exprMacro[name]
	if found {
		return &astError{
			inst: def.def,
			err:  fmt.Errorf("macro %s already defined%s", name, documentCreationString(firstDef.doc)),
		}
	}
	gs.exprMacro[name] = def
	return nil
}

func (gs *globalScope) lookupInstrMacro(name string) (*instructionMacroDef, *document) {
	gdef := gs.instrMacro[name]
	return gdef.def, gdef.doc
}

func (gs *globalScope) lookupExprMacro(name string) (*expressionMacroDef, *document) {
	gdef := gs.exprMacro[name]
	return gdef.def, gdef.doc
}

// setLabelDocument registers the document that a label was created in. This is subtly
// different from the source document of the labelDefInstruction. The distinction matters
// for labels created by macros, because macros create a new document on expansion.
//
// These documents need to be tracked here in order to report the first macro invocation
// or #include statement that created a label.
func (gs *globalScope) setLabelDocument(li *labelDefInstruction, doc *document) error {
	name := li.tok.text
	firstDefDoc := gs.labelDoc[name]
	if firstDefDoc == nil {
		gs.labelDoc[name] = doc
		return nil
	}
	firstDef := gs.label[name]
	err := errLabelAlreadyDef(firstDef, li)
	if loc := documentCreationString(firstDefDoc); loc != "" {
		err = fmt.Errorf("%w%s", err, loc)
	}
	return &astError{inst: li, err: err}
}

// setLabelPC is called by the compiler when the PC value of a label becomes available.
func (gs *globalScope) setLabelPC(name string, pc int) {
	gs.labelPC[name] = pc
}

// lookupLabel returns the PC value of a label, and also reports whether the label was found at all.
func (gs *globalScope) lookupLabel(lref *labelRefExpr) (pc int, pcValid bool, def *labelDefInstruction) {
	li, ok := gs.label[lref.ident]
	if !ok {
		return 0, false, nil
	}
	pc, pcValid = gs.labelPC[lref.ident]
	return pc, pcValid, li
}

func documentCreationString(doc *document) string {
	if doc.creation == nil {
		if doc.file == "" {
			return ""
		}
		return " in " + doc.file
	}
	return fmt.Sprintf(" by %s at %v", doc.creation.description(), doc.creation.position())
}
