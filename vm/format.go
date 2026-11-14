package vm

import (
	"math"
	"strconv"
	"strings"
)

// Umbral inferior para la notación científica: |x| < 1E-3 se muestra
// con exponente (QB imprime .001 pero 1E-04). Ajustable si las pruebas
// contra QB 4.5 real en DOSBox muestran otra cosa.
const minFixedExp = -3

// FormatNumber da la representación de QB de un número, sin el espacio
// de signo inicial: "5", "-5", ".5", "1.234568E+07", "1D+16".
func FormatNumber(v Value) string {
	switch v.K {
	case KInt, KLong:
		return strconv.FormatInt(v.I, 10)
	case KSingle:
		return formatFloat(v.F, 7, 32, 'E')
	case KDouble:
		return formatFloat(v.F, 16, 64, 'D')
	}
	return ""
}

func formatFloat(f float64, digits, bits int, expChar byte) string {
	if f == 0 {
		return "0"
	}
	// Mantisa con 'digits' dígitos significativos.
	s := strconv.FormatFloat(f, 'e', digits-1, bits)
	mant, expS, _ := strings.Cut(s, "e")
	exp, _ := strconv.Atoi(expS)
	neg := strings.HasPrefix(mant, "-")
	mant = strings.TrimPrefix(mant, "-")
	ds := strings.Replace(mant, ".", "", 1)
	ds = strings.TrimRight(ds, "0")
	if ds == "" {
		ds = "0"
	}

	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if exp >= digits || exp < minFixedExp {
		b.WriteByte(ds[0])
		if len(ds) > 1 {
			b.WriteByte('.')
			b.WriteString(ds[1:])
		}
		b.WriteByte(expChar)
		if exp < 0 {
			b.WriteByte('-')
			exp = -exp
		} else {
			b.WriteByte('+')
		}
		if exp < 10 {
			b.WriteByte('0')
		}
		b.WriteString(strconv.Itoa(exp))
		return b.String()
	}
	if exp < 0 { // .00123
		b.WriteByte('.')
		b.WriteString(strings.Repeat("0", -exp-1))
		b.WriteString(ds)
		return b.String()
	}
	intDigits := exp + 1
	if len(ds) <= intDigits {
		b.WriteString(ds)
		b.WriteString(strings.Repeat("0", intDigits-len(ds)))
		return b.String()
	}
	b.WriteString(ds[:intDigits])
	b.WriteByte('.')
	b.WriteString(ds[intDigits:])
	return b.String()
}

// PrintNumber es la forma de PRINT: espacio de signo + número + espacio final.
func PrintNumber(v Value) string {
	s := FormatNumber(v)
	var b strings.Builder
	if !strings.HasPrefix(s, "-") {
		b.WriteByte(' ')
	}
	b.WriteString(s)
	b.WriteByte(' ')
	return b.String()
}

// StrNumber es STR$: espacio de signo + número.
func StrNumber(v Value) string {
	s := FormatNumber(v)
	if strings.HasPrefix(s, "-") {
		return s
	}
	var b strings.Builder
	b.WriteByte(' ')
	b.WriteString(s)
	return b.String()
}

// ParseVal implementa VAL: ignora blancos, lee el prefijo numérico más largo.
// Devuelve un DOUBLE (o el entero exacto si corresponde).
func ParseVal(s string) Value {
	// VAL ignora espacios, tabs y saltos de línea en cualquier lugar.
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n':
		default:
			b.WriteByte(s[i])
		}
	}
	t := b.String()
	if len(t) >= 2 && t[0] == '&' {
		return parseRadix(t)
	}
	n := numericPrefix(t)
	if n == 0 {
		return Double(0)
	}
	f, err := strconv.ParseFloat(strings.NewReplacer("D", "E", "d", "E").Replace(t[:n]), 64)
	if err != nil && !math.IsInf(f, 0) {
		return Double(0)
	}
	return Double(f)
}

func parseRadix(t string) Value {
	base, digits := 8, t[1:]
	switch t[1] {
	case 'H', 'h':
		base, digits = 16, t[2:]
	case 'O', 'o':
		digits = t[2:]
	}
	end := 0
	for end < len(digits) && strings.IndexByte("0123456789ABCDEFabcdef"[:base+max(0, base-10)], digits[end]) >= 0 {
		end++
	}
	if end == 0 {
		return Double(0)
	}
	u, err := strconv.ParseUint(digits[:end], base, 32)
	if err != nil {
		return Double(0)
	}
	if u <= 0xFFFF { // como en QB: &HFFFF = -1
		return Double(float64(int16(uint16(u))))
	}
	return Double(float64(int32(uint32(u))))
}

// numericPrefix devuelve la longitud del prefijo con forma de número.
func numericPrefix(t string) int {
	i := 0
	if i < len(t) && (t[i] == '+' || t[i] == '-') {
		i++
	}
	digits := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
		digits++
	}
	if i < len(t) && t[i] == '.' {
		i++
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0
	}
	if i < len(t) && strings.IndexByte("EeDd", t[i]) >= 0 {
		j := i + 1
		if j < len(t) && (t[j] == '+' || t[j] == '-') {
			j++
		}
		k := j
		for k < len(t) && t[k] >= '0' && t[k] <= '9' {
			k++
		}
		if k > j {
			i = k
		}
	}
	// sufijos de tipo opcionales (VAL("12%") es 12)
	return i
}

// ParseInputNumber interpreta un campo numérico de INPUT. ok es false si
// el campo no es un número completo (QB responde "Redo from start").
func ParseInputNumber(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, true
	}
	if t[0] == '&' {
		v := parseRadix(t)
		return v.F, true
	}
	n := numericPrefix(t)
	rest := t[n:]
	if n == 0 || (rest != "" && !(len(rest) == 1 && strings.IndexByte("%&!#", rest[0]) >= 0)) {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.NewReplacer("D", "E", "d", "E").Replace(t[:n]), 64)
	if err != nil && !math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}
