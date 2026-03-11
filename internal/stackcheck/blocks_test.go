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
	"reflect"
	"strings"
	"testing"

	"github.com/fjl/geas/internal/loader"
)

func parseDoc(t *testing.T, input string) *loader.Program {
	t.Helper()
	l := loader.New(nil)
	prog := l.LoadSource("test", []byte(input))
	if l.Errors().HasError() {
		for _, e := range l.Errors().ErrorsAndWarnings() {
			t.Error(e)
		}
		t.Fatal("test source has errors")
	}
	return prog
}

// blockInfo captures the relevant fields of a basic block for test comparison.
type blockInfo struct {
	stmtCount                 int
	label                     string
	hasLabelComment           bool
	isJumpTarget              bool
	canFallThrough            bool
	jumpTarget                string
	endsWithTerminal          bool
	endsWithUnconditionalJump bool
	successors                []int
}

func toBlockInfos(blocks []*basicBlock) []blockInfo {
	infos := make([]blockInfo, len(blocks))
	for i, b := range blocks {
		infos[i] = blockInfo{
			stmtCount:                 len(b.statements),
			label:                     b.label,
			hasLabelComment:           b.labelComment != nil,
			isJumpTarget:              b.isJumpTarget,
			canFallThrough:            b.canFallThrough,
			jumpTarget:                b.jumpTarget,
			endsWithTerminal:          b.endsWithTerminal,
			endsWithUnconditionalJump: b.endsWithUnconditionalJump,
			successors:                b.successors,
		}
	}
	return infos
}

func stmtDescs(blk *basicBlock) []string {
	descs := make([]string, len(blk.statements))
	for i, st := range blk.statements {
		descs[i] = st.Description()
	}
	return descs
}

