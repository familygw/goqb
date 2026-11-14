package vm

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math"
	"os"
	"strings"
)

// Modos de OPEN.
const (
	ModeInput = iota + 1
	ModeOutput
	ModeAppend
	ModeRandom
	ModeBinary
)

type fieldVar struct {
	ref   *Value
	width int
}

type qfile struct {
	f      *os.File
	mode   int
	reclen int
	r      *bufio.Reader
	w      *bufio.Writer
	pos    int64 // posición (0-based) para RANDOM/BINARY
	recno  int64 // último registro accedido (RANDOM)
	column int
	wid    int
	fields []fieldVar
	nread  int64 // bytes leídos (INPUT) para LOC
	name   string
}

func (q *qfile) write(s string) {
	if q.w == nil {
		return
	}
	q.w.WriteString(s)
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		q.column = len(s) - i - 1
	} else {
		q.column += len(s)
	}
}
func (q *qfile) col() int   { return q.column + 1 }
func (q *qfile) width() int { return q.wid }
func (q *qfile) newline()   { q.write("\r\n") }

func osErr(err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return ErrFileNotFound
	case errors.Is(err, fs.ErrPermission):
		return ErrPermission
	case errors.Is(err, fs.ErrExist):
		return ErrFileExists
	}
	return ErrPathAccess
}

func fileNum(v Value) (int, error) {
	n, err := ToInt(v)
	if err != nil {
		return 0, err
	}
	if n.I < 1 || n.I > 255 {
		return 0, ErrBadFileNumber
	}
	return int(n.I), nil
}

func (m *VM) file(v Value) (*qfile, error) {
	n, err := fileNum(v)
	if err != nil {
		return nil, err
	}
	q := m.files[n]
	if q == nil {
		return nil, ErrBadFileNumber
	}
	return q, nil
}

// open implementa OPEN archivo FOR modo AS #n [LEN = largo].
func (m *VM) open(name string, mode int, num Value, reclen int) error {
	n, err := fileNum(num)
	if err != nil {
		return err
	}
	if m.files[n] != nil {
		return ErrFileOpen
	}
	if name == "" {
		return ErrBadFileName
	}
	for _, q := range m.files {
		if q.name == name && (mode != ModeInput || q.mode != ModeInput) {
			return ErrFileOpen
		}
	}
	q := &qfile{mode: mode, reclen: reclen, name: name}
	var f *os.File
	switch mode {
	case ModeInput:
		f, err = os.Open(name)
		if err == nil {
			q.r = bufio.NewReader(f)
		}
	case ModeOutput:
		f, err = os.Create(name)
		if err == nil {
			q.w = bufio.NewWriter(f)
		}
	case ModeAppend:
		f, err = os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err == nil {
			q.w = bufio.NewWriter(f)
		}
	case ModeRandom, ModeBinary:
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE, 0o644)
		if q.reclen <= 0 {
			q.reclen = 128
		}
	default:
		return ErrBadFileMode
	}
	if err != nil {
		return osErr(err)
	}
	q.f = f
	m.files[n] = q
	return nil
}

func (m *VM) closeFile(n int) {
	if q := m.files[n]; q != nil {
		if q.w != nil {
			q.w.Flush()
		}
		q.f.Close()
		delete(m.files, n)
		if m.out == printer(q) {
			m.out = nil
		}
	}
}

func (m *VM) closeAll() {
	for n := range m.files {
		m.closeFile(n)
	}
}

func (m *VM) flushFiles() {
	for _, q := range m.files {
		if q.w != nil {
			q.w.Flush()
		}
	}
}

func (q *qfile) size() int64 {
	if q.w != nil {
		q.w.Flush()
	}
	st, err := q.f.Stat()
	if err != nil {
		return 0
	}
	return st.Size()
}

// eof implementa EOF(n).
func (q *qfile) eof() bool {
	switch q.mode {
	case ModeInput:
		b, err := q.r.Peek(1)
		return err != nil || b[0] == 0x1A
	case ModeRandom, ModeBinary:
		return q.pos >= q.size()
	}
	return true
}

