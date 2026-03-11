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

// Package stackcheck verifies user-written stack comments in geas assembly programs.
package stackcheck

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
	"github.com/fjl/geas/internal/loader"
	"github.com/fjl/geas/internal/lzint"
	"github.com/fjl/geas/internal/stack"
)

type analyzer struct {
	prog   *loader.Program
	errors *loader.ErrorList

	// Precomputed effects.
	macroEffects   map[*ast.InstructionMacroDef]*stackEffect
	includeEffects map[*ast.Include]*stackEffect
}

// Check performs stack comment verification on a loaded program.
func Check(prog *loader.Program, errors *loader.ErrorList) {
	a := &analyzer{
		prog:           prog,
		errors:         errors,
		macroEffects:   make(map[*ast.InstructionMacroDef]*stackEffect),
		includeEffects: make(map[*ast.Include]*stackEffect),
	}
	// The top-level document uses closed-bottom mode because the EVM starts
	// with an empty stack.
	a.analyzeDocument(prog.Toplevel, false)
}

// analyzeDocument walks a document's statements and performs stack analysis.
// If inferred is true, the stack starts in inferred-input mode (for includes and
// macros without a start comment).
//
// It returns the computed effect of the document.
func (a *analyzer) analyzeDocument(doc *ast.Document, inferred bool) *stackEffect {
	a.analyzeDefs(doc)
	result := a.analyzeBlocks(doc, nil, inferred)
	if inferred {
		eff := effectFromStack(result.inferredInputs, result.exitItems)
		eff.jumps = result.externalJumps
		return eff
	}
	return &stackEffect{out: result.exitItems, jumps: result.externalJumps}
}

// analyzeDefs pre-analyzes all instruction macro definitions and include statements
// in the document.
func (a *analyzer) analyzeDefs(doc *ast.Document) {
	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.InstructionMacroDef:
			a.analyzeMacro(st)
		case *ast.Include:
			a.analyzeInclude(st)
		}
	}
}

// analyzeMacro analyzes an instruction macro body and stores its effect.
func (a *analyzer) analyzeMacro(def *ast.InstructionMacroDef) {
	if _, ok := a.macroEffects[def]; ok {
		return // already analyzed
	}

	// Determine the initial stack from the StartComment.
	var (
		initItems []string
		inferred  bool
	)
	if def.StartComment != nil && def.StartComment.IsStackComment() {
		items, err := stack.ParseComment(def.StartComment.InnerText())
		if err != nil {
			a.errors.AddAt(def, &stackWarning{err})
			inferred = true
		} else {
			initItems = items
		}
	} else {
		inferred = true
	}

	// Pre-analyze nested definitions.
	a.analyzeDefs(def.Body)

	// Walk the macro body.
	result := a.analyzeBlocks(def.Body, initItems, inferred)

	var eff *stackEffect
	if initItems != nil {
		in, _ := stack.StripWildcard(initItems)
		eff = &stackEffect{in: in, out: result.exitItems}
	} else {
		eff = effectFromStack(result.inferredInputs, result.exitItems)
	}
	eff.jumps = result.externalJumps
	a.macroEffects[def] = eff
}

// analyzeInclude analyzes an included document and stores its effect.
func (a *analyzer) analyzeInclude(inc *ast.Include) {
	if _, ok := a.includeEffects[inc]; ok {
		return
	}
	incdoc := a.prog.IncludeDoc(inc)
	if incdoc == nil {
		return
	}
	eff := a.analyzeDocument(incdoc, true)
	a.includeEffects[inc] = eff
}

// analysisResult holds the output of analyzeBlocks.
type analysisResult struct {
	exitItems      []string       // stack items after the last reachable block
	inferredInputs []string       // inferred inputs (only for inferred mode)
	externalJumps  []externalJump // jumps to labels not defined in this document
}

// blockState holds the per-block state computed during worklist propagation.
type blockState struct {
	entry      []string              // merged entry state (nil = not yet reached)
	exit       []string              // exit state after processing
	predExits  map[int][]string      // predecessor block index → exit state (-1 = initial)
	predWild   map[int]bool          // predecessor block index → exit wildcard flag
	predSource map[int]ast.Statement // predecessor → source statement (for error reporting)
	entryWild  bool                  // merged entry has wildcard (any predecessor has wildcard)
	exitWild   bool                  // exit state has wildcard
}