// This test verifies basic block splitting for all relevant cases.
func TestSplitBlocks(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		blocks []blockInfo
	}{
		{
			name:   "empty",
			input:  "",
			blocks: nil,
		},
		{
			name:  "singleOpcode",
			input: "push 1",
			blocks: []blockInfo{
				{stmtCount: 1, canFallThrough: true},
			},
		},
		{
			name: "linearSequence",
			input: `
				push 1
				push 2
				add
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true},
			},
		},
		{
			name: "labelSplitsBlock",
			input: `
				push 1
				target:
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 1, canFallThrough: true, successors: []int{1}},
				{stmtCount: 2, label: "target", isJumpTarget: true, canFallThrough: true},
			},
		},
		{
			name: "labelAtStart",
			input: `
				target:
				push 1
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, label: "target", isJumpTarget: true, canFallThrough: true},
			},
		},
		{
			name: "multipleLabelsSplitBlocks",
			input: `
				push 1
				a:
				push 2
				b:
				push 3
			`,
			blocks: []blockInfo{
				{stmtCount: 1, canFallThrough: true, successors: []int{1}},
				{stmtCount: 2, label: "a", isJumpTarget: true, canFallThrough: true, successors: []int{2}},
				{stmtCount: 2, label: "b", isJumpTarget: true, canFallThrough: true},
			},
		},
		{
			name: "dottedLabelDoesNotSplit",
			input: `
				push 1
				.dot:
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true},
			},
		},
		{
			name: "stopEndsBlock",
			input: `
				push 1
				stop
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "returnEndsBlock",
			input: `
				push 0
				push 0
				return
				push 3
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "revertEndsBlock",
			input: `
				push 0
				push 0
				revert
				push 3
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "unconditionalJumpEndsBlock",
			input: `
				target:
				push 1
				jump @target
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, label: "target", isJumpTarget: true, canFallThrough: true, endsWithUnconditionalJump: true, jumpTarget: "target", successors: []int{0}},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "dynamicJumpEndsBlock",
			input: `
				push 1
				jump
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true, endsWithUnconditionalJump: true},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "conditionalJumpEndsBlock",
			input: `
				target:
				push 1
				jumpi @target
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, label: "target", isJumpTarget: true, canFallThrough: true, jumpTarget: "target", successors: []int{1, 0}},
				{stmtCount: 1, canFallThrough: true},
			},
		},
		{
			name: "jumpFollowedByLabel",
			input: `
				target:
				push 1
				jump @target
				other:
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, label: "target", isJumpTarget: true, canFallThrough: true, endsWithUnconditionalJump: true, jumpTarget: "target", successors: []int{0}},
				{stmtCount: 2, label: "other", isJumpTarget: true, canFallThrough: false},
			},
		},
		{
			name: "stopFollowedByLabel",
			input: `
				push 0
				stop
				next:
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 2, label: "next", isJumpTarget: true, canFallThrough: false},
			},
		},
		{
			name: "labelWithStackComment",
			input: `
				target: ; [a, b]
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 2, label: "target", isJumpTarget: true, hasLabelComment: true, canFallThrough: true},
			},
		},
		{
			name: "labelWithNonStackComment",
			input: `
				target: ; this is not a stack comment
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 2, label: "target", isJumpTarget: true, hasLabelComment: false, canFallThrough: true},
			},
		},
		{
			name: "exprMacroDefsSkipped",
			input: `
				push 1
				#define X = 1
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true},
			},
		},
		{
			name: "includeInBlock",
			input: `
				push 1
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true},
			},
		},
		{
			name: "complexFlow",
			input: `
				push 1
				push 0
				jumpi @handler

				push 2
				stop

				handler: ; [x]
				push 3
				add
				jump @handler
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true, jumpTarget: "handler", successors: []int{1, 2}},
				{stmtCount: 2, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 4, label: "handler", isJumpTarget: true, hasLabelComment: true, canFallThrough: false, endsWithUnconditionalJump: true, jumpTarget: "handler", successors: []int{2}},
			},
		},
		{
			name: "consecutiveLabels",
			input: `
				a:
				b:
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 1, label: "a", isJumpTarget: true, canFallThrough: true, successors: []int{1}},
				{stmtCount: 2, label: "b", isJumpTarget: true, canFallThrough: true},
			},
		},
		{
			name: "sendallTerminal",
			input: `
				push 0
				sendall
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true, endsWithTerminal: true},
				{stmtCount: 1, canFallThrough: false},
			},
		},
		{
			name: "onlyLabels",
			input: `
				a:
				b:
				c:
			`,
			blocks: []blockInfo{
				{stmtCount: 1, label: "a", isJumpTarget: true, canFallThrough: true, successors: []int{1}},
				{stmtCount: 1, label: "b", isJumpTarget: true, canFallThrough: true, successors: []int{2}},
				{stmtCount: 1, label: "c", isJumpTarget: true, canFallThrough: true},
			},
		},
		{
			name: "conditionalJumpNoArg",
			input: `
				push 1
				push 2
				push 3
				jumpi
				push 4
			`,
			blocks: []blockInfo{
				{stmtCount: 4, canFallThrough: true, successors: []int{1}},
				{stmtCount: 1, canFallThrough: true},
			},
		},
		{
			name: "terminalAtEnd",
			input: `
				push 0
				push 0
				return
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true, endsWithTerminal: true},
			},
		},
		{
			name: "jumpAtEnd",
			input: `
				target:
				jump @target
			`,
			blocks: []blockInfo{
				{stmtCount: 2, label: "target", isJumpTarget: true, canFallThrough: true, endsWithUnconditionalJump: true, jumpTarget: "target", successors: []int{0}},
			},
		},
		{
			name: "multipleJumpsToSameTarget",
			input: `
				push 0
				jumpi @end
				push 1
				jumpi @end
				push 2
				end:
				stop
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true, jumpTarget: "end", successors: []int{1, 3}},
				{stmtCount: 2, canFallThrough: true, jumpTarget: "end", successors: []int{2, 3}},
				{stmtCount: 1, canFallThrough: true, successors: []int{3}},
				{stmtCount: 2, label: "end", isJumpTarget: true, canFallThrough: true, endsWithTerminal: true},
			},
		},
		{
			name: "bytesDirectiveInBlock",
			input: `
				push 1
				#bytes 0x1234
				push 2
			`,
			blocks: []blockInfo{
				{stmtCount: 3, canFallThrough: true},
			},
		},
		{
			name: "commentOnlyBlock",
			input: `
				;; just a comment
				push 1
			`,
			blocks: []blockInfo{
				{stmtCount: 2, canFallThrough: true},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := strings.TrimSpace(test.input)
			// Normalize indentation.
			input = strings.ReplaceAll(input, "\t", "")
			prog := parseDoc(t, input)
			blocks, _ := splitBlocks(prog.Toplevel, prog)
			got := toBlockInfos(blocks)

			if len(got) != len(test.blocks) {
				t.Fatalf("got %d blocks, want %d\n  got:  %+v", len(got), len(test.blocks), got)
			}
			for i := range got {
				if !reflect.DeepEqual(got[i], test.blocks[i]) {
					t.Errorf("block[%d] mismatch:\n  got:  %+v\n  want: %+v\n  stmts: %v",
						i, got[i], test.blocks[i], stmtDescs(blocks[i]))
				}
			}
		})
	}
}
