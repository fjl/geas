package evm

import (
	"sort"
	"testing"

	"golang.org/x/exp/maps"
)

func TestForkDefs(t *testing.T) {
	names := maps.Keys(ireg)
	sort.Strings(names)

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