// Predecessor keys in predExits:
//   - initialPred (-1): the virtual predecessor representing the document's initial stack
//   - >= 0: real block indices
//   - < initialPred: virtual predecessors from external jumps (macro/include calls)
const initialPred = -1

// analyzeBlocks splits the document into basic blocks and performs stack analysis
// using an iterative worklist algorithm.
//
// The analysis has two phases:
//   - Phase 1 (propagation): run the worklist to compute stable entry/exit states
//     for all reachable blocks. Comments are applied to track names, but warnings
//     are not reported.
//   - Phase 2 (checking): walk all reachable blocks once with the stable entry
//     states and report warnings for comment mismatches.
func (a *analyzer) analyzeBlocks(doc *ast.Document, initialItems []string, inferred bool) analysisResult {
	blocks, labelIndex := splitBlocks(doc, a.prog)
	if len(blocks) == 0 {
		return analysisResult{exitItems: initialItems}
	}

	// Phase 1: compute stable entry/exit states.
	states, inferredInputs := a.propagateStates(blocks, labelIndex, doc, initialItems, inferred)

	// Phase 2: check comments.
	a.checkComments(blocks, doc, states, initialItems, inferred)
	a.checkJumpDepths(blocks, states)

	// Return the exit state of the last reachable block.
	var exit []string
	for i := len(blocks) - 1; i >= 0; i-- {
		if states[i].entry != nil {
			exit = states[i].exit
			break
		}
	}

	// Collect external jumps from blocks with unresolved jump targets.
	var extJumps []externalJump
	for i, blk := range blocks {
		if blk.hasExternalJump && states[i].exit != nil {
			extJumps = append(extJumps, externalJump{
				target: blk.jumpTarget,
				items:  states[i].exit,
				wild:   states[i].exitWild,
				jumpSt: blk.jumpSt,
			})
		}
	}
	return analysisResult{exitItems: exit, inferredInputs: inferredInputs, externalJumps: extJumps}
}

// propagateStates runs the worklist to compute stable entry/exit states for all
// reachable blocks. It returns the block states and, for inferred mode, the
// inferred input items.
func (a *analyzer) propagateStates(blocks []*basicBlock, labelIndex map[string]int, doc *ast.Document, initialItems []string, inferred bool) ([]blockState, []string) {
	states := make([]blockState, len(blocks))

	// nextVPred allocates virtual predecessor keys for external jumps.
	nextVPred := initialPred

	worklist := make([]int, 0, len(blocks))
	queued := make([]bool, len(blocks))
	enqueue := func(idx int) {
		if !queued[idx] {
			worklist = append(worklist, idx)
			queued[idx] = true
		}
	}

	// setPredSource records the source statement for a predecessor edge.
	setPredSource := func(succ *blockState, predKey int, src ast.Statement) {
		if succ.predSource == nil {
			succ.predSource = make(map[int]ast.Statement)
		}
		succ.predSource[predKey] = src
	}

	// mergeSuccessors propagates the exit state of block idx to its successors,
	// and propagates external jump edges from macro/include calls to their targets.
	mergeSuccessors := func(idx int, edges []externalJump) {
		lastSt := blocks[idx].statements[len(blocks[idx].statements)-1]
		for _, succ := range blocks[idx].successors {
			if mergePredecessor(&states[succ], idx, states[idx].exit, states[idx].exitWild) {
				enqueue(succ)
			}
			setPredSource(&states[succ], idx, lastSt)
		}
		for _, edge := range edges {
			if targetIdx, ok := labelIndex[edge.target]; ok {
				nextVPred--
				if mergePredecessor(&states[targetIdx], nextVPred, edge.items, edge.wild) {
					enqueue(targetIdx)
				}
				if edge.jumpSt != nil {
					setPredSource(&states[targetIdx], nextVPred, edge.jumpSt)
				}
			}
		}
	}

	// Process block 0 with the initial stack.
	var inferredInputs []string
	s := stack.New(initialItems, nil)
	if inferred {
		s.SetInferred()
	}
	edges := a.walkBlock(blocks[0], doc, s, false)
	if inferred {
		inferredInputs = s.InferredInputs()
	}

	// Record block 0's initial state as a virtual predecessor so that
	// back-edges to block 0 merge with the initial entry.
	initItems, initWild := stack.StripWildcard(initialItems)
	if initItems == nil {
		initItems = []string{}
	}
	states[0].predExits = map[int][]string{initialPred: initItems}
	states[0].predWild = map[int]bool{initialPred: initWild}
	states[0].entry = initItems
	states[0].entryWild = initWild
	states[0].exit = s.Items()
	states[0].exitWild = s.HasWildcard()

	// Seed the worklist with block 0's successors and external jumps.
	mergeSuccessors(0, edges)

	// Run the worklist until all entry states are stable.
	for len(worklist) > 0 {
		idx := worklist[len(worklist)-1]
		worklist = worklist[:len(worklist)-1]
		queued[idx] = false

		entry := states[idx].entry
		if states[idx].entryWild {
			entry = append(entry, stack.Wildcard)
		}
		s := stack.New(entry, nil)
		a.applyLabelComment(blocks[idx], s)
		edges := a.walkBlock(blocks[idx], doc, s, false)
		states[idx].exit = s.Items()
		states[idx].exitWild = s.HasWildcard()

		mergeSuccessors(idx, edges)
	}
	return states, inferredInputs
}

