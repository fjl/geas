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
	"errors"
	"fmt"
	"math/big"
	"strings"
)

var (
	errNegativeBytes = errors.New("negative int in bytes context")
	errOddHexBytes   = errors.New("odd-length hex in bytes context")
)

const (
	flagWasHex byte = 1 << iota
	flagHexOddLength
	flagWasBytes
)

// Value is a big-integer that also tracks the number of leading zero bytes.
// This type is used to represent values during macro evaluation.
//
// Storing values this way may seem like a strange choice at first, so let me
// explain: The Geas language is meant to be simple, and values generally do not
// have a 'type'. Many macro operations are simple arithmetic and work with
// integers, and the EVM itself also operates on a stack of 256bit integers. So
// using integers as the basic type was an easy choice. However, Geas has a few
// operations on bytes as well (such as hash functions) and the language contains
// string literals. I didn't want to introduce a type system into the evaluator
// just to support them, since this would make some macros incompatible with
// others.
//
// Instead, I have chosen to stick to all values being integers, but this
// introduces some problems when an evaluation produces leading zero bytes. They
// cannot be represented by *big.Int, and thus using a hash function or including
// such values into the bytecode output would produce unexpected results.
//
// So this is how this type came to be. When a Value is created from a decimal
// integer literal, it is just an integer with no special properties. However,
// when created from a hexadecimal literal, string, or []byte in Go, leading
// zeros may be created and will be reproduced when the value is converted to
// []byte. Using an arithmetic operation on a value with leading zeros will drop
// them though.
type Value struct {
	int   big.Int
	lznib uint // leading zero nibble count
	flag  byte
}

func FromInt(i *big.Int) *Value {
	if i == nil {
		panic("nil int")
	}
	return &Value{int: *i}
}

func FromInt64(i int64) *Value {
	v := new(Value)
	v.int.SetInt64(i)
	return v
}

func FromBytes(slice []byte) *Value {
	v := new(Value)
	for _, b := range slice {
		if b != 0 {
			break
		}
		v.lznib += 2
	}
	v.int.SetBytes(slice)
	v.flag = flagWasBytes
	return v
}

// ParseNumberLiteral creates a value from a number literal.
func ParseNumberLiteral(text string) (*Value, error) {
	switch {
	case len(text) == 0:
		return nil, errors.New("empty number text")

	case strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X"):
		hex := text[2:]
		v := &Value{flag: flagWasHex}
		if len(hex)%2 != 0 {
			v.flag |= flagHexOddLength
		}
		for _, c := range hex {
			if c != '0' {
				break
			}
			v.lznib++
		}
		if _, ok := v.int.SetString(hex, 16); !ok {
			return nil, fmt.Errorf("invalid hex: %s", text)
		}
		return v, nil

	case len(text) > 1 && text[0] == '0':
		return nil, errors.New("leading zero not allowed in decimal integer")

	default:
		var v Value
		if _, ok := v.int.SetString(text, 10); !ok {
			return nil, fmt.Errorf("invalid number %s", text)
		}
		return &v, nil
	}
}

// Int converts the value to a bigint.
// This is always possible. Leading zero bytes are dropped.
func (v *Value) Int() *big.Int {
	if v == nil {
		return nil
	}
	return &v.int
}

// Bytes converts the value to a byte slice. This returns an error if the
// conversion is lossy, i.e. if the integer is negative or was an odd-length literal.
func (v *Value) Bytes() ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	if v.int.Sign() < 0 {
		return nil, errNegativeBytes
	}
	if v.flag&flagHexOddLength != 0 {
		return nil, errOddHexBytes
	}
	b := make([]byte, v.ByteLen())
	return v.int.FillBytes(b), nil
}

// ByteLen returns the length in bytes. This is always equal to the length of the slice
// that Bytes() would return, i.e. leading zeros are counted.
func (v *Value) ByteLen() int64 {
	if v == nil {
		return 0
	}
	return int64(v.lznib)/2 + (int64(v.int.BitLen())+7)/8
}

// IntegerBitLen returns the bit length of v as an integer, i.e. leading zero
// bytes are not counted.
func (v *Value) IntegerBitLen() int64 {
	if v == nil {
		return 0
	}
	return int64(v.int.BitLen())
}

func (v *Value) String() string {
	switch {
	case v == nil:
		return "nil"

	case v.flag&(flagWasHex|flagWasBytes) != 0:
		var b strings.Builder
		b.WriteString("0x")
		for range v.lznib {
			b.WriteByte('0')
		}
		if v.flag&flagWasBytes != 0 {
			fmt.Fprintf(&b, "%x", v.int.Bytes())
		} else if v.int.Sign() > 0 {
			fmt.Fprintf(&b, "%x", &v.int)
		}
		return b.String()

	default:
		return v.int.String()
	}
}
