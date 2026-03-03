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

import "fmt"

// stackEffect represents the stack effect of a code sequence (macro body, included document).
type stackEffect struct {
	in, out []string
}

func (e *stackEffect) StackIn(imm byte) []string  { return e.in }
func (e *stackEffect) StackOut(imm byte) []string { return e.out }

// effectFromStack computes the effect of an inferred stack run.
// Both inferredInputs and items are in top-first order.
func effectFromStack(inferredInputs, items []string) *stackEffect {
	in := make([]string, len(inferredInputs))
	copy(in, inferredInputs)
	// Ensure uniqueness: the stack.Op interface requires input names to be unique
	// because Apply uses them as map keys. Deduplicate by appending a suffix.
	dedup(in)

	out := make([]string, len(items))
	copy(out, items)
	return &stackEffect{in: in, out: out}
}

// dedup ensures all names in the slice are unique by appending '#N' suffixes.
func dedup(names []string) {
	seen := make(map[string]int, len(names))
	for i, name := range names {
		if n := seen[name]; n > 0 {
			names[i] = fmt.Sprintf("%s#%d", name, n)
		}
		seen[name]++
	}
}
