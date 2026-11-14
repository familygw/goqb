// Package gfx implementa los modos gráficos de QuickBASIC 4.5: framebuffer
// indexado con páginas, paletas CGA/EGA/VGA, coordenadas con VIEW/WINDOW y
// las primitivas PSET, LINE, CIRCLE, PAINT, DRAW, GET/PUT y texto.
//
// Es independiente de la presentación: los backends (bloques, kitty/sixel,
// ventana) leen un Frame y lo muestran.
package gfx

import (
	"errors"
	"math"
)

// ErrIllegal se devuelve ante argumentos inválidos (Illegal function call).
var ErrIllegal = errors.New("illegal function call")

// Mode describe un modo SCREEN.
type Mode struct {
	N       int
	W, H    int
	Colors  int // atributos disponibles
	CharH   int // alto de la celda de texto en píxeles
	Cols    int // columnas de texto
	Rows    int // filas de texto
	Pages   int
	BPP     int // bits por píxel en GET/PUT
	PalKind int // 0 CGA fija, 1 EGA (64 colores), 2 VGA (18 bits)
}

// CharW es el ancho de la celda de texto en píxeles (9 en Hercules).
func (m Mode) CharW() int { return m.W / m.Cols }

// Modes son los modos gráficos de QuickBASIC 4.5. Los modos 5 y 6 no
// existen en QB (SCREEN 5 o 6 da "Illegal function call").
var Modes = map[int]Mode{
	1:  {1, 320, 200, 4, 8, 40, 25, 1, 2, 0},
	2:  {2, 640, 200, 2, 8, 80, 25, 1, 1, 0},
	3:  {3, 720, 348, 2, 14, 80, 25, 2, 1, 0}, // Hercules (monocromo, 2 páginas)
	4:  {4, 640, 400, 2, 16, 80, 25, 1, 1, 0}, // Olivetti / AT&T 6300: negro + un color elegido con COLOR
	7:  {7, 320, 200, 16, 8, 40, 25, 8, 4, 1},
	8:  {8, 640, 200, 16, 8, 80, 25, 4, 4, 1},
	9:  {9, 640, 350, 16, 14, 80, 25, 2, 4, 1},
	10: {10, 640, 350, 4, 14, 80, 25, 2, 2, 1},
	11: {11, 640, 480, 2, 16, 80, 30, 1, 1, 2},
	12: {12, 640, 480, 16, 16, 80, 30, 1, 4, 2},
	13: {13, 320, 200, 256, 8, 40, 25, 1, 8, 2},
}

// Frame es la imagen visible lista para mostrar.
type Frame struct {
	W, H         int
	Pix          []uint8     // atributo de color por píxel
	Pal          [256]uint32 // atributo → 0xRRGGBB
	CharW, CharH int         // celda de texto del modo (para reconocer texto al reducir la imagen)
}

// RGB devuelve el color del píxel (x, y).
func (f *Frame) RGB(x, y int) uint32 { return f.Pal[f.Pix[y*f.W+x]] }

// Screen es la pantalla gráfica.
type Screen struct {
	M            Mode
	pages        [][]uint8
	apage, vpage int
	pal          [256]uint32 // atributo → RGB
	fg, bg       int

	// viewport físico (inclusive) y si sus coordenadas son absolutas (VIEW SCREEN)
	vx1, vy1, vx2, vy2 int
	viewSet, viewAbs   bool
	// WINDOW
	win, winScreen     bool
	wx1, wy1, wx2, wy2 float64
	// último punto (coordenadas lógicas)
	lx, ly float64
	// estado de DRAW
	drawScale, drawAngle int

	Version uint64
}

// New crea la pantalla para el modo n.
func New(n int) (*Screen, error) {
	m, ok := Modes[n]
	if !ok {
		return nil, ErrIllegal
	}
	s := &Screen{M: m, drawScale: 4}
	s.pages = make([][]uint8, m.Pages)
	for i := range s.pages {
		s.pages[i] = make([]uint8, m.W*m.H)
	}
	s.ResetPalette()
	s.fg = m.Colors - 1
	if m.Colors == 256 {
		s.fg = 15
	}
	s.resetView()
	return s, nil
}

func (s *Screen) touch() { s.Version++ }

func (s *Screen) pix() []uint8 { return s.pages[s.apage] }

