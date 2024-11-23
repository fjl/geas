package evm

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestOps(t *testing.T) {
	// Check op all names are uppercase.
	for _, op := range oplist {
		if op.Name != strings.ToUpper(op.Name) {
			t.Fatalf("op %s name is not all-uppercase", op.Name)
		}
	}

	// Check all ops are used in a fork.
	// First compute set of used op names.
	defnames := slices.Sorted(maps.Keys(forkReg))
	used := make(set[string], len(oplist))
	for _, name := range defnames {
		for _, op := range forkReg[name].Added {
			used.add(op.Name)
		}
	}
	usedopnames := used.members()
	slices.Sort(usedopnames)
	// Now compute sorted list of all ops.
	allopnames := make([]string, len(oplist))
	for i, op := range oplist {
		allopnames[i] = op.Name
	}
	slices.Sort(allopnames)
	// Compare.
	d := diff(allopnames, usedopnames)
	if len(d) > 0 {
		t.Error("unused ops:", d)
	}
	if len(usedopnames) > len(allopnames) {
		t.Error("forkdefs uses ops which are not in oplist")
	}
}

func TestForkDefs(t *testing.T) {
	defnames := slices.Sorted(maps.Keys(forkReg))

	// Check canon name is listed first in def.Names.
	for _, name := range defnames {
		def := forkReg[name]
		if len(def.Names) == 0 {
			t.Fatalf("instruction set %q has no Names", name)
		}
		if def.Names[0] != name {
			t.Fatalf("canon name of instruction set %q not listed first in def.Names", name)
		}
	}

	// Check lineage works.
	for _, name := range defnames {
		def := forkReg[name]
		_, err := def.lineage()
		if err != nil {
			t.Errorf("problem in lineage() of %q: %v", name, err)
		}
	}
}

// In this test, we just check for a few known ops.
func TestForkOps(t *testing.T) {
	is := FindInstructionSet("cancun")

	{
		op := is.OpByName("ADD")
		if op.Name != "ADD" {
			t.Fatal("wrong op name:", op.Name)
		}
		if op.Code != 0x01 {
			t.Fatal("wrong op code:", op.Code)
		}
		if op2 := is.OpByCode(0x01); op2 != op {
			t.Fatal("reverse lookup returned incorrect op", op2)
		}
	}
	{
		op := is.OpByName("SHR")
		if op.Name != "SHR" {
			t.Fatal("wrong op name:", op.Name)
		}
		if op.Code != 0x1c {
			t.Fatal("wrong op code:", op.Code)
		}
		if op2 := is.OpByCode(0x1c); op2 != op {
			t.Fatal("reverse lookup returned incorrect op", op2)
		}
	}
	{
		op := is.OpByName("RANDOM")
		if op.Name != "RANDOM" {
			t.Fatal("wrong op name:", op.Name)
		}
		if op.Code != 0x44 {
			t.Fatal("wrong op code:", op.Code)
		}
		if op2 := is.OpByCode(0x44); op2 != op {
			t.Fatal("reverse lookup returned incorrect op", op2)
		}
	}
	{
		op := is.OpByName("DIFFICULTY")
		if op != nil {
			t.Fatal("DIFFICULTY op found even though it was removed")
		}
		rf := is.ForkWhereOpRemoved("DIFFICULTY")
		if rf != "paris" {
			t.Fatalf("ForkWhereOpRemoved(DIFFICULTY) -> %s != %s", rf, "paris")
		}
	}
}

func TestForksWhereOpAdded(t *testing.T) {
	f := ForksWhereOpAdded("BASEFEE")
	if !slices.Equal(f, []string{"london"}) {
		t.Fatalf("wrong list for BASEFEE: %v", f)
	}
}

// diff returns the elements of a which are not in b.
func diff[X comparable](a, b []X) []X {
	set := make(set[X], len(b))
	for _, x := range b {
		set.add(x)
	}
	var diff []X
	for _, x := range a {
		if !set.includes(x) {
			diff = append(diff, x)
		}
	}
	return diff
}
