package termrun

import (
	"math"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"goqb/gfx"
)

func TestParseReports(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"\x1b[6;18;8t\x1b[4;900;800t\x1b[8;50;100t\x1b[?62;22c", 18.0 / 8},
		{"\x1b[4;900;800t\x1b[8;50;100t\x1b[?1;2c", (900.0 / 50) / (800.0 / 100)},
		{"\x1b[?1;2c", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := parseReports(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("parseReports(%q) = %v, se esperaba %v", c.in, got, c.want)
		}
	}
}

// TestFitArea: el área elegida, medida en píxeles reales, es 4:3 en
// terminales grandes y con distintas proporciones de celda.
func TestFitArea(t *testing.T) {
	scr := tcell.NewSimulationScreen("UTF-8")
	scr.Init()
	defer scr.Fini()
	for _, sz := range [][2]int{{80, 25}, {200, 60}, {300, 50}, {120, 90}} {
		scr.SetSize(sz[0], sz[1])
		for _, asp := range []float64{1.8, 2, 2.3} {
			r := FitArea(scr, asp)
			if r.W > sz[0] || r.H > sz[1] {
				t.Fatalf("%v: área %v fuera de la pantalla", sz, r)
			}
			ratio := float64(r.W) / (float64(r.H) * asp)
			if math.Abs(ratio-4.0/3) > 0.05 {
				t.Errorf("%dx%d aspecto %.1f: área %dx%d da %.3f, no 4:3", sz[0], sz[1], asp, r.W, r.H, ratio)
			}
		}
	}
}

// TestSampleKeepsLines: al achicar SCREEN 12, una línea de un píxel no desaparece.
func TestSampleKeepsLines(t *testing.T) {
	g, _ := gfx.New(12)
	g.Line(true, false, 0, 101, false, 639, 101, 15, 0, -1) // horizontal, 1 px
	g.Line(true, false, 333, 0, false, 333, 479, 14, 0, -1) // vertical, 1 px
	f := g.Frame()
	scr := tcell.NewSimulationScreen("UTF-8")
	scr.Init()
	defer scr.Fini()
	scr.SetSize(100, 40)
	area := Rect{0, 0, 100, 37}
	(&blocks{}).Present(scr, f, area)
	white, yellow := tcell.NewHexColor(int32(f.Pal[15])), tcell.NewHexColor(int32(f.Pal[14]))
	rows, cols := map[int]bool{}, map[int]bool{}
	for y := 0; y < area.H; y++ {
		for x := 0; x < area.W; x++ {
			_, _, st, _ := scr.GetContent(x, y)
			fg, bg, _ := st.Decompose()
			if fg == white || bg == white {
				rows[y] = true
			}
			if fg == yellow || bg == yellow {
				cols[x] = true
			}
		}
	}
	if len(rows) == 0 || len(cols) == 0 {
		t.Fatalf("se perdieron líneas: filas con blanco %d, columnas con amarillo %d", len(rows), len(cols))
	}
	// la línea vertical tiene que estar en todas las filas
	n := 0
	for y := 0; y < area.H; y++ {
		for x := 0; x < area.W; x++ {
			_, _, st, _ := scr.GetContent(x, y)
			fg, bg, _ := st.Decompose()
			if fg == yellow || bg == yellow {
				n++
				break
			}
		}
	}
	if n < area.H-1 {
		t.Errorf("la línea vertical aparece en %d de %d filas", n, area.H)
	}
}

// TestOverlayText: el texto impreso en modo gráfico se lee en el backend de bloques.
func TestOverlayText(t *testing.T) {
	for _, mode := range []int{12, 13, 9, 1, 3} {
		g, _ := gfx.New(mode)
		g.Line(true, false, 0, 0, false, float64(g.M.W-1), float64(g.M.H-1), 1, 2, -1) // fondo de color
		put := func(col, row int, s string, fg, bg int) {
			for i := 0; i < len(s); i++ {
				g.DrawChar(col+i, row, s[i], fg, bg)
			}
		}
		put(3, 2, "Hola mundo", 15, 0)
		put(10, 5, "QBasic 4.5!", 14, 1)
		g.Circle(false, float64(g.M.W/2), float64(g.M.H/2), float64(g.M.H/4), 2, 0, 0, 0, false, false, false)
		f := g.Frame()
		scr := tcell.NewSimulationScreen("UTF-8")
		scr.Init()
		scr.SetSize(220, 60)
		area := FitArea(scr, 2)
		(&blocks{}).Present(scr, f, area)
		var all strings.Builder
		for y := 0; y < 60; y++ {
			for x := 0; x < 220; x++ {
				m, _, _, _ := scr.GetContent(x, y)
				all.WriteRune(m)
			}
			all.WriteByte('\n')
		}
		txt := all.String()
		for _, w := range []string{"Hola", "mundo", "QBasic", "4.5!"} {
			if !strings.Contains(txt, w) {
				t.Errorf("SCREEN %d: no aparece %q", mode, w)
			}
		}
		scr.Fini()
	}
}
