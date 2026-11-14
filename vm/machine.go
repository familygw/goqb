package vm

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"goqb/gfx"
	"goqb/vm/x86"
)

// Código máquina: CALL ABSOLUTE, CALL INTERRUPT(X), puertos de E/S y los
// servicios del BIOS/DOS que suelen usar los programas de QB.

// cpuSpeed son las instrucciones por segundo del 8086 emulado (GOQB_CPU_IPS;
// 0 = sin límite). Las rutinas de demora en código máquina dependen de esto.
var cpuSpeed = func() int {
	if s := os.Getenv("GOQB_CPU_IPS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return n
		}
	}
	return 4_000_000
}()

const cpuBudget = 2_000_000_000

// bus adapta la VM a la interfaz de memoria y puertos del 8086.
type bus struct{ m *VM }

func (b bus) Read8(a uint32) byte     { return b.m.peek(a) }
func (b bus) Write8(a uint32, v byte) { b.m.poke(a, v) }
func (b bus) In(p uint16) byte        { return b.m.portIn(p) }
func (b bus) Out(p uint16, v byte)    { b.m.portOut(p, v) }

var start = time.Now()

// portIn emula los puertos más usados: estado del VGA (retrazado vertical),
// DAC de la paleta, timer y teclado.
func (m *VM) portIn(p uint16) byte {
	mm := m.memory()
	switch p {
	case 0x3DA, 0x3BA: // bit 3: retrazado vertical (60 Hz), bit 0: retrazado
		t := time.Since(start) % (16_666_667 * time.Nanosecond)
		if t < 1_250_000 {
			return 0x09
		}
		if t%(64*time.Microsecond) < 10*time.Microsecond {
			return 0x01
		}
		return 0
	case 0x3C9:
		v := mm.dac[mm.dacR%len(mm.dac)]
		mm.dacR++
		return v
	case 0x40, 0x41, 0x42: // contador del PIT (1.19 MHz)
		return byte(time.Since(start).Nanoseconds() * 1193 / 1_000_000)
	case 0x61:
		return mm.speaker
	case 0x60:
		return 0
	case 0x201: // joystick: botones sueltos
		return 0xF0
	}
	return 0xFF
}

func (m *VM) portOut(p uint16, v byte) {
	mm := m.memory()
	switch p {
	case 0x3C8:
		mm.dacW = int(v) * 3
	case 0x3C7:
		mm.dacR = int(v) * 3
	case 0x3C9:
		i := mm.dacW % len(mm.dac)
		mm.dac[i] = v & 63
		mm.dacW++
		if mm.dacW%3 == 0 { // color completo: aplicarlo como PALETTE
			idx := i / 3
			c := int64(mm.dac[idx*3]) | int64(mm.dac[idx*3+1])<<8 | int64(mm.dac[idx*3+2])<<16
			if gc, ok := m.gfxCon(); ok && gc.ScreenMode() != 0 {
				gc.WithGfx(func(g *gfx.Screen) error { g.Palette(idx, c); return nil })
			}
		}
	case 0x61:
		mm.speaker = v
	}
}

// newCPU prepara un 8086 con DS = ES = DGROUP y una pila propia.
func (m *VM) newCPU() *x86.CPU {
	c := x86.New(bus{m})
	c.S[x86.DS], c.S[x86.ES], c.S[x86.SS] = dgroupSeg, dgroupSeg, stackSeg
	c.R[x86.SP] = 0xFFF0
	c.Int = m.interrupt
	return c
}

// runCPU ejecuta hasta volver a la dirección centinela, a la velocidad de cpuSpeed.
func (m *VM) runCPU(c *x86.CPU) error {
	t0 := time.Now()
	err := c.Run(0xFFFF, 0x0000, cpuBudget, func(n int) error {
		if m.stop.Load() {
			return x86.ErrStopped
		}
		if cpuSpeed > 0 {
			want := time.Duration(int64(n) * int64(time.Second) / int64(cpuSpeed))
			if d := want - time.Since(t0); d > time.Millisecond {
				time.Sleep(d)
			}
		}
		return nil
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, x86.ErrStopped):
		return errStopped
	case errors.Is(err, x86.ErrDivide):
		return ErrDivZero
	}
	return err
}

