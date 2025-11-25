![geas](assets/geas-b.svg)

This is geas – the Good Ethereum Assembler[^1] – a macro assembler for the EVM.

You can use it to create any contract for Ethereum, though it's probably a bad idea. For
real contracts, you should use a well-tested language compiler like Solidity. The purpose
of Geas is mostly creating specialty programs and tinkering with the EVM at a low level.

[^1]: Under no circumstances must it be called the geth assembler.

### Installation

You can use the `go` tool to install the latest released version.
This creates a `geas` binary in the current directory:

    env "GOBIN=$PWD" go install github.com/fjl/geas/cmd/geas@latest

For development of geas, clone the repository and then run `go build ./cmd/geas`.

### Usage

To create bytecode from an assembly file, run the tool with a filename as argument.

    ./geas file.eas

There is also a disassembler. To disassemble hex bytecode from standard input, run:

    ./geas -d -

To see all supported flags, run `geas` with no arguments.

### Editor Support

VIM users may be interested in [vim-geas](https://github.com/lightclient/vim-geas).

### Use as a Go Library

You can also use the assembler as a library. See the [API documentation](https://pkg.go.dev/github.com/fjl/geas/asm)
to get started.

## Language

The Geas language is intended to be a direct representation of EVM bytecode.

The EVM is a stack-based virtual machine operating on 256-bit values, with a somewhat
limited instruction set. Unlike high-level languages (such as Solidity, Vyper), Geas does
not abstract these properties away. When writing contract programs in Geas you are exposed
to the design of the EVM as it really is, with very little help.

What little help there is exists to deal with mundane details. When writing and reviewing
stack-based bytecode, the thinking goes, authors should not also be bothered with the
question of whether a PUSH instruction is one or two bytes in size, or whether a JUMPDEST
exists at a particular label. That said, you can always choose to ignore the facilities
provided for convenience, they are strictly optional.

### Instructions

Programs are listings of instructions. Each instruction is written on its own line. Both
uppercase and lowercase can be used for instruction names, though lowercase is preferred
by convention.

Here is a bare-bones example program that loads storage slot zero and adds one the value
contained in it. Note that the `push1` instruction takes an immediate argument, while
other instructions do not.

        push0
        sload
        push1 1
        add

### Comments

Comments can appear anywhere and are introduced by the semicolon (;) character. A comment
always extends to the end of the current line.

        push 1    ; comment
        push 2
        add

By convention, top-level comments start with three semicolons. Groups of instructions are
titled by two semicolons, and the comment is indented like the instruction. A single
semicolon is used for instruction-level comments, typically a documentation of the
post-stack of the instruction. Here's a fully commented example showing the commenting
convention:

    ;;; This is a top-level comment which explains the program.
    ;;; Note the three semicolons.

        ;; add two numbers
        push 1              ; [b]
        push 2              ; [b, a]
        add                 ; []

    ;;; The End

### Push Instructions

PUSH instructions are the mechanism to place numbers on the stack. The EVM instruction set
defines push instructions with argument sizes of zero (PUSH0) up to a maximum size of 32
bytes (PUSH32). While you can use explictly-sized (PUSHx) instructions directly, it is
preferable to let the assembler figure out the right size for you. To do this, use the
variable-size `push` instruction.

All push-type instructions must be followed by an immediate argument on the same line. The
argument is an expression, so literal values, math operations, and references to macro
definitions can be used.

        push 5
        push (100 * 2) - 3
        push macroValue

While intermediate results in PUSH argument expressions can be of any size, the *result*
of the expression must fit into 256 bits. For explicitly-sized PUSHx, the result must fit
into the declared push size.

Since the EVM does not support negative numbers directly, `push` argument values are not
allowed be negative.

### Labels and Jumps

The EVM supports two jump instructions, JUMP and JUMPI (conditional jump). Jumps can only
target pre-declared JUMPDEST instructions.

Jump destinations are written as a label followed by the colon (:) characte and can be
referred to using the notation `@label` within expressions.

        push 1              ; [sum]
    begin:
        push 2              ; [a, sum]
        add                 ; [sum]
        push @begin         ; [label, sum]
        jump                ; [sum]

For convenience, Geas allows writing jump instructions with an argument. When written in
this way, the jump turns into a push of the label followed by the jump instruction, so the
above snippet can also be written as:

        push 1              ; [sum]
    begin:
        push 2              ; [a, sum]
        add                 ; [sum]
        jump @begin         ; [sum]

It is also possible to create labels without emitting a JUMPDEST instruction by prefixing
the label name with the dot (.) character. While dotted labels are not valid for use as an
argument to JUMP, they can be used with PUSH to measure code offsets.

        push @end
        codesize
        eq
    .end:

Finally, please note that jump destinations can be written explicitly if desired. The
assembler does not require use of the label syntax, but it is much easier to read, and
safer, too.

### #bytes

The `#bytes` directive adds raw bytes into the output. This is typically used for placing
static data such as error messages.

`#bytes` takes an arbitrary expression argument like `push`:

    #bytes "data"
    #bytes 0x01020304060708

### Named #bytes

Bytes can also be named by adding a label definition before the expression:

    #bytes named: 0x0fffe3

When used like this, the bytes value becomes available as a macro for use in expressions,
and also defines a label that can be used to get their offset in the output. The
definition above could be used like this to copy the bytes into memory:

        push len(named)     ; [size]
        push @named         ; [codeOffset, size]
        push 0              ; [dest, codeOffset, size]
        codecopy            ; []

### Expressions

Expressions are used as `push` and `#bytes` arguments.

The Geas expression language is intended for simple calculations. Expressions are just
pure functions that work on values, which are arbitrary precision integers.

Intermediate results in an expression can be of any size. Calculations cannot overflow.
Negative number values are also supported. Available arithmetic operations include:

- multiplication (*), division (/), modulo (%), bit-shifts (<<, >>), bitwise AND (&)
- addition (+), subtraction (-), bitwise OR(|), XOR (^)

There is limited support for using strings and arbitrary byte sequences in expressions.
You can write string literals using double quotes, and use hexadecimal literals with a
`0x` prefix to specify bytes. Please note that strings and bytes are internally
represented in the same way numbers are. All arithmetic and macros work on all values,
there are no 'types'.

However, there is one aspect of values where integers and bytes have a subtle difference.
A value with leading zero bytes can be created by hexadecimal literal like `0x00ff` (or by
a macro call) and the value will be stored as-written. When used in a context like
`#bytes`, the leading zero bytes will be included in the output. Operations that work with
integers, like arithmetic, will strip leading zero bytes. Here is an example:

    ;; This statement outputs two bytes, 0x00 and 0x01:
    #bytes 0x0001

    ;; But this statement outputs just one, 0x02:
    #bytes 0x0001 + 1

If in doubt, you can use the `abs()` builtin to remove any leading zero bytes, though it
is rarely necessary.

### Expression Macros

Expression macros can be created with the `#define` directive. This is mostly intended for
definitions of constants or values which are used in multiple places.

    #define CONSTANT = 0x8823

        push CONSTANT       ; [v]
        push 38             ; [offset, v]
        calldataload        ; [val, v]
        mul                 ; [product]

Macros can have parameters. Refer to parameter values using the dollar sign ($) prefix
within the macro definition.

    #define myexpr(x, y) = CONSTANT * ($x + $y)

        push myexpr(1, 2)   ; [104553]

### Builtin Expression Macros

There are several builtin macros for use in expressions.

`abs()` returns the absolute value of an integer:

    push abs(-100)          ; [100]

`len()` is for computing the byte length of a value. This returns the number of bytes
necessary to store the value.

    push len(1)             ; [1]
    push len(0x1f)          ; [2]
    push len("hello")       ; [5]

Note `len()` treats the value as bytes, i.e. leading zero bytes are counted. If this is
not desired, you can use `abs()` to remove leading zeros.

    push len(0x0000ff)      ; [3]
    push len(abs(0x0000ff)) ; [1]

`intbits()` returns the bit-length of an integer:

    push intbits(0x1f)      ; [17]

`selector()` computes 4-byte ABI selectors:

    push selector("reward(bytes32)") ; [0x8d6f4b97]

`keccak256()`, `sha256()` hash functions:

    push sha256("data")     ; [0x3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7]

`address()` is for declaring contract addresses. This macro only works with literals as an
argument. Use it to ensure the byte length and checksum of addresses are correct.

    #define otherContract = address(0x658bdf435d810c91414ec09147daa6db62406379)

`assemble()` runs the assembler on another file, and returns the bytecode. See further
down for additional information.

    #bytes assemble("otherfile.eas")

### Instruction Macros

Common groups of instructions can be defined as instruction macros. This is intended to
aid with writing repetitive code. The abstraction capability of the macro system is very
limited by design. There are no conditionals, no loops, and no recursion.

Names of instructions macros start with the percent (%) character.

    #define %inc_and_store(pointer) {
        push 1                  ; [1, x]
        add                     ; [x+1]
        push $pointer           ; [p, x+1]
        mstore                  ; []
    }

To invoke an instruction macro, write the macro name as a statement on its own line. If
the macro has no arguments, you can also leave the parentheses off.

    #define counter = 133  ; memory location of counter

        ;; increment counter
        push counter            ; [slot]
        mload                   ; [val]
        %inc_and_store(counter) ; []

When defining (local) labels within instruction macros, they will only be visible within
the macro. There is no way to refer to a local macro label from the outside, though you
can pass references to such internal labels into another macro. The example below
illustrates this, and also shows that in order to jump to a label argument within a macro,
you must use explicit PUSH and JUMP.

    #define %read_input(numBytes) {
        calldatasize            ; [size]
        push $numBytes          ; [n, size]
        eq                      ; [n==size]
        %jump_if_not(@revert)   ; []

        push $numBytes          ; [size]
        push 0                  ; [offset]
        push 0                  ; [dest, offset, size]
        calldatacopy            ; []
        jump @continue          ; []

      revert:
        push 0                  ; [size]
        push 0                  ; [offset, size]
        revert                  ; []

      continue:
    }

    #define %jump_if_not(label) {
        iszero                  ; [cond]
        push $label             ; [l, cond]
        jumpi                   ; []
    }