// ------------------------------------------------------------- INPUT #

func (q *qfile) readByte() (byte, bool) {
	b, err := q.r.ReadByte()
	if err != nil || b == 0x1A {
		return 0, false
	}
	q.nread++
	return b, true
}

// readLine lee hasta CR/LF (LINE INPUT #).
func (q *qfile) readLine() (string, error) {
	if q.mode != ModeInput {
		return "", ErrBadFileMode
	}
	if q.eof() {
		return "", ErrInputPastEnd
	}
	var sb strings.Builder
	for {
		b, ok := q.readByte()
		if !ok {
			return sb.String(), nil
		}
		if b == '\r' {
			if nb, err := q.r.Peek(1); err == nil && nb[0] == '\n' {
				q.readByte()
			}
			return sb.String(), nil
		}
		if b == '\n' {
			return sb.String(), nil
		}
		sb.WriteByte(b)
	}
}

// readField lee un campo de INPUT #: separado por coma o fin de línea;
// los strings pueden ir entre comillas; los números terminan en espacio.
func (q *qfile) readField(numeric bool) (string, error) {
	if q.mode != ModeInput {
		return "", ErrBadFileMode
	}
	// saltar blancos iniciales (y saltos de línea)
	for {
		b, err := q.r.Peek(1)
		if err != nil || b[0] == 0x1A {
			return "", ErrInputPastEnd
		}
		if b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n' {
			q.readByte()
			continue
		}
		break
	}
	var sb strings.Builder
	b, _ := q.r.Peek(1)
	if b[0] == '"' && !numeric {
		q.readByte()
		for {
			c, ok := q.readByte()
			if !ok || c == '"' {
				break
			}
			sb.WriteByte(c)
		}
		q.skipSeparator()
		return sb.String(), nil
	}
	for {
		nb, err := q.r.Peek(1)
		if err != nil || nb[0] == 0x1A {
			break
		}
		c := nb[0]
		if c == ',' || c == '\r' || c == '\n' || (numeric && (c == ' ' || c == '\t')) {
			break
		}
		q.readByte()
		sb.WriteByte(c)
	}
	q.skipSeparator()
	return strings.TrimRight(sb.String(), " \t"), nil
}

func (q *qfile) skipSeparator() {
	for {
		nb, err := q.r.Peek(1)
		if err != nil {
			return
		}
		switch nb[0] {
		case ' ', '\t':
			q.readByte()
			continue
		case ',':
			q.readByte()
		case '\r':
			q.readByte()
			if nb, err := q.r.Peek(1); err == nil && nb[0] == '\n' {
				q.readByte()
			}
		case '\n':
			q.readByte()
		}
		return
	}
}

// fileInput implementa INPUT #n, vars.
func (m *VM) fileInput(q *qfile, vars []Value) error {
	for _, v := range vars {
		dst := v.P.Deref()
		s, err := q.readField(dst.K != KString)
		if err != nil {
			return err
		}
		if dst.K == KString {
			if err := Assign(dst, Str(s)); err != nil {
				return err
			}
			continue
		}
		f, _ := ParseInputNumber(s)
		if err := Assign(dst, Double(f)); err != nil {
			return err
		}
	}
	return nil
}

// ------------------------------------------------------------- GET / PUT

// encode serializa un valor como en memoria de QB (little endian, IEEE).
func encode(v Value, out []byte) []byte {
	switch v.K {
	case KInt:
		return binary.LittleEndian.AppendUint16(out, uint16(int16(v.I)))
	case KLong:
		return binary.LittleEndian.AppendUint32(out, uint32(int32(v.I)))
	case KSingle:
		return binary.LittleEndian.AppendUint32(out, math.Float32bits(float32(v.F)))
	case KDouble:
		return binary.LittleEndian.AppendUint64(out, math.Float64bits(v.F))
	case KString:
		return append(out, v.S...)
	case KRecord:
		for _, f := range v.R.F {
			out = encode(f, out)
		}
	}
	return out
}

