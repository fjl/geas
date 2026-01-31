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

package evm

import (
	"strconv"
	"strings"

	"github.com/fjl/geas/internal/set"
)

// Op is an EVM opcode.
type Op struct {
	Name string
	Code byte

	in, out stack

	// Flags:
	// - Push is set for PUSHx
	// - Term is set for instructions that end execution
	// - Jump is set for all jumps
	// - Unconditional is set for unconditional jumps
	// - JumpDest is set for JUMPDEST
	Push, Term, Jump, Unconditional, JumpDest bool
}

func (op Op) PushSize() int {
	n, _ := strconv.Atoi(strings.TrimPrefix(op.Name, "PUSH"))
	return n
}

func (op Op) StackIn() []string {
	return op.in
}

func (op Op) StackOut() []string {
	return op.out
}

type stack = []string

// This is the list of all opcodes.
var oplist = []*Op{
	{Name: "STOP", Code: 0x0, Term: true},
	{Name: "ADD", Code: 0x1, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "MUL", Code: 0x2, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "SUB", Code: 0x3, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "DIV", Code: 0x4, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "SDIV", Code: 0x5, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "MOD", Code: 0x6, in: stack{"x", "modulus"}, out: stack{"z"}},
	{Name: "SMOD", Code: 0x7, in: stack{"x", "modulus"}, out: stack{"z"}},
	{Name: "ADDMOD", Code: 0x8, in: stack{"x", "y", "m"}, out: stack{"z"}},
	{Name: "MULMOD", Code: 0x9, in: stack{"x", "y", "m"}, out: stack{"z"}},
	{Name: "EXP", Code: 0xa, in: stack{"base", "exponent"}, out: stack{"z"}},
	{Name: "SIGNEXTEND", Code: 0xb, in: stack{"bytes", "num"}, out: stack{"z"}},
	{Name: "LT", Code: 0x10, in: stack{"x", "y"}, out: stack{"less"}},
	{Name: "GT", Code: 0x11, in: stack{"x", "y"}, out: stack{"greater"}},
	{Name: "SLT", Code: 0x12, in: stack{"x", "y"}, out: stack{"less"}},
	{Name: "SGT", Code: 0x13, in: stack{"x", "y"}, out: stack{"geater"}},
	{Name: "EQ", Code: 0x14, in: stack{"x", "y"}, out: stack{"equal"}},
	{Name: "ISZERO", Code: 0x15, in: stack{"x"}, out: stack{"z"}},
	{Name: "AND", Code: 0x16, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "OR", Code: 0x17, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "XOR", Code: 0x18, in: stack{"x", "y"}, out: stack{"z"}},
	{Name: "NOT", Code: 0x19, in: stack{"x"}, out: stack{"z"}},
	{Name: "BYTE", Code: 0x1a, in: stack{"index", "x"}, out: stack{"byte"}},
	{Name: "SHL", Code: 0x1b, in: stack{"shift", "x"}, out: stack{"z"}},
	{Name: "SHR", Code: 0x1c, in: stack{"shift", "x"}, out: stack{"z"}},
	{Name: "SAR", Code: 0x1d, in: stack{"shift", "x"}, out: stack{"z"}},
	{Name: "CLZ", Code: 0x1e, in: stack{"x"}, out: stack{"z"}},

	{Name: "KECCAK256", Code: 0x20, in: stack{"x"}, out: stack{"hash"}},
	{Name: "ADDRESS", Code: 0x30, out: stack{"address"}},
	{Name: "BALANCE", Code: 0x31, out: stack{"balance"}},
	{Name: "ORIGIN", Code: 0x32, out: stack{"origin"}},
	{Name: "CALLER", Code: 0x33, out: stack{"caller"}},
	{Name: "CALLVALUE", Code: 0x34, out: stack{"callvalue"}},
	{Name: "CALLDATALOAD", Code: 0x35, in: stack{"offset"}, out: stack{"word"}},
	{Name: "CALLDATASIZE", Code: 0x36, out: stack{"size"}},
	{Name: "CALLDATACOPY", Code: 0x37, in: stack{"memOffset", "dataOffset", "length"}},
	{Name: "CODESIZE", Code: 0x38, out: stack{"codesize"}},
	{Name: "CODECOPY", Code: 0x39, in: stack{"memOffset", "codeOffset", "length"}},
	{Name: "GASPRICE", Code: 0x3a, out: stack{"gasprice"}},
	{Name: "EXTCODESIZE", Code: 0x3b, in: stack{"address"}, out: stack{"codesize"}},
	{Name: "EXTCODECOPY", Code: 0x3c, in: stack{"address", "memOffset", "codeOffset", "length"}},
	{Name: "RETURNDATASIZE", Code: 0x3d, out: stack{"returndatasize"}},
	{Name: "RETURNDATACOPY", Code: 0x3e, in: stack{"memOffset", "dataOffset", "length"}},
	{Name: "EXTCODEHASH", Code: 0x3f, in: stack{"address"}, out: stack{"codehash"}},
	{Name: "BLOCKHASH", Code: 0x40, out: stack{"blockhash"}},
	{Name: "COINBASE", Code: 0x41, out: stack{"coinbase"}},
	{Name: "TIMESTAMP", Code: 0x42, out: stack{"timestamp"}},
	{Name: "NUMBER", Code: 0x43, out: stack{"blocknum"}},
	{Name: "DIFFICULTY", Code: 0x44, out: stack{"difficulty"}},
	{Name: "RANDOM", Code: 0x44, out: stack{"random"}},
	{Name: "GASLIMIT", Code: 0x45, out: stack{"gaslimit"}},
	{Name: "CHAINID", Code: 0x46, out: stack{"chainid"}},
	{Name: "SELFBALANCE", Code: 0x47, out: stack{"balance"}},
	{Name: "BASEFEE", Code: 0x48, out: stack{"basefee"}},
	{Name: "BLOBHASH", Code: 0x49, in: stack{"index"}, out: stack{"blobhash"}},
	{Name: "POP", Code: 0x50, in: stack{"x"}},
	{Name: "MLOAD", Code: 0x51, in: stack{"offset"}, out: stack{"word"}},
	{Name: "MSTORE", Code: 0x52, in: stack{"memOffset", "x"}},
	{Name: "MSTORE8", Code: 0x53, in: stack{"memOffset", "byte"}},
	{Name: "SLOAD", Code: 0x54, in: stack{"slot"}, out: stack{"val"}},
	{Name: "SSTORE", Code: 0x55, in: stack{"slot", "val"}},
	{Name: "JUMP", Code: 0x56, in: stack{"label"}, Jump: true, Unconditional: true},
	{Name: "JUMPI", Code: 0x57, in: stack{"cond", "label"}, Jump: true},
	{Name: "PC", Code: 0x58, out: stack{"pc"}},
	{Name: "MSIZE", Code: 0x59, out: stack{"memSize"}},
	{Name: "GAS", Code: 0x5a, out: stack{"gas"}},
	{Name: "JUMPDEST", Code: 0x5b, JumpDest: true},
	{Name: "TLOAD", Code: 0x5c, in: stack{"slot"}, out: stack{"val"}},
	{Name: "TSTORE", Code: 0x5d, in: stack{"slot", "val"}},
	{Name: "MCOPY", Code: 0x5e, in: stack{"destOffset", "srcOffset", "length"}},

	// PUSHx
	{Name: "PUSH0", Code: 0x5f, out: stack{"zero"}, Push: true},
	{Name: "PUSH1", Code: 0x60, out: stack{"val"}, Push: true},
	{Name: "PUSH2", Code: 0x61, out: stack{"val"}, Push: true},
	{Name: "PUSH3", Code: 0x62, out: stack{"val"}, Push: true},
	{Name: "PUSH4", Code: 0x63, out: stack{"val"}, Push: true},
	{Name: "PUSH5", Code: 0x64, out: stack{"val"}, Push: true},
	{Name: "PUSH6", Code: 0x65, out: stack{"val"}, Push: true},
	{Name: "PUSH7", Code: 0x66, out: stack{"val"}, Push: true},
	{Name: "PUSH8", Code: 0x67, out: stack{"val"}, Push: true},
	{Name: "PUSH9", Code: 0x68, out: stack{"val"}, Push: true},
	{Name: "PUSH10", Code: 0x69, out: stack{"val"}, Push: true},
	{Name: "PUSH11", Code: 0x6a, out: stack{"val"}, Push: true},
	{Name: "PUSH12", Code: 0x6b, out: stack{"val"}, Push: true},
	{Name: "PUSH13", Code: 0x6c, out: stack{"val"}, Push: true},
	{Name: "PUSH14", Code: 0x6d, out: stack{"val"}, Push: true},
	{Name: "PUSH15", Code: 0x6e, out: stack{"val"}, Push: true},
	{Name: "PUSH16", Code: 0x6f, out: stack{"val"}, Push: true},
	{Name: "PUSH17", Code: 0x70, out: stack{"val"}, Push: true},
	{Name: "PUSH18", Code: 0x71, out: stack{"val"}, Push: true},
	{Name: "PUSH19", Code: 0x72, out: stack{"val"}, Push: true},
	{Name: "PUSH20", Code: 0x73, out: stack{"val"}, Push: true},
	{Name: "PUSH21", Code: 0x74, out: stack{"val"}, Push: true},
	{Name: "PUSH22", Code: 0x75, out: stack{"val"}, Push: true},
	{Name: "PUSH23", Code: 0x76, out: stack{"val"}, Push: true},
	{Name: "PUSH24", Code: 0x77, out: stack{"val"}, Push: true},
	{Name: "PUSH25", Code: 0x78, out: stack{"val"}, Push: true},
	{Name: "PUSH26", Code: 0x79, out: stack{"val"}, Push: true},
	{Name: "PUSH27", Code: 0x7a, out: stack{"val"}, Push: true},
	{Name: "PUSH28", Code: 0x7b, out: stack{"val"}, Push: true},
	{Name: "PUSH29", Code: 0x7c, out: stack{"val"}, Push: true},
	{Name: "PUSH30", Code: 0x7d, out: stack{"val"}, Push: true},
	{Name: "PUSH31", Code: 0x7e, out: stack{"val"}, Push: true},
	{Name: "PUSH32", Code: 0x7f, out: stack{"val"}, Push: true},

	// DUPx
	{
		Name: "DUP1",
		Code: 0x80,
		in:   stack{"x"},
		out:  stack{"x", "x"},
	},
	{
		Name: "DUP2",
		Code: 0x81,
		in:   stack{"x1", "x2"},
		out:  stack{"x2", "x1", "x2"},
	},
	{
		Name: "DUP3",
		Code: 0x82,
		in:   stack{"x1", "x2", "x3"},
		out:  stack{"x3", "x1", "x2", "x3"},
	},
	{
		Name: "DUP4",
		Code: 0x83,
		in:   stack{"x1", "x2", "x3", "x4"},
		out:  stack{"x4", "x1", "x2", "x3", "x4"},
	},
	{
		Name: "DUP5",
		Code: 0x84,
		in:   stack{"x1", "x2", "x3", "x4", "x5"},
		out:  stack{"x5", "x1", "x2", "x3", "x4", "x5"},
	},
	{
		Name: "DUP6",
		Code: 0x85,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6"},
		out:  stack{"x6", "x1", "x2", "x3", "x4", "x5", "x6"},
	},
	{
		Name: "DUP7",
		Code: 0x86,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7"},
		out:  stack{"x7", "x1", "x2", "x3", "x4", "x5", "x6", "x7"},
	},
	{
		Name: "DUP8",
		Code: 0x87,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8"},
		out:  stack{"x8", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8"},
	},
	{
		Name: "DUP9",
		Code: 0x88,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9"},
		out:  stack{"x9", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9"},
	},
	{
		Name: "DUP10",
		Code: 0x89,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10"},
		out:  stack{"x10", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10"},
	},
	{
		Name: "DUP11",
		Code: 0x8a,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11"},
		out:  stack{"x11", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11"},
	},
	{
		Name: "DUP12",
		Code: 0x8b,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12"},
		out:  stack{"x12", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12"},
	},
	{
		Name: "DUP13",
		Code: 0x8c,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13"},
		out:  stack{"x13", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13"},
	},
	{
		Name: "DUP14",
		Code: 0x8d,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14"},
		out:  stack{"x14", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14"},
	},
	{
		Name: "DUP15",
		Code: 0x8e,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15"},
		out:  stack{"x15", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15"},
	},
	{
		Name: "DUP16",
		Code: 0x8f,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16"},
		out:  stack{"x16", "x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16"},
	},

	// SWAPx
	{
		Name: "SWAP1",
		Code: 0x90,
		in:   stack{"x1", "x2"},
		out:  stack{"x2", "x1"},
	},
	{
		Name: "SWAP2",
		Code: 0x91,
		in:   stack{"x1", "x2", "x3"},
		out:  stack{"x3", "x2", "x1"},
	},
	{
		Name: "SWAP3",
		Code: 0x92,
		in:   stack{"x1", "x2", "x3", "x4"},
		out:  stack{"x4", "x2", "x3", "x1"},
	},
	{
		Name: "SWAP4",
		Code: 0x93,
		in:   stack{"x1", "x2", "x3", "x4", "x5"},
		out:  stack{"x5", "x2", "x3", "x4", "x1"},
	},
	{
		Name: "SWAP5",
		Code: 0x94,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6"},
		out:  stack{"x6", "x2", "x3", "x4", "x5", "x1"},
	},
	{
		Name: "SWAP6",
		Code: 0x95,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7"},
		out:  stack{"x7", "x2", "x3", "x4", "x5", "x6", "x1"},
	},
	{
		Name: "SWAP7",
		Code: 0x96,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8"},
		out:  stack{"x8", "x2", "x3", "x4", "x5", "x6", "x7", "x1"},
	},
	{
		Name: "SWAP8",
		Code: 0x97,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9"},
		out:  stack{"x9", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x1"},
	},
	{
		Name: "SWAP9",
		Code: 0x98,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10"},
		out:  stack{"x10", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x1"},
	},
	{
		Name: "SWAP10",
		Code: 0x99,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11"},
		out:  stack{"x11", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x1"},
	},
	{
		Name: "SWAP11",
		Code: 0x9a,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12"},
		out:  stack{"x12", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x1"},
	},
	{
		Name: "SWAP12",
		Code: 0x9b,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13"},
		out:  stack{"x13", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x1"},
	},
	{
		Name: "SWAP13",
		Code: 0x9c,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14"},
		out:  stack{"x14", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x1"},
	},
	{
		Name: "SWAP14",
		Code: 0x9d,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15"},
		out:  stack{"x15", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x1"},
	},
	{
		Name: "SWAP15",
		Code: 0x9e,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16"},
		out:  stack{"x16", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x1"},
	},
	{
		Name: "SWAP16",
		Code: 0x9f,
		in:   stack{"x1", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16", "x17"},
		out:  stack{"x17", "x2", "x3", "x4", "x5", "x6", "x7", "x8", "x9", "x10", "x11", "x12", "x13", "x14", "x15", "x16", "x1"},
	},

	// LOGx
	{Name: "LOG0", Code: 0xa0, in: stack{"memOffset", "length"}},
	{Name: "LOG1", Code: 0xa1, in: stack{"memOffset", "length", "topic1"}},
	{Name: "LOG2", Code: 0xa2, in: stack{"memOffset", "length", "topic1", "topic2"}},
	{Name: "LOG3", Code: 0xa3, in: stack{"memOffset", "length", "topic1", "topic2", "topic3"}},
	{Name: "LOG4", Code: 0xa4, in: stack{"memOffset", "length", "topic1", "topic2", "topic3", "topic4"}},

	// TRON-specific opcodes
	{Name: "CALLTOKEN", Code: 0xd0, in: stack{"gas", "address", "value", "tokenId", "argOffset", "argLength", "returnOffset", "returnLength"}, out: stack{"ok"}},
	{Name: "TOKENBALANCE", Code: 0xd1, in: stack{"tokenId", "address"}, out: stack{"balance"}},
	{Name: "CALLTOKENVALUE", Code: 0xd2, out: stack{"value"}},
	{Name: "CALLTOKENID", Code: 0xd3, out: stack{"tokenId"}},
	{Name: "ISCONTRACT", Code: 0xd4, in: stack{"address"}, out: stack{"isContract"}},
	{Name: "FREEZE", Code: 0xd5, in: stack{"resourceType", "frozenBalance", "receiverAddress"}, out: stack{"ok"}},
	{Name: "UNFREEZE", Code: 0xd6, in: stack{"resourceType", "targetAddress"}, out: stack{"ok"}},
	{Name: "FREEZEEXPIRETIME", Code: 0xd7, in: stack{"resourceType", "targetAddress"}, out: stack{"expireTime"}},
	{Name: "VOTEWITNESS", Code: 0xd8, in: stack{"amountArrayLength", "amountArrayOffset", "witnessArrayLength", "witnessArrayOffset"}, out: stack{"ok"}},
	{Name: "WITHDRAWREWARD", Code: 0xd9, out: stack{"withdrawReward"}},
	{Name: "FREEZEBALANCEV2", Code: 0xda, in: stack{"resourceType", "frozenBalance"}, out: stack{"ok"}},
	{Name: "UNFREEZEBALANCEV2", Code: 0xdb, in: stack{"resourceType", "unfreezeBalance"}, out: stack{"ok"}},
	{Name: "CANCELALLUNFREEZEV2", Code: 0xdc, out: stack{"ok"}},
	{Name: "WITHDRAWEXPIREUNFREEZE", Code: 0xdd, out: stack{"ok"}},
	{Name: "DELEGATERESOURCE", Code: 0xde, in: stack{"resourceType", "delegateBalance", "receiverAddress"}, out: stack{"ok"}},
	{Name: "UNDELEGATERESOURCE", Code: 0xdf, in: stack{"resourceType", "unDelegateBalance", "receiverAddress"}, out: stack{"ok"}},

	// Call family
	{
		Name: "CREATE",
		Code: 0xf0,
		in:   stack{"endowment", "inOffset", "inLength", "gas"},
		out:  stack{"address"},
	},
	{
		Name: "CALL",
		Code: 0xf1,
		in:   stack{"gas", "address", "value", "inOffset", "inLength", "returnOffset", "returnLength"},
		out:  stack{"ok"},
	},
	{
		Name: "CALLCODE",
		Code: 0xf2,
		in:   stack{"gas", "address", "value", "inOffset", "inLength", "returnOffset", "returnLength"},
		out:  stack{"ok"},
	},
	{
		Name: "RETURN",
		Code: 0xf3,
		Term: true,
		in:   stack{"offset", "length"},
	},
	{
		Name: "DELEGATECALL",
		Code: 0xf4,
		in:   stack{"gas", "address", "value", "inOffset", "inLength", "returnOffset", "returnLength"},
		out:  stack{"ok"},
	},
	{
		Name: "CREATE2",
		Code: 0xf5,
		in:   stack{"endowment", "inOffset", "inLength", "salt", "gas"},
		out:  stack{"address"},
	},
	{
		Name: "STATICCALL",
		Code: 0xfa,
		in:   stack{"gas", "address", "value", "inOffset", "inLength", "returnOffset", "returnLength"},
		out:  stack{"ok"},
	},
	{
		Name: "REVERT",
		Code: 0xfd,
		Term: true,
		in:   stack{"offset", "length"},
	},
	{
		Name: "SELFDESTRUCT",
		Code: 0xff,
		Term: true,
		in:   stack{"heir"},
	},
	{
		Name: "SENDALL",
		Code: 0xff,
		Term: true,
		in:   stack{"heir"},
	},
}

var opm = computeOpsMap()

func computeOpsMap() map[string]*Op {
	stacknames := make(set.Set[string], 20)
	m := make(map[string]*Op, len(oplist))
	for _, op := range oplist {
		if m[op.Name] != nil {
			panic("BUG: duplicate op " + op.Name)
		}
		m[op.Name] = op

		// Sanity-check stacks: input items must be unique.
		clear(stacknames)
		for _, name := range op.in {
			if stacknames.Includes(name) {
				panic("BUG: op " + op.Name + " has duplicate stack input " + name)
			}
			stacknames.Add(name)
		}
	}
	return m
}
