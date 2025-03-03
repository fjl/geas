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
	"math/big"
	"path"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
	"github.com/fjl/geas/internal/lzint"
)

// Compiler turns assembly source into bytecode.
type Compiler struct {
	fsys           fs.FS
	lexDebug       bool
	maxIncDepth    int
	maxErrors      int
	defaultFork    string
	macroOverrides map[string]*lzint.Value

	globals    *globalScope
	macroStack map[*ast.InstructionMacroDef]struct{}
	includes   map[*ast.IncludeSt]*ast.Document
	errors     errorList
}

// NewCompiler creates a compiler.
// Deprecated: use New.
func NewCompiler(fsys fs.FS) *Compiler {
	return New(fsys)
}

// New creates a compiler.
// The file system is used to resolve import file names. If a nil FS is given,
// #import cannot be used.
func New(fsys fs.FS) *Compiler {
	return &Compiler{
		fsys:           fsys,
		maxIncDepth:    128,
		maxErrors:      10,
		defaultFork:    evm.LatestFork,
		macroOverrides: make(map[string]*lzint.Value),
	}
}

// reset prepares the compiler for the next run.
func (c *Compiler) reset() {
	c.globals = newGlobalScope()
	c.macroStack = make(map[*ast.InstructionMacroDef]struct{})
	c.includes = make(map[*ast.IncludeSt]*ast.Document)
	c.errors = errorList{maxErrors: c.maxErrors}
}

