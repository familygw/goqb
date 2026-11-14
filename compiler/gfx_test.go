package compiler_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"goqb/compiler"
	"goqb/gfx"
	"goqb/lang/source"
	"goqb/vm"
)

func TestGraphics(t *testing.T) {
	path, _ := filepath.Abs("../testdata/gfx/dibujo.bas")
	prog, diags := (&source.Loader{}).Load(path)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	p, _, diags := compiler.Compile(prog)
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	var out bytes.Buffer
	con := vm.NewPlainConsole(strings.NewReader(""), &out)
	m := vm.New(p, con)
	if err := m.Run(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 4 {
		t.Fatalf("salida: %q", out.String())
	}
	if got := strings.Join(strings.Fields(lines[1]), " "); got != "1 15 -1" {
		t.Errorf("POINT: %q", lines[1])
	}
	if got := strings.Join(strings.Fields(lines[2]), " "); got != "50 50" {
		t.Errorf("PMAP: %q", lines[2])
	}
	if got := strings.TrimSpace(lines[3]); got != "1" {
		t.Errorf("CIRCLE en SCREEN 13: POINT = %q", lines[3])
	}
	g := con.Graphics()
	if g == nil || g.M.N != 13 {
		t.Fatal("debería quedar en SCREEN 13")
	}
	f := g.Frame()
	if f.Pal[1] != 0xFF0000 { // 63 = rojo al máximo (azul*65536 + verde*256 + rojo)
		t.Errorf("PALETTE 1, 63 en VGA: %06X", f.Pal[1])
	}
}

func TestGraphicsPrimitives(t *testing.T) {
	g, _ := gfx.New(12)
	g.Line(true, false, 0, 0, false, 9, 0, 4, 0, -1)
	f := g.Frame()
	for x := 0; x < 10; x++ {
		if f.Pix[x] != 4 {
			t.Fatalf("LINE: píxel %d = %d", x, f.Pix[x])
		}
	}
	g.Line(true, false, 20, 20, false, 40, 40, 2, 1, -1) // caja
	g.Paint(false, 30, 30, 5, nil, 2)
	f = g.Frame()
	if f.Pix[30*640+30] != 5 || f.Pix[19*640+30] != 0 || f.Pix[20*640+30] != 2 {
		t.Error("PAINT no respetó el borde")
	}
	img, err := g.Get(false, 20, 20, false, 40, 40)
	if err != nil || len(img) != 4+21*((21*4+7)/8) {
		t.Fatalf("GET: %v, %d bytes", err, len(img))
	}
	if err := g.Put(false, 100, 100, img, "PSET"); err != nil {
		t.Fatal(err)
	}
	f = g.Frame()
	if f.Pix[110*640+110] != 5 || f.Pix[100*640+100] != 2 {
		t.Error("PUT no copió la imagen")
	}
	g.DrawChar(1, 2, 'A', 15, 0)
	f = g.Frame()
	lit := 0
	for y := 16; y < 32; y++ {
		for x := 0; x < 8; x++ {
			if f.Pix[y*640+x] == 15 {
				lit++
			}
		}
	}
	if lit < 20 {
		t.Errorf("DrawChar: sólo %d píxeles encendidos", lit)
	}
}
