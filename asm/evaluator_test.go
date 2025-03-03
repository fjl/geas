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
	"fmt"
	"math/big"
	"testing"

	"github.com/fjl/geas/internal/ast"
)

type evalTest struct {
	expr   string
	result string
}

type evalErrorTest struct {
	expr string
	err  string
}

var evalIntTests = []evalTest{
	// arithmetic
	{expr: `1`, result: "1"},
	{expr: `1 + 4`, result: "5"},
	{expr: `1 + 1 + 4`, result: "6"},
	{expr: `1 << 48`, result: "281474976710656"},
	{expr: `32 >> 1`, result: "16"},
	{expr: `0xf1 & 0xe1`, result: "0xe1"},
	{expr: `0x0f & 0xff`, result: "0x0f"},
	{expr: `0x0f | 0xf0`, result: "0xff"},
	{expr: `0xf ^ 0xf`, result: "0x00"},
	{expr: `0x0 ^ 0xf`, result: "0xf"},
	// arithmetic precedence rules
	{expr: `(2 * 3) + 4`, result: "10"},
	{expr: `2 * 3 + 4`, result: "10"},
	{expr: `4 + 2 * 3`, result: "10"},
	{expr: `10 / 5 + 2`, result: "4"},
	{expr: `1 + 1024 * 1024 * 1024`, result: "1073741825"},
	{expr: `1024 * 1024 * 1024 * 1024 + 1`, result: "1099511627777"},
	{expr: `1 + 1024 * 1024 * 1024 & 2 + 3`, result: "4"},
	{expr: `(1 + ((1024 * 1024 * 1024) & 2)) + 3`, result: "4"},
	// -- division and multiplication have same precedence
	{expr: `12 / 6 * 3`, result: "6"},
	{expr: `12 / 6 * 3`, result: "6"},
	// -- and binds more strongly than or
	{expr: `0xff00 | 0xff & 0x0f`, result: "0xff0f"},
	{expr: `0xff & 0x0f | 0xff00`, result: "0xff0f"},
	{expr: `0xff & (0x0f | 0xff00)`, result: "0x0f"},
	// -- shift binds more strongly than and/or
	{expr: `0xff >> 4 & 0x05`, result: "0x05"},
	// macro and label references
	{expr: `@label1`, result: "1"},
	{expr: `@label1 + 2`, result: "3"},
	{expr: `macro3 / @label1`, result: "3"},
	{expr: `@.label2`, result: "2"},
	{expr: `@Label3`, result: "3"},
	{expr: `@.Label4`, result: "4"},
	{expr: `macroFunc(2)`, result: "2"},
	// string literals
	{expr: `"A"`, result: "65"},
	{expr: `"foo"`, result: "6713199"},
	// builtins
	{expr: `.bitlen(0)`, result: "0"},
	{expr: `.bitlen(0xff)`, result: "8"},
	{expr: `.bitlen(0x1ff)`, result: "9"},
	{expr: `.bitlen(0x01ff)`, result: "9"},
	{expr: `.bytelen(0)`, result: "0"},
	{expr: `.bytelen(0xff)`, result: "1"},
	{expr: `.bytelen(0x1ff)`, result: "2"},
	{expr: `.bytelen(0x01ff)`, result: "2"},
	{expr: `.bytelen(0x0001ff)`, result: "3"},   // note: leading zero byte
	{expr: `.bytelen(0x000001ff)`, result: "4"}, // two leading zero bytes
	{expr: `.bytelen("foobar")`, result: "6"},
	{expr: `.abs(0 - 10)`, result: "10"},
	{expr: `.sha256("text")`, result: "68832153269555879243704685382415794081420120252170153643880971663484982053329"},
	{expr: `.sha256(33)`, result: "84783983549258160669137366770885509408211009960610860350324922232842582506338"},
	{expr: `.selector("transfer(address,uint256)")`, result: "2835717307"},
	{expr: `.address(0x658bdf435d810c91414ec09147daa6db62406379)`, result: "579727320398773179602058954232328055508812456825"},
	{expr: `.address("0x658bdf435d810c91414ec09147daa6db62406379")`, result: "579727320398773179602058954232328055508812456825"},
}

