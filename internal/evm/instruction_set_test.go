package evm

import (
	"maps"
	"slices"
	"testing"
)

func TestForkDefs(t *testing.T) {
	names := slices.Sorted(maps.Keys(ireg))

	// Check canon name is listed first in def.Names.
	for _, name := range names {
		def := ireg[name]
		if len(def.Names) == 0 {
			t.Fatalf("instruction set %q has no Names", name)
		}
		if def.Names[0] != name {
			t.Fatalf("canon name of instruction set %q not listed first in def.Names", name)
		}
	}

	// Check lineage works.
	for _, name := range names {
		def := ireg[name]
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
}
