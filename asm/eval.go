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
	"errors"
	"fmt"
	"math"
	"math/big"
)

// evaluator is for evaluating expressions.
type evaluator struct {
	inStack map[*expressionMacroDef]struct{}
	labelPC map[evalLabelKey]int
	globals *globalScope
}

type evalLabelKey struct {
	doc *document
	l   *labelDefInstruction
}

type evalEnvironment struct {
	doc  *document
	vars map[string]*big.Int
}

func newEvaluator(gs *globalScope) *evaluator {
	return &evaluator{
		inStack: make(map[*expressionMacroDef]struct{}),
		labelPC: make(map[evalLabelKey]int),
		globals: gs,
	}
}

func newEvalEnvironment(doc *document) *evalEnvironment {
	if doc == nil {
		panic("nil document")
	}
	return &evalEnvironment{doc: doc}
}

// lookupExprMacro finds a macro definition in the document chain.
func (e *evaluator) lookupExprMacro(env *evalEnvironment, name string) (*expressionMacroDef, *document) {
	if isGlobal(name) {
		return e.globals.lookupExprMacro(name)
	}
	doc := env.doc
	for doc != nil {
		if e, ok := doc.exprMacros[name]; ok {
			return e, doc
		}
		doc = doc.parent
	}
	return nil, nil
}

// setLabelPC stores the offset of a label within a document.
func (e *evaluator) setLabelPC(doc *document, li *labelDefInstruction, pc int) {
	if li.global {
		e.globals.setLabelPC(li.tok.text, pc)
	} else {
		e.labelPC[evalLabelKey{doc, li}] = pc
	}
}

func (e *evaluator) lookupLabel(doc *document, lref *labelRefExpr) (pc int, pcValid bool, err error) {
	var li *labelDefInstruction
	if lref.global {
		pc, pcValid, li = e.globals.lookupLabel(lref)
	} else {
		var srcdoc *document
		li, srcdoc = doc.lookupLabel(lref)
		pc, pcValid = e.labelPC[evalLabelKey{srcdoc, li}]
	}
	if li == nil {
		return 0, false, fmt.Errorf("undefined label %v", lref)
	}
	if li.dotted != lref.dotted {
		return 0, false, fmt.Errorf("undefined label %v (but %v exists)", lref, li)
	}
	return pc, pcValid, nil
}

func (expr *literalExpr) eval(e *evaluator, env *evalEnvironment) (*big.Int, error) {
	if expr.value != nil {
		return expr.value, nil
	}

	switch expr.tok.typ {
	case numberLiteral:
		val, ok := new(big.Int).SetString(expr.tok.text, 0)
		if !ok {
			return nil, fmt.Errorf("invalid number %q", expr.tok.text)
		}
		expr.value = val
		return val, nil

	case stringLiteral:
		val := new(big.Int).SetBytes([]byte(expr.tok.text))
		expr.value = val
		return val, nil

	default:
		panic(fmt.Errorf("invalid token %q (%s) in astLiteral", expr.tok.text, expr.tok.typ))
	}
}

func (expr *labelRefExpr) eval(e *evaluator, env *evalEnvironment) (*big.Int, error) {
	pc, pcValid, err := e.lookupLabel(env.doc, expr)
	if err != nil {
		return nil, err
	}
	if !pcValid {
		// We hit this case if evaluating before labels have been calculated. A
		// special error value is returned here to allow the compiler to recognize
		// this case.
		return nil, unassignedLabelError{lref: expr}
	}
	val := big.NewInt(int64(pc))
	return val, nil
}

var bigMaxUint = new(big.Int).SetUint64(math.MaxUint)