// callAbsolute implementa CALL ABSOLUTE([args,] offset): los argumentos se
// pasan por referencia (offsets en DGROUP) y el código está en DEF SEG:offset.
func (m *VM) callAbsolute(a []Value) error {
	off, err := offArg(a[len(a)-1])
	if err != nil {
		return err
	}
	c := m.newCPU()
	mm := m.memory()
	for _, v := range a[:len(a)-1] {
		if v.K != KPtr {
			return ErrIllegalCall
		}
		c.Push(uint16(mm.cellBlock(v.P).base - dgroupBase))
	}
	c.Push(0xFFFF) // retorno lejano a la dirección centinela
	c.Push(0x0000)
	c.S[x86.CS], c.IP = uint16(m.defSeg), uint16(off)
	return m.runCPU(c)
}

// Campos de RegType / RegTypeX (QB.BI).
var regFields = []struct {
	name string
	reg  int // índice en R (o -1-segmento)
}{
	{"ax", x86.AX}, {"bx", x86.BX}, {"cx", x86.CX}, {"dx", x86.DX},
	{"bp", x86.BP}, {"si", x86.SI}, {"di", x86.DI}, {"flags", -100},
	{"ds", -1 - x86.DS}, {"es", -1 - x86.ES},
}

func recField(r *Record, name string) *Value {
	for i, f := range r.T.Fields {
		if strings.EqualFold(f.Name, name) {
			return &r.F[i]
		}
	}
	return nil
}

// callInterrupt implementa CALL INTERRUPT(n, inregs, outregs) e INTERRUPTX.
func (m *VM) callInterrupt(a []Value) error {
	n, err := intArg(a[0])
	if err != nil {
		return err
	}
	if n < 0 || n > 255 {
		return ErrIllegalCall
	}
	in, out := a[1].P.Deref(), a[2].P.Deref()
	if in.K != KRecord || out.K != KRecord {
		return ErrTypeMismatch
	}
	c := m.newCPU()
	for _, f := range regFields {
		v := recField(in.R, f.name)
		if v == nil {
			continue
		}
		x := uint16(v.I)
		switch {
		case f.reg == -100:
			c.Flags = x&0x0FD5 | 2
		case f.reg < 0:
			if int16(x) != -1 { // -1: el valor actual (DGROUP)
				c.S[-1-f.reg] = x
			}
		default:
			c.R[f.reg] = x
		}
	}
	// INT n seguido de RETF hacia el centinela, en la pila
	c.Push(0xFFFF)
	c.Push(0x0000)
	code := uint32(stackSeg)<<4 + 0x100
	m.poke(code, 0xCD)
	m.poke(code+1, byte(n))
	m.poke(code+2, 0xCB)
	c.S[x86.CS], c.IP = stackSeg, 0x100
	if err := m.runCPU(c); err != nil {
		return err
	}
	for _, f := range regFields {
		v := recField(out.R, f.name)
		if v == nil {
			continue
		}
		var x uint16
		switch {
		case f.reg == -100:
			x = c.Flags
		case f.reg < 0:
			x = c.S[-1-f.reg]
		default:
			x = c.R[f.reg]
		}
		Assign(v, Int(int64(int16(x))))
	}
	return nil
}

// ------------------------------------------------------------- BIOS y DOS

// readKeyCode lee una tecla como el BIOS: AL = ASCII, AH = código de rastreo.
func (m *VM) keyCode(k string) uint16 {
	if k == "" {
		return 0
	}
	if len(k) == 2 && k[0] == 0 {
		return uint16(k[1]) << 8
	}
	ch := k[0]
	return uint16(scanCode(ch))<<8 | uint16(ch)
}

// scanCode aproxima el código de rastreo de una tecla ASCII.
func scanCode(ch byte) byte {
	const row1, row2, row3 = "qwertyuiop", "asdfghjkl", "zxcvbnm"
	switch {
	case ch == 27:
		return 1
	case ch >= '1' && ch <= '9':
		return ch - '1' + 2
	case ch == '0':
		return 11
	case ch == 8:
		return 14
	case ch == 9:
		return 15
	case ch == 13:
		return 28
	case ch == ' ':
		return 57
	}
	lc := ch | 0x20
	if i := strings.IndexByte(row1, lc); i >= 0 {
		return byte(16 + i)
	}
	if i := strings.IndexByte(row2, lc); i >= 0 {
		return byte(30 + i)
	}
	if i := strings.IndexByte(row3, lc); i >= 0 {
		return byte(44 + i)
	}
	return 0
}

