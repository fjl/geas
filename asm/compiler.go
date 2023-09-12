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

// Package asm implements the Good Ethereum Assembler (geas).
//
// For a description of the geas language, see the README.md file in the project root.
package asm

import (
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"path"
	"strings"

	"github.com/fjl/geas/internal/evm"
)

// Compiler performs the assembling.
type Compiler struct {
	fsys        fs.FS
	lexDebug    bool
	maxIncDepth int
	maxErrors   int

	globals    *globalScope
	errors     []error
	macroStack map[*instructionMacroDef]struct{}
}

type astInstruction interface {
	astStatement
	expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error)
}

// instruction is a step of the bytecode program.
type instruction struct {
	ast astInstruction
	op  string
	doc *document

	// compiler-assigned fields:
	pc          int    // pc at this instruction
	pushSize    int    // computed size of push instruction
	data        []byte // computed argument value
	argNoLabels bool   // true if arg expression does not contain @label
}

func newInstruction(doc *document, ast astInstruction, op string) *instruction {
	return &instruction{doc: doc, ast: ast, op: op}
}

func isPush(op string) bool {
	return strings.HasPrefix(op, "PUSH")
}

func isJump(op string) bool {
	return strings.HasPrefix(op, "JUMP")
}

// explicitPushSize returns the declared PUSH size.
func (inst *instruction) explicitPushSize() (int, bool) {
	op, ok := inst.ast.(*opcodeInstruction)
	if ok {
		return int(op.pushSize) - 1, op.pushSize > 0
	}
	return 0, false
}

// pushArg returns the instruction argument.
func (inst *instruction) pushArg() astExpr {
	if !isPush(inst.op) {
		return nil
	}
	op, ok := inst.ast.(*opcodeInstruction)
	if ok {
		return op.arg
	}
	return nil
}

// NewCompiler creates a compiler. The passed file system is used to resolve import file names.
func NewCompiler(fsys fs.FS) *Compiler {
	return &Compiler{
		fsys:        fsys,
		macroStack:  make(map[*instructionMacroDef]struct{}),
		maxIncDepth: 128,
		maxErrors:   10,
	}
}

// SetDebugLexer enables/disables printing of the token stream to stdout.
func (c *Compiler) SetDebugLexer(on bool) {
	c.lexDebug = on
}

// SetDebugLexer enables/disables printing of the token stream to stdout.
func (c *Compiler) SetIncludeDepthLimit(limit int) {
	c.maxIncDepth = limit
}

// SetMaxErrors sets the limit on the number of errors that can happen before the compiler gives up.
func (c *Compiler) SetMaxErrors(limit int) {
	if limit < 1 {
		limit = 1
	}
	c.maxErrors = limit
}

// CompileString compiles the given program text and returns the corresponding bytecode.
// If compilation fails, the returned slice is nil. Use the Errors method to get
// parsing/compilation errors.
func (c *Compiler) CompileString(input string) []byte {
	return c.compileSource("", []byte(input))
}

// CompileString compiles the given program text and returns the corresponding bytecode.
// If compilation fails, the returned slice is nil. Use the Errors method to get
// parsing/compilation errors.
func (c *Compiler) CompileFile(filename string) []byte {
	content, err := fs.ReadFile(c.fsys, filename)
	if err != nil {
		c.errors = append(c.errors, err)
		return nil
	}
	return c.compileSource(filename, content)
}

func (c *Compiler) compileSource(filename string, input []byte) []byte {
	p := newParser(filename, input, c.lexDebug)
	doc, errs := p.parse()
	for _, err := range errs {
		c.errors = append(c.errors, err)
	}
	if len(errs) > 0 {
		return nil
	}
	return c.compile(doc)
}

// Errors returns errors that have accumulated during compilation.
func (c *Compiler) Errors() []error {
	return c.errors
}

// addError pushes an error to the compiler error list.
func (c *Compiler) addError(inst astInstruction, err error) {
	c.errors = append(c.errors, &astError{inst: inst, err: err})
	if len(c.errors) > c.maxErrors {
		panic(errCancelCompilation)
	}
}

// addErrors adds multiple errors to the compiler error list.
func (c *Compiler) addErrors(errs []error) {
	for _, err := range errs {
		c.errors = append(c.errors, err)
		if len(c.errors) > c.maxErrors {
			panic(errCancelCompilation)
		}
	}
}