### #include

Other Geas source files can be included into the current program using the `#include`
directive. Top-level instructions in the included file will be inserted at the position of
the directive.

`#include` filenames are resolved relative to the file containing the directive.

    .begin:
        push @end
        push 32
        mstore

    #include "file.evm"
    .end:

### Local and Global Scope

Names of labels and macros are case-sensitive. Like in Go, the case of the first letter
determines the visibility of definitions.

Macro and label definitions whose name begins with a lower-case letter are local to the
file they're defined in. Local definitions cannot be referenced by `#include` files.

    #define macro = 1   ; this is a file-local definition

Identifiers beginning with an upper-case letter are registered in the global scope and are
available for use across files. When using `#include`, global definitions in the included
file also become available in all other files.

    #define Macro = 1   ; this is a global definition

Ordering of definitions and includes has no effect on the visibility of definitions.
Global definitions are available everywhere, regardless of where the `#include` statement
that defines them is placed.

Here is an example with two files.

File lib.eas:

    #define result = 128
    #define %StoreSum {
        add
        push result
        mstore
    }

File main.eas:

        push 1
        push 2
        %StoreSum  ; calling global macro defined in lib.eas

    #include "lib.eas"

Global identifiers must be unique across the entire program, i.e. they can only be defined
once. This uniqueness requirement has a few implications:

