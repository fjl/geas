# geas

This is `geas` – the Good Ethereum Assembler[^1] – a macro assembler for the EVM.

You can use it to create any contract for Ethereum, though it's probably a bad idea.
For real contracts, you should use a well-tested language compiler like Solidity.
The purpose of geas is mostly creating specialty programs and tinkering with the EVM
at a low level.

### Installation and Usage

To build the tool, clone the repository and then run

	go build ./cmd/geas

This creates the `geas` binary in the current directory. To create bytecode, run the tool
with a filename as argument.

	./geas file.eas

### Use as a Go Library

You can also use the assembler as a library. See the [API documentation](https://pkg.go.dev/github.com/fjl/geas/asm)
to get started.

## Language

Programs accepted by the assembler follow a simple structure. Each line is an instruction.
Both uppercase and lowercase can be used for instruction names. All known EVM instructions
are supported.

Comments can appear anywhere and are introduced by the semicolon (;) character.

		push 1  ;; comment
		push 2
		add

Opcodes listed in the program correspond directly with the bytecodes in output. There are
some conveniences though. Jump destinations are written as a label followed by colon (:)
and can be referred to using the notation @label together with JUMP or JUMPI.

	begin:
		push 1
		push 2
		add
		jump @begin

When using JUMP with an argument, it turns in to a PUSH of the label followed by the jump
instruction, so the above is equivalent to:

	begin:
		push 1
		push 2
		add
		push @begin
		jump

It is also possible to create labels without emitting a JUMPDEST instruction by prefixing
the label name with the dot (.) character. Note that dotted labels are not valid for use
as an argument to JUMP.

	.begin:
		push @.end
		push 0
		mstore
	.end:

PUSH instructions must be followed by an immediate argument on the same line. Simple math
expressions and label references can be used within the argument:

	.begin:
		push (@add_it * 2) - 3
		push 5
	add_it:
		add

At this time, there is no precedence in expressions. Use parentheses to indicate precedence.

Supported arithmetic operations include addition (+), subtraction (-), multiplication (*),
division (/), and modulo (%). There is also support for bit-shifts (<<, >>), bitwise AND
(&), OR (|), XOR (^).

All arithmetic is performed with arbitrary precision integers. The result of calculations
must fit into 256 bits in order to be valid as a PUSH argument. Negative results are not
allowed right now.

### Expression Macros

Expression macros can be created with the `#define` directive. Macros can be used within
PUSH argument expressions.

	#define z 0x8823
	#define myexpr(x, y)  (x + y) * z

		push myexpr(1, 2)

### Builtin Macros

There are several builtin macros for common EVM tasks. Names of builtins start with a dot,
and builtin macros cannot be redefined. Available builtins include:

`.abs(...)` for getting the absolute value of a number:

	push .abs(0 - 100)

`.selector(...)` for computing 4-byte ABI selectors:

	push .selector("transfer(address,uint256)")
	push 0
	mstore

`.keccak256()`, `.sha256()` hash functions:

	push .sha256("data")

`.address()` for declaring contract addresses. The checksum and byte length of the address
are verified.

	#define otherContract .address(0x658bdf435d810c91414ec09147daa6db62406379)

### Instruction Macros

Common groups of instructions can be defined as instruction macros. Names of such macros
always start with the percent (%) character.

	#define %add5_and_store(x, location) {
		push x
		push 5
		add
		push location
		mstore
	}

To invoke an instruction macro, write the macro name as a statement on its own line. If
the macro has no arguments, you can also leave the parentheses off.

	.begin:
		%add5_and_store(3, 64)
		%add5_and_store(4, 32)
		push 32
		push 64
		sha3

Nested macro definitions are not allowed. Macro recursion is also not allowed.

### Include Files

EVM assembly files can be included into the current program using the `#include`
directive. Top-level instructions in the included file will be inserted at the position of
the directive.

`#include` filenames are resolved relative to the file containing the directive.

	.begin:
		push @.end
		push 32
		mstore

	#include "file.evm"
	.end:

### Local and Global Scope

Names of labels and macros are case-sensitive. And just like in Go, the case of the first
letter determines visibility of the definition.

Macro and label definitions whose name begins with a lower-case letter are local to the
file they're defined in. This means local definitions cannot be referenced by `#include`
files.

Identifiers beginning with an upper-case letter are registered in the global scope and are
available for use in all files of the program, regardless of `#include` structure. Global
identifiers must be unique across the program, i.e. they can only be defined once.

This means that files defining global macros or labels can only be included into the
program once. It also means that instruction macros containing global labels can only be
called once. Use good judgement when structuring your includes to avoid redefinition
errors.

lib.eas:

	#define result 128
	#define StoreSum {
		add
		push result
		mstore
	}

main.eas:

	#include "lib.eas"

		push 1
		push 2
		%StoreSum  ;; calling global macro defined in lib.evm

[^1]: Under no circumstances must it be called the geth assembler.
