// Package screen implementa la pantalla de texto virtual de QB (80×25,
// 40/80 columnas, 25/43/50 filas) y el buffer de teclado. Implementa
// vm.Console; quien la muestra (goqbc run, el IDE) la dibuja leyendo
// Snapshot cuando cambia Version.
package screen

import (
	"errors"
	"sync"
	"sync/atomic"

	"goqb/gfx"
	"goqb/vm"
)

var _ vm.Console = (*Text)(nil)

// Cell es un carácter CP437 con su atributo (bits 0-3 frente, 4-6 fondo, 7 parpadeo).
type Cell struct {
	Ch   byte
	Attr byte
}

// ErrInterrupted lo devuelven las lecturas de teclado cuando el host interrumpe el programa.
var ErrInterrupted = errors.New("interrupted")

// Text es la pantalla virtual.
type Text struct {
	mu         sync.Mutex
	cols, rows int
	cells      []Cell
	row, col   int // 1-based; col puede valer cols+1 (salto de línea diferido)
	fg, bg     int
	cursor     bool
	inputMode  bool
	viewTop    int
	viewBottom int
	viewSet    bool
	scrolled   int

	g         *gfx.Screen // nil en modo texto
	mode      int
	lastFrame *gfx.Frame
	lastVer   uint64

	version atomic.Uint64
	keys    chan string
	stopMu  sync.Mutex
	stop    chan struct{}

	// OnBeep se invoca con BEEP o CHR$(7).
	OnBeep func()

	filterMu sync.Mutex
	filter   func(string) bool
}

// SetKeyFilter instala un filtro de teclas (ON KEY): si devuelve true,
// la tecla queda atrapada y no llega al buffer de INKEY$.
func (t *Text) SetKeyFilter(f func(string) bool) {
	t.filterMu.Lock()
	t.filter = f
	t.filterMu.Unlock()
}

// New crea una pantalla de cols×rows.
func New(cols, rows int) *Text {
	t := &Text{keys: make(chan string, 256), stop: make(chan struct{})}
	t.fg, t.bg = 7, 0
	t.resize(cols, rows)
	return t
}

func (t *Text) resize(cols, rows int) {
	t.cols, t.rows = cols, rows
	t.cells = make([]Cell, cols*rows)
	t.viewSet = false
	t.resetView()
	t.clear(1, rows)
	t.row, t.col = 1, 1
}

// resetView: por defecto la última fila queda fuera del área de scroll (como en QB).
func (t *Text) resetView() {
	t.viewTop, t.viewBottom = 1, t.rows-1
	if t.rows < 2 {
		t.viewBottom = t.rows
	}
}

func (t *Text) attr() byte {
	a := byte(t.fg&15) | byte(t.bg&7)<<4
	if t.fg >= 16 || t.bg >= 8 {
		a |= 0x80
	}
	return a
}

func (t *Text) touch() { t.version.Add(1) }

// Version cambia cada vez que se modifica la pantalla.
func (t *Text) Version() uint64 { return t.version.Load() }

func (t *Text) clear(from, to int) {
	if t.g != nil {
		t.g.ClearText(from, to, 0)
	}
	a := byte(t.bg&7) << 4
	for r := from; r <= to; r++ {
		for c := 0; c < t.cols; c++ {
			t.cells[(r-1)*t.cols+c] = Cell{' ', a | byte(t.fg&15)}
		}
	}
}

func (t *Text) scroll() {
	top, bot := t.viewTop, t.viewBottom
	copy(t.cells[(top-1)*t.cols:], t.cells[top*t.cols:bot*t.cols])
	if t.g != nil {
		t.g.ScrollText(top, bot, 0)
	}
	t.clear(bot, bot)
	t.scrolled++
}

func (t *Text) newline() {
	t.col = 1
	if t.row == t.viewBottom || t.row == t.rows {
		if t.row >= t.viewTop && t.row <= t.viewBottom {
			t.scroll()
			return
		}
		return
	}
	t.row++
}

func (t *Text) putc(b byte) {
	if t.col > t.cols {
		t.newline()
	}
	t.cells[(t.row-1)*t.cols+t.col-1] = Cell{b, t.attr()}
	if t.g != nil {
		t.g.DrawChar(t.col, t.row, b, t.g.Foreground(), 0)
	}
	t.col++
}

// ------------------------------------------------------------- vm.Console

func (t *Text) Print(s string) {
	t.mu.Lock()
	beep := false
	for i := 0; i < len(s); i++ {
		switch b := s[i]; b {
		case 7:
			beep = true
		case 9:
			n := 8 - (t.col-1)%8
			for j := 0; j < n; j++ {
				t.putc(' ')
			}
		case 10, 13:
			t.newline()
		default:
			t.putc(b)
		}
	}
	t.touch()
	t.mu.Unlock()
	if beep {
		t.Beep()
	}
}

