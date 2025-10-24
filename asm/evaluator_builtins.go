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
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/lzint"
	"golang.org/x/crypto/sha3"
)

var builtinMacros = make(map[string]builtinMacroFn)

func init() {
	builtinMacros["len"] = lenMacro
	builtinMacros["abs"] = absMacro
	builtinMacros["intbits"] = intbitsMacro
	builtinMacros["bytelen"] = deprecatedByteLenMacro
	builtinMacros["bitlen"] = deprecatedBitLenMacro
	builtinMacros["address"] = addressMacro
	builtinMacros["selector"] = selectorMacro
	builtinMacros["keccak256"] = keccak256Macro
	builtinMacros["sha256"] = sha256Macro
	builtinMacros["assemble"] = assembleMacro
}

type builtinMacroFn func(*evaluator, *evalEnvironment, *ast.MacroCallExpr) (*lzint.Value, error)

func intbitsMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return lzint.FromInt64(v.IntegerBitLen()), nil
}

func lenMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return lzint.FromInt64(v.ByteLen()), nil
}

func deprecatedByteLenMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if !e.cache.deprecation[call] {
		e.compiler.warnDeprecatedMacro(call, "bytelen", "len")
		e.cache.deprecation[call] = true
	}

	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return lzint.FromInt64((int64(v.Int().BitLen()) + 7) / 8), nil
}

func deprecatedBitLenMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if !e.cache.deprecation[call] {
		e.compiler.warnDeprecatedMacro(call, "bitlen", "intbits")
		e.cache.deprecation[call] = true
	}

	return intbitsMacro(e, env, call)
}

func absMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	v, err := e.eval(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	return lzint.FromInt(new(big.Int).Abs(v.Int())), nil
}

func sha256Macro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	bytes, err := e.evalAsBytes(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(bytes)
	return lzint.FromBytes(hash[:]), nil
}

func keccak256Macro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
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
	return lzint.FromBytes(hash[:]), nil
}

var (
	errSelectorWantsLiteral = fmt.Errorf(".selector(...) requires literal string argument")
)

func selectorMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
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
	return lzint.FromBytes(hash[:4]), nil
}

var (
	errAddressWantsLiteral = errors.New(".address(...) requires literal argument")
	errAddressInvalid      = errors.New("invalid Ethereum address")
	errAddressChecksum     = errors.New("address has invalid checksum")
)

func addressMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
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
	return lzint.FromBytes(addr.Address().Bytes()), nil
}

func isChecksumAddress(str string) bool {
	return strings.ContainsAny(str, "ABCDEF")
}

func assembleMacro(e *evaluator, env *evalEnvironment, call *ast.MacroCallExpr) (*lzint.Value, error) {
	if err := checkArgCount(call, 1); err != nil {
		return nil, err
	}
	filenameBytes, err := e.evalAsBytes(call.Args[0], env)
	if err != nil {
		return nil, err
	}
	filename := string(filenameBytes)

	// Get value from cache.
	cacheKey := fileNameCacheKey{env.doc, filename}
	if v := e.cache.assemble[cacheKey]; v != nil {
		return v, nil
	}

	// Perform assembly. Note we take some settings from the current compiler here,
	// like the selected instruction set and overrides.
	subc := New(e.compiler.fsys)
	subc.SetIncludeDepthLimit(e.compiler.maxIncDepth)
	subc.SetMaxErrors(math.MaxInt)
	subc.SetDefaultFork(env.prog.evm.Name())
	subc.macroOverrides = e.compiler.macroOverrides

	file, err := resolveRelative(env.doc.File, string(filename))
	if err != nil {
		return nil, err
	}
	bytecode := subc.CompileFile(file)
	e.compiler.errors.add(subc.ErrorsAndWarnings()...)

	v := lzint.FromBytes(bytecode)
	e.cache.assemble[cacheKey] = v // cache result
	return v, nil
}
