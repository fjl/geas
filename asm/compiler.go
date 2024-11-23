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
	"path"
	"strconv"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
)

// Compiler performs the assembling.
type Compiler struct {
	fsys        fs.FS
	lexDebug    bool
	maxIncDepth int
	maxErrors   int
	defaultFork string

	globals    *globalScope
	errors     []error
	macroStack map[*ast.InstructionMacroDef]struct{}
	includes   map[*ast.IncludeSt]*ast.Document
}

// NewCompiler creates a compiler. The passed file system is used to resolve import file names.
func NewCompiler(fsys fs.FS) *Compiler {
	return &Compiler{
		fsys:        fsys,
		macroStack:  make(map[*ast.InstructionMacroDef]struct{}),
		includes:    make(map[*ast.IncludeSt]*ast.Document),
		maxIncDepth: 128,
		maxErrors:   10,
		defaultFork: evm.LatestFork,
	}
}

// SetDebugLexer enables/disables printing of the token stream to stdout.
func (c *Compiler) SetDebugLexer(on bool) {
	c.lexDebug = on
}

// SetDefaultFork sets the EVM instruction set used by default.
func (c *Compiler) SetDefaultFork(f string) {
	c.defaultFork = f
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
	p := ast.NewParser(filename, input, c.lexDebug)
	doc, errs := p.Parse()
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
func (c *Compiler) addError(inst ast.Statement, err error) {
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
func (c *Compiler) compile(doc *ast.Document) (output []byte) {
	defer func() {
		panicking := recover()
		if panicking != nil && panicking != errCancelCompilation {
			panic(panicking)
		}
	}()

	c.globals = newGlobalScope()
	prog := newCompilerProg(doc)

	// First, load all #include files and register their definitions.
	// This also configures the instruction set if specified by a #pragma.
	c.processIncludes(doc, prog, nil)

	// Choose latest eth mainnet instruction set if not configured.
	if prog.evm == nil {
		prog.evm = evm.FindInstructionSet(c.defaultFork)
	}

	// Next, the AST document tree is expanded into a flat list of instructions.
	c.expand(doc, prog)
	if prog.cur != prog.toplevel {
		panic("section stack was not unwound by expansion")
	}

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

// processIncludes reads all #included documents.
func (c *Compiler) processIncludes(doc *ast.Document, prog *compilerProg, stack []ast.Statement) {
	errs := c.globals.registerDefinitions(doc)
	c.addErrors(errs)

	var list []*ast.IncludeSt
	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.IncludeSt:
			file, err := resolveRelative(doc.File, st.Filename)
			if err != nil {
				c.addError(st, err)
				continue
			}
			incdoc := c.parseIncludeFile(file, st, len(stack)+1)
			if incdoc != nil {
				c.includes[st] = incdoc
				list = append(list, st)
			}

		case *ast.PragmaSt:
			switch st.Option {
			case "fork":
				if len(stack) != 0 {
					c.addError(st, ecPragmaForkInIncludeFile)
				}
				if prog.evm != nil {
					c.addError(st, ecPragmaForkConflict)
				}
				prog.evm = evm.FindInstructionSet(st.Value)
				if prog.evm == nil {
					c.addError(st, fmt.Errorf("%w %q", ecPragmaForkUnknown, st.Value))
				}
			default:
				c.addError(st, fmt.Errorf("%w %s", ecUnknownPragma, st.Option))
			}
		}
	}

	// Process includes in macros.
	for _, m := range doc.InstrMacros() {
		c.processIncludes(m.Body, prog, append(stack, m))
	}

	// Recurse.
	for _, inst := range list {
		incdoc := c.includes[inst]
		c.processIncludes(incdoc, prog, append(stack, inst))
	}
}

func resolveRelative(basepath string, filename string) (string, error) {
	res := path.Clean(path.Join(path.Dir(basepath), filename))
	if strings.Contains(res, "..") {
		return "", fmt.Errorf("path %q escapes project root", filename)
	}
	return res, nil
}

func (c *Compiler) parseIncludeFile(file string, inst *ast.IncludeSt, depth int) *ast.Document {
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
	p := ast.NewParser(file, content, c.lexDebug)
	doc, errors := p.Parse()
	for _, e := range errors {
		c.addError(inst, e)
	}
	if len(errors) > 0 {
		return nil
	}
	// Note that included documents do NOT have the including document set as Parent.
	// The parent relationship is used during lookup of labels, macros, etc. and
	// such definitions should not be shared between include files.
	//
	// Included documents do have a Creation though.
	doc.Creation = inst
	return doc
}

// generateOutput creates the bytecode. This is also where instruction names get resolved.
func (c *Compiler) generateOutput(prog *compilerProg) []byte {
	if len(c.errors) > 0 {
		return nil
	}

	pushNameBuf := []byte{'P', 'U', 'S', 'H', 0, 0}
	var output []byte
	for _, inst := range prog.iterInstructions() {
		if len(output) != inst.pc {
			panic(fmt.Sprintf("BUG: instruction pc=%d, but output has size %d", inst.pc, len(output)))
		}

		switch {
		case isPush(inst.op):
			if inst.pushSize > 32 {
				panic("BUG: pushSize > 32")
			}
			if len(inst.data) > inst.pushSize {
				panic(fmt.Sprintf("BUG: push inst.data %d > inst.pushSize %d", len(inst.data), inst.pushSize))
			}

			// resolve the op
			var op *evm.Op
			var size = inst.pushSize
			if inst.op == "PUSH" {
				if size == 0 && !prog.evm.SupportsPush0() {
					size = 1
				}
				pushName := strconv.AppendInt(pushNameBuf[:4], int64(size), 10)
				op = prog.evm.OpByName(string(pushName))
			} else {
				op = prog.evm.OpByName(inst.op)
			}
			if op == nil {
				panic(fmt.Sprintf("BUG: opcode for %q (size %d) not found", inst.op, inst.pushSize))
			}

			// Add opcode and data padding to output.
			output = append(output, op.Code)
			if len(inst.data) < size {
				output = append(output, make([]byte, size-len(inst.data))...)
			}

		case inst.op != "":
			op := prog.evm.OpByName(inst.op)
			if op == nil {
				c.addError(inst.ast, fmt.Errorf("%w %s", ecUnknownOpcode, inst.op))
			}
			output = append(output, op.Code)
		}

		// Instruction data is always added to output.
		output = append(output, inst.data...)
	}
	return output
}