// SizeOf es el tamaño en bytes de un valor (LEN de una variable no string).
func SizeOf(v Value) int { return sizeOf(v) }

// sizeOf es el tamaño en bytes de un valor.
func sizeOf(v Value) int {
	switch v.K {
	case KInt:
		return 2
	case KLong, KSingle:
		return 4
	case KDouble:
		return 8
	case KString:
		return len(v.S)
	case KRecord:
		n := 0
		for _, f := range v.R.F {
			n += sizeOf(f)
		}
		return n
	}
	return 0
}

// decode llena dst desde b y devuelve los bytes consumidos.
func decode(dst *Value, b []byte) int {
	switch dst.K {
	case KInt:
		dst.I = int64(int16(binary.LittleEndian.Uint16(b)))
		return 2
	case KLong:
		dst.I = int64(int32(binary.LittleEndian.Uint32(b)))
		return 4
	case KSingle:
		dst.F = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		return 4
	case KDouble:
		dst.F = math.Float64frombits(binary.LittleEndian.Uint64(b))
		return 8
	case KString:
		n := len(dst.S)
		dst.S = string(b[:n])
		return n
	case KRecord:
		off := 0
		for i := range dst.R.F {
			off += decode(&dst.R.F[i], b[off:])
		}
		return off
	}
	return 0
}

// getPut implementa GET/PUT #n, [pos], [var].
func (m *VM) getPut(put bool, q *qfile, pos Value, hasPos bool, ref *Value) error {
	if q.mode != ModeRandom && q.mode != ModeBinary {
		return ErrBadFileMode
	}
	if hasPos {
		p, err := ToLong(pos)
		if err != nil {
			return err
		}
		if p.I < 1 {
			return ErrBadRecordNum
		}
		if q.mode == ModeRandom {
			q.recno = p.I
			q.pos = (p.I - 1) * int64(q.reclen)
		} else {
			q.pos = p.I - 1
		}
	} else if q.mode == ModeRandom {
		q.recno++
		q.pos = (q.recno - 1) * int64(q.reclen)
	}

	if ref == nil { // FIELD
		if q.mode != ModeRandom {
			return ErrBadFileMode
		}
		buf := make([]byte, q.reclen)
		if put {
			off := 0
			for _, fv := range q.fields {
				s := padRight(load(fv.ref).S, fv.width)
				copy(buf[off:], s)
				off += fv.width
			}
			_, err := q.f.WriteAt(buf, q.pos)
			return wrapIO(err)
		}
		q.f.ReadAt(buf, q.pos)
		off := 0
		for _, fv := range q.fields {
			fv.ref.Deref().S = string(buf[off : off+fv.width])
			off += fv.width
		}
		return nil
	}

	dst := ref.Deref()
	if dst.K == KString && dst.I == 0 && q.mode == ModeRandom {
		// string de longitud variable: ocupa el registro completo
		if put {
			_, err := q.f.WriteAt([]byte(padRight(dst.S, q.reclen)), q.pos)
			return wrapIO(err)
		}
		buf := make([]byte, q.reclen)
		q.f.ReadAt(buf, q.pos)
		dst.S = string(buf)
		return nil
	}
	n := sizeOf(*dst)
	if q.mode == ModeRandom && n > q.reclen {
		return ErrBadRecordLen
	}
	if put {
		b := encode(*dst, nil)
		_, err := q.f.WriteAt(b, q.pos)
		if q.mode == ModeBinary {
			q.pos += int64(len(b))
		}
		return wrapIO(err)
	}
	buf := make([]byte, n)
	k, _ := q.f.ReadAt(buf, q.pos)
	for i := k; i < n; i++ {
		buf[i] = 0
	}
	decode(dst, buf)
	if q.mode == ModeBinary {
		q.pos += int64(n)
	}
	return nil
}

func wrapIO(err error) error {
	if err != nil && err != io.EOF {
		return ErrDeviceIO
	}
	return nil
}
