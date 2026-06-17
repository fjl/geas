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

package loader

import (
	"fmt"

	"github.com/fjl/geas/internal/ast"
	"github.com/fjl/geas/internal/lzint"
)

// structLayout is the computed memory layout of a struct definition. Offsets and sizes are
// kept as expressions because field sizes may reference expression macros, whose values are
// only known during evaluation.
type structLayout struct {
	size    ast.Expr
	members []structMember // in declaration order, including nested members
}

// structMember is a field of a struct. For embedded structs, the members of the embedded
// struct are also included, with their name prefixed by the field name.
type structMember struct {
	name   string // path relative to the struct, e.g. "s" or "s.qx"
	offset ast.Expr
	size   ast.Expr
	line   int // source line of the declaring field
	column int // source column of the declaring field
}

// LookupStruct finds a struct definition in the document chain.
func (p *Program) LookupStruct(name string, in *ast.Document) *structLayout {
	if ast.IsGlobal(name) {
		return p.global.structs[name]
	}
	for doc := in; doc != nil; doc = doc.Parent {
		if s := p.defs[doc].structs[name]; s != nil {
			return s
		}
	}
	return nil
}

func (p *Program) registerStruct(doc *ast.Document, name string, layout *structLayout) error {
	var d definitions
	if ast.IsGlobal(name) {
		d = p.global
	} else {
		p.initDefinitions(doc)
		d = p.defs[doc]
	}
	if d.structs[name] != nil {
		return fmt.Errorf("struct %s already defined", name)
	}
	d.structs[name] = layout
	return nil
}

// defineStruct computes the layout of a struct definition, registers it, and registers the
// expression macros that give access to its size and the offsets/sizes of its fields. Any
// errors encountered are returned.
func (p *Program) defineStruct(doc *ast.Document, st *ast.StructDef) []error {
	layout, err := p.buildStruct(doc, st)
	if err != nil {
		return []error{err}
	}
	if err := p.registerStruct(doc, st.Ident, layout); err != nil {
		return []error{err}
	}

	var errs []error
	var group []*ast.ExpressionMacroDef
	pos := st.Position()
	for _, def := range layout.macroDefs(doc, st.Ident, pos.Line, pos.Column) {
		if err := p.registerExprMacro(doc, def); err != nil {
			errs = append(errs, err)
			continue
		}
		group = append(group, def)
	}
	if len(group) > 0 {
		p.structMacros = append(p.structMacros, group)
	}
	return errs
}

// buildStruct computes the layout of a struct, resolving embedded structs in the process.
// Offsets and sizes are built as expressions, so the field offset of field i is the sum of
// the sizes of fields 0..i-1, and offsets/sizes of embedded structs reference the embedded
// struct's own generated macros.
func (p *Program) buildStruct(doc *ast.Document, st *ast.StructDef) (*structLayout, error) {
	layout := &structLayout{}
	offset := ast.Expr(zero())
	seen := make(map[string]bool, len(st.Fields))

	for _, f := range st.Fields {
		if seen[f.Name] {
			return nil, fmt.Errorf("duplicate field %q in struct %s", f.Name, st.Ident)
		}
		seen[f.Name] = true

		var fieldSize ast.Expr
		if f.Type == "" {
			// Sized field.
			fieldSize = f.Size
			layout.add(f.Name, offset, fieldSize, f.Line, f.Column)
		} else {
			// Embedded struct. Its size and the offsets/sizes of its members are referenced
			// through the macros generated for the embedded struct.
			ref := p.LookupStruct(f.Type, doc)
			if ref == nil {
				return nil, fmt.Errorf("undefined struct %s embedded by field %q", f.Type, f.Name)
			}
			fieldSize = macroRef(f.Type + ".size")
			layout.add(f.Name, offset, fieldSize, f.Line, f.Column)
			for _, m := range ref.members {
				memOffset := addExpr(offset, macroRef(f.Type+"."+m.name+".offset"))
				layout.add(f.Name+"."+m.name, memOffset, macroRef(f.Type+"."+m.name+".size"), f.Line, f.Column)
			}
		}
		offset = addExpr(offset, fieldSize)
	}
	layout.size = offset
	return layout, nil
}

// add appends a member to the layout.
func (s *structLayout) add(name string, offset, size ast.Expr, line, column int) {
	s.members = append(s.members, structMember{name: name, offset: offset, size: size, line: line, column: column})
}

// macroDefs creates the expression macro definitions implied by the struct layout. The
// generated definitions carry the source position of their declaring field so that
// evaluation errors point at the right place. The per-field macros come first, and the
// aggregate '.size' macro last, so that pre-evaluation reports a faulty field at its own
// position.
func (s *structLayout) macroDefs(doc *ast.Document, name string, line, column int) []*ast.ExpressionMacroDef {
	defs := make([]*ast.ExpressionMacroDef, 0, 1+2*len(s.members))
	for _, m := range s.members {
		defs = append(defs, ast.SyntheticExprMacroDef(doc, m.line, m.column, name+"."+m.name+".offset", m.offset))
		defs = append(defs, ast.SyntheticExprMacroDef(doc, m.line, m.column, name+"."+m.name+".size", m.size))
	}
	defs = append(defs, ast.SyntheticExprMacroDef(doc, line, column, name+".size", s.size))
	return defs
}

// zero returns a literal zero expression.
func zero() ast.Expr {
	return ast.MakeNumber(lzint.FromInt64(0))
}

// macroRef returns an expression that calls the named expression macro.
func macroRef(name string) ast.Expr {
	return &ast.MacroCallExpr{Ident: name}
}

// addExpr returns an expression computing a + b.
func addExpr(a, b ast.Expr) ast.Expr {
	return &ast.BinaryExpr{Op: ast.ArithPlus, Left: a, Right: b}
}