// Frame copia la página visible.
func (s *Screen) Frame() *Frame {
	f := &Frame{W: s.M.W, H: s.M.H, Pal: s.pal, CharW: s.M.CharW(), CharH: s.M.CharH}
	f.Pix = append([]uint8(nil), s.pages[s.vpage]...)
	return f
}

// SetPages cambia la página activa y la visible.
func (s *Screen) SetPages(apage, vpage int) error {
	if apage < 0 || apage >= s.M.Pages || vpage < 0 || vpage >= s.M.Pages {
		return ErrIllegal
	}
	s.apage, s.vpage = apage, vpage
	s.touch()
	return nil
}

// PCopy copia una página en otra.
func (s *Screen) PCopy(src, dst int) error {
	if src < 0 || src >= s.M.Pages || dst < 0 || dst >= s.M.Pages {
		return ErrIllegal
	}
	copy(s.pages[dst], s.pages[src])
	s.touch()
	return nil
}

// ------------------------------------------------------------- paletas

var egaRGB = func() [64]uint32 {
	var t [64]uint32
	for i := 0; i < 64; i++ {
		// bits: 0 azul, 1 verde, 2 rojo (2/3 de intensidad), 3 azul, 4 verde, 5 rojo (1/3)
		ch := func(hi, lo int) uint32 {
			v := uint32(0)
			if i>>hi&1 != 0 {
				v += 0xAA
			}
			if i>>lo&1 != 0 {
				v += 0x55
			}
			return v
		}
		t[i] = ch(2, 5)<<16 | ch(1, 4)<<8 | ch(0, 3)
	}
	return t
}()

// CGA16 son los 16 colores de texto/EGA por defecto.
var CGA16 = [16]uint32{
	0x000000, 0x0000AA, 0x00AA00, 0x00AAAA, 0xAA0000, 0xAA00AA, 0xAA5500, 0xAAAAAA,
	0x555555, 0x5555FF, 0x55FF55, 0x55FFFF, 0xFF5555, 0xFF55FF, 0xFFFF55, 0xFFFFFF,
}

var egaDefault = [16]int{0, 1, 2, 3, 4, 5, 20, 7, 56, 57, 58, 59, 60, 61, 62, 63}

// ResetPalette restablece la paleta por defecto del modo (PALETTE sin argumentos).
func (s *Screen) ResetPalette() {
	switch s.M.N {
	case 1:
		s.cgaPalette(0, 1)
	case 2, 3, 4, 11:
		s.pal[0], s.pal[1] = 0, 0xFFFFFF
	case 10:
		s.pal[0], s.pal[1], s.pal[2], s.pal[3] = 0, 0x555555, 0xAAAAAA, 0xFFFFFF
	case 13:
		s.pal = vgaDefault
	default:
		for i := 0; i < 16; i++ {
			s.pal[i] = CGA16[i]
		}
	}
	s.touch()
}

func (s *Screen) cgaPalette(bg, which int) {
	s.pal[0] = CGA16[bg&15]
	if which&1 == 1 {
		s.pal[1], s.pal[2], s.pal[3] = CGA16[11], CGA16[13], CGA16[15]
	} else {
		s.pal[1], s.pal[2], s.pal[3] = CGA16[10], CGA16[12], CGA16[14]
	}
}

// vgaDefault aproxima la paleta por defecto del modo 13h.
var vgaDefault = func() [256]uint32 {
	var p [256]uint32
	for i := 0; i < 16; i++ {
		p[i] = CGA16[i]
	}
	gray := []int{0, 5, 8, 11, 14, 17, 20, 24, 28, 32, 36, 40, 45, 50, 56, 63}
	for i, g := range gray {
		v := uint32(g * 255 / 63)
		p[16+i] = v<<16 | v<<8 | v
	}
	// 24 tonos × 3 saturaciones × 3 intensidades
	idx := 32
	for _, inten := range []float64{1, 0.45, 0.25} {
		for _, sat := range []float64{1, 0.55, 0.3} {
			for h := 0; h < 24; h++ {
				r, g, b := hsv(float64(h)/24, sat, inten)
				p[idx] = r<<16 | g<<8 | b
				idx++
			}
		}
	}
	return p
}()

func hsv(h, s, v float64) (uint32, uint32, uint32) {
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return uint32(r * 255), uint32(g * 255), uint32(b * 255)
}

