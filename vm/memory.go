package vm

import (
	"os"
	"time"
	"unsafe"

	"goqb/gfx"
)

// Modelo de memoria de 16 bits para VARPTR, VARSEG, SADD, PEEK, POKE,
// BLOAD/BSAVE y el código máquina de CALL ABSOLUTE.
//
// Las direcciones son lineales (seg*16 + off, 20 bits):
//
//	00000-004FF  vectores y área de datos del BIOS (teclado, reloj, video)
//	10000-1FFFF  DGROUP: variables simples y strings (DEF SEG por defecto)
//	20000-8FFFF  arrays (cada uno en su propio segmento, desde el offset 0)
//	90000-9FFFF  pila del código máquina
//	A0000-AFFFF  video gráfico (SCREEN 13)
//	B0000-BFFFF  video de texto
//
// Lo que no corresponde a ninguna variable se guarda en una memoria dispersa,
// así que un POKE seguido de un PEEK devuelve lo escrito.
const (
	dgroupSeg  = 0x1000
	dgroupBase = dgroupSeg << 4
	farBase    = 0x20000
	farLimit   = 0x90000
	stackSeg   = 0x9000
)

type memBlock struct {
	base, size uint32
	arr        *Array // array (segmento propio)
	cell       *Value // variable en DGROUP
	str        bool   // datos del string de cell (SADD)
}

type memory struct {
	blocks  []*memBlock
	byArr   map[*Array]*memBlock
	byCell  map[*Value]*memBlock
	byStr   map[*Value]*memBlock
	farNext uint32
	dgNext  uint32
	ram     map[uint32]byte
	kbFlags byte
	pending string // tecla leída por INT 16h AH=01
	dac     [256 * 3]byte
	dacW    int // índice de escritura del DAC (puerto 3C8h/3C9h)
	dacR    int
	speaker byte
}

func (m *VM) memory() *memory {
	if m.mem == nil {
		m.mem = &memory{byArr: map[*Array]*memBlock{}, byCell: map[*Value]*memBlock{}, byStr: map[*Value]*memBlock{},
			farNext: farBase, dgNext: 0x0200, ram: map[uint32]byte{}}
	}
	return m.mem
}

func align16(n uint32) uint32 { return (n + 15) &^ 15 }

func (mm *memory) add(b *memBlock) {
	mm.blocks = append(mm.blocks, b)
}

// removeRange descarta los bloques que se superponen con [lo, hi).
func (mm *memory) removeRange(lo, hi uint32) {
	out := mm.blocks[:0]
	for _, b := range mm.blocks {
		if b.base < hi && b.base+b.size > lo {
			switch {
			case b.arr != nil:
				delete(mm.byArr, b.arr)
			case b.str:
				delete(mm.byStr, b.cell)
			default:
				delete(mm.byCell, b.cell)
			}
			continue
		}
		out = append(out, b)
	}
	mm.blocks = out
}

func elemSize(proto Value) uint32 {
	if proto.K == KString && proto.I == 0 {
		return 4 // descriptor de string variable
	}
	n := uint32(sizeOf(proto))
	if n == 0 {
		n = 2
	}
	return n
}

// arrayBlock devuelve (creándolo si hace falta) el segmento de un array.
func (mm *memory) arrayBlock(a *Array) *memBlock {
	if b := mm.byArr[a]; b != nil && b.size == uint32(len(a.Data))*elemSize(a.Proto) {
		return b
	}
	size := uint32(len(a.Data)) * elemSize(a.Proto)
	if size == 0 {
		size = 16
	}
	if mm.farNext+align16(size) > farLimit {
		mm.farNext = farBase
	}
	b := &memBlock{base: mm.farNext, size: size, arr: a}
	mm.removeRange(b.base, b.base+align16(size))
	mm.farNext += align16(size)
	mm.byArr[a] = b
	mm.add(b)
	return b
}

// dgAlloc reserva n bytes en DGROUP.
func (mm *memory) dgAlloc(n uint32) uint32 {
	n = (n + 1) &^ 1
	if mm.dgNext+n > 0xFFF0 {
		mm.dgNext = 0x0200
	}
	off := mm.dgNext
	mm.dgNext += n
	mm.removeRange(dgroupBase+off, dgroupBase+off+n)
	return dgroupBase + off
}

func (mm *memory) cellBlock(p *Value) *memBlock {
	size := elemSize(*p)
	if b := mm.byCell[p]; b != nil && b.size == size {
		return b
	}
	b := &memBlock{base: mm.dgAlloc(size), size: size, cell: p}
	mm.byCell[p] = b
	mm.add(b)
	return b
}

