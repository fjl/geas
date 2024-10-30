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

type Op struct {
	Name string
	Code byte
}

var (
	STOP           = Op{Name: "STOP", Code: 0x0}
	ADD            = Op{Name: "ADD", Code: 0x1}
	MUL            = Op{Name: "MUL", Code: 0x2}
	SUB            = Op{Name: "SUB", Code: 0x3}
	DIV            = Op{Name: "DIV", Code: 0x4}
	SDIV           = Op{Name: "SDIV", Code: 0x5}
	MOD            = Op{Name: "MOD", Code: 0x6}
	SMOD           = Op{Name: "SMOD", Code: 0x7}
	ADDMOD         = Op{Name: "ADDMOD", Code: 0x8}
	MULMOD         = Op{Name: "MULMOD", Code: 0x9}
	EXP            = Op{Name: "EXP", Code: 0xa}
	SIGNEXTEND     = Op{Name: "SIGNEXTEND", Code: 0xb}
	LT             = Op{Name: "LT", Code: 0x10}
	GT             = Op{Name: "GT", Code: 0x11}
	SLT            = Op{Name: "SLT", Code: 0x12}
	SGT            = Op{Name: "SGT", Code: 0x13}
	EQ             = Op{Name: "EQ", Code: 0x14}
	ISZERO         = Op{Name: "ISZERO", Code: 0x15}
	AND            = Op{Name: "AND", Code: 0x16}
	OR             = Op{Name: "OR", Code: 0x17}
	XOR            = Op{Name: "XOR", Code: 0x18}
	NOT            = Op{Name: "NOT", Code: 0x19}
	BYTE           = Op{Name: "BYTE", Code: 0x1a}
	SHL            = Op{Name: "SHL", Code: 0x1b}
	SHR            = Op{Name: "SHR", Code: 0x1c}
	SAR            = Op{Name: "SAR", Code: 0x1d}
	KECCAK256      = Op{Name: "KECCAK256", Code: 0x20}
	ADDRESS        = Op{Name: "ADDRESS", Code: 0x30}
	BALANCE        = Op{Name: "BALANCE", Code: 0x31}
	ORIGIN         = Op{Name: "ORIGIN", Code: 0x32}
	CALLER         = Op{Name: "CALLER", Code: 0x33}
	CALLVALUE      = Op{Name: "CALLVALUE", Code: 0x34}
	CALLDATALOAD   = Op{Name: "CALLDATALOAD", Code: 0x35}
	CALLDATASIZE   = Op{Name: "CALLDATASIZE", Code: 0x36}
	CALLDATACOPY   = Op{Name: "CALLDATACOPY", Code: 0x37}
	CODESIZE       = Op{Name: "CODESIZE", Code: 0x38}
	CODECOPY       = Op{Name: "CODECOPY", Code: 0x39}
	GASPRICE       = Op{Name: "GASPRICE", Code: 0x3a}
	EXTCODESIZE    = Op{Name: "EXTCODESIZE", Code: 0x3b}
	EXTCODECOPY    = Op{Name: "EXTCODECOPY", Code: 0x3c}
	RETURNDATASIZE = Op{Name: "RETURNDATASIZE", Code: 0x3d}
	RETURNDATACOPY = Op{Name: "RETURNDATACOPY", Code: 0x3e}
	EXTCODEHASH    = Op{Name: "EXTCODEHASH", Code: 0x3f}
	BLOCKHASH      = Op{Name: "BLOCKHASH", Code: 0x40}
	COINBASE       = Op{Name: "COINBASE", Code: 0x41}
	TIMESTAMP      = Op{Name: "TIMESTAMP", Code: 0x42}
	NUMBER         = Op{Name: "NUMBER", Code: 0x43}
	DIFFICULTY     = Op{Name: "DIFFICULTY", Code: 0x44}
	RANDOM         = Op{Name: "RANDOM", Code: 0x44}
	GASLIMIT       = Op{Name: "GASLIMIT", Code: 0x45}
	CHAINID        = Op{Name: "CHAINID", Code: 0x46}
	SELFBALANCE    = Op{Name: "SELFBALANCE", Code: 0x47}
	BASEFEE        = Op{Name: "BASEFEE", Code: 0x48}
	BLOBHASH       = Op{Name: "BLOBHASH", Code: 0x49}
	POP            = Op{Name: "POP", Code: 0x50}
	MLOAD          = Op{Name: "MLOAD", Code: 0x51}
	MSTORE         = Op{Name: "MSTORE", Code: 0x52}
	MSTORE8        = Op{Name: "MSTORE8", Code: 0x53}
	SLOAD          = Op{Name: "SLOAD", Code: 0x54}
	SSTORE         = Op{Name: "SSTORE", Code: 0x55}
	JUMP           = Op{Name: "JUMP", Code: 0x56}
	JUMPI          = Op{Name: "JUMPI", Code: 0x57}
	PC             = Op{Name: "PC", Code: 0x58}
	MSIZE          = Op{Name: "MSIZE", Code: 0x59}
	GAS            = Op{Name: "GAS", Code: 0x5a}
	JUMPDEST       = Op{Name: "JUMPDEST", Code: 0x5b}
	TLOAD          = Op{Name: "TLOAD", Code: 0x5c}
	TSTORE         = Op{Name: "TSTORE", Code: 0x5d}
	MCOPY          = Op{Name: "MCOPY", Code: 0x5e}
	PUSH0          = Op{Name: "PUSH0", Code: 0x5f}
	PUSH1          = Op{Name: "PUSH1", Code: 0x60}
	PUSH2          = Op{Name: "PUSH2", Code: 0x61}
	PUSH3          = Op{Name: "PUSH3", Code: 0x62}
	PUSH4          = Op{Name: "PUSH4", Code: 0x63}
	PUSH5          = Op{Name: "PUSH5", Code: 0x64}
	PUSH6          = Op{Name: "PUSH6", Code: 0x65}
	PUSH7          = Op{Name: "PUSH7", Code: 0x66}
	PUSH8          = Op{Name: "PUSH8", Code: 0x67}
	PUSH9          = Op{Name: "PUSH9", Code: 0x68}
	PUSH10         = Op{Name: "PUSH10", Code: 0x69}
	PUSH11         = Op{Name: "PUSH11", Code: 0x6a}
	PUSH12         = Op{Name: "PUSH12", Code: 0x6b}
	PUSH13         = Op{Name: "PUSH13", Code: 0x6c}
	PUSH14         = Op{Name: "PUSH14", Code: 0x6d}
	PUSH15         = Op{Name: "PUSH15", Code: 0x6e}
	PUSH16         = Op{Name: "PUSH16", Code: 0x6f}
	PUSH17         = Op{Name: "PUSH17", Code: 0x70}
	PUSH18         = Op{Name: "PUSH18", Code: 0x71}
	PUSH19         = Op{Name: "PUSH19", Code: 0x72}
	PUSH20         = Op{Name: "PUSH20", Code: 0x73}
	PUSH21         = Op{Name: "PUSH21", Code: 0x74}
	PUSH22         = Op{Name: "PUSH22", Code: 0x75}
	PUSH23         = Op{Name: "PUSH23", Code: 0x76}
	PUSH24         = Op{Name: "PUSH24", Code: 0x77}
	PUSH25         = Op{Name: "PUSH25", Code: 0x78}
	PUSH26         = Op{Name: "PUSH26", Code: 0x79}
	PUSH27         = Op{Name: "PUSH27", Code: 0x7a}
	PUSH28         = Op{Name: "PUSH28", Code: 0x7b}
	PUSH29         = Op{Name: "PUSH29", Code: 0x7c}
	PUSH30         = Op{Name: "PUSH30", Code: 0x7d}
	PUSH31         = Op{Name: "PUSH31", Code: 0x7e}
	PUSH32         = Op{Name: "PUSH32", Code: 0x7f}
	DUP1           = Op{Name: "DUP1", Code: 0x80}
	DUP2           = Op{Name: "DUP2", Code: 0x81}
	DUP3           = Op{Name: "DUP3", Code: 0x82}
	DUP4           = Op{Name: "DUP4", Code: 0x83}
	DUP5           = Op{Name: "DUP5", Code: 0x84}
	DUP6           = Op{Name: "DUP6", Code: 0x85}
	DUP7           = Op{Name: "DUP7", Code: 0x86}
	DUP8           = Op{Name: "DUP8", Code: 0x87}
	DUP9           = Op{Name: "DUP9", Code: 0x88}
	DUP10          = Op{Name: "DUP10", Code: 0x89}
	DUP11          = Op{Name: "DUP11", Code: 0x8a}
	DUP12          = Op{Name: "DUP12", Code: 0x8b}
	DUP13          = Op{Name: "DUP13", Code: 0x8c}
	DUP14          = Op{Name: "DUP14", Code: 0x8d}
	DUP15          = Op{Name: "DUP15", Code: 0x8e}
	DUP16          = Op{Name: "DUP16", Code: 0x8f}
	SWAP1          = Op{Name: "SWAP1", Code: 0x90}
	SWAP2          = Op{Name: "SWAP2", Code: 0x91}
	SWAP3          = Op{Name: "SWAP3", Code: 0x92}
	SWAP4          = Op{Name: "SWAP4", Code: 0x93}
	SWAP5          = Op{Name: "SWAP5", Code: 0x94}
	SWAP6          = Op{Name: "SWAP6", Code: 0x95}
	SWAP7          = Op{Name: "SWAP7", Code: 0x96}
	SWAP8          = Op{Name: "SWAP8", Code: 0x97}
	SWAP9          = Op{Name: "SWAP9", Code: 0x98}
	SWAP10         = Op{Name: "SWAP10", Code: 0x99}
	SWAP11         = Op{Name: "SWAP11", Code: 0x9a}
	SWAP12         = Op{Name: "SWAP12", Code: 0x9b}
	SWAP13         = Op{Name: "SWAP13", Code: 0x9c}
	SWAP14         = Op{Name: "SWAP14", Code: 0x9d}
	SWAP15         = Op{Name: "SWAP15", Code: 0x9e}
	SWAP16         = Op{Name: "SWAP16", Code: 0x9f}
	LOG0           = Op{Name: "LOG0", Code: 0xa0}
	LOG1           = Op{Name: "LOG1", Code: 0xa1}
	LOG2           = Op{Name: "LOG2", Code: 0xa2}
	LOG3           = Op{Name: "LOG3", Code: 0xa3}
	LOG4           = Op{Name: "LOG4", Code: 0xa4}
	CREATE         = Op{Name: "CREATE", Code: 0xf0}
	CALL           = Op{Name: "CALL", Code: 0xf1}
	CALLCODE       = Op{Name: "CALLCODE", Code: 0xf2}
	RETURN         = Op{Name: "RETURN", Code: 0xf3}
	DELEGATECALL   = Op{Name: "DELEGATECALL", Code: 0xf4}
	CREATE2        = Op{Name: "CREATE2", Code: 0xf5}
	STATICCALL     = Op{Name: "STATICCALL", Code: 0xfa}
	REVERT         = Op{Name: "REVERT", Code: 0xfd}
	INVALID        = Op{Name: "INVALID", Code: 0xfe}
	SELFDESTRUCT   = Op{Name: "SELFDESTRUCT", Code: 0xff}
	SENDALL        = Op{Name: "SENDALL", Code: 0xff}
)
