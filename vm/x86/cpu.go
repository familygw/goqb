// Package x86 es un intérprete del 8086 (con algunas instrucciones del
// 80186) para ejecutar las rutinas en código máquina que los programas de
// QuickBASIC cargan con POKE y llaman con CALL ABSOLUTE.
package x86

import (
	"errors"
	"fmt"
)

// Bus es la memoria (direcciones lineales de 20 bits) y los puertos de E/S.
type Bus interface {
	Read8(addr uint32) byte
	Write8(addr uint32, v byte)
	In(port uint16) byte
	Out(port uint16, v byte)
}

// Registros de 16 bits (índices de CPU.R).
const (
	AX = iota
	CX
	DX
	BX
	SP
	BP
	SI
	DI
)

// Registros de segmento (índices de CPU.S).
const (
	ES = iota
	CS
	SS
	DS
)

// Bits de FLAGS.
const (
	CF = 1 << 0
	PF = 1 << 2
	AF = 1 << 4
	ZF = 1 << 6
	SF = 1 << 7
	TF = 1 << 8
	IF = 1 << 9
	DF = 1 << 10
	OF = 1 << 11
)

// Errores de ejecución.
var (
	ErrDivide  = errors.New("división por cero en código máquina")
	ErrBudget  = errors.New("el código máquina no terminó")
	ErrHalted  = errors.New("HLT en código máquina")
	ErrStopped = errors.New("detenido")
)

// CPU es el estado del procesador.
type CPU struct {
	R     [8]uint16
	S     [4]uint16
	IP    uint16
	Flags uint16
	Bus   Bus
	// Int atiende INT n; si devuelve false se usa la tabla de vectores.
	Int func(c *CPU, n byte) bool

	segOv int // prefijo de segmento (-1: ninguno)
	rep   int // 0, repZ (F3) o repNZ (F2)
	err   error
}

const (
	repZ  = 1
	repNZ = 2
)

// New crea una CPU sobre bus.
func New(bus Bus) *CPU { return &CPU{Bus: bus, Flags: 0x0002 | IF, segOv: -1} }

// Linear calcula la dirección lineal de seg:off.
func Linear(seg, off uint16) uint32 { return (uint32(seg)<<4 + uint32(off)) & 0xFFFFF }

// Reg8 lee un registro de 8 bits (0-3: AL CL DL BL, 4-7: AH CH DH BH).
func (c *CPU) Reg8(i int) byte {
	if i < 4 {
		return byte(c.R[i])
	}
	return byte(c.R[i-4] >> 8)
}

// SetReg8 escribe un registro de 8 bits.
func (c *CPU) SetReg8(i int, v byte) {
	if i < 4 {
		c.R[i] = c.R[i]&0xFF00 | uint16(v)
	} else {
		c.R[i-4] = c.R[i-4]&0x00FF | uint16(v)<<8
	}
}

// SetFlag activa o desactiva un bit de FLAGS.
func (c *CPU) SetFlag(f uint16, on bool) {
	if on {
		c.Flags |= f
	} else {
		c.Flags &^= f
	}
}

func (c *CPU) flag(f uint16) bool { return c.Flags&f != 0 }

// Read8 / Read16 / Write8 / Write16 acceden a seg:off.
func (c *CPU) Read8(seg, off uint16) byte { return c.Bus.Read8(Linear(seg, off)) }

func (c *CPU) Read16(seg, off uint16) uint16 {
	return uint16(c.Read8(seg, off)) | uint16(c.Read8(seg, off+1))<<8
}

func (c *CPU) Write8(seg, off uint16, v byte) { c.Bus.Write8(Linear(seg, off), v) }

func (c *CPU) Write16(seg, off uint16, v uint16) {
	c.Write8(seg, off, byte(v))
	c.Write8(seg, off+1, byte(v>>8))
}

func (c *CPU) fetch8() byte {
	v := c.Read8(c.S[CS], c.IP)
	c.IP++
	return v
}

func (c *CPU) fetch16() uint16 {
	v := c.Read16(c.S[CS], c.IP)
	c.IP += 2
	return v
}

