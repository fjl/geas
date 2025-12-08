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
	StackIn() []string  // input items
	StackOut() []string // output items
}

// Stack is a symbolic EVM stack. It tracks the positions
// of items and their symbolic names.
type Stack struct {
	counter int // item counter
	stack   []int

	// item naming
	nameToItem map[string]int
	itemToName map[int]string

	// buffer for apply
	opItems map[string]int
}

func New() *Stack {
	return &Stack{
		nameToItem: make(map[string]int),
		itemToName: make(map[int]string),
		opItems:    make(map[string]int),
	}
}

// Init clears the stack and sets its contents.
func (s *Stack) Init(names []string) {
	clear(s.nameToItem)
	clear(s.itemToName)
	s.stack = make([]int, 0, len(names))
	for _, name := range slices.Backward(names) {
		if item, ok := s.nameToItem[name]; ok {
			s.push(item)
		} else {
			item = s.newItem()
			s.push(item)
			s.setName(item, name)
		}
	}
}

// Apply performs a stack manipulation.
// The comment is checked for correctness if non-nil.
func (s *Stack) Apply(op Op, comment []string) error {
	// Drop consumed items, but remember them by name.
	clear(s.opItems)
	inputs := op.StackIn()
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
	outputs := op.StackOut()
	var newOutputs set.Set[int]
	for i := len(outputs) - 1; i >= 0; i-- {
		if item, ok := s.opItems[outputs[i]]; ok {
			s.push(item)
		} else {
			item := s.newItem()
			s.push(item)
			if newOutputs == nil {
				newOutputs = make(set.Set[int], 1)
			}
			newOutputs.Add(item)
		}
	}

	// Check the comment, and apply its names to the stack.
	if comment == nil {
		return nil
	}
	for i, name := range comment {
		stackItem, ok := s.get(i)
		if !ok {
			return ErrCommentUnderflows{Items: s.Items(), Want: len(comment)}
		}
		if item, ok := s.nameToItem[name]; ok && item != stackItem {
			return ErrMismatch{Items: s.Items(), Slot: i, Want: name}
		}
		// The comment is not supposed to rename items that weren't produced by
		// this operation.
		if !newOutputs.Includes(stackItem) && s.nameToItem[name] == 0 {
			return ErrCommentRenamesItem{NewName: name, Item: s.itemToName[stackItem]}
		}
		// Rename the item according to the comment.
		s.setName(stackItem, name)
	}
	// By now the comment is known not to have more items than the stack, and all declared
	// names match the stack. Notably, there is no expectation that comments are complete,
	// i.e. it's OK if comments elide some items at the end.
	// Unfortunately, this also permits a sitation where items can be 'added back' if they
	// were dropped from the comment before.
	// Consider this example:
	//
	//     push 1    ; [a]
	//     push 2    ; [b, a]
	//     push 3    ; [c, b]    <-- a is lost here...
	//     add       ; [sum, a]  <-- but now it's back! confusing!
	//
	// I'm not sure if this should be prevented somehow.

	return nil
}

// Items returns a list of current stack items.
func (s *Stack) Items() []string {
	items := make([]string, len(s.stack))
	for i := range items {
		item, _ := s.get(i)
		items[i] = s.getName(item)
	}
	return items
}

// String returns a description of the current stack.
func (s *Stack) String() string {
	return render(s.Items())
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

// get accesses item i (zero is top).
func (s *Stack) get(i int) (val int, ok bool) {
	if i < 0 {
		panic("BUG: negative stack offset")
	}
	if i > len(s.stack)-1 {
		return 0, false
	}
	return s.stack[len(s.stack)-1-i], true
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
