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
	"github.com/fjl/geas/internal/evm"
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

// unreachableCodeCheck finds instructions that cannot be reached by execution.
// In the EVM, all jump targets must be marked by JUMPDEST. For terminal instructions
// such as STOP, if the next instruction isn't JUMPDEST, it can never be reached.
type unreachableCodeCheck struct {
	prevSt        ast.Statement
	prevOp        *evm.Op
	inUnreachable bool
}

func (chk *unreachableCodeCheck) check(c *Compiler, st ast.Statement, op *evm.Op) {
	if chk.inUnreachable && op.Name == "JUMPDEST" {
		chk.inUnreachable = false
	}
	if chk.prevOp != nil && (chk.prevOp.Term || chk.prevOp.UnconditionalJump) && op.Name != "JUMPDEST" {
		c.warnf(st, "unreachable code (previous instruction is %s at %v)", chk.prevOp.Name, chk.prevSt.Position())
		chk.inUnreachable = true
	}
	chk.prevSt, chk.prevOp = st, op
}