// Push / Pop usan SS:SP.
func (c *CPU) Push(v uint16) {
	c.R[SP] -= 2
	c.Write16(c.S[SS], c.R[SP], v)
}

func (c *CPU) Pop() uint16 {
	v := c.Read16(c.S[SS], c.R[SP])
	c.R[SP] += 2
	return v
}

// ------------------------------------------------------------- ModR/M

type operand struct {
	reg      bool
	idx      int
	seg, off uint16
}

func (c *CPU) seg(def int) uint16 {
	if c.segOv >= 0 {
		return c.S[c.segOv]
	}
	return c.S[def]
}

// modrm decodifica el byte ModR/M y devuelve el campo reg y el operando r/m.
func (c *CPU) modrm() (int, operand) {
	b := c.fetch8()
	mod, reg, rm := int(b>>6), int(b>>3&7), int(b&7)
	if mod == 3 {
		return reg, operand{reg: true, idx: rm}
	}
	var off uint16
	def := DS
	switch rm {
	case 0:
		off = c.R[BX] + c.R[SI]
	case 1:
		off = c.R[BX] + c.R[DI]
	case 2:
		off, def = c.R[BP]+c.R[SI], SS
	case 3:
		off, def = c.R[BP]+c.R[DI], SS
	case 4:
		off = c.R[SI]
	case 5:
		off = c.R[DI]
	case 6:
		if mod == 0 {
			off = c.fetch16()
		} else {
			off, def = c.R[BP], SS
		}
	case 7:
		off = c.R[BX]
	}
	switch mod {
	case 1:
		off += uint16(int8(c.fetch8()))
	case 2:
		off += c.fetch16()
	}
	return reg, operand{seg: c.seg(def), off: off}
}

func (c *CPU) get8(o operand) byte {
	if o.reg {
		return c.Reg8(o.idx)
	}
	return c.Read8(o.seg, o.off)
}

func (c *CPU) set8(o operand, v byte) {
	if o.reg {
		c.SetReg8(o.idx, v)
	} else {
		c.Write8(o.seg, o.off, v)
	}
}

func (c *CPU) get16(o operand) uint16 {
	if o.reg {
		return c.R[o.idx]
	}
	return c.Read16(o.seg, o.off)
}

func (c *CPU) set16(o operand, v uint16) {
	if o.reg {
		c.R[o.idx] = v
	} else {
		c.Write16(o.seg, o.off, v)
	}
}

func (c *CPU) get(o operand, w bool) uint32 {
	if w {
		return uint32(c.get16(o))
	}
	return uint32(c.get8(o))
}

func (c *CPU) set(o operand, w bool, v uint32) {
	if w {
		c.set16(o, uint16(v))
	} else {
		c.set8(o, byte(v))
	}
}

func regOp(i int) operand { return operand{reg: true, idx: i} }

// ------------------------------------------------------------- flags y ALU

func parity(b byte) bool {
	b ^= b >> 4
	b ^= b >> 2
	b ^= b >> 1
	return b&1 == 0
}

func masks(w bool) (uint32, uint32) {
	if w {
		return 0xFFFF, 0x8000
	}
	return 0xFF, 0x80
}

func (c *CPU) setSZP(r uint32, w bool) {
	mask, sign := masks(w)
	r &= mask
	c.SetFlag(ZF, r == 0)
	c.SetFlag(SF, r&sign != 0)
	c.SetFlag(PF, parity(byte(r)))
}

// alu ejecuta ADD OR ADC SBB AND SUB XOR CMP (op 0-7).
func (c *CPU) alu(op int, a, b uint32, w bool) uint32 {
	mask, sign := masks(w)
	var r uint32
	switch op {
	case 0, 2: // ADD, ADC
		carry := uint32(0)
		if op == 2 && c.flag(CF) {
			carry = 1
		}
		r = a + b + carry
		c.SetFlag(CF, r > mask)
		c.SetFlag(OF, (a^r)&(b^r)&sign != 0)
		c.SetFlag(AF, (a^b^r)&0x10 != 0)
	case 3, 5, 7: // SBB, SUB, CMP
		borrow := uint32(0)
		if op == 3 && c.flag(CF) {
			borrow = 1
		}
		r = a - b - borrow
		c.SetFlag(CF, a < b+borrow)
		c.SetFlag(OF, (a^b)&(a^r)&sign != 0)
		c.SetFlag(AF, (a^b^r)&0x10 != 0)
	case 1:
		r = a | b
		c.Flags &^= CF | OF | AF
	case 4:
		r = a & b
		c.Flags &^= CF | OF | AF
	case 6:
		r = a ^ b
		c.Flags &^= CF | OF | AF
	}
	r &= mask
	c.setSZP(r, w)
	return r
}