func (mm *memory) strBlock(p *Value) *memBlock {
	n := uint32(len(p.S))
	if b := mm.byStr[p]; b != nil && n <= b.size {
		return b
	}
	size := n
	if size < 16 {
		size = 16
	}
	b := &memBlock{base: mm.dgAlloc(size), size: size, cell: p, str: true}
	mm.byStr[p] = b
	mm.add(b)
	return b
}

// findIn busca p dentro de v (v mismo o un campo de registro) y devuelve su offset en bytes.
func findIn(v *Value, p *Value) (uint32, bool) {
	if v == p {
		return 0, true
	}
	if v.K == KRecord && v.R != nil {
		off := uint32(0)
		for i := range v.R.F {
			if o, ok := findIn(&v.R.F[i], p); ok {
				return off + o, true
			}
			off += uint32(sizeOf(v.R.F[i]))
		}
	}
	return 0, false
}

// arrayElem busca a qué array pertenece la celda p.
func arrayElem(a *Array, p *Value) (uint32, bool) {
	if a == nil || len(a.Data) == 0 {
		return 0, false
	}
	sz := unsafe.Sizeof(Value{})
	base := uintptr(unsafe.Pointer(&a.Data[0]))
	q := uintptr(unsafe.Pointer(p))
	es := elemSize(a.Proto)
	if q >= base && q < base+uintptr(len(a.Data))*sz {
		return uint32((q-base)/sz) * es, true
	}
	if a.Proto.K == KRecord {
		for i := range a.Data {
			if o, ok := findIn(&a.Data[i], p); ok {
				return uint32(i)*es + o, true
			}
		}
	}
	return 0, false
}

// varAddr devuelve la dirección lineal de una variable (VARPTR/VARSEG).
func (m *VM) varAddr(p *Value) uint32 {
	mm := m.memory()
	check := func(v *Value) (uint32, bool) {
		v = v.Deref()
		if v.K == KArray && v.A != nil {
			if off, ok := arrayElem(v.A, p); ok {
				return mm.arrayBlock(v.A).base + off, true
			}
		}
		return 0, false
	}
	for i := range m.Globals {
		if a, ok := check(&m.Globals[i]); ok {
			return a
		}
	}
	for fi := len(m.frames) - 1; fi >= 0; fi-- {
		f := &m.frames[fi]
		for i := range f.locals {
			if a, ok := check(&f.locals[i]); ok {
				return a
			}
		}
	}
	return mm.cellBlock(p).base
}

func segOff(lin uint32) (int, int) {
	if lin >= dgroupBase && lin < dgroupBase+0x10000 {
		return dgroupSeg, int(lin - dgroupBase)
	}
	return int(lin >> 4), int(lin & 15)
}

// addrParts devuelve segmento y offset de una variable; cada array tiene
// su propio segmento y sus elementos empiezan en el offset 0.
func (m *VM) addrParts(p *Value) (int, int) {
	lin := m.varAddr(p)
	mm := m.memory()
	for _, b := range mm.blocks {
		if b.arr != nil && lin >= b.base && lin < b.base+b.size {
			return int(b.base >> 4), int(lin - b.base)
		}
	}
	return segOff(lin)
}

// ------------------------------------------------------------- acceso a bytes

func (mm *memory) find(lin uint32) *memBlock {
	for i := len(mm.blocks) - 1; i >= 0; i-- {
		b := mm.blocks[i]
		if lin >= b.base && lin < b.base+b.size {
			return b
		}
	}
	return nil
}

func valueBytes(v Value) []byte {
	if v.K == KString && v.I == 0 {
		return []byte{byte(len(v.S)), byte(len(v.S) >> 8), 0, 0}
	}
	return encode(v, nil)
}

func ticksSinceMidnight() uint32 {
	now := time.Now()
	mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return uint32(now.Sub(mid).Seconds() * 18.2065)
}

