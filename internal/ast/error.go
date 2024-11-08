package ast

import "fmt"

// Position represents a line in a file.
type Position struct {
	File string
	Line int
}

func (p Position) String() string {
	return fmt.Sprintf("%s:%d", p.File, p.Line)
}

// ParseError is an error that happened during parsing.
type ParseError struct {
	tok  token
	file string
	err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d: %v", e.file, e.tok.line, e.err)
}

func (e *ParseError) Position() Position {
	return Position{File: e.file, Line: e.tok.line}
}

func (e *ParseError) Unwrap() error {
	return e.err
}

func ErrLabelAlreadyDef(firstDef, secondDef *LabelDefSt) error {
	dotInfo := ""
	if firstDef.Dotted && !secondDef.Dotted {
		dotInfo = " (as dotted label)"
	}
	if !firstDef.Dotted && secondDef.Dotted {
		dotInfo = " (as jumpdest)"
	}
	return fmt.Errorf("%v already defined%s", secondDef, dotInfo)
}
