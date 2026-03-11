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

package stack

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fjl/geas/internal/evm"
)

type stackTest struct {
	t *testing.T
	s *Stack
}

func newTest(t *testing.T, initial string) *stackTest {
	commentSlice, err := ParseComment(initial)
	if err != nil {
		panic("invalid stack comment: " + initial)
	}
	return &stackTest{t, New(commentSlice, nil)}
}

func newInferredTest(t *testing.T) *stackTest {
	s := New(nil, nil)
	s.SetInferred()
	return &stackTest{t, s}
}

func (t *stackTest) applyOK(op *evm.Op, imm byte, comment string) {
	t.t.Helper()
	commentSlice, err := ParseComment(comment)
	if err != nil {
		panic("invalid stack comment: " + comment)
	}
	t.t.Logf("apply %5s on %s", op.Name, t.s.String())
	if err := t.s.Apply(op, imm, commentSlice); err != nil {
		t.t.Fatalf("error: %v", err)
	}
}

func (t *stackTest) checkItems(want string) {
	t.t.Helper()
	wantSlice, err := ParseComment(want)
	if err != nil {
		panic("invalid stack comment: " + want)
	}
	got := t.s.Items()
	if !reflect.DeepEqual(got, wantSlice) {
		t.t.Fatalf("stack items mismatch:\n  got:  %v\n  want: %v", got, wantSlice)
	}
}

func (t *stackTest) checkInferredInputs(want ...string) {
	t.t.Helper()
	if want == nil {
		want = []string{}
	}
	got := t.s.InferredInputs()
	if !reflect.DeepEqual(got, want) {
		t.t.Fatalf("inferred inputs mismatch:\n  got:  %v\n  want: %v", got, want)
	}
}

func (t *stackTest) applyErr(op *evm.Op, imm byte, comment string, wantErr error) {
	t.t.Helper()
	commentSlice, err := ParseComment(comment)
	if err != nil {
		panic("invalid stack comment: " + comment)
	}
	t.t.Logf("apply %5s on %s", op.Name, t.s.String())
	err = t.s.Apply(op, imm, commentSlice)
	if err == nil {
		t.t.Fatalf("expected error, got none")
	}
	if err.Error() != wantErr.Error() {
		t.t.Fatalf("wrong error: %v\n         want: %v", err, wantErr)
	}
}

func TestStackAnalysis(t *testing.T) {
	vm := evm.FindInstructionSet("cancun")
	var (
		push0 = vm.PushBySize(0)
		push1 = vm.PushBySize(1)
		add   = vm.OpByName("ADD")
		swap2 = vm.OpByName("SWAP2")
		dup1  = vm.OpByName("DUP1")
		dup2  = vm.OpByName("DUP2")
		pop   = vm.OpByName("POP")
	)
	t.Run("ok", func(t *testing.T) {
		st := newTest(t, "[a, b, c, d]")
		st.applyOK(dup2, 0, "[b, a, b, c, d]")
		st.applyOK(add, 0, "[sum, b, c, d]")
		st.applyOK(swap2, 0, "[c, b, sum, d]")
		st.applyOK(dup1, 0, "[c, c, b, sum, d]")
		st.applyOK(push1, 0, "[val, c, c, b, sum, d]")
		st.applyOK(swap2, 0, "[c, c, val, b, sum, d]")
	})
	t.Run("numberZero", func(t *testing.T) {
		st := newTest(t, "[a]")
		st.applyOK(push0, 0, "[0, a]")
		st.applyOK(pop, 0, "[a]")
	})
	t.Run("initWithDuplicates", func(t *testing.T) {
		st := newTest(t, "[a, a, a]")
		st.applyOK(add, 0, "[sum, a]")
	})
	t.Run("commentMismatch", func(t *testing.T) {
		st := newTest(t, "[a, b, c, d]")
		st.applyErr(add, 0, "[sum, d, c]",
			fmt.Errorf("%w: item %d differs (expected %q, have %q) in [sum, c, d]", ErrMismatch, 1, "d", "c"),
		)
	})
	t.Run("opUnderflows", func(t *testing.T) {
		st := newTest(t, "[a]")
		st.applyErr(add, 0, "[sum]",
			fmt.Errorf("%w: op requires %d items, stack has %d", ErrOpUnderflow, 2, 1),
		)
	})
	t.Run("commentUnderflows", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		st.applyErr(add, 0, "[sum, b]",
			fmt.Errorf("%w: stack has %d items, comment declares %d", ErrCommentUnderflow, 1, 2),
		)
	})
	t.Run("stackItemRenamed", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		st.applyOK(push1, 0, "[x, a, b]")
		st.applyErr(add, 0, "[sum, c]",
			fmt.Errorf("%w: %s renamed to %s", ErrRename, "b", "c"),
		)
	})

	// Test that multiple new items can share the same name.
	// This is common when pushing the same value multiple times,
	// e.g., "push 0; push 0" with comment [0, 0].
	t.Run("duplicateValueNames", func(t *testing.T) {
		st := newTest(t, "[]")
		st.applyOK(push0, 0, "[0]")
		st.applyOK(push0, 0, "[0, 0]")
		st.applyOK(push0, 0, "[0, 0, 0]")
		// Verify the items are tracked correctly by consuming some
		st.applyOK(add, 0, "[sum, 0]")
		st.applyOK(add, 0, "[sum]")
	})

	// Test that a name can be reused after the original item is consumed.
	// This is common when a value is pushed, used, and
	// then the same name pushed again later.
	t.Run("nameReuseAfterConsumption", func(t *testing.T) {
		st := newTest(t, "[a]")
		st.applyOK(push1, 0, "[x, a]")
		st.applyOK(add, 0, "[sum]")
		// Now "x" and "a" are consumed, we should be able to reuse those names.
		st.applyOK(push1, 0, "[x, sum]")
		st.applyOK(push1, 0, "[a, x, sum]")
		st.applyOK(add, 0, "[result, sum]")
	})

	// Test combination: reuse name while another item with that name
	// is still on the stack (but not at the position being checked).
	t.Run("nameReuseWithExistingOnStack", func(t *testing.T) {
		st := newTest(t, "[a]")
		st.applyOK(push1, 0, "[b, a]")
		// Push another item and name it "a" - this should work because
		// the new item can share the name with the existing "a"
		st.applyOK(push1, 0, "[a, b, a]")
	})

	// Test that existing items cannot claim names belonging to other
	// items still on the stack. SWAP reorders without creating new items,
	// so claiming a swapped position has the wrong name should fail.
	t.Run("existingItemCannotStealName", func(t *testing.T) {
		swap1 := vm.OpByName("SWAP1")
		st := newTest(t, "[a, b]")
		// After SWAP1, the stack is [b, a], not [a, b].
		// Claiming it's still [a, b] should error at position 0.
		st.applyErr(swap1, 0, "[a, b]",
			fmt.Errorf("%w: item %d differs (expected %q, have %q) in [b, a]", ErrMismatch, 0, "a", "b"),
		)
	})
}

