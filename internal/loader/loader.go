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

// Package loader implements loading of source files and tracking of includes/definitions.
package loader

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/evm"
)

type Loader struct {
	fsys        fs.FS
	maxIncDepth int
	defaultFork string
	errors      *ErrorList
}

func New(fsys fs.FS) *Loader {
	return &Loader{
		fsys:        fsys,
		errors:      NewErrorList(10),
		maxIncDepth: 128,
		defaultFork: evm.LatestFork,
	}
}

func (l *Loader) SetMaxIncludeDepth(d int) {
	l.maxIncDepth = d
}

func (l *Loader) MaxIncludeDepth() int {
	return l.maxIncDepth
}

// SetFilesystem sets the file system used for resolving #include files.
// Note: if set to a nil FS, #include is not allowed.
func (l *Loader) SetFilesystem(fsys fs.FS) {
	l.fsys = fsys
}

func (l *Loader) Filesystem() fs.FS {
	return l.fsys
}

// SetDefaultFork sets the EVM instruction set used by default.
func (l *Loader) SetDefaultFork(f string) {
	l.defaultFork = f
}

// SetIncludeDepthLimit enables/disables printing of the token stream to stdout.
func (l *Loader) SetIncludeDepthLimit(limit int) {
	l.maxIncDepth = limit
}

func (l *Loader) LoadFile(filename string) *Program {
	content, err := fs.ReadFile(l.fsys, filename)
	if err != nil {
		l.errors.Add(err)
		return nil
	}
	return l.LoadSource(filename, content)
}

func (l *Loader) LoadSource(filename string, src []byte) *Program {
	p := ast.NewParser(filename, src)
	doc, pErr := p.Parse()
	if len(pErr) > 0 {
		l.errors.addParseErrors(pErr)
		return nil
	}
	return l.loadDocument(doc, nil)
}

func (l *Loader) Errors() *ErrorList {
	return l.errors
}

func (l *Loader) loadDocument(doc *ast.Document, incStack []ast.Statement) *Program {
	p := newProgram(doc)
	p.Fork = evm.FindInstructionSet(l.defaultFork)
	if p.Fork == nil {
		l.errors.Add(fmt.Errorf("unknown default fork %q", l.defaultFork))
		return p
	}

	var list []*ast.Include
	var macros []*ast.InstructionMacroDef
	for _, st := range doc.Statements {
		switch st := st.(type) {
		case *ast.Pragma:
			l.processPragma(p, st, len(incStack))

		case *ast.InstructionMacroDef:
			p.registerInstrMacro(st)
			macros = append(macros, st)

		case *ast.ExpressionMacroDef:
			p.registerExprMacro(st)

		case *ast.LabelDef:
			p.registerLabel(st)

		case *ast.Include:
			file, err := ResolveRelative(doc.File, st.Filename)
			if err != nil {
				l.errors.AddAt(st, err)
				continue
			}
			incdoc := l.parseIncludeFile(file, st, len(incStack)+1)
			if incdoc != nil {
				p.includes[st] = incdoc
				list = append(list, st)
			}

		}
	}

	for _, m := range macros {
		l.loadDocument(m.Body, append(incStack, m))
	}
	for _, inc := range list {
		l.loadDocument(p.includes[inc], append(incStack, inc))
	}
	return p
}

func (l *Loader) processPragma(p *Program, st *ast.Pragma, depth int) {
	switch st.Option {
	case "target":
		if depth > 0 {
			l.errors.AddAt(st, errPragmaTargetInIncludeFile)
		}
		if p.forkDefined {
			l.errors.AddAt(st, errPragmaTargetConflict)
		}
		p.Fork = evm.FindInstructionSet(st.Value)
		if p.Fork == nil {
			l.errors.AddAt(st, fmt.Errorf("%w %q", errPragmaTargetUnknown, st.Value))
		}
		p.forkDefined = true
	default:
		l.errors.AddAt(st, fmt.Errorf("%w %s", errUnknownPragma, st.Option))
	}
}

func (l *Loader) processInstructionMacro(p *Program, st *ast.InstructionMacroDef) {
	p.registerInstrMacro(st)
}

func (l *Loader) parseIncludeFile(file string, st *ast.Include, depth int) *ast.Document {
	if l.fsys == nil {
		l.errors.AddAt(st, errIncludeNoFS)
		return nil
	}
	if depth > l.maxIncDepth {
		l.errors.AddAt(st, errIncludeDepthLimit)
		return nil
	}

	content, err := fs.ReadFile(l.fsys, file)
	if err != nil {
		l.errors.AddAt(st, err)
		return nil
	}

	p := ast.NewParser(file, content)
	doc, errors := p.Parse()
	if l.errors.addParseErrors(errors) {
		return nil
	}
	// Note that included documents do NOT have the including document set as Parent.
	// The parent relationship is used during lookup of labels, macros, etc. and
	// such definitions should not be shared between include files.
	//
	// Included documents do have a Creation though.
	doc.Creation = st
	return doc
}

func ResolveRelative(basepath string, filename string) (string, error) {
	res := path.Clean(path.Join(path.Dir(basepath), filename))
	if strings.Contains(res, "..") {
		return "", fmt.Errorf("path %q escapes project root", filename)
	}
	return res, nil
}