// mergePredecessor merges a predecessor's exit state into a successor block's entry.
// It returns true if the successor's entry state changed.
func mergePredecessor(succ *blockState, predKey int, items []string, wild bool) bool {
	if succ.predExits == nil {
		succ.predExits = make(map[int][]string)
		succ.predWild = make(map[int]bool)
	}
	if slices.Equal(succ.predExits[predKey], items) && succ.predWild[predKey] == wild {
		return false // no change
	}
	succ.predExits[predKey] = items
	succ.predWild[predKey] = wild

	// Recompute the merged entry state from all predecessor exits.
	newEntry := succ.baseNames()
	newWild := succ.anyPredWild()
	if slices.Equal(succ.entry, newEntry) && succ.entryWild == newWild {
		return false // entry didn't change
	}
	succ.entry = newEntry
	succ.entryWild = newWild
	return true
}

// applyLabelComment applies the block's label comment to the stack (if any),
// discarding errors. This is used during propagation to establish names.
func (a *analyzer) applyLabelComment(blk *basicBlock, s *stack.Stack) {
	if blk.labelComment == nil {
		return
	}
	comment, err := stack.ParseComment(blk.labelComment.InnerText())
	if err == nil {
		s.CheckComment(comment)
	}
}

// checkComments walks all reachable blocks with their stable entry states and
// reports comment mismatch warnings.
func (a *analyzer) checkComments(blocks []*basicBlock, doc *ast.Document, states []blockState, initialItems []string, inferred bool) {
	for i, blk := range blocks {
		if states[i].entry == nil {
			continue // unreachable
		}

		var s *stack.Stack
		if i == 0 {
			s = stack.New(initialItems, nil)
			if inferred {
				s.SetInferred()
			}
			// When block 0 is a loop head (has back-edge predecessors beyond the
			// initial virtual predecessor), check for unbalanced stack effects.
			if len(states[0].predExits) > 1 {
				a.checkLoopBalance(blk, &states[0], 0)
				a.checkLabelComment(blk, &states[0], s)
			}
		} else {
			names, confirmed := states[i].computeConfirmed()
			s = stack.New(names, confirmed)
			a.checkMergeDepth(blk, &states[i], i)
			a.checkLoopBalance(blk, &states[i], i)
			a.checkLabelComment(blk, &states[i], s)
		}

		a.walkBlock(blk, doc, s, true)
	}
}

// checkMergeDepth reports a warning if forward-edge predecessor blocks have different
// stack depths. The warning is suppressed when any predecessor has a wildcard exit or
// when the block's label comment uses a wildcard, since the depth is intentionally
// unknown. Back-edge depth mismatches are handled by [checkLoopBalance].
func (a *analyzer) checkMergeDepth(blk *basicBlock, bs *blockState, blockIdx int) {
	if len(bs.predExits) < 2 {
		return
	}
	if a.hasLabelWildcard(blk) || bs.anyPredWild() {
		return
	}
	seen := make(map[int]bool)
	for pred, items := range bs.predExits {
		if pred >= blockIdx {
			continue // skip back-edges
		}
		seen[len(items)] = true
	}
	if len(seen) < 2 {
		return
	}
	depths := make([]int, 0, len(seen))
	for d := range seen {
		depths = append(depths, d)
	}
	sort.Ints(depths)
	a.errors.AddAt(blk.statements[0], &stackWarning{fmt.Errorf("%w: predecessors have depths %v", stack.ErrMergeDepth, depths)})
}

