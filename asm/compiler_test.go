// Copyright 2023 The go-ethereum Authors
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

package asm

import (
	"bytes"
	"encoding/hex"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"
)

type compilerTestInput struct {
	Code  string            `yaml:"code"`
	Files map[string]string `yaml:"files,omitempty"`
}

type compilerTestOutput struct {
	Bytecode string   `yaml:"bytecode"`
	Errors   []string `yaml:"errors,omitempty"`
	Warnings []string `yaml:"warnings,omitempty"`
}

type compilerTestYAML struct {
	Input  compilerTestInput  `yaml:"input"`
	Output compilerTestOutput `yaml:"output"`
}

func TestCompiler(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "compiler-tests.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var tests = make(map[string]compilerTestYAML)
	dec := yaml.NewDecoder(bytes.NewReader(content))
	dec.KnownFields(true)
	if err := dec.Decode(&tests); err != nil {
		t.Fatal(err)
	}

	names := slices.Sorted(maps.Keys(tests))
	for _, name := range names {
		test := tests[name]
		t.Run(name, func(t *testing.T) {
			var fsys fs.FS
			if len(test.Input.Files) > 0 {
				fm := make(fstest.MapFS, len(test.Input.Files))
				for name, content := range test.Input.Files {
					fm[name] = &fstest.MapFile{Data: []byte(content)}
				}
				fsys = fm
			}
			c := NewCompiler(fsys)
			output := c.CompileString(test.Input.Code)

			if len(test.Output.Errors) > 0 {
				// expecting errors...
				if output != nil {
					t.Error("expected nil output")
				}
				checkErrors(t, "errors", c.Errors(), test.Output.Errors)
				checkErrors(t, "warnings", c.Warnings(), test.Output.Warnings)
				return
			}

			// Test expects no errors, compilation should succeed.
			if c.Failed() {
				for _, err := range c.Errors() {
					t.Error(err)
				}
				t.Fatal("compilation failed")
			}
			checkErrors(t, "warnings", c.Warnings(), test.Output.Warnings)
			expectedOutput, err := hex.DecodeString(strings.Replace(test.Output.Bytecode, " ", "", -1))
			if err != nil {
				t.Fatalf("invalid hex: %v", err)
			}
			if !bytes.Equal(output, expectedOutput) {
				t.Errorf("incorrect output\ngot:  %x\nwant: %x\n", output, expectedOutput)
			}
		})
	}
}

func checkErrors(t *testing.T, kind string, errlist []error, expected []string) {
	if len(errlist) != len(expected) {
		t.Errorf("got %d %s, expected %d", len(errlist), kind, len(expected))
		for i := range errlist {
			t.Errorf("  [%d] %v", i, errlist[i])
		}
		return
	}
	for i := range errlist {
		if errlist[i].Error() != expected[i] {
			t.Errorf("wrong error %d: %v\n    want: %s", i, errlist[i], expected[i])
		}
	}
}

func TestExamplePrograms(t *testing.T) {
	exampleDir, err := filepath.Abs("../example")
	if err != nil {
		t.Fatal(err)
	}

	bytecodes := make(map[string]string)
	t.Run("erc20", func(t *testing.T) {
		bytecodes["erc20"] = compileExample(t, exampleDir, "erc20/erc20.eas")
	})
	t.Run("erc20_ctor", func(t *testing.T) {
		bytecodes["erc20_ctor"] = compileExample(t, exampleDir, "erc20/erc20_ctor.eas")
	})
	t.Run("4788asm", func(t *testing.T) {
		bytecodes["4788asm"] = compileExample(t, exampleDir, "4788asm.eas")
	})
	t.Run("4788asm_ctor", func(t *testing.T) {
		bytecodes["4788asm_ctor"] = compileExample(t, exampleDir, "4788asm_ctor.eas")
	})

	if os.Getenv("WRITE_TEST_FILES") == "1" {
		content, _ := yaml.Marshal(bytecodes)
		os.WriteFile("testdata/known-bytecode.yaml", content, 0644)
	}

	// compare codes
	var known map[string]string
	data, err := os.ReadFile("testdata/known-bytecode.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &known); err != nil {
		t.Fatal("YAML unmarshal failed:", err)
	}
	for name, code := range bytecodes {
		if code != known[name] {
			t.Errorf("bytecode mismatch for %s:", name)
			t.Errorf("   compiled: %s", code)
			t.Errorf("      known: %s", known[name])
		}
	}
}

func compileExample(t *testing.T, exampleDir string, file string) string {
	c := NewCompiler(os.DirFS(exampleDir))
	output := c.CompileFile(file)
	for _, err := range c.ErrorsAndWarnings() {
		t.Log(err)
	}
	if c.Failed() {
		t.Error("compilation failed:")
	}
	return hex.EncodeToString(output)
}
