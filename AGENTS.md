# Project Philosophy

Geas is an EVM assembler. It enables users to write low-level programs for the Ethereum
Virtual Machine (EVM). The Geas assembly language is similar in spirit to the Go
programming language, but also takes inspiration from Perl. In addition to writing basic
opcodes, the user can use instruction macros to capture common opcodde sequences. The
language also allows 'expression macros' which are useful for inserting constants and
pre-calculated numbers and binary data into the bytecode.

The assembler should try to be maximally helpful to its user and give good error messages
when the program has a mistake. Being a compiler for a low-level language, Geas cannot
verify the correctness of the user's program, but it should ensure the program does not
contain surface-level bugs such as invalid instructions, jumps to invalid destinations,
and stack management issues.

# Project Structure

- `cmd/geas` - command line frontend
- `asm/` - assembler implementation
- `disasm/` - disassembler implementation

# Development Tips

When modifying Go code, just run `goimports -w` on the file after editing it to update and
organize imports. Do this before compiling to ensure there is no error. If `goimports` is
not installed yet, just install it to ~/go/bin.

You can use `staticcheck` to check for common programming mistakes and style issues. If
staticcheck is not installed, you can also install it to ~/go/bin.

Sometimes it is useful to run the `geas` tool itself to perform test runs. To run the
assembler, use `go run ./cmd/geas -a` from the project root. The tool does not accept
filenames outside of the current directory.

To disassemble bytecode, use `go run ./cmd/geas -d` from the project root.

# Go Code Style

Avoid repeated expressions, and do not deeply nest expressions. Provide comments, but be
terse, do not repeat the same information across many comments.

When creating documentation comments, adhere to the Go style of using the function name as
the first word of the godoc comment. Do not use this convention for struct fields,
variables and constants. When documenting tests, do not put a godoc style comment. Rather,
add a comment like "this test does XXX".

# Compiler Tests

For the assembler, test cases are defined in a YAML file `./asm/testdata/compiler-tests.yaml`.

If possible, create new tests there whenever features are added or bugs are fixed. Do not
modify any existing tests, except to adapt error messages when necessary.

Tests related for the stack checker are in `./asm/testdata/stackcheck-tests.yaml`. When
modifying the stack checker or adding features there, make sure to create tests that
produce a related warning.

