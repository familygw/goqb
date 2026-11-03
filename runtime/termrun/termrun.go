// Package termrun muestra una pantalla virtual de QB en la terminal con
// tcell: dibujo con la paleta DOS y CP437, traducción de teclas al
// formato de INKEY$, y un runner para ejecutar programas a pantalla completa.
package termrun

import (
	"github.com/gdamore/tcell/v2"

	"goqb/lang/cp437"
	"goqb/runtime/screen"
	"goqb/runtime/sound"
	"goqb/vm"
)

// Palette son los 16 colores de la paleta CGA/EGA/VGA de modo texto.
var Palette = [16]tcell.Color{
	tcell.NewRGBColor(0x00, 0x00, 0x00), tcell.NewRGBColor(0x00, 0x00, 0xAA),
	tcell.NewRGBColor(0x00, 0xAA, 0x00), tcell.NewRGBColor(0x00, 0xAA, 0xAA),
	tcell.NewRGBColor(0xAA, 0x00, 0x00), tcell.NewRGBColor(0xAA, 0x00, 0xAA),
	tcell.NewRGBColor(0xAA, 0x55, 0x00), tcell.NewRGBColor(0xAA, 0xAA, 0xAA),
	tcell.NewRGBColor(0x55, 0x55, 0x55), tcell.NewRGBColor(0x55, 0x55, 0xFF),
	tcell.NewRGBColor(0x55, 0xFF, 0x55), tcell.NewRGBColor(0x55, 0xFF, 0xFF),
	tcell.NewRGBColor(0xFF, 0x55, 0x55), tcell.NewRGBColor(0xFF, 0x55, 0xFF),
	tcell.NewRGBColor(0xFF, 0xFF, 0x55), tcell.NewRGBColor(0xFF, 0xFF, 0xFF),
}

// Style devuelve el estilo tcell para un par de colores DOS.
func Style(fg, bg int) tcell.Style {
	return tcell.StyleDefault.Foreground(Palette[fg&15]).Background(Palette[bg&15])
}

// AttrStyle convierte un atributo de celda de modo texto.
func AttrStyle(a byte) tcell.Style {
	st := Style(int(a&15), int(a>>4&7))
	if a&0x80 != 0 {
		st = st.Blink(true)
	}
	return st
}

// Draw dibuja la pantalla virtual en scr con origen (x0, y0). Devuelve
// la posición del cursor en la terminal (o -1 si no se muestra).
func Draw(scr tcell.Screen, s screen.Snapshot, x0, y0 int) (int, int) {
	w, h := scr.Size()
	for r := 0; r < s.Rows; r++ {
		y := y0 + r
		if y < 0 || y >= h {
			continue
		}
		for c := 0; c < s.Cols; c++ {
			x := x0 + c
			if x < 0 || x >= w {
				continue
			}
			cell := s.Cells[r*s.Cols+c]
			scr.SetContent(x, y, cp437.Rune(cell.Ch), nil, AttrStyle(cell.Attr))
		}
	}
	if s.Cursor {
		return x0 + s.Col - 1, y0 + s.Row - 1
	}
	return -1, -1
}

// Origin calcula dónde dibujar una pantalla de cols×rows: centrada si
// entra; si no, alineada para que el cursor quede visible.
func Origin(scr tcell.Screen, s screen.Snapshot) (int, int) {
	w, h := scr.Size()
	x0, y0 := (w-s.Cols)/2, (h-s.Rows)/2
	if x0 < 0 {
		x0 = 0
		if s.Col > w {
			x0 = w - s.Col
		}
	}
	if y0 < 0 {
		y0 = 0
		if s.Row > h {
			y0 = h - s.Row
		}
	}
	return x0, y0
}

var keyCodes = map[tcell.Key]string{
	tcell.KeyEnter: "\r", tcell.KeyTab: "\t", tcell.KeyEsc: "\x1b",
	tcell.KeyBackspace: "\b", tcell.KeyBackspace2: "\b",
	tcell.KeyUp: "\x00H", tcell.KeyDown: "\x00P", tcell.KeyLeft: "\x00K", tcell.KeyRight: "\x00M",
	tcell.KeyHome: "\x00G", tcell.KeyEnd: "\x00O", tcell.KeyPgUp: "\x00I", tcell.KeyPgDn: "\x00Q",
	tcell.KeyInsert: "\x00R", tcell.KeyDelete: "\x00S",
	tcell.KeyF1: "\x00;", tcell.KeyF2: "\x00<", tcell.KeyF3: "\x00=", tcell.KeyF4: "\x00>",
	tcell.KeyF5: "\x00?", tcell.KeyF6: "\x00@", tcell.KeyF7: "\x00A", tcell.KeyF8: "\x00B",
	tcell.KeyF9: "\x00C", tcell.KeyF10: "\x00D", tcell.KeyF11: "\x00\x85", tcell.KeyF12: "\x00\x86",
	tcell.KeyBacktab: "\x00\x0f",
}

