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
	s := New()
	s.Init(commentSlice)
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
	if !reflect.DeepEqual(err, wantErr) {
		t.t.Fatalf("wrong error: %v\n         want: %v", err, wantErr)
	}
}

func TestStackAnalysis(t *testing.T) {
	vm := evm.FindInstructionSet("frontier")
	var (
		push1 = vm.PushBySize(1)
		add   = vm.OpByName("ADD")
		swap2 = vm.OpByName("SWAP2")
		dup1  = vm.OpByName("DUP1")
		dup2  = vm.OpByName("DUP2")
	)
	t.Run("ok", func(t *testing.T) {
		st := newTest(t, "[a, b, c, d]")
		st.applyOK(dup2, 0, "[b, a, b, c]")
		st.applyOK(add, 0, "[sum, b, c]")
		st.applyOK(swap2, 0, "[c, b, sum]")
		st.applyOK(dup1, 0, "[c, c, b, sum]")
		st.applyOK(push1, 0, "[val, c, c, b, sum]")
		st.applyOK(swap2, 0, "[c, c, val, b, sum]")
	})
	t.Run("initWithDuplicates", func(t *testing.T) {
		st := newTest(t, "[a, a, a]")
		st.applyOK(add, 0, "[sum, a]")
	})
	t.Run("commentMismatch", func(t *testing.T) {
		st := newTest(t, "[a, b, c, d]")
		st.applyErr(add, 0, "[sum, d, c]",
			ErrMismatch{
				Items: []string{"sum", "c", "d"},
				Slot:  1,
				Want:  "d",
			},
		)
	})
	t.Run("opUnderflows", func(t *testing.T) {
		st := newTest(t, "[a]")
		st.applyErr(add, 0, "[sum]",
			ErrOpUnderflows{
				Want: 2,
				Have: 1,
			},
		)
	})
	t.Run("commentUnderflows", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		st.applyErr(add, 0, "[sum, b]",
			ErrCommentUnderflows{
				Items: []string{"sum"},
				Want:  2,
			},
		)
	})
	t.Run("stackItemRenamed", func(t *testing.T) {
		st := newTest(t, "[a, b]")
		st.applyOK(push1, 0, "[x, a, b]")
		st.applyErr(add, 0, "[sum, c]",
			ErrCommentRenamesItem{
				Item:    "b",
				NewName: "c",
			},
		)
	})

}