// compile is the toplevel entry point into the compiler.
func (c *Compiler) compile(doc *document) (output []byte) {
	c.globals = newGlobalScope()
	defer func() { c.globals = nil }()

	defer func() {
		panicking := recover()
		if panicking != nil && panicking != errCancelCompilation {
			panic(panicking)
		}
	}()

	// First, load all #include files and register their definitions.
	c.processIncludes(doc, nil)

	// Next, the AST document tree is expanded into a flat list of instructions.
	var prog []*instruction
	prog = c.expand(doc, doc.instructions, prog)

	// Expansion of is now done, and all further steps work on prog.
	e := newEvaluator(c.globals)
	c.assignInitialPushSizes(e, prog)

	for {
		c.computePC(e, prog)

		// Assign immediate argument values. Here we use a trick to assign sizes for
		// "PUSH" instructions: their pushSizes are initially set to one. If we get an
		// overflow condition, the size of that PUSH increases by one and then we
		// recalculate everything.
		failedInst, err := c.assignArgs(e, prog)
		if err != nil {
			if errors.Is(err, ecVariablePushOverflow) {
				failedInst.pushSize += 1
				continue // try again
			} else if err != nil {
				c.addError(failedInst.ast, err)
				break // there was some other error
			}
		}
		break
	}

	return c.generateOutput(prog)
}

// expand appends a list of AST instructions to the program.
func (c *Compiler) expand(doc *document, input []astInstruction, prog []*instruction) []*instruction {
	for _, inst := range input {
		newprog, err := inst.expand(c, doc, prog)
		if err != nil {
			c.addError(inst, err)
			continue
		}
		prog = newprog
	}
	return prog
}

// expand creates an instruction for the label. For dotted labels, the instruction is
// empty (i.e. has size zero). For regular labels, a JUMPDEST is created.
func (li *labelDefInstruction) expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error) {
	if li.global {
		if err := c.globals.setLabelDocument(li, doc); err != nil {
			c.addErrors([]error{err})
		}
	}

	inst := newInstruction(doc, li, "")
	if !li.dotted {
		inst.op = "JUMPDEST"
	}
	prog = append(prog, inst)
	return prog, nil
}

// expand appends the instruction to a program. This is also where basic validation is done.
func (op *opcodeInstruction) expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error) {
	opcode := strings.ToUpper(op.tok.text)
	inst := newInstruction(doc, op, opcode)

	switch {
	case isPush(opcode):
		if opcode == "PUSH0" {
			if op.arg != nil {
				return nil, ecPushzeroWithArgument
			}
			break
		}
		if op.arg == nil {
			return prog, ecPushWithoutArgument
		}

	case isJump(opcode):
		if err := c.validateJumpArg(doc, op.arg); err != nil {
			return nil, err
		}
		// 'JUMP @label' instructions turn into 'PUSH @label' + 'JUMP'.
		if op.arg != nil {
			push := newInstruction(doc, op, "PUSH")
			prog = append(prog, push)
		}

	default:
		if _, ok := inst.opcode(); !ok {
			return prog, fmt.Errorf("%w %s", ecUnknownOpcode, inst.op)
		}
		if op.arg != nil {
			return prog, ecUnexpectedArgument
		}
	}

	return append(prog, inst), nil
}

// validateJumpArg checks that argument to JUMP is a defined label.
func (c *Compiler) validateJumpArg(doc *document, arg astExpr) error {
	if arg == nil {
		return nil // no argument is fine.
	}
	lref, ok := arg.(*labelRefExpr)
	if !ok {
		return ecJumpNeedsLiteralLabel
	}
	if lref.dotted {
		return fmt.Errorf("%w %v", ecJumpToDottedLabel, lref)
	}

	var li *labelDefInstruction
	if lref.global {
		li = c.globals.label[lref.ident]
	} else {
		li, _ = doc.lookupLabel(lref)
	}
	if li == nil {
		return fmt.Errorf("%w %v", ecJumpToUndefinedLabel, lref)
	}
	return nil
}