// This test verifies that unconfirmed names from merge-point Init
// can be renamed by subsequent comments without triggering warnings.
func TestMergeInit(t *testing.T) {
	vm := evm.FindInstructionSet("cancun")
	swap1 := vm.OpByName("SWAP1")

	// Simulate a merge point where positions 0 and 1 disagree across
	// predecessors (unconfirmed) and position 2 agrees (confirmed).
	t.Run("unconfirmedRenameAllowed", func(t *testing.T) {
		s := New([]string{"0", "msgLen", "msgLen"}, []bool{false, false, true})
		comment, _ := ParseComment("[val, count, msgLen]")
		err := s.Apply(swap1, 0, comment)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Confirmed names should still produce a warning when renamed.
	t.Run("confirmedRenameWarns", func(t *testing.T) {
		s := New([]string{"a", "b"}, []bool{true, true})
		comment, _ := ParseComment("[b, renamed]")
		err := s.Apply(swap1, 0, comment)
		if err == nil {
			t.Fatal("expected error, got none")
		}
		want := fmt.Errorf("%w: %s renamed to %s", ErrRename, "a", "renamed")
		if err.Error() != want.Error() {
			t.Fatalf("wrong error: %v\n         want: %v", err, want)
		}
	})
}

// Tests for inferred-input stack mode.
func TestInferred(t *testing.T) {
	vm := evm.FindInstructionSet("cancun")
	var (
		add   = vm.OpByName("ADD")
		swap1 = vm.OpByName("SWAP1")
		swap2 = vm.OpByName("SWAP2")
		dup1  = vm.OpByName("DUP1")
		push1 = vm.PushBySize(1)
		pop   = vm.OpByName("POP")
	)

	// ADD on an empty inferred stack: two items are created below,
	// then consumed by ADD, producing one new item.
	t.Run("addOnEmpty", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(add, 0, "[sum]")
		st.checkItems("[sum]")
		st.checkInferredInputs("_1", "_2")
	})

	// SWAP1 on empty inferred stack: two items created below, then swapped.
	t.Run("swapOnEmpty", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(swap1, 0, "[_2, _1]")
		st.checkItems("[_2, _1]")
		st.checkInferredInputs("_1", "_2")
	})

	// Push then add: push creates one item, add needs two — one from the
	// push and one inferred.
	t.Run("pushThenAdd", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(push1, 0, "[x]")
		st.applyOK(add, 0, "[sum]")
		st.checkItems("[sum]")
		st.checkInferredInputs("_2")
	})

	// No inferred items needed when everything is pushed explicitly.
	t.Run("noInferredInputs", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(push1, 0, "[a]")
		st.applyOK(push1, 0, "[b, a]")
		st.applyOK(add, 0, "[sum]")
		st.checkItems("[sum]")
		st.checkInferredInputs()
	})

	// DUP1 on empty stack: one item created, duplicated.
	t.Run("dupOnEmpty", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(dup1, 0, "[_1, _1]")
		st.checkItems("[_1, _1]")
		st.checkInferredInputs("_1")
	})

	// SWAP2 on empty stack: three inferred items.
	t.Run("swap2OnEmpty", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(swap2, 0, "[_3, _2, _1]")
		st.checkItems("[_3, _2, _1]")
		st.checkInferredInputs("_1", "_2", "_3")
	})

	// Multiple operations inferring inputs incrementally.
	t.Run("incrementalGrowth", func(t *testing.T) {
		st := newInferredTest(t)
		// First POP needs one inferred item.
		st.applyOK(pop, 0, "[]")
		st.checkItems("[]")
		st.checkInferredInputs("_1")
		// Second POP needs another inferred item.
		st.applyOK(pop, 0, "[]")
		st.checkItems("[]")
		st.checkInferredInputs("_1", "_2")
	})

	// Stack comment checking still works in inferred mode.
	t.Run("commentCheckInInferred", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(push1, 0, "[a]")
		st.applyOK(push1, 0, "[b, a]")
		// After SWAP1, the stack is [a, b]. Claiming [a, a] should fail
		// because position 1 contains b, not a.
		st.applyErr(swap1, 0, "[a, a]",
			fmt.Errorf("%w: item %d differs (expected %q, have %q) in [a, b]", ErrMismatch, 1, "a", "b"),
		)
	})

	// Verify that inferred inputs track names assigned by comments.
	t.Run("inferredInputsWithCommentNames", func(t *testing.T) {
		st := newInferredTest(t)
		// SWAP1 needs two items. Comments name them.
		st.applyOK(swap1, 0, "[b, a]")
		st.checkItems("[b, a]")
		st.checkInferredInputs("a", "b")
	})

	// Pass-through: inferred items are not consumed, just reordered.
	t.Run("passThrough", func(t *testing.T) {
		st := newInferredTest(t)
		st.applyOK(push1, 0, "[x]")
		// Stack is [x], swap needs two: x and one from bottom.
		st.applyOK(swap1, 0, "[_2, x]")
		st.checkItems("[_2, x]")
		st.checkInferredInputs("_2")
	})
}

