// Copyright 2025 The go-ethereum Authors
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

package disasm

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/fjl/geas/asm"
)

func TestImmediateOpcodes(t *testing.T) {
	// Test EIP-8024 opcodes: DUPN (0xe6), SWAPN (0xe7), EXCHANGE (0xe8)
	bytecode, _ := hex.DecodeString("e600e780e812e8d0")
	expectedOutput := strings.TrimSpace(`
dupn[17]
swapn[108]
exchange[2, 3]
exchange[1, 19]
`)

	var buf strings.Builder
	d := New()
	d.SetShowBlocks(false)
	d.SetTarget("amsterdam")
	d.Disassemble(bytecode, &buf)
	output := strings.TrimSpace(buf.String())
	if output != expectedOutput {
		t.Fatalf("wrong output:\ngot:\n%s\n\nwant:\n%s", output, expectedOutput)
	}

	// try round trip
	a := asm.New(nil)
	a.SetDefaultFork("amsterdam")
	rtcode := a.CompileString(output)
	if !bytes.Equal(rtcode, bytecode) {
		t.Error("disassembly did not round-trip")
	}
}

func TestImmediateOpcodeTruncated(t *testing.T) {
	bytecode, _ := hex.DecodeString("e6")
	expectedOutput := "#bytes 0xe6"

	var buf strings.Builder
	d := New()
	d.SetShowBlocks(false)
	d.SetTarget("amsterdam")
	d.Disassemble(bytecode, &buf)
	output := strings.TrimSpace(buf.String())
	if output != expectedOutput {
		t.Fatalf("wrong output:\ngot: %s\nwant: %s", output, expectedOutput)
	}
}

// This checks that the disassembler can handle immediate opcodes which are not working.
func TestImmediateOpcodeInvalid(t *testing.T) {
	bytecode, _ := hex.DecodeString("e75be6605be7610000e65fe850")
	expectedOutput := strings.TrimSpace(`
#bytes 0xe7   ; invalid SWAPN
jumpdest
#bytes 0xe6   ; invalid DUPN
push1 0x5b
#bytes 0xe7   ; invalid SWAPN
push2 0x0000
#bytes 0xe6   ; invalid DUPN
push0
#bytes 0xe8   ; invalid EXCHANGE
pop
`)

	var buf strings.Builder
	d := New()
	d.SetShowBlocks(false)
	d.SetTarget("amsterdam")
	d.Disassemble(bytecode, &buf)
	output := strings.TrimSpace(buf.String())
	if output != expectedOutput {
		t.Fatalf("wrong output:\ngot: %s\nwant: %s", output, expectedOutput)
	}
}

func TestIncompletePush(t *testing.T) {
	bytecode, _ := hex.DecodeString("6080604052348015600e575f80fd5b50603e80601a5f395ff3fe60806040525f80fdfea2646970667358221220ba4339602dd535d09d71fae3164f7aa7f6e098ec879fc9e8f36bd912d4877c5264736f6c63430008190033")
	expectedOutput := strings.TrimSpace(`
push1 0x80
push1 0x40
mstore
callvalue
dup1
iszero
push1 0x0e
jumpi
push0
dup1
revert
jumpdest
pop
push1 0x3e
dup1
push1 0x1a
push0
codecopy
push0
return
#bytes 0xfe
push1 0x80
push1 0x40
mstore
push0
dup1
revert
#bytes 0xfe
log2
push5 0x6970667358
#bytes 0x22
slt
keccak256
#bytes 0xba
number
codecopy
push1 0x2d
#bytes 0xd5
calldataload
#bytes 0xd0
swap14
push18 0xfae3164f7aa7f6e098ec879fc9e8f36bd912
#bytes 0xd4
dup8
#bytes 0x7c5264736f6c63430008190033
`)

	var buf strings.Builder
	d := New()
	d.SetShowBlocks(false)
	d.SetTarget("cancun")
	d.Disassemble(bytecode, &buf)
	output := strings.TrimSpace(buf.String())
	if output != expectedOutput {
		t.Fatal("wrong output:", output)
	}

	// try round trip
	a := asm.New(nil)
	rtcode := a.CompileString(output)
	if !bytes.Equal(rtcode, bytecode) {
		t.Error("disassembly did not round-trip")
	}
}
