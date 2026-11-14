package termrun

import (
	"bytes"
	"strings"
	"testing"

	"goqb/gfx"
)

func testFrame() *gfx.Frame {
	g, _ := gfx.New(13)
	g.Line(true, false, 0, 0, false, 319, 199, 4, 2, -1)
	g.Circle(false, 160, 100, 50, 14, 0, 0, 0, false, false, false)
	return g.Frame()
}

func TestEncoders(t *testing.T) {
	f := testFrame()
	var k bytes.Buffer
	writeKitty(&k, f, Rect{0, 0, 40, 15})
	ks := k.String()
	if !strings.Contains(ks, "\x1b_Ga=T,f=100,i=1,q=2,C=1,c=40,r=15,m=") || !strings.HasSuffix(ks, "\x1b\\") {
		t.Errorf("kitty: encabezado inválido: %q", ks[:min(80, len(ks))])
	}
	var it bytes.Buffer
	writeITerm(&it, f, Rect{0, 0, 40, 15})
	if !strings.HasPrefix(it.String(), "\x1b]1337;File=inline=1;") || !strings.HasSuffix(it.String(), "\a") {
		t.Error("iTerm2: secuencia inválida")
	}
	var sx bytes.Buffer
	writeSixel(&sx, f, 160, 100)
	s := sx.String()
	if !strings.HasPrefix(s, "\x1bPq\"1;1;160;100#0;2;") || !strings.HasSuffix(s, "-\x1b\\") {
		t.Errorf("sixel inválido: %q", s[:min(60, len(s))])
	}
	if strings.Count(s, "-") != 17 { // ceil(100/6) bandas
		t.Errorf("sixel: %d bandas", strings.Count(s, "-"))
	}
}