var evalErrorTests = []evalErrorTest{
	{expr: `20 / 0`, err: "division by zero"},
	{expr: `1 << (1 << 64)`, err: "lshift amount 18446744073709551616 overflows uint"},
	{expr: `1 >> (1 << 64)`, err: "rshift amount 18446744073709551616 overflows uint"},
	{expr: `macro3(foo, 1)`, err: "invalid number of arguments, macro macro3 needs 0"},
	// builtins
	{expr: `.selector("transfer(,,uint256)")`, err: "invalid ABI selector"},
	{expr: `.address(0x658bdf435d810c91414EC09147daa6db62406379)`, err: errAddressChecksum.Error()},
	{expr: `.sha256(0x011)`, err: "odd-length hex in bytes context"},
}

var evalTestDoc *ast.Document

func init() {
	source := `
label1:
.label2:
Label3:
.Label4:
#define macro3() = 3
#define macroFunc(a) = $a
`
	doc, errs := ast.NewParser("", []byte(source), false).Parse()
	if len(errs) != 0 {
		panic("parse error: " + errs[0].Error())
	}
	evalTestDoc = doc
}

func evaluatorForTesting() *evaluator {
	gs := newGlobalScope()
	errs := gs.registerDefinitions(evalTestDoc)
	if len(errs) > 0 {
		panic(fmt.Errorf("error in registerDefinitions: %v", errs[0]))
	}
	e := newEvaluator(gs)
	e.setLabelPC(evalTestDoc, evalTestDoc.Statements[0].(*ast.LabelDefSt), 1)
	e.setLabelPC(evalTestDoc, evalTestDoc.Statements[1].(*ast.LabelDefSt), 2)
	e.setLabelPC(evalTestDoc, evalTestDoc.Statements[2].(*ast.LabelDefSt), 3)
	e.setLabelPC(evalTestDoc, evalTestDoc.Statements[3].(*ast.LabelDefSt), 4)
	return e
}

func evalEnvironmentForTesting() *evalEnvironment {
	return newEvalEnvironment(&compilerSection{
		doc: evalTestDoc,
	})
}

func TestExprEval(t *testing.T) {
	for _, test := range evalIntTests {
		expr, err := parseExprString(test.expr)
		if err != nil {
			t.Errorf("invalid expr %q: %v", test.expr, err)
			continue
		}
		expectedResult := mustParseBigInt(test.result)
		e := evaluatorForTesting()
		env := evalEnvironmentForTesting()
		result, err := e.eval(expr, env)
		if err != nil {
			t.Errorf("eval error in %q: %v", test.expr, err)
			continue
		}
		if result.Int().Cmp(expectedResult) != 0 {
			t.Errorf("expr %q result %v, want %v", test.expr, result, expectedResult)
			continue
		}
	}
}

func TestExprEvalErrors(t *testing.T) {
	for _, test := range evalErrorTests {
		expr, err := parseExprString(test.expr)
		if err != nil {
			t.Errorf("invalid expr %q: %v", test.expr, err)
			continue
		}
		e := evaluatorForTesting()
		env := evalEnvironmentForTesting()
		result, err := e.eval(expr, env)
		if err == nil {
			t.Errorf("expected error evaluating %q, got %v", test.expr, result)
			continue
		}
		if err.Error() != test.err {
			t.Errorf("expr %q wrong error %q, want %q", test.expr, err, test.err)
			continue
		}
	}
}

func parseExprString(str string) (ast.Expr, error) {
	p := ast.NewParser("string", []byte(str), false)
	return p.ParseExpression()
}

func mustParseBigInt(str string) *big.Int {
	i, ok := new(big.Int).SetString(str, 0)
	if !ok {
		panic("invalid bigint: " + str)
	}
	return i
}
