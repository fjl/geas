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

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
	"github.com/fjl/geas/internal/loader"
	"github.com/fjl/geas/internal/lzint"
)

// Compiler turns assembly source into bytecode.
type Compiler struct {
	maxErrors      int
	macroOverrides map[string]*lzint.Value

	globals    *globalScope
	macroStack map[*ast.InstructionMacroDef]struct{}
	includes   map[*ast.Include]*ast.Document
	loader     *loader.Loader
	errors     *loader.ErrorList
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
	l := loader.New(fsys)
	return &Compiler{
		loader:         l,
		errors:         l.Errors(),
		macroOverrides: make(map[string]*lzint.Value),
	}
}

// reset prepares the compiler for the next run.
func (c *Compiler) reset() {
	c.globals = newGlobalScope()
	c.macroStack = make(map[*ast.InstructionMacroDef]struct{})
	c.includes = make(map[*ast.Include]*ast.Document)
	c.errors.Clear()
}

// SetFilesystem sets the file system used for resolving #include files.
// Note: if set to a nil FS, #include is not allowed.
func (c *Compiler) SetFilesystem(fsys fs.FS) {
	c.loader.SetFilesystem(fsys)
}

// SetDefaultFork sets the EVM instruction set used by default.
func (c *Compiler) SetDefaultFork(f string) {
	c.loader.SetDefaultFork(f)
}

// SetIncludeDepthLimit enables/disables printing of the token stream to stdout.
func (c *Compiler) SetIncludeDepthLimit(limit int) {
	c.loader.SetMaxIncludeDepth(limit)
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
	defer c.errors.CatchAbort()

	prog := c.loader.LoadSource("", []byte(input))
	return c.compile(prog)
}

// CompileString compiles the given program text and returns the corresponding bytecode.
// If compilation fails, the returned slice is nil. Use the Errors method to get
// parsing/compilation errors.
func (c *Compiler) CompileFile(filename string) []byte {
	defer c.errors.CatchAbort()

	prog := c.loader.LoadFile(filename)
	return c.compile(prog)
}

// Errors returns errors that have accumulated during compilation.
func (c *Compiler) Errors() []error {
	return c.errors.Errors()
}

// Warnings returns all warnings that have accumulated during compilation.
func (c *Compiler) Warnings() []error {
	return c.errors.Warnings()
}

// Failed reports whether compilation has failed.
func (c *Compiler) Failed() bool {
	return c.errors.HasError()
}

// ErrorsAndWarnings returns all errors and warnings which have accumulated during compilation.
func (c *Compiler) ErrorsAndWarnings() []error {
	return c.errors.ErrorsAndWarnings()
}

// warnf pushes a warning to the error list.
func (c *Compiler) warnf(inst ast.Statement, format string, args ...any) {
	c.errors.Add(&simpleWarning{pos: inst.Position(), str: fmt.Sprintf(format, args...)})
}

// warnDeprecatedMacro warns about using a legacy macro.
func (c *Compiler) warnDeprecatedMacro(expr ast.Expr, name, replacement string) {
	c.errors.Add(&simpleWarning{
		pos: expr.Position(),
		str: fmt.Sprintf("macro %s() is deprecated, use %s()", name, replacement),
	})
}

// compileDocument creates bytecode from the AST.
func (c *Compiler) compile(lprog *loader.Program) (output []byte) {
	c.reset()
	prog := newCompilerProg(lprog)

	// Apply macro overrides. This happens after include processing because macros
	// get their definitions assigned then.
	for name, val := range c.macroOverrides {
		if def, _ := c.globals.lookupExprMacro(name); def != nil && len(def.Params) > 0 {
			c.warnf(def, "overridden global macro %s has parameters", name)
		}
		c.globals.overrideExprMacroValue(name, val)
	}

	// Next, the AST document tree is expanded into a flat list of instructions.
	c.expand(lprog.Toplevel, prog)
	prog.finishExpansion()

	// Expansion of is now done, and all further steps work on prog.
	e := newEvaluator(c)
	c.preEvaluateArgs(e, prog)
	e.registerLabels(prog.labels)

	for {
		prog.computePC()

		// Assign immediate argument values of PUSH. The only argument expressions left to
		// evaluate here are the ones that use labels somehow. This is self-referential,
		// the label values depend on the size of the output, which depends on the labels,
		// etc. To compute the fixed point, the following approach is used:
		//
		//   - Each PUSH gets an initial dataSize of one.
		//   - PC values are assigned based on this assumption
		//   - We compute all arg values. If any arg size overflows the set dataSize, we bump
		//     it for this instruction and recompute another round.
		//   - Otherwise we are done.
		failedInst, err := c.evaluateArgs(e, prog)
		if err == nil {
			break // done
		} else if errors.Is(err, ecVariablePushOverflow) {
			failedInst.dataSize += 1
			continue // recompute after bump
		} else {
			c.errors.AddAt(failedInst.ast, err)
			break // there was some other error
		}
	}

	if c.errors.HasError() {
		return nil // no output if source has errors
	}

	// Run analysis. Note this is also disabled if there are errors because there could
	// be lots of useless warnings otherwise.
	c.checkLabelsUsed(prog, e)

	// Create the bytecode.
	return c.generateOutput(prog)
}

// generateOutput creates the bytecode. This is also where instruction names get resolved.
func (c *Compiler) generateOutput(prog *compilerProg) []byte {
	var unreachable unreachableCodeCheck
	var output []byte
loop:
	for _, inst := range prog.iterInstructions() {
		if len(output) != inst.pc {
			panic(fmt.Sprintf("BUG: instruction pc=%d, but output has size %d", inst.pc, len(output)))
		}

		switch {
		case isBytes(inst.op):
			output = append(output, inst.data...)

		case isPush(inst.op):
			if inst.dataSize > 32 {
				panic("BUG: push dataSize > 32")
			}
			if len(inst.data) > inst.dataSize {
				panic(fmt.Sprintf("BUG: push inst.data %d > inst.dataSize %d", len(inst.data), inst.dataSize))
			}

			// resolve the op
			var op *evm.Op
			if inst.op == "PUSH" {
				op = prog.evm.PushBySize(inst.dataSize)
			} else {
				op = prog.evm.OpByName(inst.op)
			}
			if op == nil {
				panic(fmt.Sprintf("BUG: opcode for %q (size %d) not found", inst.op, inst.dataSize))
			}

			// Unreachable code check.
			if !c.errors.HasError() {
				unreachable.check(c, inst.ast, op)
			}

			// Add opcode and data padding to output.
			output = append(output, op.Code)
			if len(inst.data) < inst.dataSize {
				output = append(output, make([]byte, inst.dataSize-len(inst.data))...)
			}
			output = append(output, inst.data...)

		case inst.op != "":
			op := prog.evm.OpByName(inst.op)
			if op == nil {
				c.errors.AddAt(inst.ast, fmt.Errorf("%w %s", ecUnknownOpcode, inst.op))
				continue loop
			}
			// Unreachable code check.
			if !c.errors.HasError() {
				unreachable.check(c, inst.ast, op)
			}
			output = append(output, op.Code)
			fallthrough

		default:
			if len(inst.data) > 0 {
				panic(fmt.Sprintf("BUG: instruction at pc=%d has unexpected data", inst.pc))
			}
		}
	}
	return output
}