func (m *VM) peekKey() string {
	mm := m.memory()
	if mm.pending == "" {
		mm.pending = m.Con.InKey()
	}
	return mm.pending
}

func (m *VM) waitKey() string {
	mm := m.memory()
	if k := mm.pending; k != "" {
		mm.pending = ""
		return k
	}
	k, _ := m.Con.ReadKey()
	return k
}

// interrupt atiende INT n. Devuelve true si el servicio está emulado.
func (m *VM) interrupt(c *x86.CPU, n byte) bool {
	ah := c.Reg8(4)
	switch n {
	case 0x10:
		m.int10(c, ah)
	case 0x16:
		switch ah {
		case 0x00, 0x10:
			c.R[x86.AX] = m.keyCode(m.waitKey())
		case 0x01, 0x11:
			k := m.peekKey()
			c.SetFlag(x86.ZF, k == "")
			if k != "" {
				c.R[x86.AX] = m.keyCode(k)
			}
			time.Sleep(time.Millisecond) // los bucles de espera no consumen todo el procesador
		case 0x02, 0x12:
			c.SetReg8(0, m.memory().kbFlags)
		}
	case 0x1A:
		if ah == 0 {
			t := ticksSinceMidnight()
			c.R[x86.CX], c.R[x86.DX] = uint16(t>>16), uint16(t)
			c.SetReg8(0, 0)
		}
	case 0x15:
		if ah == 0x86 { // esperar CX:DX microsegundos
			us := uint32(c.R[x86.CX])<<16 | uint32(c.R[x86.DX])
			time.Sleep(time.Duration(us) * time.Microsecond)
			c.SetFlag(x86.CF, false)
		} else {
			c.SetFlag(x86.CF, true)
		}
	case 0x21:
		m.int21(c, ah)
	case 0x33: // mouse: no instalado
		if c.R[x86.AX] == 0 {
			c.R[x86.AX], c.R[x86.BX] = 0, 0
		}
	case 0x11:
		c.R[x86.AX] = 0x0026
	case 0x12:
		c.R[x86.AX] = 640
	}
	return true // los servicios sin emular se ignoran
}

func (m *VM) int10(c *x86.CPU, ah byte) {
	w, h := m.Con.Size()
	switch ah {
	case 0x00: // modo de video
		modes := map[byte]int{0: 0, 1: 0, 2: 0, 3: 0, 4: 1, 5: 1, 6: 2, 0x0D: 7, 0x0E: 8, 0x0F: 10, 0x10: 9, 0x11: 11, 0x12: 12, 0x13: 13}
		al := c.Reg8(0) & 0x7F
		if mode, ok := modes[al]; ok {
			if gc, ok := m.gfxCon(); ok {
				gc.SetScreen(mode, 0, 0)
			}
			if mode == 0 {
				cols := 80
				if al < 2 {
					cols = 40
				}
				m.Con.SetWidth(cols, 25)
				m.Con.Cls()
			}
		}
	case 0x01:
		m.Con.SetCursor(c.Reg8(5)&0x20 == 0)
	case 0x02:
		m.Con.Locate(int(c.Reg8(6))+1, int(c.Reg8(2))+1)
	case 0x03:
		r, col := m.Con.Cursor()
		c.SetReg8(6, byte(r-1))
		c.SetReg8(2, byte(col-1))
		c.R[x86.CX] = 0x0607
	case 0x06, 0x07:
		m.scroll(ah == 0x07, int(c.Reg8(0)), c.Reg8(7), int(c.Reg8(5)), int(c.Reg8(1)), int(c.Reg8(6)), int(c.Reg8(2)), w, h)
	case 0x08:
		r, col := m.Con.Cursor()
		ch, attr := m.Con.CharAt(r, col)
		c.SetReg8(0, ch)
		c.SetReg8(4, attr)
	case 0x09, 0x0A:
		r, col := m.Con.Cursor()
		for i := 0; i < int(c.R[x86.CX]); i++ {
			cc, rr := col+i, r
			for cc > w {
				cc -= w
				rr++
			}
			if rr > h {
				break
			}
			_, attr := m.Con.CharAt(rr, cc)
			if ah == 0x09 {
				attr = c.Reg8(3)
			}
			m.Con.PutChar(rr, cc, c.Reg8(0), attr)
		}
	case 0x0E:
		switch ch := c.Reg8(0); ch {
		case 7:
			m.Con.Beep()
		case 13:
			r, _ := m.Con.Cursor()
			m.Con.Locate(r, 1)
		case 10:
			m.Con.Newline()
		case 8:
			r, col := m.Con.Cursor()
			if col > 1 {
				m.Con.Locate(r, col-1)
			}
		default:
			m.Con.Print(string([]byte{ch}))
		}
	case 0x0F:
		c.SetReg8(0, m.biosMode())
		c.SetReg8(4, byte(w))
		c.SetReg8(7, 0)
	case 0x1A:
		c.SetReg8(0, 0x1A)
		c.SetReg8(3, 8)
	}
}