func (t *Text) Newline() {
	t.mu.Lock()
	t.newline()
	t.touch()
	t.mu.Unlock()
}

func (t *Text) Cls() {
	t.mu.Lock()
	if t.g != nil {
		t.g.Cls()
	}
	if t.viewSet {
		t.clear(t.viewTop, t.viewBottom)
		t.row = t.viewTop
	} else {
		t.clear(1, t.rows)
		t.row = 1
	}
	t.col = 1
	t.touch()
	t.mu.Unlock()
}

func (t *Text) Locate(row, col int) {
	t.mu.Lock()
	if row > 0 {
		t.row = row
	}
	if col > 0 {
		t.col = col
	} else if t.col > t.cols {
		t.col = t.cols
	}
	t.touch()
	t.mu.Unlock()
}

func (t *Text) Cursor() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.col > t.cols {
		return t.row, t.cols
	}
	return t.row, t.col
}

func (t *Text) SetCursor(v bool) {
	t.mu.Lock()
	t.cursor = v
	t.touch()
	t.mu.Unlock()
}

func (t *Text) Color(fg, bg, border int) {
	t.mu.Lock()
	if fg >= 0 {
		t.fg = fg
	}
	if bg >= 0 {
		t.bg = bg
	}
	t.mu.Unlock()
}

func (t *Text) Size() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cols, t.rows
}

func (t *Text) SetWidth(cols, rows int) error {
	t.mu.Lock()
	if t.g != nil { // en modo gráfico el tamaño lo fija el modo
		t.mu.Unlock()
		return nil
	}
	if cols == 0 {
		cols = t.cols
	}
	if rows == 0 {
		rows = t.rows
	}
	if cols != t.cols || rows != t.rows {
		t.resize(cols, rows)
	}
	t.touch()
	t.mu.Unlock()
	return nil
}

func (t *Text) ViewPrint(top, bottom int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if top == 0 && bottom == 0 {
		t.viewSet = false
		t.resetView()
		return nil
	}
	if top < 1 || bottom > t.rows || top > bottom {
		return vm.ErrIllegalCall
	}
	t.viewTop, t.viewBottom, t.viewSet = top, bottom, true
	t.row, t.col = top, 1
	return nil
}

func (t *Text) CharAt(row, col int) (byte, byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if row < 1 || row > t.rows || col < 1 || col > t.cols {
		return ' ', 7
	}
	c := t.cells[(row-1)*t.cols+col-1]
	return c.Ch, c.Attr
}

func (t *Text) PutChar(row, col int, ch, attr byte) {
	t.mu.Lock()
	if row >= 1 && row <= t.rows && col >= 1 && col <= t.cols {
		t.cells[(row-1)*t.cols+col-1] = Cell{ch, attr}
		t.touch()
	}
	t.mu.Unlock()
}

func (t *Text) Beep() {
	if t.OnBeep != nil {
		t.OnBeep()
	}
}

func (t *Text) Flush() { t.touch() }

// ------------------------------------------------------------- teclado

// PushKey encola una tecla con el formato de INKEY$ (CHR$(0)+código para las extendidas).
func (t *Text) PushKey(k string) {
	t.filterMu.Lock()
	f := t.filter
	t.filterMu.Unlock()
	if f != nil && f(k) {
		return
	}
	select {
	case t.keys <- k:
	default: // buffer lleno: como el BIOS, se descarta
	}
}

// Interrupt desbloquea las lecturas de teclado (Ctrl+Break).
func (t *Text) Interrupt() {
	t.stopMu.Lock()
	select {
	case <-t.stop:
	default:
		close(t.stop)
	}
	t.stopMu.Unlock()
}

// Rearm permite volver a leer el teclado después de Interrupt (para continuar).
func (t *Text) Rearm() {
	t.stopMu.Lock()
	select {
	case <-t.stop:
		t.stop = make(chan struct{})
	default:
	}
	t.stopMu.Unlock()
}

func (t *Text) stopCh() chan struct{} {
	t.stopMu.Lock()
	defer t.stopMu.Unlock()
	return t.stop
}

func (t *Text) InKey() string {
	select {
	case k := <-t.keys:
		return k
	default:
		return ""
	}
}

func (t *Text) ReadKey() (string, error) {
	t.setInput(true)
	defer t.setInput(false)
	select {
	case k := <-t.keys:
		return k, nil
	case <-t.stopCh():
		return "", ErrInterrupted
	}
}

func (t *Text) setInput(on bool) {
	t.mu.Lock()
	t.inputMode = on
	t.touch()
	t.mu.Unlock()
}