func (c *CPU) incdec(v uint32, w, dec bool) uint32 {
	mask, sign := masks(w)
	var r uint32
	if dec {
		r = (v - 1) & mask
		c.SetFlag(OF, v == sign)
		c.SetFlag(AF, v&0xF == 0)
	} else {
		r = (v + 1) & mask
		c.SetFlag(OF, r == sign)
		c.SetFlag(AF, r&0xF == 0)
	}
	c.setSZP(r, w)
	return r
}

// shift ejecuta ROL ROR RCL RCR SHL SHR SAL SAR.
func (c *CPU) shift(op int, v uint32, n int, w bool) uint32 {
	mask, sign := masks(w)
	n &= 0x1F
	if n == 0 {
		return v
	}
	for i := 0; i < n; i++ {
		switch op {
		case 0: // ROL
			cf := v&sign != 0
			v = (v << 1) & mask
			if cf {
				v |= 1
			}
			c.SetFlag(CF, cf)
		case 1: // ROR
			cf := v&1 != 0
			v >>= 1
			if cf {
				v |= sign
			}
			c.SetFlag(CF, cf)
		case 2: // RCL
			cf := v&sign != 0
			v = (v << 1) & mask
			if c.flag(CF) {
				v |= 1
			}
			c.SetFlag(CF, cf)
		case 3: // RCR
			cf := v&1 != 0
			v >>= 1
			if c.flag(CF) {
				v |= sign
			}
			c.SetFlag(CF, cf)
		case 4, 6: // SHL/SAL
			c.SetFlag(CF, v&sign != 0)
			v = (v << 1) & mask
		case 5: // SHR
			c.SetFlag(CF, v&1 != 0)
			v >>= 1
		case 7: // SAR
			c.SetFlag(CF, v&1 != 0)
			v = v>>1 | v&sign
		}
	}
	switch op {
	case 0, 2:
		c.SetFlag(OF, (v&sign != 0) != c.flag(CF))
	case 1, 3:
		c.SetFlag(OF, (v^v<<1)&sign != 0)
	default:
		c.setSZP(v, w)
		if op == 5 {
			c.SetFlag(OF, false)
			if n == 1 {
				c.SetFlag(OF, (v<<1)&sign != 0)
			}
		} else {
			c.SetFlag(OF, (v&sign != 0) != c.flag(CF))
		}
	}
	return v
}

// ------------------------------------------------------------- ejecución

// Run ejecuta hasta que CS:IP llegue a stopCS:stopIP o se agote budget.
// check se invoca cada tanto con la cantidad de instrucciones ejecutadas
// (para limitar la velocidad o detener); si devuelve un error, se aborta.
func (c *CPU) Run(stopCS, stopIP uint16, budget int, check func(n int) error) error {
	for n := 0; ; n++ {
		if c.S[CS] == stopCS && c.IP == stopIP {
			return nil
		}
		if n >= budget {
			return ErrBudget
		}
		if n&4095 == 4095 && check != nil {
			if err := check(n + 1); err != nil {
				return err
			}
		}
		if err := c.Step(); err != nil {
			return err
		}
	}
}

// Interrupt ejecuta INT n (manejador propio o tabla de vectores).
func (c *CPU) Interrupt(n byte) {
	if c.Int != nil && c.Int(c, n) {
		return
	}
	vec := uint16(n) * 4
	ip, cs := c.Read16(0, vec), c.Read16(0, vec+2)
	if ip == 0 && cs == 0 {
		return // sin manejador: se ignora
	}
	c.Push(c.Flags)
	c.Push(c.S[CS])
	c.Push(c.IP)
	c.Flags &^= IF | TF
	c.S[CS], c.IP = cs, ip
}

