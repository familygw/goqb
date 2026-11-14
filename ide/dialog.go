package ide

import (
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// TextInput es un campo de texto de una línea.
type TextInput struct {
	Label string
	Text  []rune
	Pos   int
	Width int
	off   int
}

func (t *TextInput) set(s string) {
	t.Text = []rune(s)
	t.Pos = len(t.Text)
	t.off = 0
}

func (t *TextInput) handle(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyLeft:
		if t.Pos > 0 {
			t.Pos--
		}
	case tcell.KeyRight:
		if t.Pos < len(t.Text) {
			t.Pos++
		}
	case tcell.KeyHome:
		t.Pos = 0
	case tcell.KeyEnd:
		t.Pos = len(t.Text)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if t.Pos > 0 {
			t.Text = append(t.Text[:t.Pos-1], t.Text[t.Pos:]...)
			t.Pos--
		}
	case tcell.KeyDelete:
		if t.Pos < len(t.Text) {
			t.Text = append(t.Text[:t.Pos], t.Text[t.Pos+1:]...)
		}
	case tcell.KeyRune:
		t.Text = append(t.Text[:t.Pos], append([]rune{ev.Rune()}, t.Text[t.Pos:]...)...)
		t.Pos++
	default:
		return false
	}
	return true
}

func (t *TextInput) draw(c canvas, x, y int, focused bool) (int, int) {
	if t.Pos < t.off {
		t.off = t.Pos
	}
	if t.Pos >= t.off+t.Width {
		t.off = t.Pos - t.Width + 1
	}
	vis := t.Text[t.off:]
	if len(vis) > t.Width {
		vis = vis[:t.Width]
	}
	c.put(x-1, y, '[', stDialog)
	c.text(x, y, pad(string(vis), t.Width), stInput, 0)
	c.put(x+t.Width, y, ']', stDialog)
	if focused {
		return x + t.Pos - t.off, y
	}
	return -1, -1
}

// ListBox es una lista con desplazamiento.
type ListBox struct {
	Label  string
	Items  []string
	Sel    int
	Height int
	Width  int
	top    int
	box    rect
}

func (l *ListBox) handle(ev *tcell.EventKey) bool {
	n := len(l.Items)
	if n == 0 {
		return false
	}
	switch ev.Key() {
	case tcell.KeyUp:
		if l.Sel > 0 {
			l.Sel--
		}
	case tcell.KeyDown:
		if l.Sel < n-1 {
			l.Sel++
		}
	case tcell.KeyPgUp:
		l.Sel = max(0, l.Sel-l.Height)
	case tcell.KeyPgDn:
		l.Sel = min(n-1, l.Sel+l.Height)
	case tcell.KeyHome:
		l.Sel = 0
	case tcell.KeyEnd:
		l.Sel = n - 1
	case tcell.KeyRune: // salto por inicial
		r := strings.ToUpper(string(ev.Rune()))
		for i := 1; i <= n; i++ {
			j := (l.Sel + i) % n
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(l.Items[j])), r) {
				l.Sel = j
				break
			}
		}
	default:
		return false
	}
	return true
}

func (l *ListBox) draw(c canvas, x, y int, focused bool) (int, int) {
	l.box = rect{x, y, l.Width + 2, l.Height + 2}
	c.box(l.box, stDialog)
	if l.Sel < l.top {
		l.top = l.Sel
	}
	if l.Sel >= l.top+l.Height {
		l.top = l.Sel - l.Height + 1
	}
	cx, cy := -1, -1
	for i := 0; i < l.Height; i++ {
		idx := l.top + i
		if idx >= len(l.Items) {
			break
		}
		st := stDialog
		if idx == l.Sel {
			st = stListSel
			if !focused {
				st = stDialogHot
			}
			cx, cy = x+1, y+1+i
		}
		c.text(x+1, y+1+i, pad(l.Items[idx], l.Width), st, 0)
	}
	if !focused {
		return -1, -1
	}
	return cx, cy
}

// Dialog es un cuadro de diálogo modal: mensaje, campo y/o lista, y botones.
type Dialog struct {
	Title   string
	Lines   []string
	Input   *TextInput
	List    *ListBox
	Buttons []string
	Default int // botón de Enter

	// OnClose recibe el botón elegido (-1 = Esc). Si devuelve false, el diálogo sigue abierto.
	OnClose func(d *Dialog, button int) bool
	// OnSelect se invoca cuando cambia la selección de la lista.
	OnSelect func(d *Dialog)
	// OnActivate se invoca con Enter o doble clic en la lista; true = cerrar con Default.
	OnActivate func(d *Dialog) bool

	focus   int // 0..: input, list, botones
	box     rect
	buttons []rect
}

func (d *Dialog) focusables() int {
	n := len(d.Buttons)
	if d.Input != nil {
		n++
	}
	if d.List != nil {
		n++
	}
	return n
}

// kind del foco: "input", "list" o "button" (con índice).
func (d *Dialog) focused() (string, int) {
	i := d.focus
	if d.Input != nil {
		if i == 0 {
			return "input", 0
		}
		i--
	}
	if d.List != nil {
		if i == 0 {
			return "list", 0
		}
		i--
	}
	return "button", i
}

