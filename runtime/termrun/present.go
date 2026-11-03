package termrun

import (
	"github.com/gdamore/tcell/v2"

	"goqb/runtime/screen"
)

// Presenter muestra la pantalla virtual (texto o gráficos) en la terminal.
type Presenter struct {
	scr  tcell.Screen
	be   Backend
	gfx  bool
	area Rect
}

// NewPresenter crea un presentador con el backend gráfico detectado.
func NewPresenter(scr tcell.Screen) *Presenter {
	return &Presenter{scr: scr, be: DetectBackend(scr)}
}

// Backend devuelve el backend gráfico en uso.
func (p *Presenter) Backend() Backend { return p.be }

// CellAspect estima la relación alto/ancho de una celda.
func (p *Presenter) CellAspect() float64 {
	if tty, ok := p.scr.Tty(); ok {
		if ws, err := tty.WindowSize(); err == nil && ws.PixelWidth > 0 && ws.Width > 0 && ws.Height > 0 {
			cw := float64(ws.PixelWidth) / float64(ws.Width)
			ch := float64(ws.PixelHeight) / float64(ws.Height)
			if cw > 0 {
				return ch / cw
			}
		}
	}
	return 2
}

// Draw dibuja el snapshot y muestra la pantalla. footer, si no es vacío,
// se escribe en la última fila (aviso "Press any key to continue").
func (p *Presenter) Draw(s screen.Snapshot, footer string) {
	scr := p.scr
	if s.Gfx == nil {
		if p.gfx {
			p.Reset()
		}
		scr.Fill(' ', tcell.StyleDefault.Background(tcell.ColorBlack))
		x0, y0 := Origin(scr, s)
		cx, cy := Draw(scr, s, x0, y0)
		if footer != "" {
			y := y0 + s.Rows - 1
			for c := 0; c < s.Cols; c++ {
				scr.SetContent(x0+c, y, ' ', nil, Style(7, 0))
			}
			for i, r := range footer {
				scr.SetContent(x0+i, y, r, nil, Style(7, 0))
			}
			cx = -1
		}
		if cx >= 0 {
			scr.ShowCursor(cx, cy)
		} else {
			scr.HideCursor()
		}
		scr.Show()
		return
	}
	area := FitArea(scr, p.CellAspect())
	if !p.gfx || area != p.area {
		p.be.Clear(scr)
		scr.Fill(' ', tcell.StyleDefault.Background(tcell.ColorBlack))
		scr.Show()
		p.gfx, p.area = true, area
	}
	if footer != "" {
		_, h := scr.Size()
		if area.Y+area.H < h || p.be.Name() == "blocks" {
			for i, r := range footer {
				scr.SetContent(i, h-1, r, nil, Style(7, 0))
			}
		}
	}
	p.be.Present(scr, s.Gfx, area)
	scr.HideCursor()
	scr.Show()
}

// Reset borra las imágenes (al volver al IDE o a modo texto).
func (p *Presenter) Reset() {
	if p.gfx {
		p.be.Clear(p.scr)
		p.gfx = false
	}
}