// Step ejecuta una instrucción.
func (c *CPU) Step() error {
	c.segOv, c.rep, c.err = -1, 0, nil
	start := c.IP
	var op byte
	for { // prefijos
		op = c.fetch8()
		switch op {
		case 0x26, 0x2E, 0x36, 0x3E:
			c.segOv = int(op>>3) & 3
			continue
		case 0xF2:
			c.rep = repNZ
			continue
		case 0xF3:
			c.rep = repZ
			continue
		case 0xF0: // LOCK
			continue
		}
		break
	}
	switch {
	case op < 0x40 && op&7 < 6: // ALU
		aluOp, w := int(op>>3), op&1 == 1
		switch op & 7 {
		case 0, 1:
			reg, rm := c.modrm()
			r := c.alu(aluOp, c.get(rm, w), c.get(regOp(reg), w), w)
			if aluOp != 7 {
				c.set(rm, w, r)
			}
		case 2, 3:
			reg, rm := c.modrm()
			r := c.alu(aluOp, c.get(regOp(reg), w), c.get(rm, w), w)
			if aluOp != 7 {
				c.set(regOp(reg), w, r)
			}
		case 4:
			r := c.alu(aluOp, uint32(c.Reg8(0)), uint32(c.fetch8()), false)
			if aluOp != 7 {
				c.SetReg8(0, byte(r))
			}
		case 5:
			r := c.alu(aluOp, uint32(c.R[AX]), uint32(c.fetch16()), true)
			if aluOp != 7 {
				c.R[AX] = uint16(r)
			}
		}
		return c.err
	}
	switch op {
	case 0x06, 0x0E, 0x16, 0x1E:
		c.Push(c.S[op>>3])
	case 0x07, 0x17, 0x1F:
		c.S[op>>3] = c.Pop()
	case 0x27, 0x2F: // DAA, DAS
		al, cf := c.Reg8(0), c.flag(CF)
		if al&0xF > 9 || c.flag(AF) {
			if op == 0x27 {
				al += 6
			} else {
				al -= 6
			}
			c.SetFlag(AF, true)
		}
		if c.Reg8(0) > 0x99 || cf {
			if op == 0x27 {
				al += 0x60
			} else {
				al -= 0x60
			}
			c.SetFlag(CF, true)
		}
		c.SetReg8(0, al)
		c.setSZP(uint32(al), false)
	case 0x37, 0x3F: // AAA, AAS
		if c.Reg8(0)&0xF > 9 || c.flag(AF) {
			if op == 0x37 {
				c.R[AX] += 0x106
			} else {
				c.R[AX] -= 6
				c.SetReg8(4, c.Reg8(4)-1)
			}
			c.Flags |= AF | CF
		} else {
			c.Flags &^= AF | CF
		}
		c.SetReg8(0, c.Reg8(0)&0xF)
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47:
		c.R[op&7] = uint16(c.incdec(uint32(c.R[op&7]), true, false))
	case 0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
		c.R[op&7] = uint16(c.incdec(uint32(c.R[op&7]), true, true))
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57:
		c.Push(c.R[op&7])
	case 0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F:
		c.R[op&7] = c.Pop()
	case 0x60: // PUSHA
		sp := c.R[SP]
		for i := 0; i < 8; i++ {
			if i == SP {
				c.Push(sp)
			} else {
				c.Push(c.R[i])
			}
		}
	case 0x61: // POPA
		for i := 7; i >= 0; i-- {
			v := c.Pop()
			if i != SP {
				c.R[i] = v
			}
		}
	case 0x68:
		c.Push(c.fetch16())
	case 0x6A:
		c.Push(uint16(int8(c.fetch8())))
	case 0x69, 0x6B: // IMUL r16, rm16, imm
		reg, rm := c.modrm()
		var imm int32
		if op == 0x69 {
			imm = int32(int16(c.fetch16()))
		} else {
			imm = int32(int8(c.fetch8()))
		}
		r := int32(int16(c.get16(rm))) * imm
		c.R[reg] = uint16(r)
		c.SetFlag(CF, r != int32(int16(r)))
		c.SetFlag(OF, r != int32(int16(r)))
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
		d := int8(c.fetch8())
		if c.cond(op & 0xF) {
			c.IP += uint16(d)
		}
	case 0x80, 0x81, 0x82, 0x83:
		reg, rm := c.modrm()
		w := op&1 == 1
		var imm uint32
		switch op {
		case 0x81:
			imm = uint32(c.fetch16())
		case 0x83:
			imm = uint32(uint16(int8(c.fetch8())))
		default:
			imm = uint32(c.fetch8())
		}
		r := c.alu(reg, c.get(rm, w), imm, w)
		if reg != 7 {
			c.set(rm, w, r)
		}
	case 0x84, 0x85:
		reg, rm := c.modrm()
		w := op&1 == 1
		c.alu(4, c.get(rm, w), c.get(regOp(reg), w), w)
	case 0x86, 0x87:
		reg, rm := c.modrm()
		w := op&1 == 1
		a, b := c.get(rm, w), c.get(regOp(reg), w)
		c.set(rm, w, b)
		c.set(regOp(reg), w, a)
	case 0x88, 0x89:
		reg, rm := c.modrm()
		w := op&1 == 1
		c.set(rm, w, c.get(regOp(reg), w))
	case 0x8A, 0x8B:
		reg, rm := c.modrm()
		w := op&1 == 1
		c.set(regOp(reg), w, c.get(rm, w))
	case 0x8C:
		reg, rm := c.modrm()
		c.set16(rm, c.S[reg&3])
	case 0x8D:
		reg, rm := c.modrm()
		c.R[reg] = rm.off
	case 0x8E:
		reg, rm := c.modrm()
		c.S[reg&3] = c.get16(rm)
	case 0x8F:
		_, rm := c.modrm()
		c.set16(rm, c.Pop())
	case 0x90: // NOP
	case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
		c.R[AX], c.R[op&7] = c.R[op&7], c.R[AX]
	case 0x98: // CBW
		c.R[AX] = uint16(int8(c.Reg8(0)))
	case 0x99: // CWD
		if c.R[AX]&0x8000 != 0 {
			c.R[DX] = 0xFFFF
		} else {
			c.R[DX] = 0
		}
	case 0x9A: // CALL far
		ip, cs := c.fetch16(), c.fetch16()
		c.Push(c.S[CS])
		c.Push(c.IP)
		c.S[CS], c.IP = cs, ip
	case 0x9B: // WAIT
	case 0x9C:
		c.Push(c.Flags)
	case 0x9D:
		c.Flags = c.Pop()&0x0FD5 | 0x0002
	case 0x9E: // SAHF
		c.Flags = c.Flags&0xFF00 | uint16(c.Reg8(4)&0xD5) | 2
	case 0x9F: // LAHF
		c.SetReg8(4, byte(c.Flags))
	case 0xA0:
		c.SetReg8(0, c.Read8(c.seg(DS), c.fetch16()))
	case 0xA1:
		c.R[AX] = c.Read16(c.seg(DS), c.fetch16())
	case 0xA2:
		c.Write8(c.seg(DS), c.fetch16(), c.Reg8(0))
	case 0xA3:
		c.Write16(c.seg(DS), c.fetch16(), c.R[AX])
	case 0xA4, 0xA5, 0xA6, 0xA7, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF:
		c.str(op)
	case 0xA8:
		c.alu(4, uint32(c.Reg8(0)), uint32(c.fetch8()), false)
	case 0xA9:
		c.alu(4, uint32(c.R[AX]), uint32(c.fetch16()), true)
	case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
		c.SetReg8(int(op&7), c.fetch8())
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		c.R[op&7] = c.fetch16()
	case 0xC0, 0xC1, 0xD0, 0xD1, 0xD2, 0xD3:
		reg, rm := c.modrm()
		w := op&1 == 1
		n := 1
		switch op {
		case 0xC0, 0xC1:
			n = int(c.fetch8())
		case 0xD2, 0xD3:
			n = int(c.Reg8(1))
		}
		c.set(rm, w, c.shift(reg, c.get(rm, w), n, w))
	case 0xC2:
		n := c.fetch16()
		c.IP = c.Pop()
		c.R[SP] += n
	case 0xC3:
		c.IP = c.Pop()
	case 0xC4, 0xC5: // LES, LDS
		reg, rm := c.modrm()
		c.R[reg] = c.Read16(rm.seg, rm.off)
		s := ES
		if op == 0xC5 {
			s = DS
		}
		c.S[s] = c.Read16(rm.seg, rm.off+2)
	case 0xC6:
		_, rm := c.modrm()
		c.set8(rm, c.fetch8())
	case 0xC7:
		_, rm := c.modrm()
		c.set16(rm, c.fetch16())
	case 0xC8: // ENTER
		size := c.fetch16()
		c.fetch8()
		c.Push(c.R[BP])
		c.R[BP] = c.R[SP]
		c.R[SP] -= size
	case 0xC9: // LEAVE
		c.R[SP] = c.R[BP]
		c.R[BP] = c.Pop()
	case 0xCA:
		n := c.fetch16()
		c.IP = c.Pop()
		c.S[CS] = c.Pop()
		c.R[SP] += n
	case 0xCB:
		c.IP = c.Pop()
		c.S[CS] = c.Pop()
	case 0xCC:
		c.Interrupt(3)
	case 0xCD:
		c.Interrupt(c.fetch8())
	case 0xCE:
		if c.flag(OF) {
			c.Interrupt(4)
		}
	case 0xCF:
		c.IP = c.Pop()
		c.S[CS] = c.Pop()
		c.Flags = c.Pop()&0x0FD5 | 2
	case 0xD4: // AAM
		base := c.fetch8()
		if base == 0 {
			return ErrDivide
		}
		al := c.Reg8(0)
		c.SetReg8(4, al/base)
		c.SetReg8(0, al%base)
		c.setSZP(uint32(c.Reg8(0)), false)
	case 0xD5: // AAD
		base := c.fetch8()
		al := c.Reg8(0) + c.Reg8(4)*base
		c.R[AX] = uint16(al)
		c.setSZP(uint32(al), false)
	case 0xD6: // SALC
		if c.flag(CF) {
			c.SetReg8(0, 0xFF)
		} else {
			c.SetReg8(0, 0)
		}
	case 0xD7: // XLAT
		c.SetReg8(0, c.Read8(c.seg(DS), c.R[BX]+uint16(c.Reg8(0))))
	case 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF: // coprocesador: se ignora
		c.modrm()
	case 0xE0, 0xE1, 0xE2:
		d := int8(c.fetch8())
		c.R[CX]--
		jump := c.R[CX] != 0
		if op == 0xE0 {
			jump = jump && !c.flag(ZF)
		} else if op == 0xE1 {
			jump = jump && c.flag(ZF)
		}
		if jump {
			c.IP += uint16(d)
		}
	case 0xE3:
		d := int8(c.fetch8())
		if c.R[CX] == 0 {
			c.IP += uint16(d)
		}
	case 0xE4:
		c.SetReg8(0, c.Bus.In(uint16(c.fetch8())))
	case 0xE5:
		p := uint16(c.fetch8())
		c.R[AX] = uint16(c.Bus.In(p)) | uint16(c.Bus.In(p+1))<<8
	case 0xE6:
		p := uint16(c.fetch8())
		c.Bus.Out(p, c.Reg8(0))
	case 0xE7:
		p := uint16(c.fetch8())
		c.Bus.Out(p, byte(c.R[AX]))
		c.Bus.Out(p+1, byte(c.R[AX]>>8))
	case 0xE8:
		d := c.fetch16()
		c.Push(c.IP)
		c.IP += d
	case 0xE9:
		d := c.fetch16()
		c.IP += d
	case 0xEA:
		ip, cs := c.fetch16(), c.fetch16()
		c.S[CS], c.IP = cs, ip
	case 0xEB:
		d := int8(c.fetch8())
		c.IP += uint16(d)
	case 0xEC:
		c.SetReg8(0, c.Bus.In(c.R[DX]))
	case 0xED:
		c.R[AX] = uint16(c.Bus.In(c.R[DX])) | uint16(c.Bus.In(c.R[DX]+1))<<8
	case 0xEE:
		c.Bus.Out(c.R[DX], c.Reg8(0))
	case 0xEF:
		c.Bus.Out(c.R[DX], byte(c.R[AX]))
		c.Bus.Out(c.R[DX]+1, byte(c.R[AX]>>8))
	case 0xF4:
		return ErrHalted
	case 0xF5:
		c.Flags ^= CF
	case 0xF6, 0xF7:
		return c.group3(op&1 == 1)
	case 0xF8:
		c.Flags &^= CF
	case 0xF9:
		c.Flags |= CF
	case 0xFA:
		c.Flags &^= IF
	case 0xFB:
		c.Flags |= IF
	case 0xFC:
		c.Flags &^= DF
	case 0xFD:
		c.Flags |= DF
	case 0xFE:
		reg, rm := c.modrm()
		if reg > 1 {
			return c.invalid(start, op)
		}
		cf := c.flag(CF)
		c.set8(rm, byte(c.incdec(uint32(c.get8(rm)), false, reg == 1)))
		c.SetFlag(CF, cf)
	case 0xFF:
		reg, rm := c.modrm()
		switch reg {
		case 0, 1:
			cf := c.flag(CF)
			c.set16(rm, uint16(c.incdec(uint32(c.get16(rm)), true, reg == 1)))
			c.SetFlag(CF, cf)
		case 2:
			t := c.get16(rm)
			c.Push(c.IP)
			c.IP = t
		case 3:
			ip, cs := c.Read16(rm.seg, rm.off), c.Read16(rm.seg, rm.off+2)
			c.Push(c.S[CS])
			c.Push(c.IP)
			c.S[CS], c.IP = cs, ip
		case 4:
			c.IP = c.get16(rm)
		case 5:
			c.IP, c.S[CS] = c.Read16(rm.seg, rm.off), c.Read16(rm.seg, rm.off+2)
		case 6:
			c.Push(c.get16(rm))
		default:
			return c.invalid(start, op)
		}
	default:
		return c.invalid(start, op)
	}
	return c.err
}