// Palette implementa PALETTE atributo, color.
func (s *Screen) Palette(attr int, color int64) error {
	if attr < 0 || attr >= s.M.Colors || attr > 255 {
		return ErrIllegal
	}
	switch s.M.PalKind {
	case 0: // CGA: sin paleta programable, salvo el fondo
		if color < 0 || color > 15 {
			return ErrIllegal
		}
		s.pal[attr] = CGA16[color]
	case 1:
		if color < 0 || color > 63 {
			return ErrIllegal
		}
		s.pal[attr] = egaRGB[color]
	case 2: // azul*65536 + verde*256 + rojo, cada uno 0..63
		if color < 0 {
			return ErrIllegal
		}
		r, g, b := uint32(color&0x3F), uint32(color>>8&0x3F), uint32(color>>16&0x3F)
		s.pal[attr] = (r*255/63)<<16 | (g*255/63)<<8 | b*255/63
	}
	s.touch()
	return nil
}

// Color implementa COLOR en modo gráfico (argumentos -1 = omitidos).
func (s *Screen) Color(a, b int) error {
	switch s.M.N {
	case 1: // COLOR fondo, paleta
		if a >= 0 {
			if a > 15 {
				return ErrIllegal
			}
			s.pal[0] = CGA16[a]
		}
		if b >= 0 {
			bg := 0
			for i, c := range CGA16 {
				if c == s.pal[0] {
					bg = i
				}
			}
			s.cgaPalette(bg, b)
		}
	case 3: // Hercules: sin COLOR
		return ErrIllegal
	case 4: // COLOR primer plano (0-15): el color del atributo 1; sin fondo
		if b >= 0 || a > 15 {
			return ErrIllegal
		}
		if a >= 0 {
			s.pal[1] = CGA16[a]
		}
	case 2, 11:
		if a >= 0 || b >= 0 {
			if s.M.N == 2 {
				return ErrIllegal
			}
		}
		if a >= 0 {
			s.fg = a & 1
		}
	default:
		if a >= 0 {
			if a >= s.M.Colors {
				return ErrIllegal
			}
			s.fg = a
		}
		if b >= 0 && s.M.Colors == 16 && s.M.N != 12 {
			// en EGA el fondo reprograma el atributo 0
			if b > 15 {
				return ErrIllegal
			}
			s.pal[0] = CGA16[b]
		}
	}
	s.touch()
	return nil
}

// Foreground devuelve el color de dibujo por defecto.
func (s *Screen) Foreground() int { return s.fg }

// ------------------------------------------------------------- coordenadas

func (s *Screen) resetView() {
	s.vx1, s.vy1, s.vx2, s.vy2 = 0, 0, s.M.W-1, s.M.H-1
	s.viewSet, s.viewAbs, s.win = false, false, false
	s.centerLast()
}

func (s *Screen) centerLast() {
	if s.win {
		s.lx, s.ly = (s.wx1+s.wx2)/2, (s.wy1+s.wy2)/2
		return
	}
	cx, cy := float64(s.vx2-s.vx1)/2, float64(s.vy2-s.vy1)/2
	if s.viewAbs || !s.viewSet {
		cx += float64(s.vx1)
		cy += float64(s.vy1)
	}
	s.lx, s.ly = math.Round(cx), math.Round(cy)
}

// View implementa VIEW [[SCREEN] (x1,y1)-(x2,y2) [,[color][,border]]].
func (s *Screen) View(set, abs bool, x1, y1, x2, y2 int, fill, border int) error {
	if !set {
		s.resetView()
		s.touch()
		return nil
	}
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if x1 < 0 || y1 < 0 || x2 >= s.M.W || y2 >= s.M.H {
		return ErrIllegal
	}
	if border >= 0 {
		s.rectRaw(x1-1, y1-1, x2+1, y2+1, border, false)
	}
	s.vx1, s.vy1, s.vx2, s.vy2 = x1, y1, x2, y2
	s.viewSet, s.viewAbs = true, abs
	if fill >= 0 {
		s.rectRaw(x1, y1, x2, y2, fill, true)
	}
	s.centerLast()
	s.touch()
	return nil
}

// Window implementa WINDOW [[SCREEN] (x1,y1)-(x2,y2)].
func (s *Screen) Window(set, screen bool, x1, y1, x2, y2 float64) error {
	if !set {
		s.win = false
		s.centerLast()
		return nil
	}
	if x1 == x2 || y1 == y2 {
		return ErrIllegal
	}
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	s.win, s.winScreen = true, screen
	s.wx1, s.wy1, s.wx2, s.wy2 = x1, y1, x2, y2
	s.centerLast()
	return nil
}

