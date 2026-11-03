// Package ide es el entorno de desarrollo de goqb: editor con vista por
// procedimiento (F2), menús y diálogos al estilo QuickBASIC 4.5, ejecución
// integrada (F5/F4) y ventana Immediate, con un layout que se adapta al
// tamaño de la terminal.
package ide

import (
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"

	"goqb/runtime/termrun"
)

// Esquema de colores de QB 4.5 (índices de la paleta DOS).
var (
	stEditor     = termrun.Style(7, 1)  // texto del editor
	stEditorSel  = termrun.Style(1, 7)  // selección
	stFrame      = termrun.Style(7, 1)  // marco de ventana
	stTitle      = termrun.Style(1, 7)  // título de la ventana activa
	stTitleOff   = termrun.Style(7, 1)  // título inactivo
	stMenu       = termrun.Style(0, 7)  // barra de menú y menús
	stMenuHot    = termrun.Style(15, 7) // letra rápida
	stMenuSel    = termrun.Style(7, 0)  // ítem seleccionado
	stMenuSelHot = termrun.Style(15, 0)
	stMenuOff    = termrun.Style(8, 7) // ítem deshabilitado
	stDialog     = termrun.Style(0, 7)
	stDialogHot  = termrun.Style(15, 7)
	stInput      = termrun.Style(0, 7)
	stListSel    = termrun.Style(7, 0)
	stStatus     = termrun.Style(15, 3)
	stStatusKey  = termrun.Style(0, 3)
	stShadow     = termrun.Style(8, 0)
	stScroll     = termrun.Style(0, 7)
	stDesk       = termrun.Style(7, 0)
	stCurStmt    = termrun.Style(15, 1)
	stGutter     = termrun.Style(3, 1)  // números de línea
	stGutterCur  = termrun.Style(15, 1) // número de la línea actual
)

// rect es un rectángulo en celdas.
type rect struct{ x, y, w, h int }

func (r rect) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h
}

// canvas envuelve tcell.Screen con primitivas de dibujo.
type canvas struct{ s tcell.Screen }

func (c canvas) put(x, y int, r rune, st tcell.Style) { c.s.SetContent(x, y, r, nil, st) }

// text escribe s y devuelve el ancho usado (recorta a maxW si maxW > 0).
func (c canvas) text(x, y int, s string, st tcell.Style, maxW int) int {
	n := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w == 0 {
			w = 1
		}
		if maxW > 0 && n+w > maxW {
			break
		}
		c.s.SetContent(x+n, y, r, nil, st)
		n += w
	}
	return n
}

// hot escribe un texto con la letra rápida (precedida por &) resaltada.
func (c canvas) hot(x, y int, s string, st, hotSt tcell.Style) int {
	n := 0
	hl := false
	for _, r := range s {
		if r == '&' && !hl {
			hl = true
			continue
		}
		style := st
		if hl {
			style = hotSt
			hl = false
		}
		c.s.SetContent(x+n, y, r, nil, style)
		n++
	}
	return n
}

func (c canvas) fill(r rect, ch rune, st tcell.Style) {
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			c.s.SetContent(x, y, ch, nil, st)
		}
	}
}

func (c canvas) hline(x, y, w int, ch rune, st tcell.Style) {
	for i := 0; i < w; i++ {
		c.s.SetContent(x+i, y, ch, nil, st)
	}
}

// box dibuja un marco simple con fondo.
func (c canvas) box(r rect, st tcell.Style) {
	c.fill(r, ' ', st)
	for x := r.x + 1; x < r.x+r.w-1; x++ {
		c.put(x, r.y, '─', st)
		c.put(x, r.y+r.h-1, '─', st)
	}
	for y := r.y + 1; y < r.y+r.h-1; y++ {
		c.put(r.x, y, '│', st)
		c.put(r.x+r.w-1, y, '│', st)
	}
	c.put(r.x, r.y, '┌', st)
	c.put(r.x+r.w-1, r.y, '┐', st)
	c.put(r.x, r.y+r.h-1, '└', st)
	c.put(r.x+r.w-1, r.y+r.h-1, '┘', st)
}

// shadow oscurece la sombra de una ventana (abajo y a la derecha), como QB.
func (c canvas) shadow(r rect) {
	sw, sh := c.s.Size()
	dim := func(x, y int) {
		if x >= sw || y >= sh {
			return
		}
		ch, _, _, _ := c.s.GetContent(x, y)
		c.s.SetContent(x, y, ch, nil, stShadow)
	}
	for y := r.y + 1; y <= r.y+r.h; y++ {
		dim(r.x+r.w, y)
		dim(r.x+r.w+1, y)
	}
	for x := r.x + 2; x < r.x+r.w+2; x++ {
		dim(x, r.y+r.h)
	}
}

// center escribe s centrado en [x, x+w).
func (c canvas) center(x, y, w int, s string, st tcell.Style) {
	n := runewidth.StringWidth(s)
	if n > w {
		n = w
	}
	c.text(x+(w-n)/2, y, s, st, w)
}

// pad agrega espacios a s (o la recorta) hasta n runas.
func pad(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	out := make([]rune, n)
	copy(out, r)
	for i := len(r); i < n; i++ {
		out[i] = ' '
	}
	return string(out)
}
