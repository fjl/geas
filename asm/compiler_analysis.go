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

package asm

import (
	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/set"
)

// checkLabelsUsed warns about label definitions that were not hit by the evaluator.
func (c *Compiler) checkLabelsUsed(prog *compilerProg, e *evaluator) {
	// Gather documents referenced by program.
	var docs []*ast.Document
	docset := make(set.Set[*ast.Document])
	macroset := make(set.Set[*ast.InstructionMacroDef])
	for section := range prog.iterSections() {
		// Ensure to walk macroexpansions only once.
		if section.macroArgs != nil {
			if macroset.Includes(section.macroArgs.def) {
				continue
			}
			macroset.Add(section.macroArgs.def)
		}
		if !docset.Includes(section.doc) {
			docset.Add(section.doc)
			docs = append(docs, section.doc)
		}
	}

	// Check against evaluator.
	for _, doc := range docs {
		for _, st := range doc.Statements {
			switch st := st.(type) {
			case *ast.LabelDefSt:
				if !e.isLabelUsed(st) {
					c.warnf(st, "label %s unused in program", st)
				}
			}
		}
	}
}