// peek lee un byte de la memoria emulada.
func (m *VM) peek(lin uint32) byte {
	lin &= 0xFFFFF
	mm := m.memory()
	switch {
	case lin < 0x500:
		w, h := m.Con.Size()
		switch lin {
		case 0x417:
			return mm.kbFlags
		case 0x449:
			return m.biosMode()
		case 0x44A:
			return byte(w)
		case 0x44B:
			return 0
		case 0x484:
			return byte(h - 1)
		case 0x46C, 0x46D, 0x46E, 0x46F:
			t := ticksSinceMidnight()
			return byte(t >> (8 * (lin - 0x46C)))
		}
	case lin >= 0xA0000 && lin < 0xB0000:
		if v, ok := m.vgaPixel(lin-0xA0000, -1); ok {
			return v
		}
	case lin >= 0xB0000 && lin < 0xC0000:
		if v, ok := m.textCell(lin&0x7FFF, -1); ok {
			return v
		}
	}
	if b := mm.find(lin); b != nil {
		off := lin - b.base
		switch {
		case b.str:
			if int(off) < len(b.cell.S) {
				return b.cell.S[off]
			}
			return 0
		case b.arr != nil:
			es := elemSize(b.arr.Proto)
			i := int(off / es)
			if i < len(b.arr.Data) {
				return valueBytes(b.arr.Data[i])[off%es]
			}
		default:
			bs := valueBytes(*b.cell)
			if int(off) < len(bs) {
				return bs[off]
			}
		}
		return 0
	}
	return mm.ram[lin]
}

// poke escribe un byte en la memoria emulada.
func (m *VM) poke(lin uint32, v byte) {
	lin &= 0xFFFFF
	mm := m.memory()
	switch {
	case lin == 0x417:
		mm.kbFlags = v
		return
	case lin >= 0xA0000 && lin < 0xB0000:
		if _, ok := m.vgaPixel(lin-0xA0000, int(v)); ok {
			return
		}
	case lin >= 0xB0000 && lin < 0xC0000:
		if _, ok := m.textCell(lin&0x7FFF, int(v)); ok {
			return
		}
	}
	if b := mm.find(lin); b != nil {
		off := lin - b.base
		switch {
		case b.str:
			if int(off) < len(b.cell.S) {
				s := []byte(b.cell.S)
				s[off] = v
				b.cell.S = string(s)
			}
		case b.arr != nil:
			es := elemSize(b.arr.Proto)
			i := int(off / es)
			if i < len(b.arr.Data) {
				setByte(&b.arr.Data[i], off%es, v)
			}
		default:
			setByte(b.cell, off, v)
		}
		return
	}
	if v == 0 {
		delete(mm.ram, lin)
	} else {
		mm.ram[lin] = v
	}
}

func setByte(dst *Value, off uint32, v byte) {
	if dst.K == KString && dst.I == 0 {
		return // descriptor: no se modifica
	}
	bs := encode(*dst, nil)
	if int(off) >= len(bs) {
		return
	}
	bs[off] = v
	decode(dst, bs)
}

// textCell lee (v < 0) o escribe la memoria de video de texto.
func (m *VM) textCell(off uint32, v int) (byte, bool) {
	if gc, ok := m.gfxCon(); ok && gc.ScreenMode() != 0 {
		return 0, false
	}
	w, h := m.Con.Size()
	if off >= uint32(w*h*2) {
		return 0, false
	}
	r, c := int(off)/(2*w)+1, (int(off)%(2*w))/2+1
	ch, attr := m.Con.CharAt(r, c)
	if v < 0 {
		if off%2 == 0 {
			return ch, true
		}
		return attr, true
	}
	if off%2 == 0 {
		ch = byte(v)
	} else {
		attr = byte(v)
	}
	m.Con.PutChar(r, c, ch, attr)
	return 0, true
}

// vgaPixel accede a la memoria de video de SCREEN 13 (un byte por pixel).
func (m *VM) vgaPixel(off uint32, v int) (byte, bool) {
	gc, ok := m.gfxCon()
	if !ok || gc.ScreenMode() != 13 || off >= 320*200 {
		return 0, false
	}
	var r byte
	gc.WithGfx(func(g *gfx.Screen) error {
		x, y := int(off%320), int(off/320)
		if v < 0 {
			r = byte(g.Pixel(x, y))
		} else {
			g.SetPixel(x, y, v)
		}
		return nil
	})
	return r, true
}

// biosMode devuelve el modo de video del BIOS correspondiente a SCREEN.
func (m *VM) biosMode() byte {
	gc, ok := m.gfxCon()
	if !ok {
		return 3
	}
	switch gc.ScreenMode() {
	case 1:
		return 4
	case 2:
		return 6
	case 7:
		return 0x0D
	case 8:
		return 0x0E
	case 9:
		return 0x10
	case 10:
		return 0x0F
	case 11:
		return 0x11
	case 12:
		return 0x12
	case 13:
		return 0x13
	}
	if w, _ := m.Con.Size(); w == 40 {
		return 1
	}
	return 3
}

// ------------------------------------------------------------- BASIC

func (m *VM) segAddr(off int) uint32 {
	return (uint32(m.defSeg)<<4 + uint32(off&0xFFFF)) & 0xFFFFF
}

