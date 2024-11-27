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

// Package disasm is a disassembler for EVM bytecode.
package disasm

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/fjl/geas/internal/evm"
)

// Disassembler turns EVM bytecode into readable text instructions.
type Disassembler struct {
	evm       *evm.InstructionSet
	uppercase bool
	showPC    bool
	noBlanks  bool

	pcBuffer, pcHex []byte
}

func (d *Disassembler) setDefaults() {
	if d.evm == nil {
		d.evm = evm.FindInstructionSet(evm.LatestFork)
	}
}

// New creates a disassembler.
func New() *Disassembler {
	return new(Disassembler)
}

// SetTarger sets the instruction set used by the disassembler.
// It defauls to the latest known Ethereum fork.
func (d *Disassembler) SetTarget(name string) error {
	is := evm.FindInstructionSet(name)
	if is == nil {
		return fmt.Errorf("unknown instruction set %q", name)
	}
	d.evm = is
	return nil
}

// SetUppercase toggles printing instruction names in uppercase.
func (d *Disassembler) SetUppercase(on bool) {
	d.uppercase = on
}

// SetShowPC toggles printing of program counter on each line.
func (d *Disassembler) SetShowPC(on bool) {
	d.showPC = on
}

// SetShowBlocks toggles printing of blank lines at block boundaries.
func (d *Disassembler) SetShowBlocks(on bool) {
	d.noBlanks = !on
}

// Disassemble is the main entry point of the disassembler.
// It runs through the bytecode and emits text to outW.
func (d *Disassembler) Disassemble(bytecode []byte, outW io.Writer) error {
	d.setDefaults()
	d.pcBuffer = make([]byte, digitsOfPC(len(bytecode)))
	d.pcHex = make([]byte, hex.EncodedLen(len(d.pcBuffer)))
	out := bufio.NewWriter(outW)

	var prevOp *evm.Op
	for pc := 0; pc < len(bytecode); pc++ {
		op := d.evm.OpByCode(bytecode[pc])
		d.newline(out, prevOp, op)
		if op == nil {
			d.printInvalid(out, bytecode[pc])
		} else {
			d.printPrefix(out, pc)
			d.printOp(out, op)
			if op.Push {
				size := op.PushSize()
				if len(bytecode)-1-pc < size {
					d.newline(out, op, nil)
					return fmt.Errorf("bytecode truncated, ends within %s", op.Name)
				}
				data := bytecode[pc+1 : pc+size+1]
				d.printPushData(out, data)
				pc += size
			}
		}

		prevOp = op
	}
	d.newline(out, prevOp, nil)
	return out.Flush()
}

func (d *Disassembler) printPrefix(out io.Writer, pc int) {
	if d.showPC {
		for i := 0; i < len(d.pcBuffer); i++ {
			d.pcBuffer[len(d.pcBuffer)-1-i] = byte(pc >> (8 * i))
		}
		hex.Encode(d.pcHex, d.pcBuffer)
		fmt.Fprintf(out, "%s: ", d.pcHex)
	}
}

func (d *Disassembler) printInvalid(out io.Writer, b byte) {
	fmt.Fprintf(out, "<invalid %x>\n", b)
}

func (d *Disassembler) printOp(out io.Writer, op *evm.Op) {
	name := op.Name
	if !d.uppercase {
		name = strings.ToLower(op.Name)
	}
	fmt.Fprint(out, name)
}

func (d *Disassembler) printPushData(out io.Writer, data []byte) {
	fmt.Fprintf(out, " %#x", data)
}

func (d *Disassembler) newline(out io.Writer, prevOp *evm.Op, nextOp *evm.Op) {
	if prevOp == nil {
		return
	}
	out.Write([]byte{'\n'})
	if d.noBlanks || nextOp == nil {
		return
	}
	if prevOp.Jump || nextOp.JumpDest {
		out.Write([]byte{'\n'})
	}
}

func digitsOfPC(codesize int) int {
	switch {
	case codesize < (1<<16 - 1):
		return 2
	case codesize < (1<<24 - 1):
		return 3
	case codesize < (1<<32 - 1):
		return 4
	default:
		return 8
	}
}
