The Geas language should be extended by a construct that allows defining 'structs', which
are basically collections of sized fields.

The user can define a struct to document the memory layout of inputs or data structures,
and use the definition to enter computed field offsets and sizes into the program.

Syntactically, structs are a shortcut to define multiple expression macros which give
access to the sizes and offsets of the structure.

The parser has to be extended for this to allow for identifiers to contain the . character.

## Basic Example

Here is an example:

```geas
#defstruct signature {
    r :: 32, s :: 32
    qx :: 32; qy :: 32
}
```

This would be equivalent to:

```geas
#define signature.size = 128
#define signature.r.offset = 0
#define signature.r.size = 32
#define signature.s.offset = 32
#define signature.s.size = 32
#define signature.qx.offset = 64
#define signature.qx.size = 32
#define signature.qy.offset = 96
#define signature.qx.size = 32
```

i.e. the offsets are added up, while also giving access to the sizes.

## Nesting

It should also be possible to nest structs, i.e. to have a definition like:

```geas
#defstruct item {
   name  :: 10
   text  :: 90
}

#defstruct input {
    s :: signature   ; embeds the 'signature' structure defined earlier
    i :: item        ; embeds the 'item' structure
}
```

and by doing this, it defines:

input.size = 228  (signature.size + item.size)
input.s.size = 128  (signature.size)
input.s.offset = 0
input.s.r.size
inputss.
