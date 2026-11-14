package vm

import (
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

// Builtin describe una función intrínseca.
type Builtin struct {
	Name string
	// Args describe los argumentos: n numérico, s string, x cualquiera,
	// a array (referencia), v variable (referencia), f número de archivo.
	// Lo que sigue a '|' es opcional.
	Args string
	Ret  Kind
	// SameKind: el resultado tiene el tipo del primer argumento (ABS, INT, FIX).
	SameKind bool
	// FloatKind: SINGLE salvo que el argumento sea DOUBLE (SIN, SQR...).
	FloatKind bool
	Fn        func(m *VM, a []Value) (Value, error)
}

// Builtins es la tabla de funciones intrínsecas (el índice es el operando de OpBuiltin).
var Builtins []Builtin

// BuiltinIndex mapea nombre (mayúsculas, con $) → índice en Builtins.
var BuiltinIndex = map[string]int{}

func reg(b Builtin) {
	BuiltinIndex[b.Name] = len(Builtins)
	Builtins = append(Builtins, b)
}

// none es el marcador de argumento omitido.
var none = Value{}

func isNone(v Value) bool { return v.K == KNone }

func num1(f func(float64) (float64, error)) func(*VM, []Value) (Value, error) {
	return func(m *VM, a []Value) (Value, error) {
		r, err := f(a[0].Num())
		if err != nil {
			return none, err
		}
		if a[0].K == KDouble {
			return fitFloat(r, KDouble)
		}
		return fitFloat(r, KSingle)
	}
}

func intArg(v Value) (int, error) {
	x, err := ToLong(v)
	if err != nil {
		return 0, err
	}
	return int(x.I), nil
}

func init() {
	reg(Builtin{Name: "ABS", Args: "n", SameKind: true, Fn: func(m *VM, a []Value) (Value, error) {
		if a[0].Num() < 0 {
			return Neg(a[0])
		}
		return a[0], nil
	}})
	reg(Builtin{Name: "SGN", Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		x := a[0].Num()
		switch {
		case x > 0:
			return Int(1), nil
		case x < 0:
			return Int(-1), nil
		}
		return Int(0), nil
	}})
	reg(Builtin{Name: "INT", Args: "n", SameKind: true, Fn: func(m *VM, a []Value) (Value, error) {
		if a[0].K <= KLong {
			return a[0], nil
		}
		return fitFloat(math.Floor(a[0].F), a[0].K)
	}})
	reg(Builtin{Name: "FIX", Args: "n", SameKind: true, Fn: func(m *VM, a []Value) (Value, error) {
		if a[0].K <= KLong {
			return a[0], nil
		}
		return fitFloat(math.Trunc(a[0].F), a[0].K)
	}})
	reg(Builtin{Name: "CINT", Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return ToInt(a[0]) }})
	reg(Builtin{Name: "CLNG", Args: "n", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) { return ToLong(a[0]) }})
	reg(Builtin{Name: "CSNG", Args: "n", Ret: KSingle, Fn: func(m *VM, a []Value) (Value, error) { return ToSingle(a[0]) }})
	reg(Builtin{Name: "CDBL", Args: "n", Ret: KDouble, Fn: func(m *VM, a []Value) (Value, error) { return ToDouble(a[0]) }})
	reg(Builtin{Name: "SIN", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) { return math.Sin(x), nil })})
	reg(Builtin{Name: "COS", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) { return math.Cos(x), nil })})
	reg(Builtin{Name: "TAN", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) { return math.Tan(x), nil })})
	reg(Builtin{Name: "ATN", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) { return math.Atan(x), nil })})
	reg(Builtin{Name: "EXP", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) {
		r := math.Exp(x)
		if math.IsInf(r, 0) {
			return 0, ErrOverflow
		}
		return r, nil
	})})
	reg(Builtin{Name: "LOG", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) {
		if x <= 0 {
			return 0, ErrIllegalCall
		}
		return math.Log(x), nil
	})})
	reg(Builtin{Name: "SQR", Args: "n", FloatKind: true, Fn: num1(func(x float64) (float64, error) {
		if x < 0 {
			return 0, ErrIllegalCall
		}
		return math.Sqrt(x), nil
	})})
	reg(Builtin{Name: "RND", Args: "|n", Ret: KSingle, Fn: func(m *VM, a []Value) (Value, error) {
		if len(a) > 0 && !isNone(a[0]) {
			x := a[0].Num()
			if x == 0 {
				return Single(m.lastRnd), nil
			}
			if x < 0 {
				bits := math.Float32bits(float32(x))
				m.seed = (bits + (bits >> 24)) & 0xFFFFFF
			}
		}
		m.seed = (m.seed*0xFD43FD + 0xC39EC3) & 0xFFFFFF
		m.lastRnd = float64(m.seed) / 0x1000000
		return Single(m.lastRnd), nil
	}})
	reg(Builtin{Name: "TIMER", Ret: KSingle, Fn: func(m *VM, a []Value) (Value, error) {
		now := time.Now()
		mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return Single(math.Floor(now.Sub(mid).Seconds()*100) / 100), nil
	}})
	reg(Builtin{Name: "DATE$", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		return Str(time.Now().Format("01-02-2006")), nil
	}})
	reg(Builtin{Name: "TIME$", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		return Str(time.Now().Format("15:04:05")), nil
	}})

	// ---- strings
	reg(Builtin{Name: "LEN", Args: "s", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return Int(int64(len(a[0].S))), nil }})
	reg(Builtin{Name: "ASC", Args: "s", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		if a[0].S == "" {
			return none, ErrIllegalCall
		}
		return Int(int64(a[0].S[0])), nil
	}})
	reg(Builtin{Name: "CHR$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[0])
		if err != nil {
			return none, err
		}
		if n < 0 || n > 255 {
			return none, ErrIllegalCall
		}
		return Str(string([]byte{byte(n)})), nil
	}})
	reg(Builtin{Name: "LEFT$", Args: "sn", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[1])
		if err != nil || n < 0 {
			return none, ErrIllegalCall
		}
		return Str(a[0].S[:min(n, len(a[0].S))]), nil
	}})
	reg(Builtin{Name: "RIGHT$", Args: "sn", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[1])
		if err != nil || n < 0 {
			return none, ErrIllegalCall
		}
		s := a[0].S
		return Str(s[len(s)-min(n, len(s)):]), nil
	}})
	reg(Builtin{Name: "MID$", Args: "sn|n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		s := a[0].S
		st, err := intArg(a[1])
		if err != nil || st < 1 {
			return none, ErrIllegalCall
		}
		n := len(s)
		if len(a) > 2 && !isNone(a[2]) {
			if n, err = intArg(a[2]); err != nil || n < 0 {
				return none, ErrIllegalCall
			}
		}
		if st > len(s) {
			return Str(""), nil
		}
		end := min(len(s), st-1+n)
		return Str(s[st-1 : end]), nil
	}})
	reg(Builtin{Name: "INSTR", Args: "xs|s", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		start := 1
		hay, needle := a[0], a[1]
		if len(a) == 3 && !isNone(a[2]) {
			var err error
			if start, err = intArg(a[0]); err != nil || start < 1 {
				return none, ErrIllegalCall
			}
			hay, needle = a[1], a[2]
		}
		if hay.K != KString || needle.K != KString {
			return none, ErrTypeMismatch
		}
		if start > len(hay.S) {
			return Int(0), nil
		}
		if needle.S == "" {
			return Int(int64(start)), nil
		}
		i := strings.Index(hay.S[start-1:], needle.S)
		if i < 0 {
			return Int(0), nil
		}
		return Int(int64(start + i)), nil
	}})
	reg(Builtin{Name: "LCASE$", Args: "s", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(asciiMap(a[0].S, 'A', 'Z', 32)), nil }})
	reg(Builtin{Name: "UCASE$", Args: "s", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(asciiMap(a[0].S, 'a', 'z', -32)), nil }})
	reg(Builtin{Name: "LTRIM$", Args: "s", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(strings.TrimLeft(a[0].S, " ")), nil }})
	reg(Builtin{Name: "RTRIM$", Args: "s", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(strings.TrimRight(a[0].S, " ")), nil }})
	reg(Builtin{Name: "SPACE$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[0])
		if err != nil || n < 0 || n > maxStringLen {
			return none, ErrIllegalCall
		}
		return Str(strings.Repeat(" ", n)), nil
	}})
	reg(Builtin{Name: "STRING$", Args: "nx", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[0])
		if err != nil || n < 0 || n > maxStringLen {
			return none, ErrIllegalCall
		}
		var c byte
		if a[1].K == KString {
			if a[1].S == "" {
				return none, ErrIllegalCall
			}
			c = a[1].S[0]
		} else {
			code, err := intArg(a[1])
			if err != nil || code < 0 || code > 255 {
				return none, ErrIllegalCall
			}
			c = byte(code)
		}
		return Str(strings.Repeat(string([]byte{c}), n)), nil
	}})
	reg(Builtin{Name: "STR$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(StrNumber(a[0])), nil }})
	reg(Builtin{Name: "VAL", Args: "s", Ret: KDouble, Fn: func(m *VM, a []Value) (Value, error) { return ParseVal(a[0].S), nil }})
	reg(Builtin{Name: "HEX$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return radixStr(a[0], 16) }})
	reg(Builtin{Name: "OCT$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return radixStr(a[0], 8) }})
	reg(Builtin{Name: "MKI$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return mk(a[0], KInt) }})
	reg(Builtin{Name: "MKL$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return mk(a[0], KLong) }})
	reg(Builtin{Name: "MKS$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return mk(a[0], KSingle) }})
	reg(Builtin{Name: "MKD$", Args: "n", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return mk(a[0], KDouble) }})
	reg(Builtin{Name: "CVI", Args: "s", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return cv(a[0].S, Int(0)) }})
	reg(Builtin{Name: "CVL", Args: "s", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) { return cv(a[0].S, Long(0)) }})
	reg(Builtin{Name: "CVS", Args: "s", Ret: KSingle, Fn: func(m *VM, a []Value) (Value, error) { return cv(a[0].S, Single(0)) }})
	reg(Builtin{Name: "CVD", Args: "s", Ret: KDouble, Fn: func(m *VM, a []Value) (Value, error) { return cv(a[0].S, Double(0)) }})

	// ---- consola y sistema
	reg(Builtin{Name: "INKEY$", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		if m.stop.Load() {
			return none, errStopped
		}
		if m.mem != nil && m.mem.pending != "" { // tecla leída por INT 16h
			k := m.mem.pending
			m.mem.pending = ""
			return Str(k), nil
		}
		return Str(m.Con.InKey()), nil
	}})
	reg(Builtin{Name: "INPUT$", Args: "n|f", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		n, err := intArg(a[0])
		if err != nil || n < 1 || n > maxStringLen {
			return none, ErrIllegalCall
		}
		if len(a) > 1 && !isNone(a[1]) {
			q, err := m.file(a[1])
			if err != nil {
				return none, err
			}
			if q.mode == ModeInput {
				var sb strings.Builder
				for i := 0; i < n; i++ {
					b, ok := q.readByte()
					if !ok {
						return none, ErrInputPastEnd
					}
					sb.WriteByte(b)
				}
				return Str(sb.String()), nil
			}
			buf := make([]byte, n)
			k, _ := q.f.ReadAt(buf, q.pos)
			q.pos += int64(k)
			return Str(string(buf[:k])), nil
		}
		var sb strings.Builder
		for sb.Len() < n {
			k, err := m.Con.ReadKey()
			if err != nil {
				return none, m.inputErr(err)
			}
			sb.WriteString(k)
		}
		return Str(sb.String()[:n]), nil
	}})
	reg(Builtin{Name: "CSRLIN", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		r, _ := m.Con.Cursor()
		return Int(int64(r)), nil
	}})
	reg(Builtin{Name: "POS", Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		_, c := m.Con.Cursor()
		return Int(int64(c)), nil
	}})
	reg(Builtin{Name: "SCREEN", Args: "nn|n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		r, err := intArg(a[0])
		if err != nil {
			return none, err
		}
		c, err := intArg(a[1])
		if err != nil {
			return none, err
		}
		w, h := m.Con.Size()
		if r < 1 || r > h || c < 1 || c > w {
			return none, ErrIllegalCall
		}
		ch, attr := m.Con.CharAt(r, c)
		if len(a) > 2 && !isNone(a[2]) && a[2].Num() != 0 {
			return Int(int64(attr)), nil
		}
		return Int(int64(ch)), nil
	}})
	reg(Builtin{Name: "FRE", Args: "x", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) { return Long(400000), nil }})
	reg(Builtin{Name: "ERR", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return Int(int64(m.errCode)), nil }})
	reg(Builtin{Name: "ERL", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) { return Long(int64(m.erl)), nil }})
	reg(Builtin{Name: "COMMAND$", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(strings.ToUpper(m.Command)), nil }})
	reg(Builtin{Name: "ENVIRON$", Args: "x", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) {
		if a[0].K == KString {
			return Str(os.Getenv(strings.ToUpper(a[0].S))), nil
		}
		n, err := intArg(a[0])
		if err != nil || n < 1 {
			return none, ErrIllegalCall
		}
		env := os.Environ()
		if n > len(env) {
			return Str(""), nil
		}
		return Str(env[n-1]), nil
	}})
	reg(Builtin{Name: "FREEFILE", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		for i := 1; i <= 255; i++ {
			if m.files[i] == nil {
				return Int(int64(i)), nil
			}
		}
		return none, ErrTooManyFiles
	}})
	reg(Builtin{Name: "EOF", Args: "f", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		q, err := m.file(a[0])
		if err != nil {
			return none, err
		}
		if q.mode == ModeOutput || q.mode == ModeAppend {
			return none, ErrBadFileMode
		}
		return Bool(q.eof()), nil
	}})
	reg(Builtin{Name: "LOF", Args: "f", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) {
		q, err := m.file(a[0])
		if err != nil {
			return none, err
		}
		return Long(q.size()), nil
	}})
	reg(Builtin{Name: "LOC", Args: "f", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) {
		q, err := m.file(a[0])
		if err != nil {
			return none, err
		}
		switch q.mode {
		case ModeRandom:
			return Long(q.recno), nil
		case ModeBinary:
			return Long(q.pos), nil
		case ModeInput:
			return Long(q.nread / 128), nil
		}
		return Long(q.size() / 128), nil
	}})
	reg(Builtin{Name: "SEEK", Args: "f", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) {
		q, err := m.file(a[0])
		if err != nil {
			return none, err
		}
		switch q.mode {
		case ModeRandom:
			return Long(q.pos/int64(q.reclen) + 1), nil
		case ModeBinary:
			return Long(q.pos + 1), nil
		case ModeInput:
			return Long(q.nread + 1), nil
		}
		return Long(q.size() + 1), nil
	}})
	reg(Builtin{Name: "LBOUND", Args: "a|n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return bound(a, true) }})
	reg(Builtin{Name: "UBOUND", Args: "a|n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return bound(a, false) }})

	// Dispositivos sin soporte: devuelven 0 (como sin hardware) o error 73.
	for _, n := range []string{"PEN", "STICK", "STRIG", "PLAY", "LPOS"} {
		reg(Builtin{Name: n, Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return Int(0), nil }})
	}
	reg(Builtin{Name: "SETMEM", Args: "n", Ret: KLong, Fn: func(m *VM, a []Value) (Value, error) { return Long(400000), nil }})
	reg(Builtin{Name: "ERDEV", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) { return Int(0), nil }})
	reg(Builtin{Name: "ERDEV$", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return Str(""), nil }})
	reg(Builtin{Name: "IOCTL$", Args: "f", Ret: KString, Fn: func(m *VM, a []Value) (Value, error) { return none, ErrAdvancedFeature }})
}

