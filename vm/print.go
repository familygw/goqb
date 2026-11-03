package vm

import (
	"math"
	"strconv"
	"strings"
)

// printer es el destino de PRINT/WRITE: la consola, un archivo o la impresora.
type printer interface {
	write(s string)
	col() int   // columna actual, 1-based
	width() int // 0 = sin límite
	newline()
}

type consolePrinter struct{ c Console }

func (p consolePrinter) write(s string) { p.c.Print(s) }
func (p consolePrinter) col() int       { _, c := p.c.Cursor(); return c }
func (p consolePrinter) width() int     { w, _ := p.c.Size(); return w }
func (p consolePrinter) newline()       { p.c.Newline() }

// nullPrinter descarta la salida (LPRINT sin impresora).
type nullPrinter struct{ c int }

func (p *nullPrinter) write(s string) { p.c += len(s) }
func (p *nullPrinter) col() int       { return p.c + 1 }
func (p *nullPrinter) width() int     { return 80 }
func (p *nullPrinter) newline()       { p.c = 0 }

var lpt = &nullPrinter{}

func (m *VM) printer() printer {
	if m.out != nil {
		return m.out
	}
	return consolePrinter{m.Con}
}

// printValue imprime un valor como PRINT.
func (m *VM) printValue(v Value) error {
	p := m.printer()
	var s string
	switch {
	case v.K == KString:
		s = v.S
	case v.K.IsNumeric():
		s = PrintNumber(v)
		// un número no se parte entre líneas
		if w := p.width(); w > 0 && p.col() > 1 && p.col()+len(s)-1 > w {
			p.newline()
		}
	default:
		return ErrTypeMismatch
	}
	p.write(s)
	return nil
}

func (m *VM) printComma() {
	p := m.printer()
	col := p.col()
	next := ((col-1)/14+1)*14 + 1
	if w := p.width(); w > 0 && next > w {
		p.newline()
		return
	}
	p.write(strings.Repeat(" ", next-col))
}

func (m *VM) printTab(n int) {
	p := m.printer()
	if w := p.width(); w > 0 && n > w {
		n = (n-1)%w + 1
	}
	if n < 1 {
		n = 1
	}
	col := p.col()
	if col > n {
		p.newline()
		col = 1
	}
	p.write(strings.Repeat(" ", n-col))
}

func (m *VM) printSpc(n int) {
	p := m.printer()
	if w := p.width(); w > 0 && n > w {
		n %= w
	}
	if n > 0 {
		p.write(strings.Repeat(" ", n))
	}
}

// writeValue imprime un valor como WRITE: strings entre comillas, números sin espacios.
func (m *VM) writeValue(v Value, last bool) error {
	p := m.printer()
	var b strings.Builder
	switch {
	case v.K == KString:
		b.WriteByte('"')
		b.WriteString(v.S)
		b.WriteByte('"')
	case v.K.IsNumeric():
		b.WriteString(FormatNumber(v))
	default:
		return ErrTypeMismatch
	}
	if !last {
		b.WriteByte(',')
	}
	p.write(b.String())
	if last {
		p.newline()
	}
	return nil
}

// ------------------------------------------------------------- PRINT USING

type usingState struct {
	fmt    string
	pos    int
	active bool
	any    bool // se formateó al menos un campo en esta sentencia
}

func (m *VM) usingBegin(f string) error {
	if f == "" {
		return ErrIllegalCall
	}
	m.using = usingState{fmt: f, active: true}
	return nil
}

// usingValue formatea un valor con el siguiente campo del formato.
func (m *VM) usingValue(v Value) error {
	u := &m.using
	p := m.printer()
	var lit strings.Builder
	wrapped := false
	for {
		if u.pos >= len(u.fmt) {
			if wrapped || !hasField(u.fmt) {
				return ErrIllegalCall
			}
			u.pos, wrapped = 0, true
		}
		f, n := parseField(u.fmt[u.pos:])
		if f == nil {
			c := u.fmt[u.pos]
			if c == '_' && u.pos+1 < len(u.fmt) {
				lit.WriteByte(u.fmt[u.pos+1])
				u.pos += 2
				continue
			}
			lit.WriteByte(c)
			u.pos++
			continue
		}
		p.write(lit.String())
		u.pos += n
		s, err := f.format(v)
		if err != nil {
			return err
		}
		p.write(s)
		u.any = true
		return nil
	}
}

