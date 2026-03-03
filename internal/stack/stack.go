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
	"slices"
	"strings"

	"github.com/fjl/geas/internal/set"
)

// Op is an operation that modifies the stack.
type Op interface {
	StackIn(imm byte) []string  // input items
	StackOut(imm byte) []string // output items
}

// Stack is a symbolic EVM stack. It tracks the positions
// of items and their symbolic names.
type Stack struct {
	counter  int // item counter
	stack    []int
	wildcard bool // when set, the stack may have additional unknown items at the bottom

	// inferred mode: when set, get() auto-extends the stack downward instead of
	// returning !ok for out-of-bounds accesses. Items created this way are tracked
	// as the inferred inputs of the code sequence being analyzed.
	inferred      bool
	inferredItems []int // items created by downward extension, in order of creation

	// item naming
	nameToItem     map[string]int
	itemToName     map[int]string
	confirmedNames set.Set[int] // items whose names were set by user comments

	// buffers for apply
	opItems    map[string]int
	opNewItems set.Set[int]
}

// New creates a stack initialized with the given item names.
// See [Stack.Init] for the meaning of the confirmed parameter.
func New(items []string, confirmed []bool) *Stack {
	s := &Stack{
		nameToItem:     make(map[string]int),
		itemToName:     make(map[int]string),
		confirmedNames: make(set.Set[int]),
		opItems:        make(map[string]int),
		opNewItems:     make(set.Set[int]),
	}
	if len(items) > 0 {
		s.Init(items, confirmed)
	}
	return s
}

// SetInferred puts the stack into inferred-input mode. In this mode, operations that
// need more items than are on the stack will automatically extend the stack downward
// with new items. This is used for analyzing code sequences where the initial stack is
// unknown (e.g. included files, macros without start comments). The inferred inputs can
// be retrieved using [Stack.InferredInputs].
func (s *Stack) SetInferred() {
	s.inferred = true
}

// HasWildcard reports whether the stack has a wildcard, indicating that
// additional unknown items may exist below the tracked items.
func (s *Stack) HasWildcard() bool {
	return s.wildcard
}

// Init clears the stack and sets its contents. If confirmed is nil, all names are
// marked as confirmed, and duplicate names share a single item (representing the same
// value appearing multiple times). If confirmed is non-nil, only names at positions
// where confirmed[i] is true are marked as confirmed, and every position gets a
// distinct item regardless of name. The distinct-item behavior is needed for merge
// points where the same name at different positions doesn't imply the same value.
func (s *Stack) Init(names []string, confirmed []bool) {
	clear(s.nameToItem)
	clear(s.itemToName)
	clear(s.confirmedNames)
	names, s.wildcard = StripWildcard(names)
	s.stack = make([]int, 0, len(names))
	for i, name := range slices.Backward(names) {
		if confirmed == nil {
			// Full reset: duplicate names share items.
			if item, ok := s.nameToItem[name]; ok {
				s.push(item)
				continue
			}
		}
		item := s.newItem()
		s.push(item)
		s.setName(item, name)
		if confirmed == nil || confirmed[i] {
			s.confirmedNames.Add(item)
		}
	}
}

// Apply performs a stack manipulation.
// The comment is checked for correctness if non-nil.
func (s *Stack) Apply(op Op, imm byte, comment []string) error {
	// If the comment has a wildcard, set it before processing the operation
	// so that bottom-growth is enabled for deep stack accesses.
	if HasWildcard(comment) {
		s.wildcard = true
	}

	// Drop consumed items, but remember them by name.
	clear(s.opItems)
	inputs := op.StackIn(imm)
	for i, name := range inputs {
		if _, ok := s.opItems[name]; ok {
			panic("BUG: op has duplicate input stack item " + name)
		}
		val, ok := s.get(i)
		if !ok {
			return ErrOpUnderflows{Want: len(inputs), Have: len(s.stack)}
		}
		s.opItems[name] = val
	}
	s.stack = s.stack[:len(s.stack)-len(inputs)]

	// Add output items. If any names from the operation's input list are reused, their
	// item identifiers will be restored. For all other names, new items are created.
	outputs := op.StackOut(imm)
	clear(s.opNewItems)
	for i := len(outputs) - 1; i >= 0; i-- {
		if item, ok := s.opItems[outputs[i]]; ok {
			s.push(item)
		} else {
			item := s.newItem()
			s.push(item)
			s.opNewItems.Add(item)
		}
	}

	// Check the comment, and apply its names to the stack.
	return s.checkComment(comment)
}

// CheckComment verifies a standalone stack comment against the current stack state.
// This is used for label comments at merge points. Unlike [Apply], no stack
// transformation is performed — only the comment's names are checked and applied.
// Unconfirmed names can be freely overwritten. All names touched by the comment
// become confirmed.
func (s *Stack) CheckComment(comment []string) error {
	clear(s.opNewItems)
	return s.checkComment(comment)
}

