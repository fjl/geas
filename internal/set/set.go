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

package set

import (
	"maps"
	"slices"
)

// Set is a wrapper over map.
// I don't want to depend on a set library just for this.
type Set[X comparable] map[X]struct{}

func (s Set[X]) Add(k X) {
	s[k] = struct{}{}
}

func (s Set[X]) Includes(k X) bool {
	_, ok := s[k]
	return ok
}

func (s Set[X]) Members() []X {
	return slices.Collect(maps.Keys(s))
}

// Diff returns the elements of a which are not in b.
func Diff[X comparable](a, b []X) []X {
	set := make(Set[X], len(b))
	for _, x := range b {
		set.Add(x)
	}
	var diff []X
	for _, x := range a {
		if !set.Includes(x) {
			diff = append(diff, x)
		}
	}
	return diff
}
