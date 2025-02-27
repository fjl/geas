// Copyright 2024 The go-ethereum Authors
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
	"github.com/fjl/geas/internal/ast"
)

// statement wraps an AST statement in a document.
type statement interface {
	ast.Statement
	expand(c *Compiler, doc *ast.Document, prog *compilerProg) error
}

// Statement types.
type (
	opcodeStatement    struct{ *ast.OpcodeSt }
	labelDefStatement  struct{ *ast.LabelDefSt }
	macroCallStatement struct{ *ast.MacroCallSt }
	includeStatement   struct{ *ast.IncludeSt }
	assembleStatement  struct{ *ast.AssembleSt }
	bytesStatement     struct{ *ast.BytesSt }
)

// statementFromAST converts AST statements into compiler statements. Note this function
// returns nil for statement types the compiler doesn't care about.
func statementFromAST(st ast.Statement) statement {
	switch st := st.(type) {
	case *ast.OpcodeSt:
		return opcodeStatement{st}
	case *ast.LabelDefSt:
		return labelDefStatement{st}
	case *ast.MacroCallSt:
		return macroCallStatement{st}
	case *ast.IncludeSt:
		return includeStatement{st}
	case *ast.AssembleSt:
		return assembleStatement{st}
	case *ast.BytesSt:
		return bytesStatement{st}
	default:
		return nil
	}
}
