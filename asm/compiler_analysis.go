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
)

// checkLabelsUsed warns about label definitions that were not hit by the evaluator.
func (c *Compiler) checkLabelsUsed(doc *ast.Document, e *evaluator) {
	stack := []*ast.Document{doc}
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		for _, st := range top.Statements {
			switch st := st.(type) {
			case *ast.LabelDefSt:
				if !e.isLabelUsed(st) {
					c.warnf(st, "label %s unused in program", st)
				}
			case *ast.IncludeSt:
				if incdoc := c.includes[st]; incdoc != nil {
					stack = append(stack, incdoc)
				}
			}
		}
		stack = stack[1:]
	}
}
