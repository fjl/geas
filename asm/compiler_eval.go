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
	"errors"
	"math/big"
)

var zero = new(big.Int)

// assignInitialPushSizes sets the pushSize of all PUSH and PUSH<n> instructions.
// Arguments are pre-evaluated in this compilation step if they contain no label references.
func (c *Compiler) assignInitialPushSizes(e *evaluator, prog *compilerProg) {
	for section, inst := range prog.iterInstructions() {
		argument := inst.pushArg()
		if argument == nil {
			continue
		}
		inst.pushSize = 1
		if s, ok := inst.explicitPushSize(); ok {
			inst.pushSize = s
		}

		// Pre-evaluate argument.
		v, err := e.eval(argument, section.env)
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
		if err := c.assignPushArg(inst, v, true); err != nil {
			c.addError(inst.ast, err)
			continue
		}
	}
}

// computePC assigns the PC values of all instructions and labels.
func (c *Compiler) computePC(e *evaluator, prog *compilerProg) {
	var pc int
	for section, inst := range prog.iterInstructions() {
		if li, ok := inst.ast.(labelDefStatement); ok {
			e.setLabelPC(section.doc, li.LabelDefSt, pc)
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
func (c *Compiler) assignArgs(e *evaluator, prog *compilerProg) (inst *instruction, err error) {
	for section, inst := range prog.iterInstructions() {
		if inst.argNoLabels {
			continue // pre-calculated
		}
		argument := inst.pushArg()
		if argument == nil {
			continue // no arg
		}
		v, err := e.eval(argument, section.env)
		if err != nil {
			return inst, err
		}
		if err := c.assignPushArg(inst, v, false); err != nil {
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
func (c *Compiler) assignPushArg(inst *instruction, v *big.Int, setSize bool) error {
	if v.Sign() < 0 {
		return ecNegativeResult
	}
	b := v.Bytes()
	if len(b) > 32 {
		return ecPushOverflow256
	}
	// TODO: also handle negative int

	_, hasExplicitSize := inst.explicitPushSize()
	if setSize && !hasExplicitSize {
		inst.pushSize = c.autoPushSize(b)
	}
	if len(b) > inst.pushSize {
		if !hasExplicitSize {
			return ecVariablePushOverflow
		}
		return ecFixedSizePushOverflow
	}

	// Store data. Note there is no padding applied here.
	// Padding will be added at the bytecode output stage.
	inst.data = b
	return nil
}

func (c *Compiler) autoPushSize(value []byte) int {
	if len(value) > 32 {
		panic("value too big")
	}
	return len(value)
}
