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
	"strconv"
	"strings"

	"github.com/fjl/geas/internal/set"
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
	name      string
	byName    map[string]*Op
	byCode    map[byte]*Op
	opRemoved map[string]string // forks where op was last removed
}

// FindInstructionSet resolves a fork name to a set of opcodes.
func FindInstructionSet(name string) *InstructionSet {
	name = strings.ToLower(name)
	def, ok := forkReg[name]
	if !ok {
		return nil
	}
	is := &InstructionSet{
		name:      def.Name(),
		byName:    make(map[string]*Op),
		byCode:    make(map[byte]*Op),
		opRemoved: make(map[string]string),
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
// Name has to be all uppercase.
func (is *InstructionSet) OpByName(opname string) *Op {
	return is.byName[opname]
}

// PushBySize resolves a push op by its size.
func (is *InstructionSet) PushBySize(size int) *Op {
	buf := []byte{'P', 'U', 'S', 'H', 0, 0}
	name := strconv.AppendInt(buf[:4], int64(size), 10)
	return is.byName[string(name)]
}

// OpByCode resolves an opcode by its code.
func (is *InstructionSet) OpByCode(code byte) *Op {
	return is.byCode[code]
}

// ForkWhereOpRemoved returns the fork where a given op was removed from the instruction
// set. This is intended to be called when op is known to not exist. Note this will return
// an empty string in several cases:
//
//   - op is invalid
//   - op is valid, but does not appear in lineage of instruction set
//   - op is valid and exists in instruction set
func (is *InstructionSet) ForkWhereOpRemoved(op string) string {
	return is.opRemoved[op]
}

// lineage computes the definition chain of an instruction set.
func (def *InstructionSetDef) lineage() ([]*InstructionSetDef, error) {
	var visited = make(set.Set[*InstructionSetDef])
	var lin []*InstructionSetDef
	for {
		if visited.Includes(def) {
			return nil, fmt.Errorf("instruction set parent cycle: %s <- %s", lin[len(lin)-1].Name(), def.Name())
		}
		visited.Add(def)
		lin = append(lin, def)

		if def.Parent == "" {
			break
		}
		parent, ok := forkReg[def.Parent]
		if !ok {
			return nil, fmt.Errorf("instruction set %s has unknown parent %s", def.Name(), def.Parent)
		}
		def = parent
	}
	slices.Reverse(lin)
	return lin, nil
}

// resolveDefs computes the full opcode set of a fork from its lineage.
func (is *InstructionSet) resolveDefs(toplevel *InstructionSetDef) error {
	lineage, err := toplevel.lineage()
	if err != nil {
		return err
	}

	for _, def := range lineage {
		for _, op := range def.Removed {
			if _, ok := is.byName[op.Name]; !ok {
				return fmt.Errorf("removed op %s does not exist in fork %s", op.Name, def.Name())
			}
			if _, ok := is.byCode[op.Code]; !ok {
				return fmt.Errorf("removed opcode %d (%s) does not exist in fork %s", op.Code, op.Name, def.Name())
			}
			delete(is.byName, op.Name)
			delete(is.byCode, op.Code)
			is.opRemoved[op.Name] = def.Name()
		}
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
			is.byCode[op.Code] = op
			delete(is.opRemoved, op.Name)
		}
	}
	return nil
}

// opAddedInForkMap contains all ops and the forks they were added in.
var opAddedInForkMap = computeOpAddedInFork()

func computeOpAddedInFork() map[string][]string {
	m := make(map[string][]string)
	for _, def := range forkReg {
		for _, op := range def.Added {
			m[op.Name] = append(m[op.Name], def.Name())
		}
	}
	return m
}

// ForksWhereOpAdded returns the fork names where a given op is added.
// If this returns nil, op is invalid.
func ForksWhereOpAdded(op string) []string {
	return opAddedInForkMap[op]
}