- Files defining global macros or labels can only be included into the program once.
- Instruction macros which define a global label can only be called once.

You have to keep this in mind when structuring a multi-file project. If you want to
maintain library macros in a separate source file, it is best to include this file once
within the project's top-level entry point. This will add the definitions to the global
namespace and make them available to all other included files.

### Configuring the Target Instruction Set

The EVM is a changing environment. Opcodes may be added (and sometimes removed) as new
versions of the EVM are released in protocol forks.

Geas is aware of EVM forks and their respective instruction sets. When assembling, a
specific EVM instruction set is used. Geas targets the latest known eth mainnet fork by
default, i.e. all opcodes available in that fork are available, and opcodes that have been
removed in any prior fork are not.

Use the `#pragma target` directive to change the target instruction set. The basic syntax is:

    #pragma target "name"

where `name` is a lower-case execution-layer fork name like `homestead`, `berlin`, or `prague`.

Here is an example. This contract uses the CHAINID instruction to check if it is running
on mainnet, and destroys itself otherwise. CHAINID became available in the "istanbul"
fork, and SELFDESTRUCT was removed in a later revision of the EVM, so this program is only
applicable to a certain range of past EVM versions.

    #pragma target "berlin"

        chainid             ; [id]
        push 1              ; [1, id]
        eq                  ; [id==1]
        jumpi @mainnet      ; []
        push 0x0            ; [zeroaddr]
        selfdestruct        ; []
    mainnet:

Note that declaring the target instruction set using `#pragma target` will not prevent the
output bytecode from running on a different EVM version, since it is just a compiler
setting. The example program above will start behaving differently from its intended
version on EVM version "cancun", because SELFDESTRUCT was turned into SENDALL in that
fork. It may even stop working entirely in a later fork.

`#pragma target` can only appear in the program once. It cannot be placed in an include
file. You have to put the directive in the main program file.

### The assemble() Macro

When writing contract constructors and CALL/CREATE scenarios, it can be necessary to
include subprograms into the bytecode as-is.

`assemble()` runs the assembler on the specified file, and returns the output bytecode as
a value. In conjunction with named `#bytes`, this enables writing contract constructor
bytecode like this:

        ;; copy the contract to memory
        push len(code)      ; [size]
        push @code          ; [offset, size]
        push 0              ; [ptr, offset, size]
        codecopy            ; []

        ;; return the bytecode
        push len(code)      ; [size]
        push 0              ; [ptr, size]
        return              ; []

    #bytes code: assemble("program.eas")

Note the current target instruction set will also be used for assembling the subprogram.
However, the subprogram file can override the instruction set using its own `#pragma
target` directive.
