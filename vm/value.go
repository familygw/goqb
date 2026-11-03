// Package vm es la máquina virtual de goqb: valores con la semántica de
// QuickBASIC 4.5, intérprete de bytecode y runtime (consola, archivos,
// funciones intrínsecas).
package vm

import (
	"math"
	"strings"
)

// Kind es el tipo dinámico de un Value.
type Kind uint8

const (
	KNone   Kind = iota
	KInt         // INTEGER (16 bits)
	KLong        // LONG (32 bits)
	KSingle      // SINGLE (float32)
	KDouble      // DOUBLE
	KString      // STRING (I > 0 → longitud fija)
	KRecord      // variable de tipo TYPE
	KArray       // array (en un slot de variable)
	KPtr         // referencia a otro Value (parámetros por referencia, direcciones)
)

var kindNames = [...]string{"NONE", "INTEGER", "LONG", "SINGLE", "DOUBLE", "STRING", "RECORD", "ARRAY", "PTR"}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return "?"
}

// IsNumeric reporta si k es un tipo numérico.
func (k Kind) IsNumeric() bool { return k >= KInt && k <= KDouble }

// Value es una celda de almacenamiento o un valor en la pila de la VM.
// Las variables se inicializan con el cero de su tipo, y la asignación
// convierte al tipo de la celda destino (ver Assign).
type Value struct {
	K Kind
	I int64   // INTEGER/LONG; en STRING de longitud fija, la longitud
	F float64 // SINGLE/DOUBLE
	S string  // STRING (bytes CP437)
	P *Value  // KPtr
	R *Record // KRecord
	A *Array  // KArray
}

// FieldDef es un campo de un TYPE.
type FieldDef struct {
	Name  string
	Proto Value // valor cero del campo
	Size  int   // bytes en disco (GET/PUT)
}

// RecordType describe un TYPE...END TYPE.
type RecordType struct {
	Name   string
	Fields []FieldDef
	Size   int
}

// Record es una instancia de un TYPE.
type Record struct {
	T *RecordType
	F []Value
}

// Array es un array de QB, en orden por columnas (el primer índice varía más rápido).
type Array struct {
	Proto  Value // valor cero de cada elemento
	Lo, Hi []int
	Data   []Value
	Static bool
}

// Constructores.
func Int(i int64) Value      { return Value{K: KInt, I: i} }
func Long(i int64) Value     { return Value{K: KLong, I: i} }
func Single(f float64) Value { return Value{K: KSingle, F: float64(float32(f))} }
func Double(f float64) Value { return Value{K: KDouble, F: f} }
func Str(s string) Value     { return Value{K: KString, S: s} }
func Bool(b bool) Value {
	if b {
		return Int(-1)
	}
	return Int(0)
}

// FixedStr es el cero de un STRING * n.
func FixedStr(n int) Value { return Value{K: KString, I: int64(n), S: strings.Repeat("\x00", n)} }

// NewRecord crea una instancia con sus campos en cero.
func NewRecord(t *RecordType) *Record {
	r := &Record{T: t, F: make([]Value, len(t.Fields))}
	for i, f := range t.Fields {
		r.F[i] = f.Proto.Clone()
	}
	return r
}

// RecordValue es el cero de una variable de tipo t.
func RecordValue(t *RecordType) Value { return Value{K: KRecord, R: NewRecord(t)} }

// Clone devuelve una copia profunda (registros), apta para almacenar.
func (v Value) Clone() Value {
	if v.K == KRecord && v.R != nil {
		r := &Record{T: v.R.T, F: make([]Value, len(v.R.F))}
		for i, f := range v.R.F {
			r.F[i] = f.Clone()
		}
		v.R = r
	}
	return v
}

// Num devuelve el valor numérico como float64.
func (v Value) Num() float64 {
	switch v.K {
	case KInt, KLong:
		return float64(v.I)
	case KSingle, KDouble:
		return v.F
	}
	return 0
}

// IsTrue: cualquier número distinto de cero.
func (v Value) IsTrue() bool {
	switch v.K {
	case KInt, KLong:
		return v.I != 0
	case KSingle, KDouble:
		return v.F != 0
	}
	return false
}

// Deref sigue referencias hasta la celda real.
func (v *Value) Deref() *Value {
	for v.K == KPtr {
		v = v.P
	}
	return v
}

// ------------------------------------------------------------- conversiones

// roundEven redondea al par más cercano, como CINT/CLNG de QB.
func roundEven(f float64) float64 { return math.RoundToEven(f) }

