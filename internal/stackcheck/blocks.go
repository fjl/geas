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

package stackcheck

import (
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/loader"
)

// basicBlock represents a sequence of statements with no internal jumps.
type basicBlock struct {
	statements   []ast.Statement // statements in the block
	label        string          // non-empty if the block starts with a JUMPDEST label definition
	labelComment *ast.Comment    // the stack comment on the label, if any
	jumpTarget   string          // label name if the block ends with a jump to a known label
	jumpSt       ast.Statement   // the jump statement (last in block)
	successors   []int           // indices of successor blocks (fall-through and/or jump target)

	isJumpTarget              bool // whether this block is the target of a known jump
	endsWithTerminal          bool // block ends with a terminal instruction
	endsWithUnconditionalJump bool // block ends with JUMP (unconditional)
	hasExternalJump           bool // jump target is not defined in this document
	// true when the previous block can fall through to this one
	// (i.e. its last instruction is not a terminal or unconditional jump).
	canFallThrough bool
}

// splitBlocks divides a document's statements into basic blocks.
// Blocks are split at:
//   - non-dotted label definitions (JUMPDEST boundaries)
//   - after terminal instructions (STOP, RETURN, REVERT, etc.)
//   - after jumps
func splitBlocks(doc *ast.Document, prog *loader.Program) ([]*basicBlock, map[string]int) {
	var blocks []*basicBlock
	cur := &basicBlock{canFallThrough: true}

	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.LabelDef:
			if st.Dotted {
				cur.statements = append(cur.statements, st)
				continue
			}
			// Non-dotted label: start a new block.
			if len(cur.statements) > 0 {
				blocks = append(blocks, cur)
				prev := cur
				cur = &basicBlock{
					canFallThrough: !prev.endsWithTerminal && !prev.endsWithUnconditionalJump,
				}
			}
			cur.label = st.Ident
			if c := st.Comment(); c != nil && c.IsStackComment() {
				cur.labelComment = c
			}
			cur.isJumpTarget = true // labels are potential jump targets
			cur.statements = append(cur.statements, st)

		case *ast.Opcode:
			cur.statements = append(cur.statements, st)
			name := strings.ToUpper(st.Op)
			evmOp := prog.Fork.OpByName(name)
			if evmOp == nil {
				continue
			}
			if evmOp.Term || evmOp.Unconditional {
				// Record jump target if it's a jump with a label argument.
				if evmOp.Jump {
					if lref, ok := st.Arg.(*ast.LabelRefExpr); ok && !lref.Dotted {
						cur.jumpTarget = lref.Ident
						cur.jumpSt = st
					}
				}
				cur.endsWithTerminal = evmOp.Term
				cur.endsWithUnconditionalJump = evmOp.Unconditional
				blocks = append(blocks, cur)
				cur = &basicBlock{canFallThrough: false}
			} else if evmOp.Jump {
				// Conditional jump (JUMPI): record target, end block.
				if lref, ok := st.Arg.(*ast.LabelRefExpr); ok && !lref.Dotted {
					cur.jumpTarget = lref.Ident
					cur.jumpSt = st
				}
				blocks = append(blocks, cur)
				cur = &basicBlock{canFallThrough: true}
			}

		default:
			cur.statements = append(cur.statements, st)
		}
	}

	// Append the final block if it has statements.
	if len(cur.statements) > 0 {
		blocks = append(blocks, cur)
	}

	// Build a label-to-block index for resolving jump targets.
	labelIndex := make(map[string]int, len(blocks))
	for i, blk := range blocks {
		if blk.label != "" {
			labelIndex[blk.label] = i
		}
	}

	// Compute successor edges.
	for i, blk := range blocks {
		if i+1 < len(blocks) && !blk.endsWithTerminal && !blk.endsWithUnconditionalJump {
			blk.successors = append(blk.successors, i+1)
		}
		if blk.jumpTarget != "" {
			if j, ok := labelIndex[blk.jumpTarget]; ok {
				blk.successors = append(blk.successors, j)
			} else {
				blk.hasExternalJump = true
			}
		}
	}

	return blocks, labelIndex
}
