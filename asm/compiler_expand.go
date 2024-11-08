package asm

import (
	"fmt"
	"math"
	"strings"

	"github.com/fjl/geas/internal/ast"
)

// expand appends a list of AST instructions to the program.
func (c *Compiler) expand(doc *ast.Document, prog *compilerProg) {
	for _, astSt := range doc.Statements {
		st := statementFromAST(astSt)
		if st == nil {
			continue
		}
		err := st.expand(c, doc, prog)
		if err != nil {
			c.addError(astSt, err)
			continue
		}
	}
}

// expand creates an instruction for the label. For dotted labels, the instruction is
// empty (i.e. has size zero). For regular labels, a JUMPDEST is created.
func (li labelDefStatement) expand(c *Compiler, doc *ast.Document, prog *compilerProg) error {
	if li.Global {
		ast := li.LabelDefSt
		if err := c.globals.setLabelDocument(ast, doc); err != nil {
			return err
		}
	}

	inst := newInstruction(li, "")
	if !li.Dotted {
		inst.op = "JUMPDEST"
	}
	prog.addInstruction(inst)
	return nil
}

// expand appends the instruction to a program. This is also where basic validation is done.
func (op opcodeStatement) expand(c *Compiler, doc *ast.Document, prog *compilerProg) error {
	opcode := strings.ToUpper(op.Op)
	inst := newInstruction(op, opcode)

	switch {
	case isPush(opcode):
		if opcode == "PUSH0" {
			if op.Arg != nil {
				return ecPushzeroWithArgument
			}
			break
		}
		if op.Arg == nil {
			return ecPushWithoutArgument
		}

	case isJump(opcode):
		if err := c.validateJumpArg(doc, op.Arg); err != nil {
			return err
		}
		// 'JUMP @label' instructions turn into 'PUSH @label' + 'JUMP'.
		if op.Arg != nil {
			push := newInstruction(op, "PUSH")
			prog.addInstruction(push)
		}

	default:
		if _, ok := inst.opcode(); !ok {
			return fmt.Errorf("%w %s", ecUnknownOpcode, inst.op)
		}
		if op.Arg != nil {
			return ecUnexpectedArgument
		}
	}

	prog.addInstruction(inst)
	return nil
}

// validateJumpArg checks that argument to JUMP is a defined label.
func (c *Compiler) validateJumpArg(doc *ast.Document, arg ast.Expr) error {
	if arg == nil {
		return nil // no argument is fine.
	}
	lref, ok := arg.(*ast.LabelRefExpr)
	if !ok {
		return ecJumpNeedsLiteralLabel
	}
	if lref.Dotted {
		return fmt.Errorf("%w %v", ecJumpToDottedLabel, lref)
	}

	var li *ast.LabelDefSt
	if lref.Global {
		li = c.globals.label[lref.Ident]
	} else {
		li, _ = doc.LookupLabel(lref)
	}
	if li == nil {
		return fmt.Errorf("%w %v", ecJumpToUndefinedLabel, lref)
	}
	return nil
}

// expand appends the output of an instruction macro call to the program.
func (inst macroCallStatement) expand(c *Compiler, doc *ast.Document, prog *compilerProg) error {
	var (
		name   = inst.Ident
		def    *ast.InstructionMacroDef
		defdoc *ast.Document
	)
	if ast.IsGlobal(name) {
		def, defdoc = c.globals.lookupInstrMacro(name)
	} else {
		def, defdoc = doc.LookupInstrMacro(name)
	}
	if def == nil {
		return fmt.Errorf("%w %%%s", ecUndefinedInstrMacro, name)
	}

	// Prevent recursion and check args match.
	if !c.enterMacro(def) {
		return fmt.Errorf("%w %%%s", ecRecursiveCall, name)
	}
	defer c.exitMacro(def)
	if len(inst.Args) != len(def.Params) {
		return fmt.Errorf("%w, macro %%%s needs %d", ecInvalidArgumentCount, name, len(def.Params))
	}

	// Clone the macro's body document. This is a shallow clone for setting
	// Parent/Creation, which is done to for error location reporting reasons. Cloning the
	// document also means by-document caching does not treat all expansions of a macro as
	// the same code.
	macroDoc := *def.Body
	macroDoc.Parent = defdoc
	macroDoc.Creation = inst

	// Arguments of instruction macros cannot be evaluated during expansion. They are
	// evaluated in a later pass where all intermediate arguments are processed. In order
	// to compute the value then, we need to keep track of macro argument expressions and
	// their origin document chain. An example:
	//
	// #define %MacroA(a) {
	//      %MacroB($a)
	// }
	// #define %MacroB(b) {
	//      push $b
	// }
	//
	// When the evaluator processes 'push $b' generated by MacroB, it first finds
	// that $b = $a. However, the expression $a must not be evaluated in the context of
	// MacroB, but in the context of MacroA, because that's where $a is defined.
	//
	// To keep track of this, we store the callsite of the macro along with the arguments
	// into the output section. The evaluator uses this callsite as the evaluation context
	// for variables.
	callsite := prog.currentSection()
	args := &instrMacroArgs{callsite: callsite, def: def, args: inst.Args}
	prog.pushSection(&macroDoc, args)
	defer prog.popSection()

	// Expand body.
	c.expand(&macroDoc, prog)
	return nil
}

func (c *Compiler) enterMacro(m *ast.InstructionMacroDef) bool {
	if _, onStack := c.macroStack[m]; onStack {
		return false
	}
	c.macroStack[m] = struct{}{}
	return true
}

func (c *Compiler) exitMacro(m *ast.InstructionMacroDef) {
	delete(c.macroStack, m)
}

// expand of #include appends the included file's instructions to the program.
// Note this accesses the documents parsed by processIncludes.
func (inst includeStatement) expand(c *Compiler, doc *ast.Document, prog *compilerProg) error {
	incdoc := c.includes[inst.IncludeSt]
	if incdoc == nil {
		// The document is not in doc.includes, so there must've been a parse error.
		// We can just ignore the statement here since the error was already reported.
		return nil
	}
	prog.pushSection(incdoc, nil)
	defer prog.popSection()
	c.expand(incdoc, prog)
	return nil
}

// expand of #assemble performs compilation of the given assembly file.
func (inst assembleStatement) expand(c *Compiler, doc *ast.Document, prog *compilerProg) error {
	subc := NewCompiler(c.fsys)
	subc.SetIncludeDepthLimit(c.maxIncDepth)
	subc.SetMaxErrors(math.MaxInt)

	file, err := resolveRelative(doc.File, inst.Filename)
	if err != nil {
		return err
	}
	bytecode := c.CompileFile(file)
	if len(c.Errors()) > 0 {
		return nil
	}
	datainst := &instruction{data: bytecode}
	prog.addInstruction(datainst)
	return nil
}