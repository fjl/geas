# Stack Comments and the Stack Checker

Geas has a built-in stack checker that verifies user-written stack comments. the assembler
tracks the contents of the stack symbolically and reports warnings when a comment
contradicts the analysis. Stack checking is purely advisory — warnings do not prevent
assembly and the output bytecode is unaffected.

At this time, stack checking is not enabled by default. You must explicitly enable it
on the command-line.

    geas -a -stackcheck <file>

## Writing Stack Comments

A stack comment is a semicolon comment whose text begins with a square bracket. The
comment documents the state of the stack *after* the instruction on the same line
executes. Items are listed with the leftmost item being the top of the stack.

        push 1              ; [top]
        push 2              ; [v, top]
        add                 ; [sum]

Item names are arbitrary, and can contain any symbol. Whitespace within items is stripped,
so `x + 1` and `x+1` are the same name. Parentheses and brackets are allowed inside items
as long as they are balanced, which is useful for descriptive names:

        add                 ; [fn(a,b)]
        swap1               ; [arr[0], sum]

Stack comments are optional. You can annotate some instructions and leave others bare.
Instructions without a comment will still be processed by the analysis.

## Item Tracking

The checker assigns a symbolic identity to each stack value. When a name is given to a
value via a comment, that name sticks to the value as it moves through the stack. If a
comment places a name on a position where the checker knows a *different* value resides, a
warning is produced.

        push 1              ; [a]
        push 2              ; [b, a]
        swap1               ; [a, a]    <- warning: item 1 is "b", not "a"

New values (produced by the current instruction) can freely receive any name, including
a name already in use. This is common when pushing the same constant multiple times:

        push 0              ; [0]
        push 0              ; [0, 0]

Once a name is confirmed on an existing value, the comment cannot rename it:

        push 1              ; [a]
        push 2              ; [b, a]
        dup1                ; [b, b, new]  <- warning: "a" renamed to "new"

## Comment Depth

A stack comment does not need to list every item on the stack. Eliding items at the bottom
is permitted:

        push 1              ; [a]
        push 2              ; [b, a]
        push 3              ; [c, b]    ; only two items shown, "a" still exists

However, a comment must not declare *more* items than are on the stack:

        push 1              ; [a]
        push 2              ; [b, a]
        add                 ; [sum, extra]  <- warning: stack has 1 item, comment declares 2

## Stack Underflow

When an instruction requires more inputs than are available, the checker reports an
underflow:

        push 1              ; [a]
        add                 ; [sum]     <- warning: op requires 2 items, stack has 1

This catches a real bug: the program would fail at runtime.

## Push Literal Matching

When a `push` instruction has a literal number argument and the stack comment also names a
number for the pushed value, the checker verifies that the two values match:

        push 1              ; [2]       <- warning: number 2 in comment does not match pushed value 1

## Wildcards

A wildcard (`..` or `...`) at the end of a stack comment indicates that additional unknown
items may exist below the listed ones. This is useful in code that operates on the top of
the stack without knowing or caring about its full depth.

          push 1            ; [a]
          push 0            ; [cond, a]
          jumpi @target     ; [a]
          push 2            ; [b, a]
        target:             ; [x, ..]
          pop               ; [..]

Note that wildcards suppress stack underflow warnings. When an instruction has a comment
with a wildcard, it effectively operates on an unlimited stack. This behavior persists
until the next stack comment without a wildcard. So if you have a region of code where the
stack checker just doesn't 'get it', you can label the first instruction with a wildcard
to remove warnings.

## Labels and Merge Points

The analyzer will verify the stack through control flow (jumps).

When multiple paths converge at a label, the checker merges the stack states from all
predecessors. A label can have a stack comment to document the expected state at the merge
point:

          push 1            ; [a]
          push 0            ; [cond, a]
          jumpi @target     ; [a]
          push 2            ; [b, a]
          add               ; [sum]
        target:             ; [x]        <- note label here
          pop               ; []

At a merge point, the checker intersects predecessor stacks. If all predecessors agree on
a name at a given position, that name is *confirmed* — subsequent comments cannot rename
it. If predecessors disagree, the name at that position is *unconfirmed* and the label
comment (or a later instruction comment) can freely assign a new name.

### Inconsistent Depth

If predecessors arrive with different stack depths, a warning is produced:

          push 1            ; [a]
          push 0            ; [cond, a]
          jumpi @target     ; [a]
          push 2            ; [b, a]
        target:             ; [x]       <- warning: predecessors have inconsistent depth [1 2]

This usually indicates a bug — one path pushes or pops a different number of items than
another. Note that such warnings are also suppressed by the presence of a wildcard (`..`).

### Loops and Renaming

The checker handles loops, including nested loops, through iterative analysis.

          push 0            ; [count]
          push 99           ; [val, count]
        loop:               ; [val, count]
          swap1             ; [count, val]
          push 1            ; [1, count, val]
          add               ; [count+1, val]
          swap1             ; [val, count+1]
          push 10           ; [10, val, count+1]
          swap1             ; [val, 10, count+1]
          div               ; [val/10, count+1]
          dup2              ; [count+1, val/10, count+1]
          jumpi @loop       ; [val/10, count+1]

          pop               ; [count+1]
          stop

When the back-edge of a loop produces different names than the forward edge, the
disagreeing positions at the loop header become unconfirmed and can be renamed by the
label comment. If both edges agree on a name, it is confirmed and cannot be renamed:

          push 1            ; [x]
        loop:               ; [wrong]   <- warning: cannot rename confirmed item "x"
          dup1              ; [x, x]
          pop               ; [x]
          push 0            ; [cond, x]
          jumpi @loop       ; [x]
          stop

Loops are verified to be stack-balanced, i.e. they must not leave more items on the stack
than are available at the entry point. In the example below, the loop body always produces
an item, leading to an eventual stack overflow.

      loop:                 ; []       <- warning: loop has unbalanced stack
        push 1              ; [x]
        gas                 ; [gas, x]
        push 100            ; [tv, gas, x]
        lt                  ; [tv<gas, x]
        jumpi @loop         ; [x]

## Instruction Macros and Include Files

The checker analyzes instruction macro bodies and `#include` files independently before
they are used. If a macro definition has a stack comment on the opening brace line, it
declares the expected input stack:

    #define %addone() {     ; [x]
        push 1              ; [1, x]
        add                 ; [x+1]
    }

When the macro is called, the checker uses its computed effect (inputs consumed, outputs
produced) to update the caller's stack:

        push 5              ; [val]
        %addone()           ; [val+1]

If the macro definition has no start comment, the checker infers the macro's inputs from
the operations in its body.
