// Copyright 2025 The go-ethereum Authors
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

package printer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fjl/geas/internal/ast"
)

var exprTests = []struct {
	in, out string
}{
	{"1", "1"},                             // decimal literal
	{"0x03", "0x03"},                       // hex number literal
	{`"abc"`, `"abc"`},                     // string literal
	{`"newline\n"`, `"newline\n"`},         // string literal with escape
	{"-2", "-2"},                           // unary op
	{"-(1 + 3)", "-(1 + 3)"},               // binary in unary
	{"-1 + 3", "-1 + 3"},                   // unary in binary
	{"1 + 2*3", "1 + 2*3"},                 // binary precedence
	{"2*3 + 1", "2*3 + 1"},                 // binary precedence (other way)
	{"1 + 2 + 3", "1 + 2 + 3"},             // binary chain
	{"(2 * 3) + 1", "(2 * 3) + 1"},         // parens not stripped
	{"1 - (2 * 3) + 1", "1 - (2 * 3) + 1"}, // parens not stripped
	{"ab(1, 2)", "ab(1, 2)"},               // macro call
	{".builtin(1, 2)", ".builtin(1, 2)"},   // builtin macro call
	{"noarg()", "noarg"},                   // macro call w/o args
	{".noarg()", ".noarg"},                 // builtin macro call w/o args
}

func TestPrintExpr(t *testing.T) {
	for _, test := range exprTests {
		parser := ast.NewParser("", []byte(test.in))
		e, err := parser.ParseExpression()
		if err != nil {
			t.Fatalf("%s: parse error: %v", test.in, err)
		}
		var (
			s strings.Builder
			p Printer
		)
		if err := p.Expr(&s, e); err != nil {
			t.Fatalf("%s: write error: %v", test.in, err)
		}
		if s.String() != test.out {
			t.Fatalf("%s: wrong output:\n  %s\nwant:\n  %s", test.in, s.String(), test.out)
		}
	}
}

func TestPrintDocument(t *testing.T) {
	inputFiles, err := filepath.Glob(filepath.Join("testdata", "in.*.eas"))
	if err != nil {
		t.Fatal(err)
	}

	for _, inFile := range inputFiles {
		name := filepath.Base(inFile)
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(inFile)
			if err != nil {
				t.Fatal(err)
			}

			parser := ast.NewParser(inFile, input)
			doc, errs := parser.Parse()
			if len(errs) > 0 {
				t.Error("parse errors:")
				for _, err := range errs {
					t.Error(err)
				}
				return
			}

			var printer Printer
			var buf bytes.Buffer
			if err := printer.Document(&buf, doc); err != nil {
				t.Fatal(err)
			}

			outFile := strings.Replace(inFile, "in.", "out.", 1)
			if os.Getenv("WRITE_TEST_FILES") == "1" {
				os.WriteFile(outFile, buf.Bytes(), 0644)
			}
			expectedOutput, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf.Bytes(), expectedOutput) {
				t.Errorf("output mismatch:\n%s", buf.Bytes())
			}
		})
	}
}
