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
)

type evalTest struct {
	expr   string
	result string
}

type evalErrorTest struct {
	expr string
	err  string
}

var evalTests = []evalTest{
	// arithmetic
	{expr: `1`, result: "1"},
	{expr: `1 + 4`, result: "5"},
	{expr: `(2 * 3) + 4`, result: "10"},
	{expr: `(1024 * 1024 * 1024 * 1024) + 1`, result: "1099511627777"},
	{expr: `1 << 48`, result: "281474976710656"},
	{expr: `32 >> 1`, result: "16"},
	{expr: `0xf1 & 0xe1`, result: "0xe1"},
	{expr: `0x0f & 0xff`, result: "0x0f"},
	{expr: `0x0f | 0xf0`, result: "0xff"},
	{expr: `0xf ^ 0xf`, result: "0x00"},
	{expr: `0x0 ^ 0xf`, result: "0xf"},

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
}

var evalTestDoc = newDocument("", nil)

func init() {
	evalTestDoc.labels = map[string]*labelDefInstruction{
		"label1": {tok: token{typ: label, text: "label1"}, src: evalTestDoc},
		"label2": {tok: token{typ: dottedLabel, text: "label2"}, dotted: true, src: evalTestDoc},
		"Label3": {tok: token{typ: label, text: "Label3"}, global: true, src: evalTestDoc},
		"Label4": {tok: token{typ: dottedLabel, text: "Label4"}, global: true, dotted: true, src: evalTestDoc},
	}
	evalTestDoc.exprMacros = map[string]*expressionMacroDef{
		"macro3": {body: &literalExpr{tok: token{typ: numberLiteral, text: "3"}}},
		"macroFunc": {
			body:   &variableExpr{ident: "a"},
			params: []string{"a"},
		},
	}
}

func evaluatorForTesting() *evaluator {
	gs := newGlobalScope()
	errs := gs.registerDefinitions(evalTestDoc)
	if len(errs) > 0 {
		panic(fmt.Errorf("error in registerDefinitions: %v", errs[0]))
	}
	e := newEvaluator(gs)
	e.setLabelPC(evalTestDoc, evalTestDoc.labels["label1"], 1)
	e.setLabelPC(evalTestDoc, evalTestDoc.labels["label2"], 2)
	e.setLabelPC(evalTestDoc, evalTestDoc.labels["Label3"], 3)
	e.setLabelPC(evalTestDoc, evalTestDoc.labels["Label4"], 4)
	return e
}

func TestExprEval(t *testing.T) {
	for _, test := range evalTests {
		expr, err := parseExprString(test.expr)
		if err != nil {
			t.Errorf("invalid expr %q: %v", test.expr, err)
			continue
		}
		expectedResult := mustParseBigInt(test.result)
		e := evaluatorForTesting()
		env := newEvalEnvironment(evalTestDoc)
		result, err := expr.eval(e, env)
		if err != nil {
			t.Errorf("eval error in %q: %v", test.expr, err)
			continue
		}
		if result.Cmp(expectedResult) != 0 {
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
		env := newEvalEnvironment(evalTestDoc)
		result, err := expr.eval(e, env)
		if err == nil {
			t.Errorf("expected error evaluating %q, got %d", test.expr, result)
			continue
		}
		if err.Error() != test.err {
			t.Errorf("expr %q wrong error %q, want %q", test.expr, err, test.err)
			continue
		}
	}
}

func parseExprString(str string) (astExpr, error) {
	p := newParser("string", []byte(str), false)
	return p.parseExpression()
}

func mustParseBigInt(str string) *big.Int {
	i, ok := new(big.Int).SetString(str, 0)
	if !ok {
		panic("invalid bigint: " + str)
	}
	return i
}