// checkComment is the shared comment-checking logic used by both [Apply] and
// [CheckComment]. It assumes opNewItems has been set up by the caller.
func (s *Stack) checkComment(comment []string) error {
	if comment == nil {
		return nil
	}

	// Strip wildcard from comment and update the wildcard flag.
	// A comment with wildcard sets the flag; a comment without clears it.
	// Operations without comments (nil) don't change the flag.
	var wild bool
	comment, wild = StripWildcard(comment)
	s.wildcard = wild

	// Comments never trigger inferred-input growth — only operation inputs do.
	// In inferred mode, comments that extend beyond the current stack are
	// silently truncated since the extra items can't be verified.
	var namingError error
	for i, name := range comment {
		stackItem, ok := s.peek(i)
		if !ok {
			if s.inferred {
				break // can't verify deeper items
			}
			return ErrCommentUnderflows{Items: s.Items(), Want: len(comment)}
		}
		// Name is taken by a different item. Allow this if:
		// - The current item is NEW (new items can reuse names for duplicate values
		//   like multiple "push 0" with comment "[0, 0]"), OR
		// - The current item already has this name (multiple items sharing a name), OR
		// - The conflicting item is no longer on the stack (stale mapping)
		if item, ok := s.nameToItem[name]; ok && item != stackItem {
			if !s.opNewItems.Includes(stackItem) && s.itemToName[stackItem] != name && slices.Contains(s.stack, item) {
				if namingError == nil {
					namingError = ErrMismatch{Items: s.Items(), Slot: i, Want: name}
				}
			}
		}
		// The comment is not supposed to rename items that weren't produced by
		// this operation. Items that have never been explicitly named (e.g. items
		// created by inferred-input growth) may be named freely. Items with
		// unconfirmed names (inherited from a merge point) may also be renamed.
		if !s.opNewItems.Includes(stackItem) && s.nameToItem[name] == 0 {
			if s.confirmedNames.Includes(stackItem) {
				if namingError == nil {
					namingError = ErrCommentRenamesItem{NewName: name, Item: s.itemToName[stackItem]}
				}
			}
		}
		// Rename the item according to the comment and mark it as confirmed.
		// Note this is done even if a conflict is detected above.
		// That way, naming errors don't cascade and will be reported only once.
		s.setName(stackItem, name)
		s.confirmedNames.Add(stackItem)
	}

	// By now the comment is known not to have more items than the stack, and all declared
	// names match the stack. Notably, there is no expectation that comments are complete,
	// i.e. it's OK if comments elide some items at the end.
	// Unfortunately, this also permits a situation where items can be 'added back' if they
	// were dropped from the comment before.
	// Consider this example:
	//
	//     push 1    ; [a]
	//     push 2    ; [b, a]
	//     push 3    ; [c, b]    <-- a is lost here...
	//     add       ; [sum, a]  <-- but now it's back! confusing!
	//
	// I'm not sure if this should be prevented somehow.

	return namingError
}

// Items returns a list of current stack items.
func (s *Stack) Items() []string {
	items := make([]string, len(s.stack))
	for i := range items {
		item, _ := s.peek(i)
		items[i] = s.getName(item)
	}
	return items
}

// Len returns the number of items on the stack.
func (s *Stack) Len() int {
	return len(s.stack)
}

// InferredInputs returns the names of items that were created by extending the stack
// downward in inferred-input mode. The result is in top-first order: the item closest
// to the top of the initial stack comes first.
func (s *Stack) InferredInputs() []string {
	if !s.inferred {
		panic("BUG: InferredInputs used but stack is not in inferred mode")
	}
	items := make([]string, len(s.inferredItems))
	for i, item := range s.inferredItems {
		items[i] = s.getName(item)
	}
	return items
}

// String returns a description of the current stack.
func (s *Stack) String() string {
	items := s.Items()
	if s.wildcard {
		items = append(items, Wildcard)
	}
	return render(items)
}

func render(stk []string) string {
	var out strings.Builder
	out.WriteByte('[')
	for i, name := range stk {
		if i > 0 {
			out.WriteString(", ")
		}
		out.WriteString(name)
	}
	out.WriteByte(']')
	return out.String()
}

// push adds an item at the top of the stack.
func (s *Stack) push(item int) {
	s.stack = append(s.stack, item)
}

// peek accesses item i (zero is top). In wildcard mode, accessing beyond the
// stack's current depth creates new items at the bottom, allowing comments to
// name items below the tracked portion.
func (s *Stack) peek(i int) (val int, ok bool) {
	if i < 0 {
		panic("BUG: negative stack offset")
	}
	if i > len(s.stack)-1 {
		if s.wildcard {
			s.growBottom(i - (len(s.stack) - 1))
		} else {
			return 0, false
		}
	}
	return s.stack[len(s.stack)-1-i], true
}

// get accesses item i (zero is top). In inferred or wildcard mode, accessing
// beyond the stack's current depth creates new items at the bottom.
func (s *Stack) get(i int) (val int, ok bool) {
	if i < 0 {
		panic("BUG: negative stack offset")
	}
	if i > len(s.stack)-1 {
		if !s.inferred && !s.wildcard {
			return 0, false
		}
		s.growBottom(i - (len(s.stack) - 1))
	}
	return s.stack[len(s.stack)-1-i], true
}

// growBottom extends the stack downward by n items. In inferred mode,
// the new items are also tracked as inferred inputs.
func (s *Stack) growBottom(n int) {
	items := make([]int, n)
	for i := range n {
		item := s.newItem()
		items[i] = item
		if s.inferred {
			s.inferredItems = append(s.inferredItems, item)
		}
	}
	s.stack = append(items, s.stack...)
}

// newItem creates a new item (but does not add it to the stack).
func (s *Stack) newItem() int {
	s.counter++
	return s.counter
}

// setName sets the name of a stack item.
func (s *Stack) setName(item int, name string) {
	s.itemToName[item] = name
	s.nameToItem[name] = item
}

// getName reports the known name of an item, or invents one.
func (s *Stack) getName(item int) string {
	name, ok := s.itemToName[item]
	if ok {
		return name
	}
	return fmt.Sprintf("_%d", item)
}
