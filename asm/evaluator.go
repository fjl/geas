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
	"slices"

	"github.com/fjl/geas/internal/ast"
)

// evaluator is for evaluating expressions.
type evaluator struct {
	inStack    map[*ast.ExpressionMacroDef]struct{}
	labelPC    map[evalLabelKey]int
	usedLabels map[*ast.LabelDefSt]struct{}
	globals    *globalScope
}

type evalLabelKey struct {
	doc *ast.Document
	l   *ast.LabelDefSt
}

type evalEnvironment struct {
	doc       *ast.Document
	macroArgs *instrMacroArgs
	variables map[string]*big.Int
}

func newEvaluator(gs *globalScope) *evaluator {
	return &evaluator{
		inStack:    make(map[*ast.ExpressionMacroDef]struct{}),
		labelPC:    make(map[evalLabelKey]int),
		usedLabels: make(map[*ast.LabelDefSt]struct{}),
		globals:    gs,
	}
}

func newEvalEnvironment(s *compilerSection) *evalEnvironment {
	if s == nil {
		panic("nil section")
	}
	return &evalEnvironment{doc: s.doc, macroArgs: s.macroArgs}
}

// lookupExprMacro finds a macro definition in the document chain.
func (e *evaluator) lookupExprMacro(env *evalEnvironment, name string) (*ast.ExpressionMacroDef, *ast.Document) {
	if ast.IsGlobal(name) {
		return e.globals.lookupExprMacro(name)
	}
	if e, doc := env.doc.LookupExprMacro(name); e != nil {
		return e, doc
	}
	return nil, nil
}

// setLabelPC stores the offset of a label within a document.
func (e *evaluator) setLabelPC(doc *ast.Document, li *ast.LabelDefSt, pc int) {
	if li.Global {
		e.globals.setLabelPC(li.Name(), pc)
	} else {
		e.labelPC[evalLabelKey{doc, li}] = pc
	}
}

// lookupLabel resolves a label reference.
func (e *evaluator) lookupLabel(doc *ast.Document, lref *ast.LabelRefExpr) (pc int, pcValid bool, err error) {
	var li *ast.LabelDefSt
	if lref.Global {
		pc, pcValid, li = e.globals.lookupLabel(lref)
	} else {
		var srcdoc *ast.Document
		li, srcdoc = doc.LookupLabel(lref)
		pc, pcValid = e.labelPC[evalLabelKey{srcdoc, li}]
	}
	if li == nil {
		return 0, false, fmt.Errorf("undefined label %v", lref)
	}
	if li.Dotted != lref.Dotted {
		return 0, false, fmt.Errorf("undefined label %v (but %v exists)", lref, li)
	}
	// mark label used (for unused label analysis)
	e.usedLabels[li] = struct{}{}
	return pc, pcValid, nil
}

// isLabelUsed reports whether the given label definition was used during expression evaluation.
func (e *evaluator) isLabelUsed(li *ast.LabelDefSt) bool {
	_, ok := e.usedLabels[li]
	return ok
}

func (e *evaluator) eval(expr ast.Expr, env *evalEnvironment) (*big.Int, error) {
	switch expr := expr.(type) {
	case *ast.LiteralExpr:
		return e.evalLiteral(expr, env)
	case *ast.LabelRefExpr:
		return e.evalLabelRef(expr, env)
	case *ast.ArithExpr:
		return e.evalArith(expr, env)
	case *ast.VariableExpr:
		return e.evalVariable(expr, env)
	case *ast.MacroCallExpr:
		return e.evalMacroCall(expr, env)
	default:
		panic(fmt.Sprintf("unhandled expr %T", expr))
	}
}

func (e *evaluator) evalLiteral(expr *ast.LiteralExpr, env *evalEnvironment) (*big.Int, error) {
	if expr.Value != nil {
		return expr.Value, nil
	}

	switch {
	case expr.IsNumber():
		val, ok := new(big.Int).SetString(expr.Text(), 0)
		if !ok {
			return nil, fmt.Errorf("invalid number %q", expr.Text())
		}
		expr.Value = val
		return val, nil

	case expr.IsString():
		val := new(big.Int).SetBytes([]byte(expr.Text()))
		expr.Value = val
		return val, nil

	default:
		panic(fmt.Errorf("unhandled astLiteral %q (not string|number)", expr.Text()))
	}
}