// ToInt convierte a INTEGER con verificación de overflow.
func ToInt(v Value) (Value, error) {
	switch v.K {
	case KInt:
		return v, nil
	case KLong:
		if v.I < math.MinInt16 || v.I > math.MaxInt16 {
			return v, ErrOverflow
		}
		return Int(v.I), nil
	case KSingle, KDouble:
		r := roundEven(v.F)
		if math.IsNaN(r) || r < math.MinInt16 || r > math.MaxInt16 {
			return v, ErrOverflow
		}
		return Int(int64(r)), nil
	}
	return v, ErrTypeMismatch
}

// ToLong convierte a LONG con verificación de overflow.
func ToLong(v Value) (Value, error) {
	switch v.K {
	case KInt, KLong:
		return Long(v.I), nil
	case KSingle, KDouble:
		r := roundEven(v.F)
		if math.IsNaN(r) || r < math.MinInt32 || r > math.MaxInt32 {
			return v, ErrOverflow
		}
		return Long(int64(r)), nil
	}
	return v, ErrTypeMismatch
}

// ToSingle convierte a SINGLE con verificación de overflow.
func ToSingle(v Value) (Value, error) {
	if !v.K.IsNumeric() {
		return v, ErrTypeMismatch
	}
	f := v.Num()
	if math.Abs(f) > math.MaxFloat32 && !math.IsInf(f, 0) || math.IsInf(f, 0) || math.IsNaN(f) {
		return v, ErrOverflow
	}
	return Single(f), nil
}

// ToDouble convierte a DOUBLE.
func ToDouble(v Value) (Value, error) {
	if !v.K.IsNumeric() {
		return v, ErrTypeMismatch
	}
	f := v.Num()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return v, ErrOverflow
	}
	return Double(f), nil
}

// Convert convierte v al tipo numérico o string k.
func Convert(v Value, k Kind) (Value, error) {
	switch k {
	case KInt:
		return ToInt(v)
	case KLong:
		return ToLong(v)
	case KSingle:
		return ToSingle(v)
	case KDouble:
		return ToDouble(v)
	case KString:
		if v.K != KString {
			return v, ErrTypeMismatch
		}
		return Value{K: KString, S: v.S}, nil
	}
	return v, nil
}

// Assign guarda src en la celda dst respetando el tipo de dst.
func Assign(dst *Value, src Value) error {
	dst = dst.Deref()
	switch dst.K {
	case KInt, KLong, KSingle, KDouble:
		c, err := Convert(src, dst.K)
		if err != nil {
			return err
		}
		dst.I, dst.F = c.I, c.F
		return nil
	case KString:
		if src.K != KString {
			return ErrTypeMismatch
		}
		if dst.I > 0 { // longitud fija: alinea a izquierda y rellena con espacios
			dst.S = padRight(src.S, int(dst.I))
		} else {
			dst.S = src.S
		}
		return nil
	case KRecord:
		if src.K != KRecord || src.R.T != dst.R.T {
			return ErrTypeMismatch
		}
		*dst = src.Clone()
		return nil
	case KNone:
		*dst = src.Clone()
		return nil
	}
	return ErrTypeMismatch
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return strings.Repeat(" ", n-len(s)) + s
}

// ------------------------------------------------------------- arrays

// NewArray crea un array con los límites dados.
func NewArray(proto Value, lo, hi []int) (*Array, error) {
	n := 1
	for i := range lo {
		if hi[i] < lo[i] {
			return nil, ErrSubscript
		}
		n *= hi[i] - lo[i] + 1
		if n > 16*1024*1024 {
			return nil, ErrOutOfMemory
		}
	}
	a := &Array{Proto: proto, Lo: append([]int(nil), lo...), Hi: append([]int(nil), hi...), Data: make([]Value, n)}
	for i := range a.Data {
		a.Data[i] = proto.Clone()
	}
	return a, nil
}

// Elem devuelve la celda del elemento con los índices dados.
func (a *Array) Elem(idx []int) (*Value, error) {
	if len(idx) != len(a.Lo) {
		return nil, ErrSubscript
	}
	off, mul := 0, 1
	for i, x := range idx {
		if x < a.Lo[i] || x > a.Hi[i] {
			return nil, ErrSubscript
		}
		off += (x - a.Lo[i]) * mul
		mul *= a.Hi[i] - a.Lo[i] + 1
	}
	return &a.Data[off], nil
}

// Clear pone todos los elementos en cero (ERASE de array estático).
func (a *Array) Clear() {
	for i := range a.Data {
		a.Data[i] = a.Proto.Clone()
	}
}