// Tests for wildcard stack comments.
func TestWildcard(t *testing.T) {
	vm := evm.FindInstructionSet("cancun")
	var (
		push1 = vm.PushBySize(1)
		push0 = vm.PushBySize(0)
		add   = vm.OpByName("ADD")
		pop   = vm.OpByName("POP")
		swap1 = vm.OpByName("SWAP1")
		swap2 = vm.OpByName("SWAP2")
		dup2  = vm.OpByName("DUP2")
	)

	// Init with wildcard at end.
	t.Run("initWithWildcard", func(t *testing.T) {
		st := newTest(t, "[a, b, ..]")
		if !st.s.HasWildcard() {
			t.Fatal("expected wildcard")
		}
		if st.s.Len() != 2 {
			t.Fatalf("expected len 2, got %d", st.s.Len())
		}
		want := "[a, b, ..]"
		if st.s.String() != want {
			t.Fatalf("got %s, want %s", st.s.String(), want)
		}
		st.checkItems("[a, b]")
	})

	// Bare wildcard: unknown stack of any depth.
	t.Run("bareWildcard", func(t *testing.T) {
		s := New([]string{Wildcard}, nil)
		if !s.HasWildcard() {
			t.Fatal("expected wildcard")
		}
		if s.Len() != 0 {
			t.Fatalf("expected len 0, got %d", s.Len())
		}
		if s.String() != "[..]" {
			t.Fatalf("got %s, want [..]", s.String())
		}
	})

	// Init without wildcard.
	t.Run("initWithoutWildcard", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		if st.s.HasWildcard() {
			t.Fatal("unexpected wildcard")
		}
	})

	// A comment with wildcard sets the flag on the stack.
	t.Run("commentSetsWildcard", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		if st.s.HasWildcard() {
			t.Fatal("unexpected wildcard before apply")
		}
		st.applyOK(push1, 0, "[x, a, ..]")
		if !st.s.HasWildcard() {
			t.Fatal("expected wildcard after apply")
		}
	})

	// A comment without wildcard clears the flag.
	t.Run("commentClearsWildcard", func(t *testing.T) {
		st := newTest(t, "[a, b, ..]")
		st.applyOK(push1, 0, "[x, a, b]")
		if st.s.HasWildcard() {
			t.Fatal("wildcard should be cleared by non-wildcard comment")
		}
	})

	// A comment with wildcard preserves the flag.
	t.Run("wildcardCommentPreserves", func(t *testing.T) {
		st := newTest(t, "[a, b, ..]")
		st.applyOK(push1, 0, "[x, a, ..]")
		if !st.s.HasWildcard() {
			t.Fatal("wildcard should persist with wildcard comment")
		}
	})

	// An operation without any comment does not change the wildcard flag.
	t.Run("noCommentPreservesWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		if err := st.s.Apply(pop, 0, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !st.s.HasWildcard() {
			t.Fatal("wildcard should persist when no comment is present")
		}
	})

	// Wildcard allows operations to access items below the tracked portion.
	t.Run("opAccessesBelowWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		// SWAP1 needs two items. The wildcard provides the second.
		st.applyOK(swap1, 0, "[_2, a, ..]")
		if st.s.Len() != 2 {
			t.Fatalf("expected len 2, got %d", st.s.Len())
		}
	})

	// SWAP2 on a wildcard stack with one tracked item.
	t.Run("swap2BelowWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		// SWAP2 needs 3 items. Two are provided by wildcard growth.
		st.applyOK(swap2, 0, "[_3, _2, a, ..]")
	})

	// DUP2 on a wildcard stack with one tracked item.
	t.Run("dup2BelowWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		// DUP2 needs 2 items. The second is from wildcard.
		st.applyOK(dup2, 0, "[_2, a, _2, ..]")
	})

	// Pop from a wildcard stack until tracked items are gone.
	t.Run("popThroughWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		st.applyOK(pop, 0, "[..]")
		if st.s.Len() != 0 {
			t.Fatalf("expected len 0, got %d", st.s.Len())
		}
		if !st.s.HasWildcard() {
			t.Fatal("wildcard should persist after pop")
		}
		// Pop again: wildcard provides the item.
		st.applyOK(pop, 0, "[..]")
	})

	// Comment names items below the wildcard boundary.
	t.Run("commentNamesWildcardItems", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		comment, _ := ParseComment("[a, b, c, ..]")
		if err := st.s.CheckComment(comment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		st.checkItems("[a, b, c]")
		if st.s.Len() != 3 {
			t.Fatalf("expected len 3, got %d", st.s.Len())
		}
	})

	// Wildcard comment on stack with sufficient items: named items are verified.
	t.Run("wildcardCommentChecksMismatch", func(t *testing.T) {
		st := newTest(t, "[a, b, ..]")
		st.applyErr(swap1, 0, "[a, a, ..]",
			fmt.Errorf("%w: item %d differs (expected %q, have %q) in [b, a, ..]", ErrMismatch, 0, "a", "b"),
		)
	})

	// CheckComment (standalone) also handles wildcards.
	t.Run("checkCommentWithWildcard", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		comment, _ := ParseComment("[a, ..]")
		if err := st.s.CheckComment(comment); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !st.s.HasWildcard() {
			t.Fatal("expected wildcard after CheckComment")
		}
	})

	// Wildcard with ADD that consumes a tracked item and a wildcard item.
	t.Run("addConsumesWildcardItem", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		st.applyOK(add, 0, "[sum, ..]")
		st.checkItems("[sum]")
		if !st.s.HasWildcard() {
			t.Fatal("wildcard should persist")
		}
	})

	// Push onto a wildcard stack and verify items are correct.
	t.Run("pushOnWildcard", func(t *testing.T) {
		st := newTest(t, "[a, ..]")
		st.applyOK(push0, 0, "[0, a, ..]")
		st.applyOK(push1, 0, "[x, 0, a, ..]")
		st.checkItems("[x, 0, a]")
		if !st.s.HasWildcard() {
			t.Fatal("wildcard should persist")
		}
	})

	// Init replaces wildcard state.
	t.Run("initClearsWildcard", func(t *testing.T) {
		s := New([]string{"a", Wildcard}, nil)
		if !s.HasWildcard() {
			t.Fatal("expected wildcard")
		}
		s.Init([]string{"x", "y"}, nil)
		if s.HasWildcard() {
			t.Fatal("wildcard should be cleared after Init without wildcard")
		}
	})

	// Init with wildcard sets wildcard state.
	t.Run("initSetsWildcard", func(t *testing.T) {
		s := New([]string{"a"}, nil)
		if s.HasWildcard() {
			t.Fatal("unexpected wildcard")
		}
		s.Init([]string{"x", Wildcard}, nil)
		if !s.HasWildcard() {
			t.Fatal("expected wildcard after Init with wildcard")
		}
	})
}