func offArg(v Value) (int, error) {
	off, err := intArg(v)
	if err != nil {
		return 0, err
	}
	if off < -32768 || off > 0xFFFF {
		return 0, ErrIllegalCall
	}
	return off & 0xFFFF, nil
}

func signed16(n int) Value {
	return Int(int64(int16(uint16(n))))
}

func init() {
	reg(Builtin{Name: "PEEK", Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		off, err := offArg(a[0])
		if err != nil {
			return none, err
		}
		return Int(int64(m.peek(m.segAddr(off)))), nil
	}})
	regStmt("POKE", func(m *VM, a []Value) error {
		off, err := offArg(a[0])
		if err != nil {
			return err
		}
		v, err := intArg(a[1])
		if err != nil {
			return err
		}
		if v < 0 || v > 255 {
			return ErrIllegalCall
		}
		m.poke(m.segAddr(off), byte(v))
		return nil
	})
	regStmt("DEF_SEG", func(m *VM, a []Value) error {
		seg, err := optInt(a, 0, dgroupSeg)
		if err != nil {
			return err
		}
		if seg < -32768 || seg > 0xFFFF {
			return ErrIllegalCall
		}
		m.defSeg = seg & 0xFFFF
		return nil
	})
	reg(Builtin{Name: "VARPTR", Args: "v", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		_, off := m.addrParts(a[0].P)
		return signed16(off), nil
	}})
	reg(Builtin{Name: "VARSEG", Args: "v", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		seg, _ := m.addrParts(a[0].P)
		return signed16(seg), nil
	}})
	reg(Builtin{Name: "SADD", Args: "v", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		p := a[0].P.Deref()
		if p.K != KString {
			return none, ErrTypeMismatch
		}
		return signed16(int(m.memory().strBlock(p).base - dgroupBase)), nil
	}})
	reg(Builtin{Name: "INP", Args: "n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		port, err := intArg(a[0])
		if err != nil {
			return none, err
		}
		if port < 0 || port > 0xFFFF {
			return none, ErrIllegalCall
		}
		return Int(int64(m.portIn(uint16(port)))), nil
	}})
	regStmt("OUT", func(m *VM, a []Value) error {
		port, err := intArg(a[0])
		if err != nil {
			return err
		}
		v, err := intArg(a[1])
		if err != nil {
			return err
		}
		if port < 0 || port > 0xFFFF || v < 0 || v > 255 {
			return ErrIllegalCall
		}
		m.portOut(uint16(port), byte(v))
		return nil
	})
	regStmt("WAIT", func(m *VM, a []Value) error {
		port, err := intArg(a[0])
		if err != nil {
			return err
		}
		and, err := intArg(a[1])
		if err != nil {
			return err
		}
		xor, err := optInt(a, 2, 0)
		if err != nil {
			return err
		}
		for i := 0; ; i++ {
			if (int(m.portIn(uint16(port)))^xor)&and != 0 {
				return nil
			}
			if m.stop.Load() {
				return errStopped
			}
			if i > 100 {
				time.Sleep(200 * time.Microsecond)
			}
		}
	})
	regStmt("BSAVE", func(m *VM, a []Value) error {
		off, err := offArg(a[1])
		if err != nil {
			return err
		}
		n, err := intArg(a[2])
		if err != nil {
			return err
		}
		if n < 0 || n > 0xFFFF {
			return ErrIllegalCall
		}
		buf := []byte{0xFD, byte(m.defSeg), byte(m.defSeg >> 8), byte(off), byte(off >> 8), byte(n), byte(n >> 8)}
		for i := 0; i < n; i++ {
			buf = append(buf, m.peek(m.segAddr(off+i)))
		}
		return fileErr(os.WriteFile(a[0].S, buf, 0o644))
	})
	regStmt("BLOAD", func(m *VM, a []Value) error {
		data, err := os.ReadFile(a[0].S)
		if err != nil {
			return fileErr(err)
		}
		if len(data) < 7 || data[0] != 0xFD {
			return ErrBadFileMode
		}
		seg := int(data[1]) | int(data[2])<<8
		off := int(data[3]) | int(data[4])<<8
		n := int(data[5]) | int(data[6])<<8
		if len(a) > 1 && !isNone(a[1]) {
			if off, err = offArg(a[1]); err != nil {
				return err
			}
			seg = m.defSeg
		}
		data = data[7:]
		if n > len(data) {
			n = len(data)
		}
		base := uint32(seg) << 4
		for i := 0; i < n; i++ {
			m.poke(base+uint32((off+i)&0xFFFF), data[i])
		}
		return nil
	})
}

func fileErr(err error) error {
	if err == nil {
		return nil
	}
	return osErr(err)
}
