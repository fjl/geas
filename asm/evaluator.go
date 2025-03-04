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
	"github.com/fjl/geas/internal/lzint"
)

// evaluator is for evaluating expressions.
type evaluator struct {
	inStack     map[*ast.ExpressionMacroDef]struct{}
	labelInstr  map[evalLabelKey]*instruction
	usedLabels  map[*ast.LabelDefSt]struct{}
	globals     *globalScope
	labelsValid bool // while false, evaluating labels returns unassignedLabelErr
}

type evalLabelKey struct {
	doc *ast.Document
	l   *ast.LabelDefSt
}

// evalEnvironment holds the definitions available for evaluation.
type evalEnvironment struct {
	doc       *ast.Document           // for resolving local macros
	macroArgs *instrMacroArgs         // args of the current instruction macro
	variables map[string]*lzint.Value // args of the current expression macro
}

func newEvalEnvironment(s *compilerSection) *evalEnvironment {
	if s == nil {
		panic("nil section")
	}
	return &evalEnvironment{doc: s.doc, macroArgs: s.macroArgs}
}

// makeCallEnvironment creates the environment for an expression macro call.
func (env *evalEnvironment) makeCallEnvironment(defdoc *ast.Document, def *ast.ExpressionMacroDef) *evalEnvironment {
	return &evalEnvironment{
		doc:       defdoc,
		variables: make(map[string]*lzint.Value, len(def.Params)),
	}
}

func newEvaluator(gs *globalScope) *evaluator {
	return &evaluator{
		inStack:    make(map[*ast.ExpressionMacroDef]struct{}),
		labelInstr: make(map[evalLabelKey]*instruction),
		usedLabels: make(map[*ast.LabelDefSt]struct{}),
		globals:    gs,
	}
}

// registerLabels sets up the label-to-instruction mapping. This also makes labels
// available for evaluation, so the compiler calls this after attempting to pre-evaluate
// the arguments.
func (e *evaluator) registerLabels(labels []*compilerLabel) {
	for _, cl := range labels {
		if cl.def.Global {
			e.globals.setLabelInstr(cl.def.Name(), cl.instr)
		} else {
			e.labelInstr[evalLabelKey{cl.doc, cl.def}] = cl.instr
		}
	}
	e.labelsValid = true
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

// lookupLabel resolves a label reference.
func (e *evaluator) lookupLabel(doc *ast.Document, lref *ast.LabelRefExpr) (pc int, pcValid bool, err error) {
	if !e.labelsValid {
		return 0, false, nil
	}

	var li *ast.LabelDefSt
	var instr *instruction
	if lref.Global {
		instr, li = e.globals.lookupLabel(lref)
	} else {
		var srcdoc *ast.Document
		li, srcdoc = doc.LookupLabel(lref)
		instr = e.labelInstr[evalLabelKey{srcdoc, li}]
	}
	if li == nil {
		return 0, false, fmt.Errorf("undefined label %v", lref)
	}
	if lref.Dotted && !li.Dotted {
		//lint:ignore ST1005 using : at the end of message here to refer to a label definition
		return 0, false, fmt.Errorf("can't use %v to refer to label %s:", lref, li.Name())
	}
	if instr == nil {
		return 0, false, nil
	}
	// mark label used (for unused label analysis)
	e.usedLabels[li] = struct{}{}
	return instr.pc, true, nil
}

// isLabelUsed reports whether the given label definition was used during expression evaluation.
func (e *evaluator) isLabelUsed(li *ast.LabelDefSt) bool {
	_, ok := e.usedLabels[li]
	return ok
}

func (e *evaluator) eval(expr ast.Expr, env *evalEnvironment) (*lzint.Value, error) {
	switch expr := expr.(type) {
	case *ast.LiteralExpr:
		return e.evalLiteral(expr)
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

// evalAsBytes gives the byte value of an expression.
func (e *evaluator) evalAsBytes(expr ast.Expr, env *evalEnvironment) ([]byte, error) {
	v, err := e.eval(expr, env)
	if err != nil {
		return nil, err
	}
	return v.Bytes()
}

func (e *evaluator) evalLiteral(expr *ast.LiteralExpr) (*lzint.Value, error) {
	if expr.Value != nil {
		return expr.Value, nil
	}

	switch {
	case expr.IsNumber():
		val, err := lzint.ParseNumberLiteral(expr.Text())
		if err != nil {
			return nil, err
		}
		expr.Value = val
		return val, nil

	case expr.IsString():
		val := lzint.FromBytes([]byte(expr.Text()))
		expr.Value = val
		return val, nil

	default:
		panic(fmt.Errorf("unhandled astLiteral %q (not string|number)", expr.Text()))
	}
}

func (e *evaluator) evalLabelRef(expr *ast.LabelRefExpr, env *evalEnvironment) (*lzint.Value, error) {
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
	return lzint.FromInt(big.NewInt(int64(pc))), nil
}

var bigMaxUint = new(big.Int).SetUint64(math.MaxUint)

func (e *evaluator) evalArith(expr *ast.ArithExpr, env *evalEnvironment) (*lzint.Value, error) {
	// compute operands
	leftVal, err := e.eval(expr.Left, env)
	if err != nil {
		return nil, err
	}
	rightVal, err := e.eval(expr.Right, env)
	if err != nil {
		return nil, err
	}
	left, right := leftVal.Int(), rightVal.Int()

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

	return lzint.FromInt(v), nil
}

func (e *evaluator) evalVariable(expr *ast.VariableExpr, env *evalEnvironment) (*lzint.Value, error) {
	v, ok := env.variables[expr.Ident]
	if ok {
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

func (e *evaluator) evalMacroCall(expr *ast.MacroCallExpr, env *evalEnvironment) (*lzint.Value, error) {
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
	if err := checkArgCount(expr, len(def.Params)); err != nil {
		return nil, err
	}
	if len(expr.Args) != len(def.Params) {
		return nil, fmt.Errorf("%w, macro %s needs %d", ecInvalidArgumentCount, expr.Ident, len(def.Params))
	}
	macroEnv := env.makeCallEnvironment(defdoc, def)
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