// checkLoopBalance reports a warning if any back-edge predecessor has a different stack
// depth than the forward-edge entry depth. This detects loops whose body has a net
// push or pop effect, causing unbounded stack growth or underflow.
func (a *analyzer) checkLoopBalance(blk *basicBlock, bs *blockState, blockIdx int) {
	if a.hasLabelWildcard(blk) || bs.anyPredWild() {
		return
	}

	// Compute entry depth from forward predecessors.
	entryDepth := -1
	for pred, items := range bs.predExits {
		if pred < blockIdx {
			if entryDepth < 0 || len(items) < entryDepth {
				entryDepth = len(items)
			}
		}
	}
	if entryDepth < 0 {
		return
	}

	// Check back-edge depths.
	for pred, items := range bs.predExits {
		if pred >= blockIdx && len(items) != entryDepth {
			a.errors.AddAt(blk.statements[0], &stackWarning{fmt.Errorf("%w: entry depth %d, back-edge depth %d", stack.ErrLoopUnbalanced, entryDepth, len(items))})
			return
		}
	}
}

// hasLabelWildcard reports whether the block's label comment uses a wildcard.
func (a *analyzer) hasLabelWildcard(blk *basicBlock) bool {
	if blk.labelComment == nil {
		return false
	}
	comment, err := stack.ParseComment(blk.labelComment.InnerText())
	return err == nil && stack.HasWildcard(comment)
}

// checkLabelComment verifies the block's label comment against the merged stack state.
func (a *analyzer) checkLabelComment(blk *basicBlock, bs *blockState, s *stack.Stack) {
	if blk.labelComment == nil {
		return
	}
	comment, err := stack.ParseComment(blk.labelComment.InnerText())
	if err != nil {
		a.errors.AddAt(blk.statements[0], &stackWarning{err})
		return
	}
	if err := s.CheckComment(comment); err != nil {
		if errors.Is(err, stack.ErrCommentDepth) {
			// Depth errors are reported here only when the comment is wrong, i.e.
			// no predecessor matches the declared depth. For virtual-only labels,
			// the comment is trusted as ground truth and checkJumpDepths reports
			// per-jump errors instead.
			if !bs.onlyVirtualPreds() {
				commentItems, _ := stack.StripWildcard(comment)
				anyMatch := false
				for _, items := range bs.predExits {
					if len(items) == len(commentItems) {
						anyMatch = true
						break
					}
				}
				if !anyMatch {
					a.errors.AddAt(blk.statements[0], &stackWarning{err})
				}
			}
		} else if !bs.onlyVirtualPreds() {
			// Name errors from virtual predecessors are suppressed because the names
			// come from external jumps and are not meaningful in this scope.
			a.errors.AddAt(blk.statements[0], &stackWarning{err})
		}
	}
}