// physX convierte una coordenada lógica x a física (sin redondear).
func (s *Screen) physX(x float64) float64 {
	if s.win {
		return float64(s.vx1) + (x-s.wx1)*float64(s.vx2-s.vx1)/(s.wx2-s.wx1)
	}
	if s.viewSet && !s.viewAbs {
		return x + float64(s.vx1)
	}
	return x
}

func (s *Screen) physY(y float64) float64 {
	if s.win {
		h := float64(s.vy2 - s.vy1)
		if s.winScreen {
			return float64(s.vy1) + (y-s.wy1)*h/(s.wy2-s.wy1)
		}
		return float64(s.vy1) + (s.wy2-y)*h/(s.wy2-s.wy1)
	}
	if s.viewSet && !s.viewAbs {
		return y + float64(s.vy1)
	}
	return y
}

func (s *Screen) logX(px float64) float64 {
	if s.win {
		return s.wx1 + (px-float64(s.vx1))*(s.wx2-s.wx1)/float64(s.vx2-s.vx1)
	}
	if s.viewSet && !s.viewAbs {
		return px - float64(s.vx1)
	}
	return px
}

func (s *Screen) logY(py float64) float64 {
	if s.win {
		h := float64(s.vy2 - s.vy1)
		if s.winScreen {
			return s.wy1 + (py-float64(s.vy1))*(s.wy2-s.wy1)/h
		}
		return s.wy2 - (py-float64(s.vy1))*(s.wy2-s.wy1)/h
	}
	if s.viewSet && !s.viewAbs {
		return py - float64(s.vy1)
	}
	return py
}

func round(f float64) int { return int(math.RoundToEven(f)) }

// point resuelve un punto (con STEP relativo al último) y lo deja como último.
func (s *Screen) point(step bool, x, y float64) (int, int) {
	if step {
		x += s.lx
		y += s.ly
	}
	s.lx, s.ly = x, y
	return round(s.physX(x)), round(s.physY(y))
}

// relOrigin es el origen de las coordenadas físicas que ve el programa:
// con VIEW (sin SCREEN) son relativas a la esquina del viewport.
func (s *Screen) relOrigin() (float64, float64) {
	if s.viewSet && !s.viewAbs {
		return float64(s.vx1), float64(s.vy1)
	}
	return 0, 0
}

// PMap implementa PMAP(coord, función).
func (s *Screen) PMap(v float64, fn int) (float64, error) {
	ox, oy := s.relOrigin()
	switch fn {
	case 0:
		return float64(round(s.physX(v))) - ox, nil
	case 1:
		return float64(round(s.physY(v))) - oy, nil
	case 2:
		return s.logX(v + ox), nil
	case 3:
		return s.logY(v + oy), nil
	}
	return 0, ErrIllegal
}

// Point implementa POINT(x, y): atributo del píxel, -1 fuera del viewport.
func (s *Screen) Point(x, y float64) int {
	px, py := round(s.physX(x)), round(s.physY(y))
	if !s.inView(px, py) {
		return -1
	}
	return int(s.pix()[py*s.M.W+px])
}

// PointFn implementa POINT(n): posición del último punto.
func (s *Screen) PointFn(n int) (float64, error) {
	ox, oy := s.relOrigin()
	switch n {
	case 0:
		return float64(round(s.physX(s.lx))) - ox, nil
	case 1:
		return float64(round(s.physY(s.ly))) - oy, nil
	case 2:
		return s.lx, nil
	case 3:
		return s.ly, nil
	}
	return 0, ErrIllegal
}

// ------------------------------------------------------------- píxeles

func (s *Screen) inView(x, y int) bool {
	return x >= s.vx1 && x <= s.vx2 && y >= s.vy1 && y <= s.vy2
}

func (s *Screen) set(x, y, c int) {
	if s.inView(x, y) {
		s.pix()[y*s.M.W+x] = uint8(c % s.M.Colors)
	}
}

func (s *Screen) get(x, y int) int { return int(s.pix()[y*s.M.W+x]) }

