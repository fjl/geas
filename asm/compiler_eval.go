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

// preEvaluateArgs computes the initial argument values of instructions.
//
// Here we assign the inst.pushSize of all PUSH and PUSH<n> instructions.
// The argument value, inst.data, is assigned this compilation step if the arg expression
// contains no label references.
func (c *Compiler) preEvaluateArgs(e *evaluator, prog *compilerProg) {
loop:
	for section, inst := range prog.iterInstructions() {
		switch {
		case isBytes(inst.op):
			v, err := e.evalAsBytes(inst.expr(), section.env)
			if err == nil {
				inst.argNoLabels = true
				inst.data = v
			}

		case isPush(inst.op):
			if inst.expr() == nil {
				continue loop // push0
			}
			inst.pushSize = 1
			if s, ok := inst.explicitPushSize(); ok {
				inst.pushSize = s
			}
			// Pre-evaluate argument.
			v, err := e.eval(inst.expr(), section.env)
			var labelErr unassignedLabelError
			if errors.As(err, &labelErr) {
				// Expression depends on label position calculation, leave it for later.
				continue loop
			}
			inst.argNoLabels = true
			if err != nil {
				c.errorAt(inst.ast, err)
				continue loop
			}
			if err := prog.assignPushArg(inst, v.Int(), true); err != nil {
				c.errorAt(inst.ast, err)
				continue loop
			}
		}
	}
}

// evaluateArgs computes the argument values of instructions.
func (c *Compiler) evaluateArgs(e *evaluator, prog *compilerProg) (inst *instruction, err error) {
loop:
	for section, inst := range prog.iterInstructions() {
		switch {
		case inst.argNoLabels:
			// value already assigned by preEvaluateArgs

		case isBytes(inst.op):
			v, err := e.evalAsBytes(inst.expr(), section.env)
			if err != nil {
				return inst, err
			}
			inst.data = v

		case isPush(inst.op):
			if inst.expr() == nil {
				continue loop // push0
			}
			v, err := e.eval(inst.expr(), section.env)
			if err != nil {
				return inst, err
			}
			if err := prog.assignPushArg(inst, v.Int(), false); err != nil {
				return inst, err
			}
		}
	}
	return nil, nil
}

// assignPushArg sets the argument value of an instruction to v. The byte size of the
// value is checked against the declared "PUSH<n>" data size.
//
// If setSize is true, the pushSize of variable-size "PUSH" instructions will be assigned
// based on the value.
func (prog *compilerProg) assignPushArg(inst *instruction, v *big.Int, setSize bool) error {
	if v.Sign() < 0 {
		return ecNegativeResult
	}
	b := v.Bytes()
	if len(b) > 32 {
		return ecPushOverflow256
	}

	_, hasExplicitSize := inst.explicitPushSize()
	if setSize && !hasExplicitSize {
		inst.pushSize = prog.autoPushSize(b)
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

func (prog *compilerProg) autoPushSize(value []byte) int {
	if len(value) > 32 {
		panic("value too big")
	}
	if len(value) == 0 && !prog.evm.SupportsPush0() {
		return 1
	}
	return len(value)
}
