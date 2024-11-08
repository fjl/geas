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
	default:
		return nil
	}
}
