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

// Op is an EVM opcode.
type Op struct {
	Name string
	Code byte

	// Flags:
	// - Term is set for instructions that end execution.
	Term, UnconditionalJump bool
}

// This is the list of all opcodes.
var oplist = []*Op{
	{Name: "STOP", Code: 0x0, Term: true},
	{Name: "ADD", Code: 0x1},
	{Name: "MUL", Code: 0x2},
	{Name: "SUB", Code: 0x3},
	{Name: "DIV", Code: 0x4},
	{Name: "SDIV", Code: 0x5},
	{Name: "MOD", Code: 0x6},
	{Name: "SMOD", Code: 0x7},
	{Name: "ADDMOD", Code: 0x8},
	{Name: "MULMOD", Code: 0x9},
	{Name: "EXP", Code: 0xa},
	{Name: "SIGNEXTEND", Code: 0xb},
	{Name: "LT", Code: 0x10},
	{Name: "GT", Code: 0x11},
	{Name: "SLT", Code: 0x12},
	{Name: "SGT", Code: 0x13},
	{Name: "EQ", Code: 0x14},
	{Name: "ISZERO", Code: 0x15},
	{Name: "AND", Code: 0x16},
	{Name: "OR", Code: 0x17},
	{Name: "XOR", Code: 0x18},
	{Name: "NOT", Code: 0x19},
	{Name: "BYTE", Code: 0x1a},
	{Name: "SHL", Code: 0x1b},
	{Name: "SHR", Code: 0x1c},
	{Name: "SAR", Code: 0x1d},
	{Name: "KECCAK256", Code: 0x20},
	{Name: "ADDRESS", Code: 0x30},
	{Name: "BALANCE", Code: 0x31},
	{Name: "ORIGIN", Code: 0x32},
	{Name: "CALLER", Code: 0x33},
	{Name: "CALLVALUE", Code: 0x34},
	{Name: "CALLDATALOAD", Code: 0x35},
	{Name: "CALLDATASIZE", Code: 0x36},
	{Name: "CALLDATACOPY", Code: 0x37},
	{Name: "CODESIZE", Code: 0x38},
	{Name: "CODECOPY", Code: 0x39},
	{Name: "GASPRICE", Code: 0x3a},
	{Name: "EXTCODESIZE", Code: 0x3b},
	{Name: "EXTCODECOPY", Code: 0x3c},
	{Name: "RETURNDATASIZE", Code: 0x3d},
	{Name: "RETURNDATACOPY", Code: 0x3e},
	{Name: "EXTCODEHASH", Code: 0x3f},
	{Name: "BLOCKHASH", Code: 0x40},
	{Name: "COINBASE", Code: 0x41},
	{Name: "TIMESTAMP", Code: 0x42},
	{Name: "NUMBER", Code: 0x43},
	{Name: "DIFFICULTY", Code: 0x44},
	{Name: "RANDOM", Code: 0x44},
	{Name: "GASLIMIT", Code: 0x45},
	{Name: "CHAINID", Code: 0x46},
	{Name: "SELFBALANCE", Code: 0x47},
	{Name: "BASEFEE", Code: 0x48},
	{Name: "BLOBHASH", Code: 0x49},
	{Name: "POP", Code: 0x50},
	{Name: "MLOAD", Code: 0x51},
	{Name: "MSTORE", Code: 0x52},
	{Name: "MSTORE8", Code: 0x53},
	{Name: "SLOAD", Code: 0x54},
	{Name: "SSTORE", Code: 0x55},
	{Name: "JUMP", Code: 0x56, UnconditionalJump: true},
	{Name: "JUMPI", Code: 0x57},
	{Name: "PC", Code: 0x58},
	{Name: "MSIZE", Code: 0x59},
	{Name: "GAS", Code: 0x5a},
	{Name: "JUMPDEST", Code: 0x5b},
	{Name: "TLOAD", Code: 0x5c},
	{Name: "TSTORE", Code: 0x5d},
	{Name: "MCOPY", Code: 0x5e},
	{Name: "PUSH0", Code: 0x5f},
	{Name: "PUSH1", Code: 0x60},
	{Name: "PUSH2", Code: 0x61},
	{Name: "PUSH3", Code: 0x62},
	{Name: "PUSH4", Code: 0x63},
	{Name: "PUSH5", Code: 0x64},
	{Name: "PUSH6", Code: 0x65},
	{Name: "PUSH7", Code: 0x66},
	{Name: "PUSH8", Code: 0x67},
	{Name: "PUSH9", Code: 0x68},
	{Name: "PUSH10", Code: 0x69},
	{Name: "PUSH11", Code: 0x6a},
	{Name: "PUSH12", Code: 0x6b},
	{Name: "PUSH13", Code: 0x6c},
	{Name: "PUSH14", Code: 0x6d},
	{Name: "PUSH15", Code: 0x6e},
	{Name: "PUSH16", Code: 0x6f},
	{Name: "PUSH17", Code: 0x70},
	{Name: "PUSH18", Code: 0x71},
	{Name: "PUSH19", Code: 0x72},
	{Name: "PUSH20", Code: 0x73},
	{Name: "PUSH21", Code: 0x74},
	{Name: "PUSH22", Code: 0x75},
	{Name: "PUSH23", Code: 0x76},
	{Name: "PUSH24", Code: 0x77},
	{Name: "PUSH25", Code: 0x78},
	{Name: "PUSH26", Code: 0x79},
	{Name: "PUSH27", Code: 0x7a},
	{Name: "PUSH28", Code: 0x7b},
	{Name: "PUSH29", Code: 0x7c},
	{Name: "PUSH30", Code: 0x7d},
	{Name: "PUSH31", Code: 0x7e},
	{Name: "PUSH32", Code: 0x7f},
	{Name: "DUP1", Code: 0x80},
	{Name: "DUP2", Code: 0x81},
	{Name: "DUP3", Code: 0x82},
	{Name: "DUP4", Code: 0x83},
	{Name: "DUP5", Code: 0x84},
	{Name: "DUP6", Code: 0x85},
	{Name: "DUP7", Code: 0x86},
	{Name: "DUP8", Code: 0x87},
	{Name: "DUP9", Code: 0x88},
	{Name: "DUP10", Code: 0x89},
	{Name: "DUP11", Code: 0x8a},
	{Name: "DUP12", Code: 0x8b},
	{Name: "DUP13", Code: 0x8c},
	{Name: "DUP14", Code: 0x8d},
	{Name: "DUP15", Code: 0x8e},
	{Name: "DUP16", Code: 0x8f},
	{Name: "SWAP1", Code: 0x90},
	{Name: "SWAP2", Code: 0x91},
	{Name: "SWAP3", Code: 0x92},
	{Name: "SWAP4", Code: 0x93},
	{Name: "SWAP5", Code: 0x94},
	{Name: "SWAP6", Code: 0x95},
	{Name: "SWAP7", Code: 0x96},
	{Name: "SWAP8", Code: 0x97},
	{Name: "SWAP9", Code: 0x98},
	{Name: "SWAP10", Code: 0x99},
	{Name: "SWAP11", Code: 0x9a},
	{Name: "SWAP12", Code: 0x9b},
	{Name: "SWAP13", Code: 0x9c},
	{Name: "SWAP14", Code: 0x9d},
	{Name: "SWAP15", Code: 0x9e},
	{Name: "SWAP16", Code: 0x9f},
	{Name: "LOG0", Code: 0xa0},
	{Name: "LOG1", Code: 0xa1},
	{Name: "LOG2", Code: 0xa2},
	{Name: "LOG3", Code: 0xa3},
	{Name: "LOG4", Code: 0xa4},
	{Name: "CREATE", Code: 0xf0},
	{Name: "CALL", Code: 0xf1},
	{Name: "CALLCODE", Code: 0xf2},
	{Name: "RETURN", Code: 0xf3, Term: true},
	{Name: "DELEGATECALL", Code: 0xf4},
	{Name: "CREATE2", Code: 0xf5},
	{Name: "STATICCALL", Code: 0xfa},
	{Name: "REVERT", Code: 0xfd, Term: true},
	{Name: "SELFDESTRUCT", Code: 0xff, Term: true},
	{Name: "SENDALL", Code: 0xff, Term: true},
}

var opm = computeOpsMap()

func computeOpsMap() map[string]*Op {
	m := make(map[string]*Op, len(oplist))
	for _, op := range oplist {
		if m[op.Name] != nil {
			panic("duplicate op " + op.Name)
		}
		m[op.Name] = op
	}
	return m
}