// usingEnd emite el texto literal restante hasta el próximo campo.
func (m *VM) usingEnd(newline bool) {
	u := &m.using
	p := m.printer()
	var lit strings.Builder
	for u.pos < len(u.fmt) {
		if f, _ := parseField(u.fmt[u.pos:]); f != nil {
			break
		}
		if u.fmt[u.pos] == '_' && u.pos+1 < len(u.fmt) {
			u.pos++
		}
		lit.WriteByte(u.fmt[u.pos])
		u.pos++
	}
	p.write(lit.String())
	if newline {
		p.newline()
	}
	m.using.active = false
}

func hasField(f string) bool {
	for i := 0; i < len(f); i++ {
		if f[i] == '_' {
			i++
			continue
		}
		if fd, _ := parseField(f[i:]); fd != nil {
			return true
		}
	}
	return false
}

type usingField struct {
	str     byte // '!', '&', '\\' o 0 para numérico
	strLen  int
	plus    bool // + inicial
	trailPM byte // '+' o '-' final
	stars   bool // **
	dollar  bool // $$
	intDig  int
	decDig  int
	point   bool
	comma   bool
	exp     int // cantidad de ^ (4 o 5)
}

// parseField reconoce un campo de formato al inicio de s.
func parseField(s string) (*usingField, int) {
	if s == "" {
		return nil, 0
	}
	switch s[0] {
	case '!':
		return &usingField{str: '!'}, 1
	case '&':
		return &usingField{str: '&'}, 1
	case '\\':
		j := 1
		for j < len(s) && s[j] == ' ' {
			j++
		}
		if j < len(s) && s[j] == '\\' {
			return &usingField{str: '\\', strLen: j + 1}, j + 1
		}
		return nil, 0
	}
	f := &usingField{}
	i := 0
	if s[i] == '+' {
		f.plus = true
		i++
	}
	start := i
	switch {
	case strings.HasPrefix(s[i:], "**$"):
		f.stars, f.dollar = true, true
		f.intDig += 3
		i += 3
	case strings.HasPrefix(s[i:], "**"):
		f.stars = true
		f.intDig += 2
		i += 2
	case strings.HasPrefix(s[i:], "$$"):
		f.dollar = true
		f.intDig += 2
		i += 2
	}
	for i < len(s) && (s[i] == '#' || s[i] == ',') {
		if s[i] == ',' {
			f.comma = true
		}
		f.intDig++
		i++
	}
	if i < len(s) && s[i] == '.' && (i+1 < len(s) && s[i+1] == '#' || i > start) {
		f.point = true
		i++
		for i < len(s) && s[i] == '#' {
			f.decDig++
			i++
		}
	}
	if i == start { // no hay dígitos
		return nil, 0
	}
	if f.intDig == 0 && !f.point {
		return nil, 0
	}
	if strings.HasPrefix(s[i:], "^^^^^") {
		f.exp, i = 5, i+5
	} else if strings.HasPrefix(s[i:], "^^^^") {
		f.exp, i = 4, i+4
	}
	if !f.plus && i < len(s) && (s[i] == '+' || s[i] == '-') {
		f.trailPM = s[i]
		i++
	}
	return f, i
}