// SetFilesystem sets the file system used for resolving #include files.
// Note: if set to a nil FS, #include is not allowed.
func (c *Compiler) SetFilesystem(fsys fs.FS) {
	c.fsys = fsys
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

// SetGlobal sets the value of a global expression macro.
// Note the name must start with an uppercase letter to make it global.
func (c *Compiler) SetGlobal(name string, v *big.Int) {
	if !ast.IsGlobal(name) {
		panic(fmt.Sprintf("override name %q is not global (uppercase)", name))
	}
	if v == nil {
		delete(c.macroOverrides, name)
	} else {
		c.macroOverrides[name] = lzint.FromInt(v)
	}
}

// ClearGlobals removes all definitions created by SetGlobal.
func (c *Compiler) ClearGlobals() {
	clear(c.macroOverrides)
}

// CompileString compiles the given program text and returns the corresponding bytecode.
// If compilation fails, the returned slice is nil. Use the Errors method to get
// parsing/compilation errors.
func (c *Compiler) CompileString(input string) []byte {
	defer c.errors.catchAbort()

	return c.compileSource("", []byte(input))
}

// CompileString compiles the given program text and returns the corresponding bytecode.
// If compilation fails, the returned slice is nil. Use the Errors method to get
// parsing/compilation errors.
func (c *Compiler) CompileFile(filename string) []byte {
	defer c.errors.catchAbort()

	content, err := fs.ReadFile(c.fsys, filename)
	if err != nil {
		c.errors.add(err)
		return nil
	}
	return c.compileSource(filename, content)
}

// Errors returns errors that have accumulated during compilation.
func (c *Compiler) Errors() []error {
	return c.errors.errors()
}

// Warnings returns all warnings that have accumulated during compilation.
func (c *Compiler) Warnings() []error {
	return c.errors.warnings()
}

// Failed reports whether compilation has failed.
func (c *Compiler) Failed() bool {
	return c.errors.numErrors > 0
}

// ErrorsAndWarnings returns all errors and warnings which have accumulated during compilation.
func (c *Compiler) ErrorsAndWarnings() []error {
	return c.errors.list
}

// errorAt pushes an error to the compiler error list.
func (c *Compiler) errorAt(inst ast.Statement, err error) {
	if err == nil {
		panic("BUG: errorAt(st, nil)")
	}
	c.errors.add(&statementError{inst: inst, err: err})
}

// warnf pushes a warning to the error list.
func (c *Compiler) warnf(inst ast.Statement, format string, args ...any) {
	c.errors.add(&simpleWarning{pos: inst.Position(), str: fmt.Sprintf(format, args...)})
}

func (c *Compiler) compileSource(filename string, input []byte) []byte {
	c.reset()
	p := ast.NewParser(filename, input, c.lexDebug)
	doc, errs := p.Parse()
	if c.errors.addParseErrors(errs) {
		return nil // abort compilation due to failed parse
	}
	return c.compileDocument(doc)
}

// compileDocument creates bytecode from the AST.
func (c *Compiler) compileDocument(doc *ast.Document) (output []byte) {
	prog := newCompilerProg(doc)

	// First, load all #include files and register their definitions.
	// This also configures the instruction set if specified by a #pragma.
	c.processIncludes(doc, prog, nil)

	// Apply macro overrides. This happens after include processing because macros
	// get their definitions assigned then.
	for name, val := range c.macroOverrides {
		if def, _ := c.globals.lookupExprMacro(name); def != nil && len(def.Params) > 0 {
			c.warnf(def, "overridden global macro %s has parameters", name)
		}
		c.globals.overrideExprMacroValue(name, val)
	}

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
	c.preEvaluateArgs(e, prog)

	for {
		c.computePC(e, prog)

		// Assign immediate argument values. Here we use a trick to assign sizes for
		// "PUSH" instructions: their pushSizes are initially set to one. If we get an
		// overflow condition, the size of that PUSH increases by one and then we
		// recalculate everything.
		failedInst, err := c.evaluateArgs(e, prog)
		if err != nil {
			if errors.Is(err, ecVariablePushOverflow) {
				failedInst.pushSize += 1
				continue // try again
			}
			c.errorAt(failedInst.ast, err)
			break // there was some other error
		}
		break
	}

	if c.errors.hasError() {
		return nil // no output if source has errors
	}

	// Run analysis. Note this is also disabled if there are errors because there could
	// be lots of useless warnings otherwise.
	c.checkLabelsUsed(prog, e)

	// Create the bytecode.
	return c.generateOutput(prog)
}

// processIncludes reads all #included documents.
func (c *Compiler) processIncludes(doc *ast.Document, prog *compilerProg, stack []ast.Statement) {
	errs := c.globals.registerDefinitions(doc)
	c.errors.add(errs...)

	var list []*ast.IncludeSt
	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.IncludeSt:
			file, err := resolveRelative(doc.File, st.Filename)
			if err != nil {
				c.errorAt(st, err)
				continue
			}
			incdoc := c.parseIncludeFile(file, st, len(stack)+1)
			if incdoc != nil {
				c.includes[st] = incdoc
				list = append(list, st)
			}

		case *ast.PragmaSt:
			switch st.Option {
			case "target":
				if len(stack) != 0 {
					c.errorAt(st, ecPragmaTargetInIncludeFile)
				}
				if prog.evm != nil {
					c.errorAt(st, ecPragmaTargetConflict)
				}
				prog.evm = evm.FindInstructionSet(st.Value)
				if prog.evm == nil {
					c.errorAt(st, fmt.Errorf("%w %q", ecPragmaTargetUnknown, st.Value))
				}
			default:
				c.errorAt(st, fmt.Errorf("%w %s", ecUnknownPragma, st.Option))
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
		c.errorAt(inst, ecIncludeNoFS)
		return nil
	}
	if depth > c.maxIncDepth {
		c.errorAt(inst, ecIncludeDepthLimit)
		return nil
	}

	content, err := fs.ReadFile(c.fsys, file)
	if err != nil {
		c.errorAt(inst, err)
		return nil
	}
	p := ast.NewParser(file, content, c.lexDebug)
	doc, errors := p.Parse()
	if c.errors.addParseErrors(errors) {
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
	var unreachable unreachableCodeCheck
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
			if inst.op == "PUSH" {
				op = prog.evm.PushBySize(inst.pushSize)
			} else {
				op = prog.evm.OpByName(inst.op)
			}
			if op == nil {
				panic(fmt.Sprintf("BUG: opcode for %q (size %d) not found", inst.op, inst.pushSize))
			}

			// Unreachable code check.
			if !c.errors.hasError() {
				unreachable.check(c, inst.ast, op)
			}

			// Add opcode and data padding to output.
			output = append(output, op.Code)
			if len(inst.data) < inst.pushSize {
				output = append(output, make([]byte, inst.pushSize-len(inst.data))...)
			}

		case inst.op != "":
			op := prog.evm.OpByName(inst.op)
			if op == nil {
				c.errorAt(inst.ast, fmt.Errorf("%w %s", ecUnknownOpcode, inst.op))
			}
			// Unreachable code check.
			if !c.errors.hasError() {
				unreachable.check(c, inst.ast, op)
			}
			output = append(output, op.Code)
		}

		// Instruction data is always added to output.
		output = append(output, inst.data...)
	}
	return output
}
