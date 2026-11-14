package x86

import "testing"

type ram [1 << 20]byte

func (r *ram) Read8(a uint32) byte     { return r[a] }
func (r *ram) Write8(a uint32, v byte) { r[a] = v }
func (r *ram) In(uint16) byte          { return 0xFF }
func (r *ram) Out(uint16, byte)        {}

// run carga code en 1000:0000 y lo ejecuta hasta el RETF al centinela FFFF:0000.
func run(t *testing.T, code []byte, setup func(c *CPU)) (*CPU, *ram) {
	t.Helper()
	m := &ram{}
	copy(m[0x10000:], code)
	c := New(m)
	c.S[SS], c.R[SP] = 0x9000, 0xFFF0
	c.S[DS], c.S[ES] = 0x2000, 0x2000
	c.Push(0xFFFF)
	c.Push(0)
	c.S[CS], c.IP = 0x1000, 0
	if setup != nil {
		setup(c)
	}
	if err := c.Run(0xFFFF, 0, 1_000_000, nil); err != nil {
		t.Fatal(err)
	}
	return c, m
}

func TestArithmetic(t *testing.T) {
	c, _ := run(t, []byte{
		0xB8, 0xFF, 0x7F, // MOV AX,7FFF
		0x05, 0x01, 0x00, // ADD AX,1
		0x9C,             // PUSHF
		0x5A,             // POP DX
		0xB8, 0x34, 0x12, // MOV AX,1234
		0xB9, 0x10, 0x00, // MOV CX,10
		0xF7, 0xE1, // MUL CX -> DX:AX = 12340
		0xF7, 0xF1, // DIV CX -> AX = 1234
		0xBB, 0x05, 0x00, // MOV BX,5
		0xD1, 0xE3, // SHL BX,1
		0x4B, // DEC BX -> 9
		0xCB, // RETF
	}, nil)
	if c.R[AX] != 0x1234 || c.R[BX] != 9 {
		t.Errorf("AX=%04X BX=%04X", c.R[AX], c.R[BX])
	}
}

func TestOverflowFlags(t *testing.T) {
	c, _ := run(t, []byte{0xB8, 0xFF, 0x7F, 0x05, 0x01, 0x00, 0xCB}, nil)
	if c.Flags&OF == 0 || c.Flags&SF == 0 || c.Flags&CF != 0 || c.R[AX] != 0x8000 {
		t.Errorf("AX=%04X flags=%04X", c.R[AX], c.Flags)
	}
	c, _ = run(t, []byte{0xB0, 0x00, 0x2C, 0x01, 0xCB}, nil) // MOV AL,0 / SUB AL,1
	if c.Flags&CF == 0 || c.Reg8(0) != 0xFF {
		t.Errorf("AL=%02X flags=%04X", c.Reg8(0), c.Flags)
	}
}

func TestStrings(t *testing.T) {
	_, m := run(t, []byte{
		0xBE, 0x00, 0x00, // MOV SI,0
		0xBF, 0x10, 0x00, // MOV DI,10
		0xB9, 0x05, 0x00, // MOV CX,5
		0xFC,       // CLD
		0xF3, 0xA4, // REP MOVSB
		0xB0, 0x2A, // MOV AL,'*'
		0xB9, 0x03, 0x00, // MOV CX,3
		0xF3, 0xAA, // REP STOSB
		0xCB,
	}, func(c *CPU) {
		for i, b := range []byte("hola!") {
			c.Write8(0x2000, uint16(i), b)
		}
	})
	if got := string(m[0x20010:0x20018]); got != "hola!***" {
		t.Errorf("%q", got)
	}
}

func TestLoopAndInt(t *testing.T) {
	var got byte
	c, _ := run(t, []byte{
		0x31, 0xC0, // XOR AX,AX
		0xB9, 0x0A, 0x00, // MOV CX,10
		0x40,       // INC AX
		0xE2, 0xFD, // LOOP -3
		0xB4, 0x02, // MOV AH,2
		0xB2, 0x41, // MOV DL,'A'
		0xCD, 0x21, // INT 21h
		0xCB,
	}, func(c *CPU) {
		c.Int = func(c *CPU, n byte) bool { got = c.Reg8(2); return true }
	})
	if c.Reg8(0) != 10 || got != 'A' {
		t.Errorf("AL=%d int=%c", c.Reg8(0), got)
	}
}
