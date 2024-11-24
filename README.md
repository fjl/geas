![geas](assets/geas-b.svg)

This is geas – the Good Ethereum Assembler[^1] – a macro assembler for the EVM.

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

### Editor Support

VIM users may be interested in [vim-geas](https://github.com/lightclient/vim-geas).

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

Opcodes listed in the program correspond directly with the bytecodes in output.

### Jump

Jump destinations are written as a label followed by colon (:) and can be referred to
using the notation `@label` together with JUMP or JUMPI.

    begin:
        push 1
        push 2
        add
        jump @begin

When using JUMP with an argument, it turns into a PUSH of the label followed by the jump
instruction, so the above is equivalent to:

    begin:
        push 1
        push 2
        add
        push @begin
        jump

It is also possible to create labels without emitting a JUMPDEST instruction by prefixing
the label name with the dot (.) character. While dotted labels are not valid for use as an
argument to JUMP, they can be used with PUSH to measure code offsets.

        push @.end
        codesize
        eq
    .end:

### Push

The EVM instruction has sized push instructions from size zero (`PUSH0`) up to a size of
32 bytes (`PUSH32`). While you can use sized push instructions directly, it is preferable
to let the assembler figure out the right size for you. To do this use the variable-size
`PUSH` instruction.

All PUSH-type instructions must be followed by an immediate argument on the same line.
Simple math expressions and label references can be used within the argument:

    .begin:
        push (@add_it * 2) - 3
        push 5
    add_it:
        add

Supported arithmetic operations include addition (+), subtraction (-), multiplication (*),
division (/), and modulo (%). There is also support for bit-shifts (<<, >>), bitwise AND
(&), OR (|), XOR (^).

All arithmetic is performed with arbitrary precision integers. The result of calculations
must fit into 256 bits in order to be valid as a PUSH argument. For sized push, the result
must fit into the declared push size. Negative results are not allowed.

### Expression Macros

Expression macros can be created with the `#define` directive. Macros can be used within
PUSH argument expressions.

Macros can have parameters. Refer to parameter values using the dollar sign ($) prefix
within the macro.

    #define z = 0x8823
    #define myexpr(x, y) = ($x + $y) * z

        push myexpr(1, 2)

### Builtin Macros

There are several builtin macros for common EVM tasks. Names of builtins start with a dot,
and builtin macros cannot be redefined. Available builtins include:

`.abs()` for getting the absolute value of a number:

    push .abs(0 - 100)

`.selector()` for computing 4-byte ABI selectors:

    push .selector("transfer(address,uint256)")
    push 0
    mstore

`.keccak256()`, `.sha256()` hash functions:

    push .sha256("data")

`.address()` for declaring contract addresses. The checksum and byte length of the address
are verified.

    #define otherContract = .address(0x658bdf435d810c91414ec09147daa6db62406379)

### Instruction Macros

Common groups of instructions can be defined as instruction macros. Names of such macros
always start with the percent (%) character.

    #define %add5_and_store(x, location) {
        push $x
        push 5
        add
        push $location
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

When defining (local) labels within instruction macros, they will only be visible within
the macro. There is no way to refer to a local macro label from the outside, though you
can pass references to such internal labels into another macro. The example below
illustrates this, and also shows that in order to jump to a label argument within a macro,
you must use explicit PUSH and JUMP.

    #define %jump_if_not(label) {
        iszero
        push $label
        jumpi
    }

    #define %read_input(bytes) {
        calldatasize
        push $bytes
        eq
        %jump_if_not(@revert)

        push 0
        push $bytes
        calldataload
        jump @continue

      revert:
        push 0
        push 0
        revert

      continue:
    }

### Including Files

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
letter determines visibility of definitions.

Macro and label definitions whose name begins with a lower-case letter are local to the
file they're defined in. This means local definitions cannot be referenced by `#include`
files.

Identifiers beginning with an upper-case letter are registered in the global scope and are
available for use across files. When using `#include`, global definitions in the included
file also become available in all other files.

Global identifiers must be unique across the program, i.e. they can only be defined once.
Files defining global macros or labels can only be included into the program once. Note
that the uniqueness requirement also means that instruction macros containing global
labels can only be called once. Use good judgement when structuring your includes to avoid
redefinition errors.

lib.eas:

    #define result = 128
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

### Configuring the target instruction set

The EVM is a changing environment. Opcodes may be added (and sometimes removed) as new
versions of the EVM are released in protocol forks. Geas is aware of EVM forks and their
respective instruction sets.

Geas always operates on a specific EVM instruction set. It targets the latest known eth
mainnet fork by default, i.e. all opcodes available in that fork can be used, and opcodes
that have been removed in any prior fork cannot.

Use the `#pragma target` directive to change the target instruction set. The basic syntax is

    #pragma target "name"

where `name` is a lower-case execution-layer fork name like `homestead`, `berlin`, or `prague`.

Here is an example. This contract uses the CHAINID instruction to check if it is running
on mainnet, and destroys itself otherwise. CHAINID became available in the "istanbul"
fork, and SELFDESTRUCT was removed in a later revision of the EVM, so this program is only
applicable to a certain range of past EVM versions.

    #pragma target "berlin"

        chainid                ; [id]
        push 1                 ; [1, id]
        eq                     ; [id = 1]
        jumpi @mainnet         ; []
        push 0x0               ; [zeroaddr]
        selfdestruct           ; []
    mainnet:

Note that declaring the target instruction set using `#pragma target` will not prevent the
output bytecode from running on a different EVM version, since it is just a compiler
setting. The example program above will start behaving differently from its intended
version on EVM version "cancun", because SELFDESTRUCT was turned into SENDALL in that
fork. It may even stop working entirely in a later fork.

`#pragma target` can only appear in the program once. It cannot be placed in an include
file. You have to put the directive in the main program file.

### #assemble

When writing contract constructors and advanced CALL scenarios, it can be necessary to
include subprogram bytecode as-is. The `#assemble` directive does this for you.

Using `#assemble` runs the assembler on the specified file, and includes the resulting
bytecode into the current program. Labels of the subprogram will start at offset zero.
Unlike with `#include`, global definitions of the subprogram are not imported.

        ;; copy subprogram to memory
        push @.end - @.begin   ; [size]
        push @.begin           ; [offset, size]
        push 128               ; [dest, offset, codesize]
        codecopy               ; []

    .begin:
    #assemble "subprogram.eas"
    .end

If a target instruction set is configured with `#pragma target`, it will also be used for
assembling the subprogram. However, the subprogram file can override the instruction set
using its own `#pragma target` directive.

[^1]: Under no circumstances must it be called the geth assembler.
