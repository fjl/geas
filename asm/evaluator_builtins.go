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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fjl/geas/internal/ast"
	"golang.org/x/crypto/sha3"
)

var builtinMacros = make(map[string]builtinMacroFn)

func init() {
	builtinMacros["bitlen"] = bitlenMacro
	builtinMacros["bytelen"] = bytelenMacro
	builtinMacros["abs"] = absMacro
	builtinMacros["address"] = addressMacro
	builtinMacros["selector"] = selectorMacro
	builtinMacros["keccak256"] = keccak256Macro
	builtinMacros["sha256"] = sha256Macro
}

type builtinMacroFn func(*evaluator, *evalEnvironment, *ast.MacroCallExpr) (*big.Int, error)

func bitlenMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return big.NewInt(int64(v.BitLen())), nil
}

func bytelenMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	bytes := (v.BitLen() + 7) / 8
	return big.NewInt(int64(bytes)), nil
}

func absMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return new(big.Int).Abs(v), nil
}

func sha256Macro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	bytes, err := e.evalAsBytes(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(bytes)
	return new(big.Int).SetBytes(hash[:]), nil
}

func keccak256Macro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	bytes, err := e.evalAsBytes(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	w := sha3.NewLegacyKeccak256()
	w.Write(bytes)
	hash := w.Sum(nil)
	return new(big.Int).SetBytes(hash[:]), nil
}

var (
	errSelectorWantsLiteral = fmt.Errorf(".selector(...) requires literal string argument")
)

func selectorMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	lit, ok := call.Args[0].(*ast.LiteralExpr)
	if !ok {
		return nil, errSelectorWantsLiteral
	}
	text := lit.Text()
	if _, err := abi.ParseSelector(text); err != nil {
		return nil, fmt.Errorf("invalid ABI selector")
	}
	w := sha3.NewLegacyKeccak256()
	w.Write([]byte(text))
	hash := w.Sum(nil)
	return new(big.Int).SetBytes(hash[:4]), nil
}

var (
	errAddressWantsLiteral = errors.New(".address(...) requires literal argument")
	errAddressInvalid      = errors.New("invalid Ethereum address")
	errAddressChecksum     = errors.New("address has invalid checksum")
)

func addressMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*big.Int, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	lit, ok := call.Args[0].(*ast.LiteralExpr)
	if !ok {
		return nil, errAddressWantsLiteral
	}
	text := lit.Text()
	addr, err := common.NewMixedcaseAddressFromString(text)
	if err != nil {
		return nil, errAddressInvalid
	}
	if isChecksumAddress(text) {
		if !addr.ValidChecksum() {
			return nil, errAddressChecksum
		}
	}
	return addr.Address().Big(), nil
}

func isChecksumAddress(str string) bool {
	return strings.ContainsAny(str, "ABCDEF")
}

// evalAsBytes gives the byte value of an expression.
// In most cases, this is just the big-endian encoding of an integer value.
// For string literals, the bytes of the literal are used directly.
func (e *evaluator) evalAsBytes(expr ast.Expr, env *evalEnvironment) ([]byte, error) {
	lit, ok := expr.(*ast.LiteralExpr)
	if ok {
		txt := lit.Text()
		if lit.IsString() {
			return []byte(txt), nil
		} else if strings.HasPrefix(txt, "0x") {
			if len(txt)%2 == 1 {
				return nil, ecOddLengthBytesLiteral
			}
			return hex.DecodeString(txt[2:])
		}
	}
	v, err := e.eval(expr, env)
	if err != nil {
		return nil, err
	}
	if v.Sign() < 0 {
		return nil, ecNegativeResult
	}
	return v.Bytes(), nil
}
