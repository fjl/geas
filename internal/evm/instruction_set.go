// Copyright 2014 The go-ethereum Authors
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

package evm

import (
	"fmt"
	"slices"
	"strings"
)

// InstructionSetDef is the definition of an EVM instruction set.
type InstructionSetDef struct {
	Names   []string // all names of this instruction set
	Parent  string
	Added   []*Op
	Removed []*Op
}

// Name returns the canonical name.
func (def *InstructionSetDef) Name() string {
	return def.Names[0]
}

// InstructionSet is an EVM instruction set.
type InstructionSet struct {
	name   string
	byName map[string]*Op
	byCode map[byte]*Op
}

func FindInstructionSet(name string) *InstructionSet {
	name = strings.ToLower(name)
	def, ok := ireg[name]
	if !ok {
		return nil
	}
	is := &InstructionSet{
		name:   def.Name(),
		byName: make(map[string]*Op),
		byCode: make(map[byte]*Op),
	}
	if err := is.resolveDefs(def); err != nil {
		panic(err)
	}
	return is
}

// Name returns the canonical instruction set name.
func (is *InstructionSet) Name() string {
	return is.name
}

// SupportsPush0 reports whether the instruction set includes the PUSH0 instruction.
func (is *InstructionSet) SupportsPush0() bool {
	return is.byName["PUSH0"] != nil
}

// OpByName resolves an opcode by its name.
func (is *InstructionSet) OpByName(name string) *Op {
	return is.byName[name]
}

// OpByCode resolves an opcode by its code.
func (is *InstructionSet) OpByCode(code byte) *Op {
	return is.byCode[code]
}

// lineage computes the definition chain of an instruction set.
func (def *InstructionSetDef) lineage() ([]*InstructionSetDef, error) {
	var visited = make(set[*InstructionSetDef])
	var lin []*InstructionSetDef
	for {
		if visited.includes(def) {
			return nil, fmt.Errorf("instruction set parent cycle: %s <- %s", lin[len(lin)-1].Name(), def.Name())
		}
		visited.add(def)
		lin = append(lin, def)

		if def.Parent == "" {
			break
		}
		parent, ok := ireg[def.Parent]
		if !ok {
			return nil, fmt.Errorf("instruction set %s has unknown parent %s", def.Name(), def.Parent)
		}
		def = parent
	}
	slices.Reverse(lin)
	return lin, nil
}

func (is *InstructionSet) resolveDefs(toplevel *InstructionSetDef) error {
	lineage, err := toplevel.lineage()
	if err != nil {
		return err
	}

	for _, def := range lineage {
		for _, op := range def.Added {
			_, nameDefined := is.byName[op.Name]
			if nameDefined {
				return fmt.Errorf("instruction %s added multiple times", op.Name)
			}
			is.byName[op.Name] = op
			_, codeDefined := is.byCode[op.Code]
			if codeDefined {
				return fmt.Errorf("opcode %v added multiple times (adding %s, existing def %s)", op.Code, op.Name, is.byCode[op.Code].Name)
			}
		}
		for _, op := range def.Removed {
			// TODO: check
			delete(is.byName, op.Name)
			delete(is.byCode, op.Code)
		}
	}
	return nil
}

type set[X comparable] map[X]struct{}

func (s set[X]) add(k X) {
	s[k] = struct{}{}
}

func (s set[X]) includes(k X) bool {
	_, ok := s[k]
	return ok
}