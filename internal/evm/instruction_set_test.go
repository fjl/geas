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
