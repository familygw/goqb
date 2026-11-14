package termrun

import (
	"sync"

	"github.com/gdamore/tcell/v2"

	"goqb/gfx"
	"goqb/lang/cp437"
)

// Texto sobre los gráficos en el backend de bloques.
//
// Al reducir la imagen a medios bloques, las letras de 8 píxeles quedan en
// dos o tres columnas de la terminal y no se leen. Por eso se reconocen en
// el framebuffer las celdas de texto (PRINT en modo gráfico: un glifo de la
// fuente en dos colores, alineado a la grilla) y se escriben como caracteres
// reales de la terminal, con sus colores, en la zona que ocupan. Las
// palabras se escriben juntas desde la columna donde empiezan.

var (
	glyphOnce  sync.Once
	glyphIndex map[uint64]byte
)

// reconocibles: letras, dígitos, signos y los caracteres acentuados y
// griegos de CP437. Se excluyen los de recuadro y los bloques, que se
// confunden con dibujos y que los medios bloques ya muestran bien.
func recognizable(c int) bool {
	return c >= 0x21 && c <= 0x7E || c >= 0x80 && c <= 0xAF || c >= 0xE0 && c <= 0xFE
}

func buildGlyphIndex() {
	glyphIndex = map[uint64]byte{}
	for c := 0; c < 256; c++ {
		if !recognizable(c) {
			continue
		}
		k := glyphKey(gfx.Font8x8[c])
		if _, dup := glyphIndex[k]; !dup {
			glyphIndex[k] = byte(c)
		}
	}
}

func glyphKey(g [8]byte) uint64 {
	var k uint64
	for _, b := range g {
		k = k<<8 | uint64(b)
	}
	return k
}

// recognizeCell devuelve el carácter de la celda de texto (col, row) (base 0)
// si sus píxeles son exactamente un glifo de la fuente en dos colores.
func recognizeCell(f *gfx.Frame, col, row int) (ch byte, fg, bg uint8, ok bool) {
	cw, chh := f.CharW, f.CharH
	x0, y0 := col*cw, row*chh
	if x0+8 > f.W || y0 >= f.H {
		return 0, 0, 0, false
	}
	rows := min(chh, f.H-y0)
	// colores de la celda
	a, b := f.Pix[y0*f.W+x0], f.Pix[y0*f.W+x0]
	two := false
	for y := 0; y < rows; y++ {
		line := f.Pix[(y0+y)*f.W+x0 : (y0+y)*f.W+x0+8]
		for _, p := range line {
			if p != a {
				if !two {
					b, two = p, true
				} else if p != b {
					return 0, 0, 0, false
				}
			}
		}
	}
	if !two {
		return 0, 0, 0, false
	}
	// mapa de bits con el color a como "encendido"; las filas de la fuente
	// se estiran a la altura de la celda: todas las líneas de una misma fila
	// tienen que coincidir
	var g [8]byte
	var seen [8]bool
	for y := 0; y < rows; y++ {
		var bits byte
		line := f.Pix[(y0+y)*f.W+x0 : (y0+y)*f.W+x0+8]
		for x, p := range line {
			if p == a {
				bits |= 0x80 >> uint(x)
			}
		}
		fr := y * 8 / chh
		if seen[fr] && g[fr] != bits {
			return 0, 0, 0, false
		}
		g[fr], seen[fr] = bits, true
	}
	for i := range g {
		if !seen[i] { // fila cortada (Hercules): no se puede verificar
			return 0, 0, 0, false
		}
	}
	glyphOnce.Do(buildGlyphIndex)
	if c, found := glyphIndex[glyphKey(g)]; found {
		return c, a, b, true
	}
	for i := range g {
		g[i] = ^g[i]
	}
	if c, found := glyphIndex[glyphKey(g)]; found {
		return c, b, a, true
	}
	return 0, 0, 0, false
}

// overlayText escribe sobre los medios bloques el texto reconocido.
func overlayText(scr tcell.Screen, f *gfx.Frame, area Rect) {
	if f.CharW <= 0 || f.CharH <= 0 || area.W <= 0 || area.H <= 0 {
		return
	}
	cols, rows := f.W/f.CharW, (f.H+f.CharH-1)/f.CharH
	tx := func(c int) int { return area.X + c*area.W/cols }
	ty := func(r int) int { return area.Y + r*area.H/rows }
	type cell struct {
		ch     byte
		fg, bg uint8
	}
	line := make([]cell, cols)
	found := make([]bool, cols)
	for r := 0; r < rows; r++ {
		any := false
		for c := 0; c < cols; c++ {
			ch, fg, bg, ok := recognizeCell(f, c, r)
			found[c] = ok
			if ok {
				line[c] = cell{ch, fg, bg}
				any = true
			}
		}
		if !any {
			continue
		}
		// los espacios entre palabras (celdas lisas del color de fondo del
		// texto, hasta 3 seguidas) también son parte del texto: así la línea
		// se escribe de corrido y no quedan restos de los glifos reducidos
		for c := 0; c < cols; {
			if found[c] {
				c++
				continue
			}
			start := c
			for c < cols && !found[c] {
				c++
			}
			if start == 0 || c == cols || c-start > 3 {
				continue
			}
			bg := line[start-1].bg
			if line[c].bg != bg {
				continue
			}
			ok := true
			for i := start; i < c && ok; i++ {
				u, uni := uniformCell(f, i, r)
				ok = uni && u == bg
			}
			if ok {
				for i := start; i < c; i++ {
					line[i], found[i] = cell{' ', line[start-1].fg, bg}, true
				}
			}
		}
		y0, y1 := ty(r), max(ty(r+1), ty(r)+1)
		mid := y0 + (y1-y0-1)/2
		for c := 0; c < cols; {
			if !found[c] {
				c++
				continue
			}
			// una palabra: celdas reconocidas seguidas
			start := c
			for c < cols && found[c] {
				c++
			}
			x0 := tx(start)
			// fondo de toda la zona que ocupaba la palabra
			for i := start; i < c; i++ {
				bgSt := tcell.StyleDefault.Background(tcell.NewHexColor(int32(f.Pal[line[i].bg])))
				for y := y0; y < y1; y++ {
					for x := tx(i); x < max(tx(i+1), tx(i)+1); x++ {
						scr.SetContent(x, y, ' ', nil, bgSt)
					}
				}
			}
			// letras juntas desde el comienzo de la palabra
			for i := start; i < c; i++ {
				x := x0 + (i - start)
				st := tcell.StyleDefault.
					Foreground(tcell.NewHexColor(int32(f.Pal[line[i].fg]))).
					Background(tcell.NewHexColor(int32(f.Pal[line[i].bg])))
				scr.SetContent(x, mid, cp437.Rune(line[i].ch), nil, st)
			}
		}
	}
}

// uniformCell informa si la celda de texto es de un solo color.
func uniformCell(f *gfx.Frame, col, row int) (uint8, bool) {
	x0, y0 := col*f.CharW, row*f.CharH
	if x0+f.CharW > f.W || y0 >= f.H {
		return 0, false
	}
	a := f.Pix[y0*f.W+x0]
	for y := y0; y < min(y0+f.CharH, f.H); y++ {
		for _, p := range f.Pix[y*f.W+x0 : y*f.W+x0+f.CharW] {
			if p != a {
				return 0, false
			}
		}
	}
	return a, true
}