// expand appends the output of a macro call to the program.
func (inst *macroCallInstruction) expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error) {
	name := inst.ident.text

	var def *instructionMacroDef
	if isGlobal(name) {
		def, _ = c.globals.lookupInstrMacro(name)
	} else {
		def, _ = doc.lookupInstrMacro(name)
	}
	if def == nil {
		return nil, fmt.Errorf("%w %%%s", ecUndefinedInstrMacro, name)
	}

	// Prevent recursion.
	if !c.enterMacro(def) {
		return nil, fmt.Errorf("%w %%%s", ecRecursiveCall, name)
	}
	defer c.exitMacro(def)

	if len(inst.args) != len(def.params) {
		return nil, fmt.Errorf("%w, macro %%%s needs %d", ecInvalidArgumentCount, name, len(def.params))
	}
	args := make(map[string]astExpr)
	for i, param := range def.params {
		args[param] = inst.args[i]
	}

	// Expand. Here we clone the macro document and then run it.
	macroDoc := *def.body
	macroDoc.parent = doc
	macroDoc.creation = inst
	macroDoc.instrMacroArgs = args

	prog = c.expand(&macroDoc, def.body.instructions, prog)
	return prog, nil
}

func (c *Compiler) enterMacro(m *instructionMacroDef) bool {
	if _, onStack := c.macroStack[m]; onStack {
		return false
	}
	c.macroStack[m] = struct{}{}
	return true
}

func (c *Compiler) exitMacro(m *instructionMacroDef) {
	delete(c.macroStack, m)
}

// processIncludes reads all #included documents.
func (c *Compiler) processIncludes(doc *document, stack []astStatement) {
	errs := c.globals.registerDefinitions(doc)
	c.addErrors(errs)

	var list []*includeInstruction
	for _, inst := range doc.instructions {
		inc, ok := inst.(*includeInstruction)
		if !ok {
			continue
		}
		file, err := resolveRelative(doc.file, inc.filename)
		if err != nil {
			c.addError(inst, err)
			continue
		}
		incdoc := c.parseIncludeFile(file, inc, len(stack)+1)
		if incdoc == nil {
			continue // there were parse errors
		}
		doc.includes[inc] = incdoc
		list = append(list, inc)
	}

	// Process includes in macros.
	for _, name := range sortedKeys(doc.instrMacros) {
		m := doc.instrMacros[name]
		c.processIncludes(m.body, append(stack, m))
	}

	// Recurse.
	for _, inst := range list {
		incdoc := doc.includes[inst]
		c.processIncludes(incdoc, append(stack, inst))
	}
}

func resolveRelative(basepath string, filename string) (string, error) {
	res := path.Clean(path.Join(path.Dir(basepath), filename))
	if strings.Contains(res, "..") {
		return "", fmt.Errorf("path %q escapes project root", filename)
	}
	return res, nil
}

func (c *Compiler) parseIncludeFile(file string, inst *includeInstruction, depth int) *document {
	if c.fsys == nil {
		c.addError(inst, ecIncludeNoFS)
		return nil
	}
	if depth > c.maxIncDepth {
		c.addError(inst, ecIncludeDepthLimit)
		return nil
	}

	content, err := fs.ReadFile(c.fsys, file)
	if err != nil {
		c.addError(inst, err)
		return nil
	}
	p := newParser(file, content, c.lexDebug)
	doc, errors := p.parse()
	for _, e := range errors {
		c.addError(inst, e)
	}
	if len(errors) > 0 {
		return nil
	}
	doc.creation = inst
	return doc
}

// expand of #include appends the included file's instructions to the program.
// Note this accesses the documents parsed by processIncludes.
func (inst *includeInstruction) expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error) {
	incdoc := doc.includes[inst]
	if incdoc == nil {
		return prog, nil // There was a parse error.
	}
	prog = c.expand(incdoc, incdoc.instructions, prog)
	return prog, nil
}

// expand of #assemble performs compilation of the given assembly file.
func (inst *assembleInstruction) expand(c *Compiler, doc *document, prog []*instruction) ([]*instruction, error) {
	subc := NewCompiler(c.fsys)
	subc.SetIncludeDepthLimit(c.maxIncDepth)
	subc.SetMaxErrors(math.MaxInt)

	file, err := resolveRelative(doc.file, inst.filename)
	if err != nil {
		return prog, err
	}
	bytecode := c.CompileFile(file)
	if len(c.Errors()) > 0 {
		c.addErrors(c.Errors())
		return prog, nil
	}
	datainst := &instruction{data: bytecode}
	return append(prog, datainst), nil
}