// ReadLine edita una línea en la posición del cursor (INPUT / LINE INPUT).
func (t *Text) ReadLine() (string, error) {
	t.mu.Lock()
	startRow, startCol := t.row, t.col
	if startCol > t.cols {
		t.newline()
		startRow, startCol = t.row, 1
	}
	t.mu.Unlock()
	var buf []byte
	pos := 0
	redraw := func(extra int) {
		t.mu.Lock()
		before := t.scrolled
		t.row, t.col = startRow, startCol
		for _, b := range buf {
			t.putc(b)
		}
		for i := 0; i < extra; i++ {
			t.putc(' ')
		}
		startRow -= t.scrolled - before
		if startRow < 1 {
			startRow = 1
		}
		// cursor en pos
		off := startCol - 1 + pos
		t.row, t.col = startRow+off/t.cols, off%t.cols+1
		t.touch()
		t.mu.Unlock()
	}
	for {
		k, err := t.ReadKey()
		if err != nil {
			return string(buf), err
		}
		switch {
		case k == "\r":
			pos = len(buf)
			redraw(0)
			return string(buf), nil
		case k == "\b":
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
				redraw(1)
			}
		case k == "\x1b":
			n := len(buf)
			buf, pos = buf[:0], 0
			redraw(n)
		case k == "\x00K": // ←
			if pos > 0 {
				pos--
				redraw(0)
			}
		case k == "\x00M": // →
			if pos < len(buf) {
				pos++
				redraw(0)
			}
		case k == "\x00G": // Home
			pos = 0
			redraw(0)
		case k == "\x00O": // End
			pos = len(buf)
			redraw(0)
		case k == "\x00S": // Supr
			if pos < len(buf) {
				buf = append(buf[:pos], buf[pos+1:]...)
				redraw(1)
			}
		case len(k) == 1 && k[0] >= 32 && len(buf) < 255:
			buf = append(buf[:pos], append([]byte{k[0]}, buf[pos:]...)...)
			pos++
			redraw(0)
		}
	}
}

// ------------------------------------------------------------- lectura para dibujar

// Snapshot copia el contenido para dibujarlo.
type Snapshot struct {
	Cols, Rows int
	Cells      []Cell
	Row, Col   int
	Cursor     bool       // mostrar cursor
	Gfx        *gfx.Frame // imagen del modo gráfico (nil en modo texto)
	Mode       int
}

// Snapshot devuelve una copia consistente de la pantalla.
func (t *Text) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := Snapshot{Cols: t.cols, Rows: t.rows, Cells: append([]Cell(nil), t.cells...), Row: t.row, Col: min(t.col, t.cols), Cursor: t.cursor || t.inputMode, Mode: t.mode}
	if t.g != nil {
		if t.lastFrame == nil || t.lastVer != t.g.Version {
			t.lastFrame, t.lastVer = t.g.Frame(), t.g.Version
		}
		s.Gfx = t.lastFrame
		s.Cursor = false
	}
	return s
}

// Restore reemplaza el contenido (el IDE guarda la pantalla de salida entre ejecuciones).
func (t *Text) Restore(s Snapshot) {
	t.mu.Lock()
	if s.Cols > 0 {
		t.cols, t.rows = s.Cols, s.Rows
		t.cells = append([]Cell(nil), s.Cells...)
		t.row, t.col = s.Row, s.Col
		t.resetView()
	}
	t.touch()
	t.mu.Unlock()
}

// Reset restablece colores, cursor, tamaño 80×25 y vacía el teclado, conservando el contenido.
func (t *Text) Reset() {
	t.mu.Lock()
	if t.g != nil || t.cols != 80 || t.rows != 25 {
		t.g, t.mode, t.lastFrame = nil, 0, nil
		t.resize(80, 25)
	}
	t.fg, t.bg, t.cursor = 7, 0, false
	t.viewSet = false
	t.resetView()
	t.mu.Unlock()
	for len(t.keys) > 0 {
		<-t.keys
	}
	t.Rearm()
}

// ------------------------------------------------------------- modos gráficos

var _ vm.GraphicsConsole = (*Text)(nil)

// SetScreen cambia de modo de pantalla (0 = texto) y de páginas.
func (t *Text) SetScreen(mode, apage, vpage int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	defer t.touch()
	if mode == 0 {
		if apage != 0 || vpage != 0 {
			return gfx.ErrIllegal
		}
		if t.g != nil {
			t.g, t.mode, t.lastFrame = nil, 0, nil
			t.resize(80, 25)
		}
		return nil
	}
	if t.g == nil || t.mode != mode {
		g, err := gfx.New(mode)
		if err != nil {
			return err
		}
		t.g, t.mode, t.lastFrame = g, mode, nil
		t.resize(g.M.Cols, g.M.Rows)
	}
	return t.g.SetPages(apage, vpage)
}

// ScreenMode devuelve el modo actual.
func (t *Text) ScreenMode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.mode
}

// WithGfx ejecuta fn sobre la pantalla gráfica bajo el bloqueo de la pantalla.
func (t *Text) WithGfx(fn func(g *gfx.Screen) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.g == nil {
		return gfx.ErrIllegal
	}
	err := fn(t.g)
	t.touch()
	return err
}
