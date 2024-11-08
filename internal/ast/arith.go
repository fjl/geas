package ast

//go:generate go run golang.org/x/tools/cmd/stringer@latest -type ArithOp

// ArithOp is an arithmetic operation.
type ArithOp byte

const (
	ArithPlus   = ArithOp(iota + 1) // +
	ArithMinus                      // -
	ArithMul                        // *
	ArithDiv                        // /
	ArithMod                        // %
	ArithLshift                     // <<
	ArithRshift                     // >>
	ArithAnd                        // &
	ArithOr                         // |
	ArithHat                        // ^
)

// arithChars contains all the single-character arithmetic operations.
// note that '%' is also absent from this list since it has a dual purpose.
var arithChars = map[rune]ArithOp{
	'+': ArithPlus,
	'-': ArithMinus,
	'*': ArithMul,
	'/': ArithDiv,
	'&': ArithAnd,
	'|': ArithOr,
	'^': ArithHat,
}

func tokenArithOp(tok token) ArithOp {
	if tok.typ != arith {
		panic("token is not arith")
	}
	switch {
	case tok.text == "<<":
		return ArithLshift
	case tok.text == ">>":
		return ArithRshift
	case tok.text == "%":
		return ArithMod
	default:
		op, ok := arithChars[[]rune(tok.text)[0]]
		if !ok {
			panic("invalid arith op")
		}
		return op
	}
}