func (expr *arithExpr) eval(e *evaluator, env *evalEnvironment) (*big.Int, error) {
	// compute operands
	left, err := expr.left.eval(e, env)
	if err != nil {
		return nil, err
	}
	right, err := expr.right.eval(e, env)
	if err != nil {
		return nil, err
	}

	// apply op
	var v *big.Int
	switch expr.op.typ {
	case arithPlus:
		v = new(big.Int).Add(left, right)

	case arithMinus:
		v = new(big.Int).Sub(left, right)

	case arithMul:
		v = new(big.Int).Mul(left, right)

	case arithDiv:
		if right.Sign() == 0 {
			return nil, errors.New("division by zero")
		}
		v = new(big.Int).Div(left, right)

	case arithMod:
		v = new(big.Int).Mod(left, right)

	case arithAnd:
		v = new(big.Int).And(left, right)

	case arithOr:
		v = new(big.Int).Or(left, right)

	case arithHat:
		v = new(big.Int).Xor(left, right)

	case arithLshift:
		if right.Sign() == -1 {
			return nil, errors.New("negative lshift amount")
		}
		if right.Cmp(bigMaxUint) > 0 {
			return nil, fmt.Errorf("lshift amount %d overflows uint", right)
		}
		amount := uint(right.Uint64())
		v = new(big.Int).Lsh(left, amount)

	case arithRshift:
		if right.Sign() == -1 {
			return nil, errors.New("negative rshift amount")
		}
		if right.Cmp(bigMaxUint) > 0 {
			return nil, fmt.Errorf("rshift amount %d overflows uint", right)
		}
		amount := uint(right.Uint64())
		v = new(big.Int).Rsh(left, amount)

	default:
		panic(fmt.Errorf("invalid op %v", expr.op.typ))
	}
	return v, nil
}

func (expr *variableExpr) eval(e *evaluator, env *evalEnvironment) (*big.Int, error) {
	v := env.vars[expr.ident]
	if v != nil {
		return v, nil
	}
	// Check for instruction macro args.
	vexpr, ok := env.doc.instrMacroArgs[expr.ident]
	if !ok {
		return nil, fmt.Errorf("%w $%s", ecUndefinedVariable, expr.ident)
	}
	// Evaluate it in the parent scope.
	return vexpr.eval(e, newEvalEnvironment(env.doc.parent))
}

func (expr *macroCallExpr) eval(e *evaluator, env *evalEnvironment) (*big.Int, error) {
	if expr.builtin {
		builtin, ok := builtinMacros[expr.ident]
		if ok {
			return builtin(e, env, expr)
		}
		return nil, fmt.Errorf("%w .%s", ecUndefinedBuiltinMacro, expr.ident)
	}
	def, defdoc := e.lookupExprMacro(env, expr.ident)
	if def == nil {
		return nil, fmt.Errorf("%w %s", ecUndefinedMacro, expr.ident)
	}

	// Prevent recursion.
	if !e.enterMacro(def) {
		return nil, fmt.Errorf("%w %s", ecRecursiveCall, expr.ident)
	}
	defer e.exitMacro(def)

	// Bind arguments.
	macroEnv := &evalEnvironment{
		vars: make(map[string]*big.Int, len(def.params)),
		doc:  defdoc,
	}
	if err := expr.checkArgCount(len(def.params)); err != nil {
		return nil, err
	}
	if len(expr.args) != len(def.params) {
		return nil, fmt.Errorf("%w, macro %s needs %d", ecInvalidArgumentCount, expr.ident, len(def.params))
	}
	for i, param := range def.params {
		v, err := expr.args[i].eval(e, env)
		if err != nil {
			return nil, err
		}
		macroEnv.vars[param] = v
	}

	// Compute the macro result value.
	return def.body.eval(e, macroEnv)
}

func (expr *macroCallExpr) checkArgCount(n int) error {
	if len(expr.args) != n {
		return fmt.Errorf("%w, macro %s needs %d", ecInvalidArgumentCount, expr.ident, n)
	}
	return nil
}

func (e *evaluator) enterMacro(m *expressionMacroDef) bool {
	_, found := e.inStack[m]
	if found {
		return false
	}
	e.inStack[m] = struct{}{}
	return true
}

func (e *evaluator) exitMacro(m *expressionMacroDef) {
	delete(e.inStack, m)
}
