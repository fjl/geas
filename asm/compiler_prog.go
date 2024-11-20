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
	"iter"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
)

// compilerProg is the output program of the compiler.
// It contains sections of instructions.
type compilerProg struct {
	toplevel *compilerSection
	cur      *compilerSection
	evm      *evm.InstructionSet
}

// compilerSection is a section of the output program.
type compilerSection struct {
	doc *ast.Document
	env *evalEnvironment

	// This tracks the arguments of instruction macro calls. When the compiler expands a
	// macro, it creates a unique section for each call site. The arguments of the call
	// are stored for use by the expression evaluator.
	macroArgs *instrMacroArgs

	parent   *compilerSection
	children []any
}

type instrMacroArgs struct {
	callsite *compilerSection
	def      *ast.InstructionMacroDef
	args     []ast.Expr
}

func newCompilerProg(topdoc *ast.Document) *compilerProg {
	p := new(compilerProg)
	p.toplevel = p.pushSection(topdoc, nil)
	return p
}

// pushSection creates a new section as a child of the current one.
func (p *compilerProg) pushSection(doc *ast.Document, macroArgs *instrMacroArgs) *compilerSection {
	s := &compilerSection{doc: doc, macroArgs: macroArgs}
	s.env = newEvalEnvironment(s)
	if p.cur != nil {
		s.parent = p.cur
		p.cur.children = append(p.cur.children, s)
	}
	p.cur = s
	return s
}

// popSection returns to the parent section.
func (p *compilerProg) popSection() {
	if p.cur.parent == nil {
		panic("too much pop")
	}
	p.cur = p.cur.parent
}

// currentSection returns the current (most recently added) section.
func (p *compilerProg) currentSection() *compilerSection {
	return p.cur
}

// addInstruction appends an instruction to the current section.
func (p *compilerProg) addInstruction(inst *instruction) {
	p.cur.children = append(p.cur.children, inst)
}

// iterInstructions returns an iterator over all instructions in the program.
func (p *compilerProg) iterInstructions() iter.Seq2[*compilerSection, *instruction] {
	type stackElem struct {
		s *compilerSection
		i int
	}
	stack := []stackElem{{p.toplevel, 0}}
	return func(yield func(*compilerSection, *instruction) bool) {
	outer:
		for len(stack) > 0 {
			e := &stack[len(stack)-1]
			for e.i < len(e.s.children) {
				cld := e.s.children[e.i]
				e.i++
				switch cld := cld.(type) {
				case *instruction:
					if !yield(e.s, cld) {
						return
					}
				case *compilerSection:
					stack = append(stack, stackElem{cld, 0})
					continue outer
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
}

// instruction is a step of the compiler output program.
type instruction struct {
	// fields assigned during expansion:
	ast statement
	op  string

	// fields assigned during compilation:
	pc          int    // pc at this instruction
	pushSize    int    // computed size of push instruction
	data        []byte // computed argument value
	argNoLabels bool   // true if arg expression does not contain @label
}

func newInstruction(ast statement, op string) *instruction {
	return &instruction{ast: ast, op: op}
}

func isPush(op string) bool {
	return strings.HasPrefix(op, "PUSH")
}

func isJump(op string) bool {
	return strings.HasPrefix(op, "JUMP")
}

// explicitPushSize returns the declared PUSH size.
func (inst *instruction) explicitPushSize() (int, bool) {
	op, ok := inst.ast.(opcodeStatement)
	if ok {
		return int(op.PushSize) - 1, op.PushSize > 0
	}
	return 0, false
}

// pushArg returns the instruction argument.
func (inst *instruction) pushArg() ast.Expr {
	if !isPush(inst.op) {
		return nil
	}
	op, ok := inst.ast.(opcodeStatement)
	if ok {
		return op.Arg
	}
	return nil
}