// Pixel lee un pixel de la página activa sin VIEW ni WINDOW (memoria de video).
func (s *Screen) Pixel(x, y int) int {
	if x < 0 || y < 0 || x >= s.M.W || y >= s.M.H {
		return 0
	}
	return s.get(x, y)
}

// SetPixel escribe un pixel de la página activa sin VIEW ni WINDOW.
func (s *Screen) SetPixel(x, y, c int) {
	if x < 0 || y < 0 || x >= s.M.W || y >= s.M.H {
		return
	}
	s.pix()[y*s.M.W+x] = uint8(c)
	s.touch()
}

func (s *Screen) color(c int) int {
	if c < 0 {
		return s.fg
	}
	return c
}

// PSet implementa PSET/PRESET [STEP](x,y)[,color].
func (s *Screen) PSet(step bool, x, y float64, c int, preset bool) {
	px, py := s.point(step, x, y)
	if c < 0 {
		if preset {
			c = 0
		} else {
			c = s.fg
		}
	}
	s.set(px, py, c)
	s.touch()
}

// Line implementa LINE; box: 0 línea, 1 B, 2 BF. style < 0: sin estilo.
func (s *Screen) Line(hasFrom, step1 bool, x1, y1 float64, step2 bool, x2, y2 float64, c, box, style int) {
	var px1, py1 int
	if hasFrom {
		px1, py1 = s.point(step1, x1, y1)
	} else {
		px1, py1 = round(s.physX(s.lx)), round(s.physY(s.ly))
	}
	px2, py2 := s.point(step2, x2, y2)
	c = s.color(c)
	switch box {
	case 2:
		s.rectRaw(px1, py1, px2, py2, c, true)
	case 1:
		s.rectRaw(px1, py1, px2, py2, c, false)
	default:
		mask := uint16(0xFFFF)
		if style >= 0 {
			mask = uint16(style)
		}
		s.lineRaw(px1, py1, px2, py2, c, mask)
	}
	s.touch()
}

func (s *Screen) lineRaw(x1, y1, x2, y2, c int, mask uint16) {
	dx, dy := abs(x2-x1), -abs(y2-y1)
	sx, sy := 1, 1
	if x1 > x2 {
		sx = -1
	}
	if y1 > y2 {
		sy = -1
	}
	err := dx + dy
	bit := uint16(0x8000)
	for {
		if mask&bit != 0 {
			s.set(x1, y1, c)
		}
		bit = bit>>1 | bit<<15
		if x1 == x2 && y1 == y2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x1 += sx
		}
		if e2 <= dx {
			err += dx
			y1 += sy
		}
	}
}

func (s *Screen) rectRaw(x1, y1, x2, y2, c int, fill bool) {
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	if fill {
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				s.set(x, y, c)
			}
		}
		return
	}
	s.lineRaw(x1, y1, x2, y1, c, 0xFFFF)
	s.lineRaw(x1, y2, x2, y2, c, 0xFFFF)
	s.lineRaw(x1, y1, x1, y2, c, 0xFFFF)
	s.lineRaw(x2, y1, x2, y2, c, 0xFFFF)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DefaultAspect es la relación de aspecto por defecto de CIRCLE en el modo.
func (s *Screen) DefaultAspect() float64 {
	return 4.0 / 3.0 * float64(s.M.H) / float64(s.M.W)
}