func (e *evaluator) evalLabelRef(expr *ast.LabelRefExpr, env *evalEnvironment) (*big.Int, error) {
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

func (e *evaluator) evalArith(expr *ast.ArithExpr, env *evalEnvironment) (*big.Int, error) {
	// compute operands
	left, err := e.eval(expr.Left, env)
	if err != nil {
		return nil, err
	}
	right, err := e.eval(expr.Right, env)
	if err != nil {
		return nil, err
	}

	// apply op
	var v *big.Int
	switch expr.Op {
	case ast.ArithPlus:
		v = new(big.Int).Add(left, right)

	case ast.ArithMinus:
		v = new(big.Int).Sub(left, right)

	case ast.ArithMul:
		v = new(big.Int).Mul(left, right)

	case ast.ArithDiv:
		if right.Sign() == 0 {
			return nil, errors.New("division by zero")
		}
		v = new(big.Int).Div(left, right)

	case ast.ArithMod:
		v = new(big.Int).Mod(left, right)

	case ast.ArithAnd:
		v = new(big.Int).And(left, right)

	case ast.ArithOr:
		v = new(big.Int).Or(left, right)

	case ast.ArithXor:
		v = new(big.Int).Xor(left, right)

	case ast.ArithLshift:
		if right.Sign() == -1 {
			return nil, errors.New("negative lshift amount")
		}
		if right.Cmp(bigMaxUint) > 0 {
			return nil, fmt.Errorf("lshift amount %d overflows uint", right)
		}
		amount := uint(right.Uint64())
		v = new(big.Int).Lsh(left, amount)

	case ast.ArithRshift:
		if right.Sign() == -1 {
			return nil, errors.New("negative rshift amount")
		}
		if right.Cmp(bigMaxUint) > 0 {
			return nil, fmt.Errorf("rshift amount %d overflows uint", right)
		}
		amount := uint(right.Uint64())
		v = new(big.Int).Rsh(left, amount)

	default:
		panic(fmt.Errorf("invalid arith op %v", expr.Op))
	}
	return v, nil
}

func (e *evaluator) evalVariable(expr *ast.VariableExpr, env *evalEnvironment) (*big.Int, error) {
	v := env.variables[expr.Ident]
	if v != nil {
		return v, nil
	}
	// Check for instruction macro args.
	if a := env.macroArgs; a != nil {
		i := slices.Index(a.def.Params, expr.Ident)
		if i == -1 {
			return nil, fmt.Errorf("%w $%s", ecUndefinedVariable, expr.Ident)
		}
		arg := a.args[i]
		// Evaluate it in the parent scope.
		return e.eval(arg, newEvalEnvironment(a.callsite))
	}
	return nil, fmt.Errorf("%w $%s", ecUndefinedVariable, expr.Ident)
}

func (e *evaluator) evalMacroCall(expr *ast.MacroCallExpr, env *evalEnvironment) (*big.Int, error) {
	if expr.Builtin {
		builtin, ok := builtinMacros[expr.Ident]
		if ok {
			return builtin(e, env, expr)
		}
		return nil, fmt.Errorf("%w .%s", ecUndefinedBuiltinMacro, expr.Ident)
	}
	def, defdoc := e.lookupExprMacro(env, expr.Ident)
	if def == nil {
		return nil, fmt.Errorf("%w %s", ecUndefinedMacro, expr.Ident)
	}

	// Prevent recursion.
	if !e.enterMacro(def) {
		return nil, fmt.Errorf("%w %s", ecRecursiveCall, expr.Ident)
	}
	defer e.exitMacro(def)

	// Bind arguments.
	macroEnv := &evalEnvironment{
		variables: make(map[string]*big.Int, len(def.Params)),
		doc:       defdoc,
	}
	if err := checkArgCount(expr, len(def.Params)); err != nil {
		return nil, err
	}
	if len(expr.Args) != len(def.Params) {
		return nil, fmt.Errorf("%w, macro %s needs %d", ecInvalidArgumentCount, expr.Ident, len(def.Params))
	}
	for i, param := range def.Params {
		v, err := e.eval(expr.Args[i], env)
		if err != nil {
			return nil, err
		}
		macroEnv.variables[param] = v
	}

	// Compute the macro result value.
	return e.eval(def.Body, macroEnv)
}

func checkArgCount(expr *ast.MacroCallExpr, n int) error {
	if len(expr.Args) != n {
		return fmt.Errorf("%w, macro %s needs %d", ecInvalidArgumentCount, expr.Ident, n)
	}
	return nil
}

func (e *evaluator) enterMacro(m *ast.ExpressionMacroDef) bool {
	_, found := e.inStack[m]
	if found {
		return false
	}
	e.inStack[m] = struct{}{}
	return true
}

func (e *evaluator) exitMacro(m *ast.ExpressionMacroDef) {
	delete(e.inStack, m)
}
