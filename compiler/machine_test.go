package compiler_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"goqb/gfx"
	"goqb/runtime/screen"
	"goqb/vm"
)

// TestMachineCode prueba el modelo de memoria y el 8086 emulado.
func TestMachineCode(t *testing.T) {
	src := `DEFINT A-Z
TYPE RegType
  ax AS INTEGER
  bx AS INTEGER
  cx AS INTEGER
  dx AS INTEGER
  bp AS INTEGER
  si AS INTEGER
  di AS INTEGER
  flags AS INTEGER
END TYPE
DIM SHARED code(1 TO 16)
DIM r AS RegType, o AS RegType
' duplica su argumento: PUSH BP / MOV BP,SP / MOV BX,[BP+6] / MOV AX,[BX] / ADD AX,AX / MOV [BX],AX / POP BP / RETF 2
DATA &H55,&H89,&HE5,&H8B,&H5E,&H06,&H8B,&H07,&H01,&HC0,&H89,&H07,&H5D,&HCA,&H02,&H00
P = VARPTR(code(1))
DEF SEG = VARSEG(code(1))
FOR i = 0 TO 15: READ b: POKE P + i, b: NEXT
DEF SEG
x = 21
DEF SEG = VARSEG(code(1))
CALL ABSOLUTE(x, VARPTR(code(1)))
DEF SEG
r1$ = STR$(x) + " " + HEX$(code(1))
' bucle de 32 bits (como una rutina de demora)
RESTORE lazo
FOR i = 0 TO 27: READ b: DEF SEG = VARSEG(code(1)): POKE VARPTR(code(1)) + i, b: NEXT
t& = 70000
CALL ABSOLUTE(t&, VARPTR(code(1)))
DEF SEG
r2$ = STR$(t&)
s$ = "ABC"
r3$ = STR$(PEEK(SADD(s$))) + STR$(PEEK(SADD(s$) + 2))
POKE SADD(s$) + 1, 122
' scroll de MONEY.BAS (INT 10h AH=06h)
CLS
FOR i = 1 TO 5: LOCATE i, 1: PRINT "fila"; i: NEXT
scrolldata:
DATA &HB8,&H01,&H06,&HB9,&H00,&H00,&HBA,&H10,&H04,&HB7,&H07,&HCD,&H10,&HCB
RESTORE scrolldata
FOR i = 0 TO 13: READ b: DEF SEG = VARSEG(code(1)): POKE VARPTR(code(1)) + i, b: NEXT
CALL ABSOLUTE(VARPTR(code(1)))
DEF SEG
LOCATE 10, 1: PRINT CHR$(SCREEN(1, 1)); CHR$(SCREEN(1, 6)); "|"; CHR$(SCREEN(5, 6)); "|"
' CALL INTERRUPT: posición del cursor
LOCATE 5, 7
r.ax = &H300
CALL INTERRUPT(&H10, r, o)
LOCATE 11, 1: PRINT HEX$(o.dx)
DEF SEG = &HB800: POKE 0, 65: POKE 1, 30: DEF SEG
LOCATE 12, 1: PRINT CHR$(SCREEN(1, 1)); SCREEN(1, 1, 1)
DEF SEG = 0: PRINT PEEK(&H44A); : DEF SEG
code(2) = 1234
DEF SEG = VARSEG(code(1))
BSAVE "arr.bin", VARPTR(code(1)), 32
code(2) = 0
BLOAD "arr.bin"
DEF SEG
PRINT code(2)
LOCATE 14, 1: PRINT r1$; "|"; r2$; "|"; r3$; "|"; s$
END
lazo:
' PUSH BP / MOV BP,SP / MOV BX,[BP+6] / MOV CX,[BX] / MOV DX,[BX+2] / L: SUB CX,1 / SBB DX,0 / MOV AX,CX / OR AX,DX / JNZ L / MOV [BX],DX / POP BP / RETF 2 / NOP
DATA &H55,&H89,&HE5,&H8B,&H5E,&H06,&H8B,&H0F,&H8B,&H57,&H02,&H83,&HE9,&H01,&H83,&HDA,&H00,&H89,&HC8,&H09,&HD0,&H75,&HF4,&H89,&H17,&H5D,&HCA,&H02
`
	os.Setenv("GOQB_CPU_IPS", "0")
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)
	p := compileSrc(t, src)
	txt := screen.New(80, 25)
	m := vm.New(p, txt)
	if err := m.Run(); err != nil {
		t.Fatal(err)
	}
	row := func(r int) string {
		var b strings.Builder
		for c := 1; c <= 40; c++ {
			ch, _ := txt.CharAt(r, c)
			b.WriteByte(ch)
		}
		return strings.TrimRight(b.String(), " ")
	}
	want := map[int]string{10: "f2| |", 11: "406", 12: "A 30", 13: " 80  1234", 14: " 42 8955| 65536| 65 67|AzC"}
	for r, w := range want {
		if got := row(r); got != w {
			t.Errorf("fila %d: %q, se esperaba %q", r, got, w)
		}
	}
}

// TestBIOSFont: la fuente 8×8 de la BIOS en F000:FA6E y la de INT 43h, de solo lectura.
func TestBIOSFont(t *testing.T) {
	src := `DEFINT A-Z
DEF SEG = &HF000
FOR i = 0 TO 7: a$ = a$ + HEX$(PEEK(&HFA6E + ASC("A") * 8 + i)) + " ": NEXT
POKE &HFA6E + 65 * 8, 0
b = PEEK(&HFA6E + 65 * 8)
DEF SEG = 0
sg& = PEEK(&H10E) + 256& * PEEK(&H10F): ofs& = PEEK(&H10C) + 256& * PEEK(&H10D)
DEF SEG = sg&
c = PEEK(ofs& + 200 * 8 + 4)
DEF SEG
PRINT a$
PRINT b; HEX$(sg&); c
`
	p := compileSrc(t, src)
	txt := screen.New(80, 25)
	if err := vm.New(p, txt).Run(); err != nil {
		t.Fatal(err)
	}
	row := func(r int) string {
		var b strings.Builder
		for c := 1; c <= 40; c++ {
			ch, _ := txt.CharAt(r, c)
			b.WriteByte(ch)
		}
		return strings.TrimRight(b.String(), " ")
	}
	var want strings.Builder
	for _, v := range gfx.Font8x8['A'] {
		want.WriteString(strings.ToUpper(strconv.FormatInt(int64(v), 16)))
		want.WriteString(" ")
	}
	if got := row(1); got != strings.TrimSpace(want.String()) {
		t.Errorf("fuente de 'A': %q, se esperaba %q", got, want.String())
	}
	w2 := " " + strconv.Itoa(int(gfx.Font8x8['A'][0])) + " C000 " + strconv.Itoa(int(gfx.Font8x8[200][4]))
	if got := row(2); got != w2 {
		t.Errorf("ROM / INT 43h: %q, se esperaba %q", got, w2)
	}
}