func (d *Dialog) size(sw, sh int) (int, int) {
	w := runewidth.StringWidth(d.Title) + 8
	for _, l := range d.Lines {
		w = max(w, runewidth.StringWidth(l)+4)
	}
	bw := 0
	for _, b := range d.Buttons {
		bw += runewidth.StringWidth(b) + 6
	}
	w = max(w, bw+2)
	h := 2 + len(d.Lines) + 2 // marco + líneas + separador + botones
	if len(d.Lines) > 0 {
		h++
	}
	if d.Input != nil {
		w = max(w, runewidth.StringWidth(d.Input.Label)+d.Input.Width+6)
		h += 2
	}
	if d.List != nil {
		w = max(w, d.List.Width+6)
		h += d.List.Height + 3
	}
	if w > sw-2 {
		w = sw - 2
		if d.Input != nil {
			d.Input.Width = max(8, w-runewidth.StringWidth(d.Input.Label)-6)
		}
		if d.List != nil {
			d.List.Width = max(8, w-6)
		}
	}
	if h > sh-2 && d.List != nil {
		d.List.Height = max(2, d.List.Height-(h-(sh-2)))
		h = sh - 2
	}
	return w, h
}

// Draw dibuja el diálogo centrado y devuelve la posición del cursor.
func (d *Dialog) Draw(c canvas, sw, sh int) (int, int) {
	w, h := d.size(sw, sh)
	d.box = rect{(sw - w) / 2, max(1, (sh-h)/2), w, h}
	b := d.box
	c.box(b, stDialog)
	c.shadow(b)
	if d.Title != "" {
		c.center(b.x, b.y, b.w, " "+d.Title+" ", stDialog)
	}
	y := b.y + 1
	cx, cy := -1, -1
	kind, bi := d.focused()
	for _, l := range d.Lines {
		c.center(b.x+1, y, b.w-2, l, stDialog)
		y++
	}
	if len(d.Lines) > 0 {
		y++
	}
	if d.Input != nil {
		lw := c.text(b.x+2, y, d.Input.Label, stDialog, 0)
		if x, yy := d.Input.draw(c, b.x+3+lw, y, kind == "input"); x >= 0 {
			cx, cy = x, yy
		}
		y += 2
	}
	if d.List != nil {
		if d.List.Label != "" {
			c.text(b.x+2, y, d.List.Label, stDialog, 0)
		}
		y++
		if x, yy := d.List.draw(c, b.x+2, y, kind == "list"); x >= 0 {
			cx, cy = x, yy
		}
		y += d.List.Height + 2
	}
	// separador y botones
	sy := b.y + b.h - 3
	c.put(b.x, sy, '├', stDialog)
	c.hline(b.x+1, sy, b.w-2, '─', stDialog)
	c.put(b.x+b.w-1, sy, '┤', stDialog)
	total := 0
	for _, s := range d.Buttons {
		total += runewidth.StringWidth(s) + 4
	}
	total += 2 * (len(d.Buttons) - 1)
	bx := b.x + (b.w-total)/2
	d.buttons = d.buttons[:0]
	for i, s := range d.Buttons {
		st := stDialog
		if kind == "button" && bi == i {
			st = stDialogHot
			cx, cy = bx+2, sy+1
		}
		n := c.text(bx, sy+1, "< ", st, 0)
		n += c.text(bx+n, sy+1, s, st, 0)
		n += c.text(bx+n, sy+1, " >", st, 0)
		if i == d.Default { // el botón de Enter lleva los corchetes resaltados
			c.put(bx, sy+1, '<', stDialogHot)
			c.put(bx+n-1, sy+1, '>', stDialogHot)
		}
		d.buttons = append(d.buttons, rect{bx, sy + 1, n, 1})
		bx += n + 2
	}
	return cx, cy
}

// HandleKey procesa una tecla; devuelve true si el diálogo se cerró.
func (d *Dialog) HandleKey(ev *tcell.EventKey) bool {
	kind, bi := d.focused()
	switch ev.Key() {
	case tcell.KeyEsc:
		return d.close(-1)
	case tcell.KeyTab:
		d.focus = (d.focus + 1) % d.focusables()
		return false
	case tcell.KeyBacktab:
		d.focus = (d.focus + d.focusables() - 1) % d.focusables()
		return false
	case tcell.KeyEnter:
		switch kind {
		case "button":
			return d.close(bi)
		case "list":
			if d.OnActivate != nil && !d.OnActivate(d) {
				return false
			}
		}
		return d.close(d.Default)
	}
	switch kind {
	case "input":
		d.Input.handle(ev)
	case "list":
		before := d.List.Sel
		if d.List.handle(ev) && d.List.Sel != before && d.OnSelect != nil {
			d.OnSelect(d)
		}
	case "button":
		switch ev.Key() {
		case tcell.KeyLeft:
			d.focus = (d.focus + d.focusables() - 1) % d.focusables()
		case tcell.KeyRight:
			d.focus = (d.focus + 1) % d.focusables()
		case tcell.KeyRune: // inicial del botón
			r := strings.ToUpper(string(ev.Rune()))
			for i, b := range d.Buttons {
				if strings.HasPrefix(strings.ToUpper(b), r) {
					return d.close(i)
				}
			}
		}
	}
	return false
}

// Click procesa un clic; devuelve true si el diálogo se cerró.
func (d *Dialog) Click(x, y int, double bool) bool {
	for i, r := range d.buttons {
		if r.contains(x, y) {
			return d.close(i)
		}
	}
	if d.List != nil && d.List.box.contains(x, y) {
		i := d.List.top + y - d.List.box.y - 1
		if i >= 0 && i < len(d.List.Items) {
			d.List.Sel = i
			d.focus = 0
			if d.Input != nil {
				d.focus = 1
			}
			if d.OnSelect != nil {
				d.OnSelect(d)
			}
			if double {
				if d.OnActivate == nil || d.OnActivate(d) {
					return d.close(d.Default)
				}
			}
		}
	}
	return false
}

func (d *Dialog) close(button int) bool {
	if d.OnClose != nil && !d.OnClose(d, button) {
		return false
	}
	return true
}

// InputText devuelve el texto del campo.
func (d *Dialog) InputText() string {
	if d.Input == nil {
		return ""
	}
	return string(d.Input.Text)
}