func (c *CPU) invalid(ip uint16, op byte) error {
	return fmt.Errorf("instrucción 8086 no soportada %02Xh en %04X:%04X", op, c.S[CS], ip)
}

// cond evalúa la condición de Jcc (0-15).
func (c *CPU) cond(n byte) bool {
	var r bool
	switch n >> 1 {
	case 0:
		r = c.flag(OF)
	case 1:
		r = c.flag(CF)
	case 2:
		r = c.flag(ZF)
	case 3:
		r = c.flag(CF) || c.flag(ZF)
	case 4:
		r = c.flag(SF)
	case 5:
		r = c.flag(PF)
	case 6:
		r = c.flag(SF) != c.flag(OF)
	case 7:
		r = c.flag(ZF) || c.flag(SF) != c.flag(OF)
	}
	if n&1 == 1 {
		return !r
	}
	return r
}

// group3: TEST, NOT, NEG, MUL, IMUL, DIV, IDIV.
func (c *CPU) group3(w bool) error {
	reg, rm := c.modrm()
	mask, _ := masks(w)
	v := c.get(rm, w)
	switch reg {
	case 0, 1:
		var imm uint32
		if w {
			imm = uint32(c.fetch16())
		} else {
			imm = uint32(c.fetch8())
		}
		c.alu(4, v, imm, w)
	case 2:
		c.set(rm, w, ^v&mask)
	case 3:
		r := c.alu(5, 0, v, w)
		c.set(rm, w, r)
		c.SetFlag(CF, v != 0)
	case 4: // MUL
		if w {
			r := uint32(c.R[AX]) * v
			c.R[AX], c.R[DX] = uint16(r), uint16(r>>16)
			c.SetFlag(CF, c.R[DX] != 0)
			c.SetFlag(OF, c.R[DX] != 0)
		} else {
			r := uint16(c.Reg8(0)) * uint16(v)
			c.R[AX] = r
			c.SetFlag(CF, r>>8 != 0)
			c.SetFlag(OF, r>>8 != 0)
		}
	case 5: // IMUL
		if w {
			r := int32(int16(c.R[AX])) * int32(int16(v))
			c.R[AX], c.R[DX] = uint16(r), uint16(uint32(r)>>16)
			c.SetFlag(CF, r != int32(int16(r)))
			c.SetFlag(OF, r != int32(int16(r)))
		} else {
			r := int16(int8(c.Reg8(0))) * int16(int8(v))
			c.R[AX] = uint16(r)
			c.SetFlag(CF, r != int16(int8(r)))
			c.SetFlag(OF, r != int16(int8(r)))
		}
	case 6: // DIV
		if v == 0 {
			return ErrDivide
		}
		if w {
			n := uint32(c.R[DX])<<16 | uint32(c.R[AX])
			q := n / v
			if q > 0xFFFF {
				return ErrDivide
			}
			c.R[AX], c.R[DX] = uint16(q), uint16(n%v)
		} else {
			n := uint32(c.R[AX])
			q := n / v
			if q > 0xFF {
				return ErrDivide
			}
			c.SetReg8(0, byte(q))
			c.SetReg8(4, byte(n%v))
		}
	case 7: // IDIV
		if v == 0 {
			return ErrDivide
		}
		if w {
			n := int32(uint32(c.R[DX])<<16 | uint32(c.R[AX]))
			d := int32(int16(v))
			q := n / d
			if q != int32(int16(q)) {
				return ErrDivide
			}
			c.R[AX], c.R[DX] = uint16(q), uint16(n%d)
		} else {
			n := int16(c.R[AX])
			d := int16(int8(v))
			q := n / d
			if q != int16(int8(q)) {
				return ErrDivide
			}
			c.SetReg8(0, byte(q))
			c.SetReg8(4, byte(n%d))
		}
	}
	return nil
}

