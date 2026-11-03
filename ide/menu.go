package ide

import (
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

// MenuItem es una opción de menú. Label usa & antes de la letra rápida.
type MenuItem struct {
	Label    string
	Shortcut string
	Action   func()
	Enabled  func() bool
	Checked  func() bool
	Sep      bool
}

func (it *MenuItem) enabled() bool { return !it.Sep && (it.Enabled == nil || it.Enabled()) }

// Menu es un menú desplegable de la barra.
type Menu struct {
	Title string
	Items []*MenuItem
	x     int // columna en la barra
}

// MenuBar es la barra de menús de QB: F10 o Alt la activan, las flechas
// navegan, la letra rápida elige, Esc cierra.
type MenuBar struct {
	Menus   []*Menu
	active  bool // barra con foco (sin desplegar)
	open    int  // menú desplegado (-1: ninguno)
	sel     int  // ítem seleccionado en el desplegado
	focus   int  // menú resaltado en la barra
	compact bool
	dropBox rect
}

func hotRune(label string) rune {
	if i := strings.IndexByte(label, '&'); i >= 0 && i+1 < len(label) {
		return unicode.ToLower(rune(label[i+1]))
	}
	return 0
}

func plain(label string) string { return strings.Replace(label, "&", "", 1) }

// Active reporta si la barra tiene el foco del teclado.
func (m *MenuBar) Active() bool { return m.active || m.open >= 0 }

// Activate da el foco a la barra (F10 / Alt).
func (m *MenuBar) Activate() {
	m.active, m.open, m.focus = true, -1, 0
}

// Close devuelve el foco al editor.
func (m *MenuBar) Close() { m.active, m.open = false, -1 }

// OpenMenu despliega el menú i.
func (m *MenuBar) OpenMenu(i int) {
	m.active, m.open, m.focus, m.sel = true, i, i, 0
	m.firstEnabled(1)
}

func (m *MenuBar) firstEnabled(dir int) {
	items := m.Menus[m.open].Items
	for n := 0; n < len(items); n++ {
		if items[m.sel].enabled() {
			return
		}
		m.sel = (m.sel + dir + len(items)) % len(items)
	}
}

// OpenByHot abre el menú cuya letra rápida es r (Alt+letra). Devuelve true si lo encontró.
func (m *MenuBar) OpenByHot(r rune) bool {
	r = unicode.ToLower(r)
	for i, mn := range m.Menus {
		if hotRune(mn.Title) == r {
			m.OpenMenu(i)
			return true
		}
	}
	return false
}

// HandleKey procesa teclas mientras la barra está activa.
func (m *MenuBar) HandleKey(ev *tcell.EventKey) {
	n := len(m.Menus)
	if m.open < 0 { // barra sin desplegar
		switch ev.Key() {
		case tcell.KeyLeft:
			m.focus = (m.focus + n - 1) % n
		case tcell.KeyRight:
			m.focus = (m.focus + 1) % n
		case tcell.KeyEnter, tcell.KeyDown:
			m.OpenMenu(m.focus)
		case tcell.KeyEsc, tcell.KeyF10:
			m.Close()
		case tcell.KeyRune:
			if !m.OpenByHot(ev.Rune()) {
				m.Close()
			}
		}
		return
	}
	items := m.Menus[m.open].Items
	switch ev.Key() {
	case tcell.KeyLeft:
		m.OpenMenu((m.open + n - 1) % n)
	case tcell.KeyRight:
		m.OpenMenu((m.open + 1) % n)
	case tcell.KeyUp:
		m.sel = (m.sel + len(items) - 1) % len(items)
		m.firstEnabled(-1)
	case tcell.KeyDown:
		m.sel = (m.sel + 1) % len(items)
		m.firstEnabled(1)
	case tcell.KeyEnter:
		m.choose(items[m.sel])
	case tcell.KeyEsc, tcell.KeyF10:
		m.Close()
	case tcell.KeyRune:
		r := unicode.ToLower(ev.Rune())
		for _, it := range items {
			if it.enabled() && hotRune(it.Label) == r {
				m.choose(it)
				return
			}
		}
	}
}

func (m *MenuBar) choose(it *MenuItem) {
	if !it.enabled() {
		return
	}
	m.Close()
	if it.Action != nil {
		it.Action()
	}
}

// Click procesa un clic; devuelve true si cayó en la barra o en el menú desplegado.
func (m *MenuBar) Click(x, y int) bool {
	if y == 0 {
		for i, mn := range m.Menus {
			w := m.titleWidth(mn)
			if x >= mn.x && x < mn.x+w {
				if m.open == i {
					m.Close()
				} else {
					m.OpenMenu(i)
				}
				return true
			}
		}
		m.Close()
		return true
	}
	if m.open >= 0 {
		if m.dropBox.contains(x, y) {
			i := y - m.dropBox.y - 1
			items := m.Menus[m.open].Items
			if i >= 0 && i < len(items) {
				m.choose(items[i])
			}
			return true
		}
		m.Close()
		return true
	}
	return false
}

func (m *MenuBar) titleWidth(mn *Menu) int {
	if m.compact {
		return 3
	}
	return runewidth.StringWidth(plain(mn.Title)) + 2
}

// Draw dibuja la barra en la fila 0 y el menú desplegado, si hay.
func (m *MenuBar) Draw(c canvas, w int) {
	c.hline(0, 0, w, ' ', stMenu)
	x := 2
	if m.compact {
		x = 1
	}
	for i, mn := range m.Menus {
		mn.x = x
		st, hot := stMenu, stMenuHot
		if m.Active() && m.focus == i {
			st, hot = stMenuSel, stMenuSelHot
		}
		label := " " + mn.Title + " "
		if m.compact {
			label = " &" + string(unicode.ToUpper(hotRune(mn.Title))) + " "
		}
		c.hot(x, 0, label, st, hot)
		x += m.titleWidth(mn)
	}
	if m.open < 0 {
		return
	}
	mn := m.Menus[m.open]
	width := 0
	for _, it := range mn.Items {
		iw := runewidth.StringWidth(plain(it.Label)) + 4
		if it.Shortcut != "" {
			iw += runewidth.StringWidth(it.Shortcut) + 3
		}
		width = max(width, iw)
	}
	bx := mn.x - 1
	if bx+width+2 > w {
		bx = max(0, w-width-2)
	}
	box := rect{bx, 1, width + 2, len(mn.Items) + 2}
	m.dropBox = box
	c.box(box, stMenu)
	c.shadow(box)
	for i, it := range mn.Items {
		y := box.y + 1 + i
		if it.Sep {
			c.put(box.x, y, '├', stMenu)
			c.hline(box.x+1, y, box.w-2, '─', stMenu)
			c.put(box.x+box.w-1, y, '┤', stMenu)
			continue
		}
		st, hot := stMenu, stMenuHot
		if !it.enabled() {
			st, hot = stMenuOff, stMenuOff
		} else if i == m.sel {
			st, hot = stMenuSel, stMenuSelHot
		}
		c.hline(box.x+1, y, box.w-2, ' ', st)
		mark := " "
		if it.Checked != nil && it.Checked() {
			mark = "•"
		}
		c.text(box.x+1, y, mark, st, 0)
		c.hot(box.x+2, y, it.Label, st, hot)
		if it.Shortcut != "" {
			sw := runewidth.StringWidth(it.Shortcut)
			c.text(box.x+box.w-2-sw, y, it.Shortcut, st, 0)
		}
	}
}