// Alt+letra: códigos de exploración del teclado AT.
var altScan = map[rune]byte{
	'q': 16, 'w': 17, 'e': 18, 'r': 19, 't': 20, 'y': 21, 'u': 22, 'i': 23, 'o': 24, 'p': 25,
	'a': 30, 's': 31, 'd': 32, 'f': 33, 'g': 34, 'h': 35, 'j': 36, 'k': 37, 'l': 38,
	'z': 44, 'x': 45, 'c': 46, 'v': 47, 'b': 48, 'n': 49, 'm': 50,
}

// KeyString traduce un evento de teclado al formato de INKEY$ ("" si no aplica).
func KeyString(ev *tcell.EventKey) string {
	if ev.Key() == tcell.KeyRune {
		r := ev.Rune()
		if ev.Modifiers()&tcell.ModAlt != 0 {
			if sc, ok := altScan[r|0x20]; ok {
				return string([]byte{0, sc})
			}
		}
		if b, ok := cp437.Byte(r); ok {
			return string([]byte{b})
		}
		return ""
	}
	if s, ok := keyCodes[ev.Key()]; ok {
		return s
	}
	if k := ev.Key(); k >= tcell.KeyCtrlA && k <= tcell.KeyCtrlZ {
		return string([]byte{byte(k)})
	}
	return ""
}

// IsBreak reporta si el evento es Ctrl+Break (Ctrl+C en la terminal).
func IsBreak(ev *tcell.EventKey) bool {
	return ev.Key() == tcell.KeyCtrlC || ev.Key() == tcell.KeyPause
}

// Run ejecuta prog a pantalla completa. Si wait, al terminar muestra
// "Press any key to continue" como el IDE de QB.
func Run(prog *vm.Program, command string, wait bool) error {
	scr, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := scr.Init(); err != nil {
		return err
	}
	defer scr.Fini()
	scr.Clear()

	txt := screen.New(80, 25)
	txt.OnBeep = func() { scr.Beep() }
	m := vm.New(prog, txt)
	m.Command = command
	m.Sound = sound.Device()
	done := make(chan error, 1)
	go func() { done <- m.Run() }()

	return Loop(scr, txt, done, func() { m.Stop(); txt.Interrupt() }, wait)
}

// Loop dibuja txt y le envía las teclas hasta que llega el resultado
// por done. onBreak se invoca con Ctrl+Break.
func Loop(scr tcell.Screen, txt *screen.Text, done <-chan error, onBreak func(), wait bool) error {
	events := make(chan tcell.Event, 64)
	quit := make(chan struct{})
	go func() {
		for {
			ev := scr.PollEvent()
			if ev == nil {
				return
			}
			select {
			case events <- ev:
			case <-quit:
				return
			}
		}
	}()
	defer close(quit)

	var last uint64 = ^uint64(0)
	pr := NewPresenter(scr)
	defer pr.Reset()
	redraw := func() { pr.Draw(txt.Snapshot(), "") }
	tick := newTicker()
	defer tick.Stop()
	for {
		select {
		case ev := <-events:
			switch e := ev.(type) {
			case *tcell.EventKey:
				if IsBreak(e) {
					onBreak()
					continue
				}
				if k := KeyString(e); k != "" {
					txt.PushKey(k)
				}
			case *tcell.EventResize:
				scr.Sync()
				last = ^uint64(0)
			}
		case err := <-done:
			if wait {
				pr.Draw(txt.Snapshot(), "Press any key to continue")
				waitKey(scr, events)
			} else {
				redraw()
			}
			return err
		case <-tick.C:
			if v := txt.Version(); v != last {
				last = v
				redraw()
			}
		}
	}
}

// waitKey espera una tecla.
func waitKey(scr tcell.Screen, events <-chan tcell.Event) {
	for ev := range events {
		switch ev.(type) {
		case *tcell.EventKey:
			return
		case *tcell.EventResize:
			scr.Sync()
		}
	}
}