func asciiMap(s string, lo, hi byte, delta int) string {
	b := []byte(s)
	for i, c := range b {
		if c >= lo && c <= hi {
			b[i] = byte(int(c) + delta)
		}
	}
	return string(b)
}

func radixStr(v Value, base int) (Value, error) {
	x, err := ToLong(v)
	if err != nil {
		return none, err
	}
	u := uint64(uint32(int32(x.I)))
	if x.I >= -32768 && x.I < 0 {
		u = uint64(uint16(int16(x.I)))
	}
	return Str(strings.ToUpper(strconv.FormatUint(u, base))), nil
}

func mk(v Value, k Kind) (Value, error) {
	c, err := Convert(v, k)
	if err != nil {
		return none, err
	}
	return Str(string(encode(c, nil))), nil
}

func cv(s string, proto Value) (Value, error) {
	n := sizeOf(proto)
	if len(s) < n {
		return none, ErrIllegalCall
	}
	decode(&proto, []byte(s))
	return proto, nil
}

func bound(a []Value, lower bool) (Value, error) {
	arr := a[0].P.Deref()
	if arr.K != KArray || arr.A == nil {
		return none, ErrSubscript
	}
	d := 1
	if len(a) > 1 && !isNone(a[1]) {
		var err error
		if d, err = intArg(a[1]); err != nil {
			return none, err
		}
	}
	if d < 1 || d > len(arr.A.Lo) {
		return none, ErrSubscript
	}
	if lower {
		return Int(int64(arr.A.Lo[d-1])), nil
	}
	return Int(int64(arr.A.Hi[d-1])), nil
}