func (f *usingField) format(v Value) (string, error) {
	if f.str != 0 {
		if v.K != KString {
			return "", ErrTypeMismatch
		}
		switch f.str {
		case '!':
			if v.S == "" {
				return " ", nil
			}
			return v.S[:1], nil
		case '&':
			return v.S, nil
		}
		return padRight(v.S, f.strLen), nil
	}
	if !v.K.IsNumeric() {
		return "", ErrTypeMismatch
	}
	x := v.Num()
	neg := x < 0
	ax := math.Abs(x)
	width := f.intDig + f.decDig
	if f.point {
		width++
	}
	var body string
	if f.exp > 0 {
		digits := f.intDig
		if !f.plus && f.trailPM == 0 && digits > 0 {
			digits-- // una posición para el signo
		}
		e := 0
		if ax != 0 {
			e = int(math.Floor(math.Log10(ax))) - digits + 1
		}
		mant := ax / math.Pow(10, float64(e))
		ms := strconv.FormatFloat(mant, 'f', f.decDig, 64)
		if ip, _, _ := strings.Cut(ms, "."); len(ip) > digits { // el redondeo agregó un dígito
			e++
			mant = ax / math.Pow(10, float64(e))
			ms = strconv.FormatFloat(mant, 'f', f.decDig, 64)
		}
		if !f.point {
			ms, _, _ = strings.Cut(ms, ".")
		}
		var b strings.Builder
		b.WriteString(ms)
		b.WriteByte('E')
		if e < 0 {
			b.WriteByte('-')
			e = -e
		} else {
			b.WriteByte('+')
		}
		es := strconv.Itoa(e)
		b.WriteString(strings.Repeat("0", f.exp-2-len(es)))
		b.WriteString(es)
		body = b.String()
		width += f.exp
	} else {
		s := strconv.FormatFloat(ax, 'f', f.decDig, 64)
		ip, dp, _ := strings.Cut(s, ".")
		if f.comma {
			ip = withCommas(ip)
		}
		if ip == "0" && f.intDig-boolInt(f.dollar)*2-boolInt(f.stars)*2 <= 0 && f.point {
			ip = ""
		}
		var b strings.Builder
		b.WriteString(ip)
		if f.point {
			b.WriteByte('.')
			b.WriteString(dp)
		}
		body = b.String()
	}

	// signo
	var pre, post string
	switch {
	case f.plus:
		if neg {
			pre = "-"
		} else {
			pre = "+"
		}
		width++
	case f.trailPM == '+':
		if neg {
			post = "-"
		} else {
			post = "+"
		}
		width++
	case f.trailPM == '-':
		if neg {
			post = "-"
		} else {
			post = " "
		}
		width++
	case neg:
		pre = "-"
	}
	if f.dollar {
		pre += "$"
	}
	s := pre + body
	fill := " "
	if f.stars {
		fill = "*"
	}
	out := s
	if len(s) < width-len(post) {
		out = strings.Repeat(fill, width-len(post)-len(s)) + s
	} else if len(s) > width-len(post) {
		out = "%" + s // no entra en el campo
	}
	return out + post, nil
}

func withCommas(s string) string {
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	if lead > 0 {
		b.WriteString(s[:lead])
	}
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ------------------------------------------------------------- INPUT

// splitInput separa los campos de una línea de INPUT (comas, comillas).
func splitInput(line string) []inField {
	var out []inField
	i := 0
	for {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i < len(line) && line[i] == '"' {
			j := strings.IndexByte(line[i+1:], '"')
			var f inField
			if j < 0 {
				f = inField{line[i+1:], true}
				i = len(line)
			} else {
				f = inField{line[i+1 : i+1+j], true}
				i += j + 2
			}
			out = append(out, f)
			for i < len(line) && line[i] != ',' {
				i++
			}
		} else {
			j := strings.IndexByte(line[i:], ',')
			if j < 0 {
				out = append(out, inField{strings.TrimRight(line[i:], " "), false})
				i = len(line)
			} else {
				out = append(out, inField{strings.TrimRight(line[i:i+j], " "), false})
				i += j
			}
		}
		if i >= len(line) {
			return out
		}
		i++ // coma
	}
}

type inField struct {
	text   string
	quoted bool
}

// assignInput asigna los campos a las variables; false si hay que repetir.
func assignInput(fields []inField, vars []Value) (bool, error) {
	if len(fields) != len(vars) {
		return false, nil
	}
	for i, v := range vars {
		dst := v.P.Deref()
		if dst.K == KString {
			if err := Assign(dst, Str(fields[i].text)); err != nil {
				return false, err
			}
			continue
		}
		if fields[i].quoted {
			return false, nil
		}
		f, ok := ParseInputNumber(fields[i].text)
		if !ok {
			return false, nil
		}
		if err := Assign(dst, Double(f)); err != nil {
			return false, nil // overflow: "Redo from start"
		}
	}
	return true, nil
}

// consoleInput implementa INPUT [;] ["prompt"{;|,}] vars desde el teclado.
func (m *VM) consoleInput(noNewline, question bool, prompt string, vars []Value) error {
	for {
		m.Con.Print(prompt)
		if question {
			m.Con.Print("? ")
		}
		line, err := m.Con.ReadLine()
		if err != nil {
			return m.inputErr(err)
		}
		if !noNewline {
			m.Con.Newline()
		}
		ok, err := assignInput(splitInput(line), vars)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		if noNewline {
			m.Con.Newline()
		}
		m.Con.Print("Redo from start")
		m.Con.Newline()
	}
}

func (m *VM) inputErr(err error) error {
	if m.stop.Load() {
		return errStopped
	}
	if err != nil && err.Error() == "EOF" {
		return errEnd // fin de la entrada estándar: termina el programa
	}
	return errStopped
}