// scroll implementa INT 10h AH=06h/07h sobre la pantalla de texto.
func (m *VM) scroll(down bool, lines int, attr byte, top, left, bottom, right, w, h int) {
	if gc, ok := m.gfxCon(); ok && gc.ScreenMode() != 0 {
		return
	}
	top, left, bottom, right = top+1, left+1, min(bottom+1, h), min(right+1, w)
	if top > bottom || left > right {
		return
	}
	height := bottom - top + 1
	if lines == 0 || lines > height {
		lines = height
	}
	for i := 0; i < height; i++ {
		r := top + i
		src := r + lines
		if down {
			r = bottom - i
			src = r - lines
		}
		for col := left; col <= right; col++ {
			if src >= top && src <= bottom {
				ch, at := m.Con.CharAt(src, col)
				m.Con.PutChar(r, col, ch, at)
			} else {
				m.Con.PutChar(r, col, ' ', attr)
			}
		}
	}
}

func (m *VM) int21(c *x86.CPU, ah byte) {
	switch ah {
	case 0x01, 0x07, 0x08:
		k := m.waitKey()
		if k != "" {
			c.SetReg8(0, k[0])
			if ah == 0x01 && k[0] >= 32 {
				m.Con.Print(k[:1])
			}
		}
	case 0x02:
		m.dosChar(c.Reg8(2))
	case 0x06:
		if dl := c.Reg8(2); dl != 0xFF {
			m.dosChar(dl)
		} else {
			k := m.peekKey()
			c.SetFlag(x86.ZF, k == "")
			if k != "" {
				c.SetReg8(0, k[0])
				m.memory().pending = k[1:]
			}
		}
	case 0x09:
		for off := c.R[x86.DX]; ; off++ {
			ch := c.Read8(c.S[x86.DS], off)
			if ch == '$' {
				break
			}
			m.dosChar(ch)
		}
	case 0x0B:
		if m.peekKey() != "" {
			c.SetReg8(0, 0xFF)
		} else {
			c.SetReg8(0, 0)
		}
	case 0x2A:
		now := time.Now()
		c.R[x86.CX] = uint16(now.Year())
		c.SetReg8(6, byte(now.Month()))
		c.SetReg8(2, byte(now.Day()))
		c.SetReg8(0, byte(now.Weekday()))
	case 0x2C:
		now := time.Now()
		c.SetReg8(5, byte(now.Hour()))
		c.SetReg8(1, byte(now.Minute()))
		c.SetReg8(6, byte(now.Second()))
		c.SetReg8(2, byte(now.Nanosecond()/10_000_000))
	case 0x30:
		c.R[x86.AX] = 0x0005 // DOS 5.0
	}
}

func (m *VM) dosChar(ch byte) {
	switch ch {
	case 13:
		r, _ := m.Con.Cursor()
		m.Con.Locate(r, 1)
	case 10:
		m.Con.Newline()
	case 7:
		m.Con.Beep()
	default:
		m.Con.Print(string([]byte{ch}))
	}
}

func init() {
	regStmt("ABSOLUTE", func(m *VM, a []Value) error { return m.callAbsolute(a) })
	regStmt("INTERRUPT", func(m *VM, a []Value) error { return m.callInterrupt(a) })
}
