package asm

import (
	"reflect"
	"slices"
	"testing"

	"github.com/fjl/geas/internal/ast"
)

func TestIterInstructions(t *testing.T) {
	var (
		doc     = make([]ast.Document, 4)
		instr   = make([]*instruction, 8)
		prog    = newCompilerProg(&doc[0])
		section = make([]*compilerSection, 4)
	)
	for i := range instr {
		instr[i] = new(instruction)
	}

	// create section structure
	{
		section[0] = prog.toplevel
		prog.addInstruction(instr[0])
		{
			section[1] = prog.pushSection(&doc[1], nil)
			prog.addInstruction(instr[1])
			prog.addInstruction(instr[2])
			prog.popSection()
		}
		prog.addInstruction(instr[3])
		{
			section[2] = prog.pushSection(&doc[2], nil)
			prog.addInstruction(instr[4])
			{
				section[3] = prog.pushSection(&doc[3], nil)
				prog.addInstruction(instr[5])
				prog.popSection()
			}
			prog.addInstruction(instr[6])
			prog.addInstruction(instr[7])
		}
		prog.popSection()
	}

	// iterate and gather list
	type item struct {
		*compilerSection
		*instruction
	}
	var result []item
	for section, inst := range prog.iterInstructions() {
		result = append(result, item{section, inst})
	}

	// compare
	expected := []item{
		{section[0], instr[0]},
		{section[1], instr[1]},
		{section[1], instr[2]},
		{section[0], instr[3]},
		{section[2], instr[4]},
		{section[3], instr[5]},
		{section[2], instr[6]},
		{section[2], instr[7]},
	}
	if !reflect.DeepEqual(result, expected) {
		t.Log("result:")
		for _, item := range result {
			t.Logf("  s%d (%p): instr%d (%p)", slices.Index(section, item.compilerSection), item.compilerSection, slices.Index(instr, item.instruction), item.instruction)
		}
		t.Error("result mismatch")
	}
}
