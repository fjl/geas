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
	"iter"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
)

// compilerProg is the output program of the compiler.
// It contains sections of instructions.
type compilerProg struct {
	elems []any
	evm   *evm.InstructionSet

	// Toplevel is the topmost section.
	toplevel *compilerSection

	// This tracks the current section.
	cur *compilerSection

	// Labels contains all labels in the program.
	labels []*compilerLabel

	// This tracks the latest label definitions which haven't been assigned to an
	// instruction yet. When the next instruction is added after a label, the
	// instruction will be linked to the label.
	currentLabels []*compilerLabel
}

// compilerSection is a section of the output program.
type compilerSection struct {
	doc    *ast.Document
	env    *evalEnvironment
	parent *compilerSection

	// Bounds of the section.
	startPC int
	endPC   int

	// This tracks the arguments of instruction macro calls. When the compiler expands a
	// macro, it creates a unique section for each call site. The arguments of the call
	// are stored for use by the expression evaluator.
	macroArgs *instrMacroArgs
}

type sectionStartElem struct {
	section *compilerSection
}

type sectionEndElem struct {
	section *compilerSection
}

type compilerLabel struct {
	doc   *ast.Document
	def   *ast.LabelDefSt
	instr *instruction // pointed-to instruction
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

// finishExpansion is called after the Compiler is done with expansion. Here we add an empty
// instruction at the program end, as a destination for labels.
func (p *compilerProg) finishExpansion() {
	if len(p.currentLabels) > 0 {
		p.addInstruction(newInstruction(nil, ""))
	}
	if p.cur != p.toplevel {
		panic("section stack was not unwound by expansion")
	}
	p.elems = append(p.elems, sectionEndElem{p.toplevel})
}

// pushSection creates a new section as a child of the current one.
func (p *compilerProg) pushSection(doc *ast.Document, macroArgs *instrMacroArgs) *compilerSection {
	s := &compilerSection{doc: doc, macroArgs: macroArgs}
	s.env = newEvalEnvironment(s)

	if p.cur != nil {
		s.parent = p.cur
	}
	p.elems = append(p.elems, sectionStartElem{s})
	p.cur = s
	return s
}

// popSection returns to the parent section.
func (p *compilerProg) popSection() {
	if p.cur.parent == nil {
		panic("too much pop")
	}
	p.elems = append(p.elems, sectionEndElem{p.cur})
	p.cur = p.cur.parent
}

// currentSection returns the current (most recently added) section.
func (p *compilerProg) currentSection() *compilerSection {
	return p.cur
}

// addLabel appends a label definition to the program.
func (p *compilerProg) addLabel(l *ast.LabelDefSt, doc *ast.Document) {
	cl := &compilerLabel{doc: doc, def: l}
	p.currentLabels = append(p.currentLabels, cl)
	p.labels = append(p.labels, cl)
}

// addInstruction appends an instruction to the current section.
func (p *compilerProg) addInstruction(inst *instruction) {
	p.elems = append(p.elems, inst)
	for _, cl := range p.currentLabels {
		cl.instr = inst
	}
	p.currentLabels = p.currentLabels[:0]
}

// iterInstructions returns an iterator over all instructions in the program.
func (p *compilerProg) iterInstructions() iter.Seq2[*compilerSection, *instruction] {
	return func(yield func(*compilerSection, *instruction) bool) {
		var s = p.toplevel
		for _, elem := range p.elems {
			switch elem := elem.(type) {
			case *instruction:
				if !yield(s, elem) {
					return
				}
			case sectionStartElem:
				s = elem.section
			case sectionEndElem:
				s = elem.section.parent
			default:
				panic(fmt.Sprintf("BUG: unhandled section type %T", elem))
			}
		}
	}
}

// iterSections returns an iterator over all sections in the program.
func (p *compilerProg) iterSections() iter.Seq[*compilerSection] {
	return func(yield func(*compilerSection) bool) {
		for _, elem := range p.elems {
			if elem, ok := elem.(sectionStartElem); ok {
				if !yield(elem.section) {
					return
				}
			}
		}
	}
}

// computePC assigns the PC values of all instructions and labels.
func (p *compilerProg) computePC() {
	var pc int
	for _, elem := range p.elems {
		switch elem := elem.(type) {
		case *instruction:
			elem.pc = pc
			pc += elem.encodedSize()
		case sectionStartElem:
			elem.section.startPC = pc
		case sectionEndElem:
			elem.section.endPC = pc
		default:
			panic(fmt.Sprintf("BUG: unhandled section type %T", elem))
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
	dataSize    int    // computed size of data field
	data        []byte // computed argument value
	argNoLabels bool   // true if arg expression does not contain @label
}

func newInstruction(ast statement, op string) *instruction {
	return &instruction{ast: ast, op: op}
}

func isPush(op string) bool {
	return strings.HasPrefix(op, "PUSH")
}

func isBytes(op string) bool {
	return op == "#bytes"
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

// expr returns the instruction argument.
func (inst *instruction) expr() ast.Expr {
	switch st := inst.ast.(type) {
	case opcodeStatement:
		return st.Arg
	case bytesStatement:
		return st.Value
	default:
		return nil
	}
}

// encodedSize gives the size of the instruction in bytecode.
func (inst *instruction) encodedSize() int {
	size := 0
	if !isBytes(inst.op) {
		size = 1
	}
	return size + inst.dataSize
}
