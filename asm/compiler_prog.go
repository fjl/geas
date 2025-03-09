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
	"slices"
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
	labels   []*compilerLabel

	// This tracks the last label definition.
	// When the next instruction is added after a label, the instruction
	// will be linked to the label.
	currentLabels []*compilerLabel
}

type compilerProgElem interface {
	// comparePC returns
	//   -1 if pc is before,
	//    0 if pc is within,
	//    1 if pc is after
	// the item.
	comparePC(pc int) int
}

// compilerSection is a section of the output program.
type compilerSection struct {
	doc *ast.Document
	env *evalEnvironment

	// This tracks the PC bounds of the section.
	pcLow, pcHigh int

	// This tracks the arguments of instruction macro calls. When the compiler expands a
	// macro, it creates a unique section for each call site. The arguments of the call
	// are stored for use by the expression evaluator.
	macroArgs *instrMacroArgs

	parent   *compilerSection
	children []compilerProgElem
}

// comparePC implements compilerProgItem.
func (s *compilerSection) comparePC(pc int) int {
	if pc < s.pcLow {
		return 1
	}
	if pc > s.pcHigh {
		return -1
	}
	return 0
}

type compilerLabel struct {
	doc   *ast.Document
	def   *ast.LabelDefSt
	instr *instruction
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

// finish is called after the Compiler is done with expansion. Here we add an empty
// instruction at the program end, as a destination for labels.
func (p *compilerProg) finish() {
	if len(p.currentLabels) > 0 {
		p.addInstruction(newInstruction(nil, ""))
	}
}

// pushSection creates a new section as a child of the current one.
func (p *compilerProg) pushSection(doc *ast.Document, macroArgs *instrMacroArgs) *compilerSection { //
	s := &compilerSection{doc: doc, macroArgs: macroArgs}
	s.env = newEvalEnvironment(p, s)
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

// addLabel appends a label definition to the program.
func (p *compilerProg) addLabel(l *ast.LabelDefSt, doc *ast.Document) {
	cl := &compilerLabel{doc: doc, def: l}
	p.currentLabels = append(p.currentLabels, cl)
	p.labels = append(p.labels, cl)
}

// addInstruction appends an instruction to the current section.
func (p *compilerProg) addInstruction(inst *instruction) {
	p.cur.children = append(p.cur.children, inst)
	for _, cl := range p.currentLabels {
		cl.instr = inst
	}
	p.currentLabels = p.currentLabels[:0]
}

type iterPos struct {
	elem           compilerProgElem
	exitingSection bool // true when visiting section at end
}

// iter returns an iterator over all elements of the program.
// Note this visits all sections twice: once when entering, and
// another time on exit.
func (p *compilerProg) iter() iter.Seq[iterPos] {
	type stackElem struct {
		s *compilerSection
		i int
	}
	stack := []stackElem{{p.toplevel, 0}}
	return func(yield func(iterPos) bool) {
	outer:
		for len(stack) > 0 {
			e := &stack[len(stack)-1]
			if !yield(iterPos{e.s, false}) {
				return
			}
			for e.i < len(e.s.children) {
				cld := e.s.children[e.i]
				e.i++
				switch elem := cld.(type) {
				case *instruction:
					if !yield(iterPos{elem, false}) {
						return
					}
				case *compilerSection:
					stack = append(stack, stackElem{elem, 0})
					continue outer
				}
			}
			if !yield(iterPos{e.s, true}) {
				return
			}
			stack = stack[:len(stack)-1]
		}
	}
}

// iterInstructions returns an iterator over all instructions in the program.
func (p *compilerProg) iterInstructions() iter.Seq2[*compilerSection, *instruction] {
	return func(yield func(*compilerSection, *instruction) bool) {
		var s = p.toplevel
		for pos := range p.iter() {
			switch elem := pos.elem.(type) {
			case *instruction:
				if !yield(s, elem) {
					return
				}
			case *compilerSection:
				s = elem
			}
		}
	}
}

// iterSections returns an iterator over all sections in the program.
func (p *compilerProg) iterSections() iter.Seq[*compilerSection] {
	stack := []*compilerSection{p.toplevel}
	return func(yield func(*compilerSection) bool) {
		for len(stack) > 0 {
			section := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !yield(section) {
				return
			}
			for _, cld := range slices.Backward(section.children) {
				if clds, ok := cld.(*compilerSection); ok {
					stack = append(stack, clds)
				}
			}
		}
	}
}

// instructionAtPC locates the instruction containing the given PC.
func (p *compilerProg) instructionAtPC(pc int) *instruction {
	s := p.toplevel
	for {
		index, ok := slices.BinarySearchFunc(s.children, pc, (compilerProgElem).comparePC)
		if !ok {
			return nil
		}
		switch match := s.children[index].(type) {
		case *compilerSection:
			s = match
		case *instruction:
			return match
		}
	}
}

// computePC assigns the PC values of all instructions and labels.
func (p *compilerProg) computePC() {
	var pc int
	for pos := range p.iter() {
		switch elem := pos.elem.(type) {
		case *instruction:
			elem.pc = pc
			pc += elem.encodedSize()
		case *compilerSection:
			if pos.exitingSection {
				elem.pcHigh = pc
			} else {
				elem.pcLow = pc
			}
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

// dataSize gives the size of the encoded argument.
func (inst *instruction) dataSize() int {
	if isPush(inst.op) {
		return inst.pushSize
	}
	return len(inst.data)
}

// encodedSize gives the size of the instruction in bytecode.
func (inst *instruction) encodedSize() int {
	size := 0
	if !isBytes(inst.op) {
		size = 1
	}
	return size + inst.dataSize()
}

// comparePC implements compilerProgItem.
func (inst *instruction) comparePC(pc int) (r int) {
	if pc < inst.pc {
		return 1
	}
	end := inst.pc + inst.encodedSize()
	if pc >= end {
		return -1
	}
	return 0
}