// checkJumpDepths verifies that each predecessor of a labeled block sends the
// correct number of stack items. This runs as a second pass after all label comments
// have been validated, and only checks labels whose comments are correct.
func (a *analyzer) checkJumpDepths(blocks []*basicBlock, states []blockState) {
	for i, blk := range blocks {
		if states[i].entry == nil || blk.labelComment == nil {
			continue
		}
		comment, err := stack.ParseComment(blk.labelComment.InnerText())
		if err != nil {
			continue
		}
		commentItems, commentWild := stack.StripWildcard(comment)
		if commentWild {
			continue
		}
		// Only report jump depth errors when the label comment is plausibly correct.
		// This is the case when at least one predecessor agrees with the declared
		// depth, or when all predecessors are virtual (external jumps from
		// macros/includes), in which case the comment is trusted as ground truth.
		// When all real predecessors disagree, the comment itself is wrong and
		// checkLabelComment already reported it.
		if !states[i].onlyVirtualPreds() {
			anyMatch := false
			for _, items := range states[i].predExits {
				if len(items) == len(commentItems) {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				continue
			}
		}

		var labelName string
		if ld, ok := blk.statements[0].(*ast.LabelDef); ok {
			labelName = ld.Ident
		}
		reported := make(map[ast.Statement]bool)
		preds := slices.Sorted(maps.Keys(states[i].predExits))
		for _, pred := range preds {
			items := states[i].predExits[pred]
			if len(items) != len(commentItems) {
				if src, ok := states[i].predSource[pred]; ok && !reported[src] {
					reported[src] = true
					a.errors.AddAt(src, &stackWarning{fmt.Errorf(
						"%w: label @%s expects %d items, jump sends %d",
						stack.ErrCommentDepth, labelName, len(commentItems), len(items))})
				}
			}
		}
	}
}

// anyPredWild reports whether any predecessor has a wildcard exit.
func (bs *blockState) anyPredWild() bool {
	for _, w := range bs.predWild {
		if w {
			return true
		}
	}
	return false
}

// baseNames returns the merged entry names from all predecessor exit states, using the
// minimum depth across predecessors as the entry depth.
func (bs *blockState) baseNames() []string {
	if len(bs.predExits) == 0 {
		return nil
	}
	minLen := -1
	var base []string
	for _, items := range bs.predExits {
		if minLen < 0 || len(items) < minLen {
			minLen = len(items)
			base = items
		}
	}
	names := make([]string, minLen)
	copy(names, base[:minLen])
	return names
}

// computeConfirmed computes the merged entry state and a confirmed mask from all
// predecessor exit states. A position is confirmed only if all non-virtual predecessors
// agree on the name at that position. Virtual predecessors from external jumps (macro
// and include calls) do not confirm names because their names come from a different scope.
func (bs *blockState) computeConfirmed() (names []string, confirmed []bool) {
	names = bs.baseNames()
	if names == nil {
		return nil, nil
	}
	confirmed = make([]bool, len(names))
	for i := range confirmed {
		confirmed[i] = true
	}
	for pred, items := range bs.predExits {
		for j := range len(names) {
			if pred < initialPred || items[j] != names[j] {
				confirmed[j] = false
			}
		}
	}
	// Append the wildcard sentinel so that stack.New sets the wildcard flag.
	// The confirmed slice intentionally has one fewer element; stack.Init
	// strips the wildcard before using confirmed.
	if bs.entryWild {
		names = append(names, stack.Wildcard)
	}
	return names, confirmed
}

// onlyVirtualPreds reports whether all predecessors are virtual, i.e. from
// external jumps in macro/include calls.
func (bs *blockState) onlyVirtualPreds() bool {
	for pred := range bs.predExits {
		if pred >= initialPred {
			return false
		}
	}
	return true
}

// walkBlock applies all statements in a basic block to the stack.
// It returns external jump edges discovered from macro/include calls within the block.
func (a *analyzer) walkBlock(blk *basicBlock, doc *ast.Document, s *stack.Stack, report bool) []externalJump {
	var edges []externalJump
	for _, st := range blk.statements {
		edges = append(edges, a.applyStatement(st, doc, s, report)...)
	}
	return edges
}

// applyStatement applies a single statement's stack effect to the virtual stack.
// If the statement is a macro or include call with external jumps, the returned edges
// represent those jumps translated to the caller's stack context.
func (a *analyzer) applyStatement(st ast.Statement, doc *ast.Document, s *stack.Stack, report bool) []externalJump {
	var (
		op    stack.Op
		imm   byte
		jumps []externalJump // external jumps from macro/include effect
	)
	switch st := st.(type) {
	case *ast.Opcode:
		op, imm = a.resolveOpcode(st)
		if op == nil {
			return nil
		}

	case *ast.InstructionMacroCall:
		def := a.prog.LookupInstrMacro(st.Ident, doc)
		if def == nil {
			return nil
		}
		eff := a.macroEffects[def]
		if eff == nil {
			return nil
		}
		op = eff
		jumps = eff.jumps

	case *ast.Include:
		eff := a.includeEffects[st]
		if eff == nil {
			return nil
		}
		op = eff
		jumps = eff.jumps

	default:
		return nil // skip other statements
	}

	// Compute external jump edges before applying the operation, since we need
	// the pre-application stack state to determine the caller's context below
	// the macro's input.
	var edges []externalJump
	if len(jumps) > 0 {
		edges = externalJumpEdges(s, op, jumps)
	}

	// Parse the comment.
	var comment []string
	if c := st.Comment(); c != nil && c.IsStackComment() {
		var err error
		if comment, err = stack.ParseComment(c.InnerText()); err != nil {
			if report {
				a.errors.AddAt(st, &stackWarning{err})
			}
			return edges
		}
	}

	// Apply the operation.
	if err := s.Apply(op, imm, comment); err != nil && report {
		a.errors.AddAt(st, &stackWarning{err})
	}

	// Check push literal vs. comment number.
	if report && comment != nil {
		a.checkPushLiteral(st, comment)
	}

	return edges
}

// externalJumpEdges translates a macro/include's external jumps into edges for the
// calling context. Each jump's items are combined with the caller's stack items below
// the macro input to produce the actual stack at each external jump target.
func externalJumpEdges(s *stack.Stack, op stack.Op, jumps []externalJump) []externalJump {
	callerItems := s.Items()
	inputDepth := len(op.StackIn(0))

	edges := make([]externalJump, len(jumps))
	for i, j := range jumps {
		actual := make([]string, 0, len(j.items)+len(callerItems)-inputDepth)
		actual = append(actual, j.items...)
		if len(callerItems) > inputDepth {
			actual = append(actual, callerItems[inputDepth:]...)
		}
		edges[i] = externalJump{
			target: j.target,
			items:  actual,
			wild:   j.wild || s.HasWildcard(),
			jumpSt: j.jumpSt,
		}
	}
	return edges
}

// checkPushLiteral verifies that when a push instruction has a literal number argument
// and the stack comment names a number for the pushed value, they match.
func (a *analyzer) checkPushLiteral(st ast.Statement, comment []string) {
	op, ok := st.(*ast.Opcode)
	if !ok || !ast.IsPush(strings.ToUpper(op.Op)) {
		return
	}

	var pushValue *lzint.Value
	if ast.IsPush0(op.Op) && op.Arg == nil {
		pushValue = lzint.FromInt64(0)
	} else if lit, ok := op.Arg.(*ast.LiteralExpr); ok {
		pushValue = lit.Value()
	}
	if pushValue == nil {
		return
	}

	commentValue, err := lzint.ParseNumberLiteral(comment[0])
	if err != nil {
		return // not a number, no check
	}
	if pushValue.Int().Cmp(commentValue.Int()) != 0 {
		a.errors.AddAt(st, &stackWarning{fmt.Errorf("%w: number %s in comment does not match pushed value %s", stack.ErrPushLiteral, comment[0], pushValue.String())})
	}
}

// resolveOpcode resolves an opcode statement to a stack.Op and immediate byte.
func (a *analyzer) resolveOpcode(op *ast.Opcode) (stack.Op, byte) {
	name := strings.ToUpper(op.Op)

	// Handle JUMP/JUMPI with label argument: these are compiled as PUSH + JUMP.
	// The push produces a value, and the jump consumes it. The net effect is
	// just consuming the other JUMP inputs (for JUMPI, that's the condition).
	if ast.IsJump(name) && op.Arg != nil {
		evmOp := a.prog.Fork.OpByName(name)
		if evmOp == nil {
			return nil, 0
		}
		return jumpWithArgEffect{op: evmOp}, 0
	}

	// Generic 'push' (with or without explicit size) always pushes one value.
	// The compiler resolves the actual PUSH<N> instruction later based on
	// the argument size, but for stack analysis it's just a push.
	if ast.IsPush(name) {
		evmOp := a.prog.Fork.OpByName(name)
		if evmOp != nil {
			return evmOp, 0
		}
		// Generic 'push <arg>' or 'push<n> <arg>' — treat as pushing one value.
		return pushEffect{}, 0
	}

	evmOp := a.prog.Fork.OpByName(name)
	if evmOp == nil {
		return nil, 0
	}

	// Compute the immediate byte for ops with immediates.
	var imm byte
	if evmOp.HasImmediate && len(op.Immediates) > 0 {
		var err error
		imm, err = evmOp.EncodeImmediateArgs(op.Immediates)
		if err != nil {
			return nil, 0
		}
	}

	return evmOp, imm
}

// pushEffect models a generic PUSH instruction.
type pushEffect struct{}

func (p pushEffect) StackIn(imm byte) []string  { return nil }
func (p pushEffect) StackOut(imm byte) []string { return []string{"val"} }

// jumpWithArgEffect models a 'JUMP @label' / 'JUMPI @label' instruction.
// The label arg is implicitly pushed, so the net effect is consuming
// everything except the label slot.
type jumpWithArgEffect struct {
	op *evm.Op
}

func (j jumpWithArgEffect) StackIn(imm byte) []string {
	in := j.op.StackIn(imm)
	// The first input is the jump destination (label), which is provided by the
	// implicit push. Return the remaining inputs.
	if len(in) > 1 {
		return in[1:]
	}
	return nil
}

func (j jumpWithArgEffect) StackOut(imm byte) []string {
	return nil // jumps produce nothing
}

// stackWarning wraps a stack analysis error as a compiler warning.
type stackWarning struct {
	err error
}

func (w *stackWarning) Error() string {
	return w.err.Error()
}

func (w *stackWarning) Unwrap() error {
	return w.err
}

func (w *stackWarning) IsWarning() bool {
	return true
}
