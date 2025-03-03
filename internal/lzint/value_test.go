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

package lzint

import (
	"math/big"
	"reflect"
	"testing"
)

var valueTests = []struct {
	Name             string
	V                *Value
	ExpectedString   string
	ExpectedByteLen  int64
	ExpectedBitLen   int64
	ExpectedInt      *big.Int
	ExpectedBytes    []byte
	ExpectedBytesErr error
}{
	{
		Name:            "nil",
		V:               nil,
		ExpectedString:  "nil",
		ExpectedByteLen: 0,
		ExpectedBitLen:  0,
		ExpectedBytes:   nil,
		ExpectedInt:     nil,
	},
	{
		Name:            "Int64(0)",
		V:               FromInt64(0),
		ExpectedString:  "0",
		ExpectedByteLen: 0,
		ExpectedBitLen:  0,
		ExpectedBytes:   []byte{},
		ExpectedInt:     new(big.Int),
	},
	{
		Name:            "Int64(99)",
		V:               FromInt64(99),
		ExpectedString:  "99",
		ExpectedByteLen: 1,
		ExpectedBitLen:  7,
		ExpectedBytes:   []byte{99},
		ExpectedInt:     big.NewInt(99),
	},
	{
		Name:            "Int(256)",
		V:               FromInt(big.NewInt(256)),
		ExpectedString:  "256",
		ExpectedByteLen: 2,
		ExpectedBitLen:  9,
		ExpectedBytes:   []byte{1, 0},
		ExpectedInt:     big.NewInt(256),
	},
	{
		Name:             "Int(-256)",
		V:                FromInt(big.NewInt(-256)),
		ExpectedString:   "-256",
		ExpectedByteLen:  2,
		ExpectedBitLen:   9,
		ExpectedBytesErr: errNegativeBytes,
		ExpectedInt:      big.NewInt(-256),
	},
	{
		Name:            "Bytes(0x)",
		V:               FromBytes([]byte{}),
		ExpectedString:  "0x",
		ExpectedByteLen: 0,
		ExpectedBitLen:  0,
		ExpectedBytes:   []byte{},
		ExpectedInt:     new(big.Int),
	},
	{
		Name:            "Bytes(0x00)",
		V:               FromBytes([]byte{0}),
		ExpectedString:  "0x00",
		ExpectedByteLen: 1,
		ExpectedBitLen:  0,
		ExpectedBytes:   []byte{0},
		ExpectedInt:     new(big.Int),
	},
	{
		Name:            "Bytes(0x00000102)",
		V:               FromBytes([]byte{0, 0, 1, 2}),
		ExpectedString:  "0x00000102",
		ExpectedByteLen: 4,
		ExpectedBitLen:  9,
		ExpectedBytes:   []byte{0, 0, 1, 2},
		ExpectedInt:     new(big.Int).SetBytes([]byte{1, 2}),
	},
	{
		Name:            "NumberLiteral(0)",
		V:               mustParseNum("0"),
		ExpectedString:  "0",
		ExpectedByteLen: 0,
		ExpectedBitLen:  0,
		ExpectedBytes:   []byte{},
		ExpectedInt:     big.NewInt(0),
	},
	{
		Name:            "NumberLiteral(99)",
		V:               mustParseNum("99"),
		ExpectedString:  "99",
		ExpectedByteLen: 1,
		ExpectedBitLen:  7,
		ExpectedBytes:   []byte{99},
		ExpectedInt:     big.NewInt(99),
	},
	{
		Name:            "NumberLiteral(0xff01)",
		V:               mustParseNum("0xff01"),
		ExpectedString:  "0xff01",
		ExpectedByteLen: 2,
		ExpectedBitLen:  16,
		ExpectedBytes:   []byte{0xff, 0x01},
		ExpectedInt:     big.NewInt(0xff01),
	},
	{
		Name:            "NumberLiteral(0x00000099ff01)",
		V:               mustParseNum("0x00000099ff01"),
		ExpectedString:  "0x00000099ff01",
		ExpectedByteLen: 6,
		ExpectedBitLen:  24,
		ExpectedBytes:   []byte{0x00, 0x00, 0x00, 0x99, 0xff, 01},
		ExpectedInt:     big.NewInt(0x99ff01),
	},
	{
		Name:             "NumberLiteral(0x0)",
		V:                mustParseNum("0x0"),
		ExpectedString:   "0x0",
		ExpectedByteLen:  0,
		ExpectedBitLen:   0,
		ExpectedBytesErr: errOddHexBytes,
		ExpectedInt:      big.NewInt(0),
	},
	{
		Name:             "NumberLiteral(0xf)",
		V:                mustParseNum("0xf"),
		ExpectedString:   "0xf",
		ExpectedByteLen:  1,
		ExpectedBitLen:   4,
		ExpectedBytesErr: errOddHexBytes,
		ExpectedInt:      big.NewInt(15),
	},
	{
		Name:             "NumberLiteral(0x456)",
		V:                mustParseNum("0x456"),
		ExpectedString:   "0x456",
		ExpectedByteLen:  2,
		ExpectedBitLen:   11,
		ExpectedBytesErr: errOddHexBytes,
		ExpectedInt:      big.NewInt(0x456),
	},
}

func mustParseNum(input string) *Value {
	v, err := ParseNumberLiteral(input)
	if err != nil {
		panic(err)
	}
	return v
}

func TestValue(t *testing.T) {
	for _, test := range valueTests {
		t.Run(test.Name, func(t *testing.T) {
			if s := test.V.String(); s != test.ExpectedString {
				t.Errorf("wrong String: %q", s)
			}
			if l := test.V.ByteLen(); l != test.ExpectedByteLen {
				t.Errorf("wrong ByteLen: %d", l)
			}
			if l := test.V.IntegerBitLen(); l != test.ExpectedBitLen {
				t.Errorf("wrong BitLen: %d", l)
			}

			i := test.V.Int()
			if i == nil && test.ExpectedInt != nil {
				t.Errorf("wrong Int: <nil>, expected %d", test.ExpectedInt)
			} else if i != nil && test.ExpectedInt == nil {
				t.Errorf("wrong Int: %d, expected <nil>", i)
			} else if i.Cmp(test.ExpectedInt) != 0 {
				t.Errorf("wrong Int: %d, expected %d", i, test.ExpectedInt)
			}

			b, err := test.V.Bytes()
			if test.ExpectedBytesErr != nil {
				if err == nil {
					t.Errorf("Bytes did not return expected error")
				} else if err != test.ExpectedBytesErr {
					t.Errorf("Bytes returned wrong error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Bytes returned error: %v", err)
				} else if !reflect.DeepEqual(b, test.ExpectedBytes) {
					t.Errorf("wrong Bytes: %+v", b)
				}
			}
		})
	}
}

var literalErrorTests = []struct {
	Input string
	Err   string
}{
	{
		Input: "0xag",
		Err:   "invalid hex: 0xag",
	},
	{
		Input: "006",
		Err:   "leading zero not allowed in decimal integer",
	},
	{
		Input: "",
		Err:   "empty number text",
	},
	{
		Input: "42g",
		Err:   "invalid number 42g",
	},
}

func TestParseLiteral(t *testing.T) {
	for _, test := range literalErrorTests {
		_, err := ParseNumberLiteral(test.Input)
		if err == nil {
			t.Errorf("input %q: expected error", test.Input)
		} else if err.Error() != test.Err {
			t.Errorf("input %q: wrong error %v", test.Input, err)
		}
	}
}
