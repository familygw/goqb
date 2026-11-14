package vm

import (
	"math"
	"strings"
)

// maxStringLen es el límite de QB para un string.
const maxStringLen = 32767

func maxKind(a, b Kind) Kind {
	if a > b {
		return a
	}
	return b
}

// fitInt verifica el rango del resultado entero según el tipo.
func fitInt(r int64, k Kind) (Value, error) {
	if k == KInt {
		if r < math.MinInt16 || r > math.MaxInt16 {
			return Value{}, ErrOverflow
		}
		return Int(r), nil
	}
	if r < math.MinInt32 || r > math.MaxInt32 {
		return Value{}, ErrOverflow
	}
	return Long(r), nil
}

func fitFloat(f float64, k Kind) (Value, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Value{}, ErrOverflow
	}
	if k == KSingle {
		if math.Abs(f) > math.MaxFloat32 {
			return Value{}, ErrOverflow
		}
		return Single(f), nil
	}
	return Double(f), nil
}

func numOperands(a, b Value) (Kind, error) {
	if !a.K.IsNumeric() || !b.K.IsNumeric() {
		return 0, ErrTypeMismatch
	}
	return maxKind(a.K, b.K), nil
}

// Add suma números o concatena strings.
func Add(a, b Value) (Value, error) {
	if a.K == KString && b.K == KString {
		if len(a.S)+len(b.S) > maxStringLen {
			return Value{}, ErrStringSpace
		}
		return Str(a.S + b.S), nil
	}
	k, err := numOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if k <= KLong {
		return fitInt(a.I+b.I, k)
	}
	return fitFloat(a.Num()+b.Num(), k)
}

func Sub(a, b Value) (Value, error) {
	k, err := numOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if k <= KLong {
		return fitInt(a.I-b.I, k)
	}
	return fitFloat(a.Num()-b.Num(), k)
}

func Mul(a, b Value) (Value, error) {
	k, err := numOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if k <= KLong {
		return fitInt(a.I*b.I, k)
	}
	return fitFloat(a.Num()*b.Num(), k)
}

// Div es la división de punto flotante (/): SINGLE salvo que haya un DOUBLE.
func Div(a, b Value) (Value, error) {
	k, err := numOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if b.Num() == 0 {
		return Value{}, ErrDivZero
	}
	if k != KDouble {
		k = KSingle
	}
	return fitFloat(a.Num()/b.Num(), k)
}

// intOperands convierte ambos operandos a entero para \, MOD y lógicos:
// INTEGER si ambos son INTEGER, si no LONG.
func intOperands(a, b Value) (int64, int64, Kind, error) {
	if !a.K.IsNumeric() || !b.K.IsNumeric() {
		return 0, 0, 0, ErrTypeMismatch
	}
	k := KLong
	if a.K == KInt && b.K == KInt {
		k = KInt
	}
	x, err := ToLong(a)
	if err != nil {
		return 0, 0, 0, err
	}
	y, err := ToLong(b)
	if err != nil {
		return 0, 0, 0, err
	}
	return x.I, y.I, k, nil
}

func IDiv(a, b Value) (Value, error) {
	x, y, k, err := intOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if y == 0 {
		return Value{}, ErrDivZero
	}
	return fitInt(x/y, k)
}

func Mod(a, b Value) (Value, error) {
	x, y, k, err := intOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if y == 0 {
		return Value{}, ErrDivZero
	}
	return fitInt(x%y, k)
}

func Pow(a, b Value) (Value, error) {
	k, err := numOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	if k != KDouble {
		k = KSingle
	}
	x, y := a.Num(), b.Num()
	if x == 0 && y < 0 {
		return Value{}, ErrDivZero
	}
	if x < 0 && y != math.Trunc(y) {
		return Value{}, ErrIllegalCall
	}
	return fitFloat(math.Pow(x, y), k)
}

func Neg(a Value) (Value, error) {
	switch a.K {
	case KInt, KLong:
		return fitInt(-a.I, a.K)
	case KSingle:
		return Single(-a.F), nil
	case KDouble:
		return Double(-a.F), nil
	}
	return Value{}, ErrTypeMismatch
}

// Compare devuelve -1, 0 o 1.
func Compare(a, b Value) (int, error) {
	if a.K == KString || b.K == KString {
		if a.K != b.K {
			return 0, ErrTypeMismatch
		}
		return strings.Compare(a.S, b.S), nil
	}
	if !a.K.IsNumeric() || !b.K.IsNumeric() {
		return 0, ErrTypeMismatch
	}
	if a.K <= KLong && b.K <= KLong {
		switch {
		case a.I < b.I:
			return -1, nil
		case a.I > b.I:
			return 1, nil
		}
		return 0, nil
	}
	x, y := a.Num(), b.Num()
	if a.K == KSingle || b.K == KSingle {
		if maxKind(a.K, b.K) != KDouble { // comparación en precisión simple
			x, y = float64(float32(x)), float64(float32(y))
		}
	}
	switch {
	case x < y:
		return -1, nil
	case x > y:
		return 1, nil
	}
	return 0, nil
}

// Logic aplica un operador lógico/bit a bit.
func Logic(op Op, a, b Value) (Value, error) {
	x, y, k, err := intOperands(a, b)
	if err != nil {
		return Value{}, err
	}
	var r int64
	switch op {
	case OpAnd:
		r = x & y
	case OpOr:
		r = x | y
	case OpXor:
		r = x ^ y
	case OpEqv:
		r = ^(x ^ y)
	case OpImp:
		r = ^x | y
	}
	if k == KInt {
		return Int(int64(int16(r))), nil
	}
	return Long(int64(int32(r))), nil
}

func Not(a Value) (Value, error) {
	if !a.K.IsNumeric() {
		return Value{}, ErrTypeMismatch
	}
	if a.K == KInt {
		return Int(^a.I), nil
	}
	x, err := ToLong(a)
	if err != nil {
		return Value{}, err
	}
	return Long(int64(int32(^x.I))), nil
}