// str ejecuta MOVS, CMPS, STOS, LODS y SCAS con REP.
func (c *CPU) str(op byte) {
	w := op&1 == 1
	size := uint16(1)
	if w {
		size = 2
	}
	step := size
	if c.flag(DF) {
		step = -size
	}
	for {
		if c.rep != 0 && c.R[CX] == 0 {
			return
		}
		src := c.seg(DS)
		switch op &^ 1 {
		case 0xA4: // MOVS
			if w {
				c.Write16(c.S[ES], c.R[DI], c.Read16(src, c.R[SI]))
			} else {
				c.Write8(c.S[ES], c.R[DI], c.Read8(src, c.R[SI]))
			}
			c.R[SI] += step
			c.R[DI] += step
		case 0xA6: // CMPS
			if w {
				c.alu(7, uint32(c.Read16(src, c.R[SI])), uint32(c.Read16(c.S[ES], c.R[DI])), true)
			} else {
				c.alu(7, uint32(c.Read8(src, c.R[SI])), uint32(c.Read8(c.S[ES], c.R[DI])), false)
			}
			c.R[SI] += step
			c.R[DI] += step
		case 0xAA: // STOS
			if w {
				c.Write16(c.S[ES], c.R[DI], c.R[AX])
			} else {
				c.Write8(c.S[ES], c.R[DI], c.Reg8(0))
			}
			c.R[DI] += step
		case 0xAC: // LODS
			if w {
				c.R[AX] = c.Read16(src, c.R[SI])
			} else {
				c.SetReg8(0, c.Read8(src, c.R[SI]))
			}
			c.R[SI] += step
		case 0xAE: // SCAS
			if w {
				c.alu(7, uint32(c.R[AX]), uint32(c.Read16(c.S[ES], c.R[DI])), true)
			} else {
				c.alu(7, uint32(c.Reg8(0)), uint32(c.Read8(c.S[ES], c.R[DI])), false)
			}
			c.R[DI] += step
		}
		if c.rep == 0 {
			return
		}
		c.R[CX]--
		if op&^1 == 0xA6 || op&^1 == 0xAE {
			if c.rep == repZ && !c.flag(ZF) || c.rep == repNZ && c.flag(ZF) {
				return
			}
		}
	}
}