// Circle implementa CIRCLE [STEP](x,y), r [,[c][,[inicio][,[fin][,aspecto]]]].
func (s *Screen) Circle(step bool, x, y, r float64, c int, start, end, aspect float64, hasStart, hasEnd, hasAspect bool) error {
	if !hasAspect {
		aspect = s.DefaultAspect()
	}
	if math.Abs(start) > 2*math.Pi+1e-6 || math.Abs(end) > 2*math.Pi+1e-6 {
		return ErrIllegal
	}
	if step {
		x += s.lx
		y += s.ly
	}
	s.lx, s.ly = x, y
	cx, cy := s.physX(x), s.physY(y)
	// radio en unidades físicas del eje x
	pr := r
	if s.win {
		pr = r * float64(s.vx2-s.vx1) / (s.wx2 - s.wx1)
	}
	rx, ry := pr, pr*aspect
	if aspect > 1 {
		rx, ry = pr/aspect, pr
	}
	c = s.color(c)
	if !hasStart {
		start = 0
	}
	if !hasEnd {
		end = 2 * math.Pi
	}
	full := !hasStart && !hasEnd
	lineStart, lineEnd := start < 0, end < 0 // ángulo negativo: radio hasta ese punto
	a0, a1 := math.Abs(start), math.Abs(end)
	if a1 < a0 {
		a1 += 2 * math.Pi
	}
	steps := int(math.Max(16, 2*math.Pi*math.Max(rx, ry)*1.5))
	span := a1 - a0
	if full {
		span = 2 * math.Pi
	}
	n := int(math.Ceil(float64(steps) * span / (2 * math.Pi)))
	if n < 1 {
		n = 1
	}
	px, py := 0, 0
	for i := 0; i <= n; i++ {
		a := a0 + span*float64(i)/float64(n)
		x2 := round(cx + rx*math.Cos(a))
		y2 := round(cy - ry*math.Sin(a))
		if i > 0 {
			s.lineRaw(px, py, x2, y2, c, 0xFFFF)
		}
		px, py = x2, y2
	}
	icx, icy := round(cx), round(cy)
	if lineStart {
		s.lineRaw(icx, icy, round(cx+rx*math.Cos(a0)), round(cy-ry*math.Sin(a0)), c, 0xFFFF)
	}
	if lineEnd {
		e := math.Abs(end)
		s.lineRaw(icx, icy, round(cx+rx*math.Cos(e)), round(cy-ry*math.Sin(e)), c, 0xFFFF)
	}
	s.touch()
	return nil
}

// Paint implementa PAINT [STEP](x,y)[,relleno[,borde]]. tile != nil: patrón de mosaico.
func (s *Screen) Paint(step bool, x, y float64, fill int, tile []byte, border int) {
	px, py := s.point(step, x, y)
	fill = s.color(fill)
	if border < 0 {
		border = fill
		if tile != nil {
			border = s.fg
		}
	}
	if !s.inView(px, py) || s.get(px, py) == border {
		s.touch()
		return
	}
	w := s.M.W
	visited := make([]bool, w*s.M.H)
	colorAt := func(x, y int) int {
		if tile == nil {
			return fill
		}
		return s.tileColor(tile, x, y)
	}
	type span struct{ x, y int }
	stack := []span{{px, py}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := p.x, p.y
		if visited[y*w+x] || s.get(x, y) == border {
			continue
		}
		l := x
		for l-1 >= s.vx1 && !visited[y*w+l-1] && s.get(l-1, y) != border {
			l--
		}
		r := x
		for r+1 <= s.vx2 && !visited[y*w+r+1] && s.get(r+1, y) != border {
			r++
		}
		for i := l; i <= r; i++ {
			visited[y*w+i] = true
			s.set(i, y, colorAt(i, y))
		}
		for _, ny := range []int{y - 1, y + 1} {
			if ny < s.vy1 || ny > s.vy2 {
				continue
			}
			in := false
			for i := l; i <= r; i++ {
				ok := !visited[ny*w+i] && s.get(i, ny) != border
				if ok && !in {
					stack = append(stack, span{i, ny})
				}
				in = ok
			}
		}
	}
	s.touch()
}

// tileColor calcula el color del mosaico de PAINT en (x, y).
func (s *Screen) tileColor(tile []byte, x, y int) int {
	switch s.M.BPP {
	case 1:
		b := tile[y%len(tile)]
		return int(b>>(7-uint(x%8))) & 1
	case 2:
		b := tile[y%len(tile)]
		return int(b>>(6-2*uint(x%4))) & 3
	case 8:
		rows := (len(tile) + 7) / 8
		return int(tile[(y%rows)*8%len(tile)+x%8%len(tile)])
	}
	// 4 planos: un byte por plano en cada fila
	rows := len(tile) / 4
	if rows == 0 {
		return int(tile[0]) & 15
	}
	base := (y % rows) * 4
	c := 0
	for p := 0; p < 4; p++ {
		if tile[base+p]>>(7-uint(x%8))&1 != 0 {
			c |= 1 << uint(p)
		}
	}
	return c
}

// Cls borra el área gráfica (el viewport si hay VIEW) con el color de fondo.
func (s *Screen) Cls() {
	if s.viewSet {
		s.rectRaw(s.vx1, s.vy1, s.vx2, s.vy2, 0, true)
	} else {
		p := s.pix()
		for i := range p {
			p[i] = 0
		}
	}
	s.centerLast()
	s.touch()
}
