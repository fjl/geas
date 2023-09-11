// Copyright 2019 The go-ethereum Authors
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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

type compilerTestInput struct {
	Code  string            `yaml:"code"`
	Files map[string]string `yaml:"files,omitempty"`
}

type compilerTestOutput struct {
	Bytecode string   `yaml:"bytecode"`
	Errors   []string `yaml:"errors,omitempty"`
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

	names := maps.Keys(tests)
	sort.Strings(names)
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
				errlist := c.Errors()
				t.Log("errors:", errlist)
				if len(errlist) != len(test.Output.Errors) {
					t.Errorf("got %d errors, expected %d", len(errlist), len(test.Output.Errors))
					for i := range errlist {
						t.Errorf("error %d: %v", i, errlist[i])
					}
					return
				}
				for i := range errlist {
					if errlist[i].Error() != test.Output.Errors[i] {
						t.Errorf("wrong error %d: %v\n    want: %s", i, errlist[i], test.Output.Errors[i])
					}
				}
				return
			}

			// Test expects no errors, compilation should succeed.
			if len(c.Errors()) > 0 {
				for _, err := range c.Errors() {
					t.Error(err)
				}
				t.Fatal("compilation failed")
			}
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