var zero = new(big.Int)

// assignInitialPushSizes sets the pushSize of all PUSH and PUSH<n> instructions.
// Arguments are pre-evaluated in this compilation step if they contain no label references.
func (c *Compiler) assignInitialPushSizes(e *evaluator, prog []*instruction) {
	for _, inst := range prog {
		argument := inst.pushArg()
		if argument == nil {
			continue
		}
		inst.pushSize = 1
		if s, ok := inst.explicitPushSize(); ok {
			inst.pushSize = s
		}

		// Pre-evaluate argument.
		env := newEvalEnvironment(inst.doc)
		v, err := argument.eval(e, env)
		var labelErr unassignedLabelError
		if errors.As(err, &labelErr) {
			// Expression depends on label position calculation, leave it for later.
			continue
		}
		inst.argNoLabels = true
		if err != nil {
			c.addError(inst.ast, err)
			continue
		}
		if err := inst.assignPushArg(v, true); err != nil {
			c.addError(inst.ast, err)
			continue
		}
	}
}

// computePC assigns the PC values of all instructions and labels.
func (c *Compiler) computePC(e *evaluator, prog []*instruction) {
	var pc int
	for _, inst := range prog {
		if li, ok := inst.ast.(*labelDefInstruction); ok {
			e.setLabelPC(inst.doc, li, pc)
		}

		inst.pc = pc
		size := 0
		if inst.op != "" {
			size = 1
		}
		if isPush(inst.op) {
			size += inst.pushSize
		} else {
			size += len(inst.data)
		}
		pc += size
	}
}

// assignArgs computes the argument values of all push instructions.
func (c *Compiler) assignArgs(e *evaluator, prog []*instruction) (inst *instruction, err error) {
	for _, inst := range prog {
		if inst.argNoLabels {
			continue // pre-calculated
		}
		argument := inst.pushArg()
		if argument == nil {
			continue // no arg
		}
		env := newEvalEnvironment(inst.doc)
		v, err := argument.eval(e, env)
		if err != nil {
			return inst, err
		}
		if err := inst.assignPushArg(v, false); err != nil {
			return inst, err
		}
	}
	return nil, nil
}

// assignPushArg sets the argument value of an instruction to v. The byte size of the
// value is checked against the declared "PUSH<n>" data size.
//
// If setSize is true, the pushSize of variable-size "PUSH" instructions will be assigned
// based on the value.
func (inst *instruction) assignPushArg(v *big.Int, setSize bool) error {
	if v.Sign() < 0 {
		return ecNegativeResult
	}
	bytesSize := (v.BitLen() + 7) / 8
	if bytesSize > 32 {
		return ecPushOverflow256
	}
	// TODO: also handle negative int

	_, hasExplicitSize := inst.explicitPushSize()
	if setSize && !hasExplicitSize {
		inst.pushSize = bytesSize
	}
	if bytesSize > inst.pushSize {
		if !hasExplicitSize {
			return ecVariablePushOverflow
		}
		return ecFixedSizePushOverflow
	}

	// Store data padded.
	b := v.Bytes()
	inst.data = make([]byte, inst.pushSize)
	copy(inst.data[len(inst.data)-len(b):], b)
	return nil
}

// generateOutput creates the bytecode. This is also where instruction names get resolved.
func (c *Compiler) generateOutput(prog []*instruction) []byte {
	if len(c.errors) > 0 {
		return nil
	}
	var output []byte
	for i, inst := range prog {
		if len(output) != inst.pc {
			panic(fmt.Sprintf("BUG: instruction %d has pc=%d, but output has size %d", i, inst.pc, len(output)))
		}
		if inst.op != "" {
			opcode, ok := inst.opcode()
			if !ok {
				c.addError(inst.ast, fmt.Errorf("%w %s", ecUnknownOpcode, inst.op))
				continue
			}
			output = append(output, byte(opcode))
		}
		output = append(output, inst.data...)
	}
	return output
}

// opcode returns the EVM opcode of the instruction.
func (inst *instruction) opcode() (evm.OpCode, bool) {
	if isPush(inst.op) {
		if inst.pushSize > 32 {
			panic("BUG: pushSize > 32")
		}
		return evm.PUSH1 + evm.OpCode(inst.pushSize-1), true
	}
	return evm.OpByName(inst.op)
}
