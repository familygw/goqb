package ide

import (
	"strconv"
	"strings"

	"goqb/gfx"
	"goqb/runtime/screen"
	"goqb/runtime/termrun"
)

// aspectDialog permite fijar la relación alto/ancho de las celdas de la
// terminal (para que los gráficos no se deformen) y muestra un patrón de prueba.
func (a *App) aspectDialog() {
	cur := ""
	if termrun.CellAspectOverride > 0 {
		cur = strconv.FormatFloat(termrun.CellAspectOverride, 'f', 2, 64)
	}
	if a.pres == nil {
		a.pres = termrun.NewPresenter(a.scr)
	}
	detected := "2.00"
	{
		saved := termrun.CellAspectOverride
		termrun.CellAspectOverride = 0
		detected = strconv.FormatFloat(a.pres.CellAspect(), 'f', 2, 64)
		termrun.CellAspectOverride = saved
	}
	in := &TextInput{Label: "Alto/ancho de la celda (vacío: automático " + detected + "):", Width: 12}
	in.set(cur)
	a.pushDialog(&Dialog{Title: "Graphics Aspect", Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b != 0 {
				return true
			}
			s := strings.TrimSpace(strings.ReplaceAll(d.InputText(), ",", "."))
			v := 0.0
			if s != "" && !strings.EqualFold(s, "auto") {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil || f < 0.5 || f > 4 {
					a.message("", "Valor inválido: usá un número entre 0.5 y 4 (por ejemplo 2.2) o dejalo vacío.")
					return true
				}
				v = f
			}
			termrun.CellAspectOverride = v
			a.saveConfig()
			a.showAspectTest()
			return true
		}})
}

// showAspectTest muestra en SCREEN 12 un círculo dentro de un cuadrado.
func (a *App) showAspectTest() {
	t := screen.New(80, 25)
	if err := t.SetScreen(12, 0, 0); err != nil {
		return
	}
	t.WithGfx(func(g *gfx.Screen) error {
		g.Line(true, false, 120, 40, false, 519, 439, 14, 1, -1)
		g.Circle(false, 320, 240, 199, 15, 0, 0, 0, false, false, false)
		g.Line(true, false, 120, 240, false, 519, 240, 7, 0, 0xCCCC)
		g.Line(true, false, 320, 40, false, 320, 439, 7, 0, 0xCCCC)
		return nil
	})
	t.Locate(2, 2)
	t.Print("El círculo debe ser redondo y el recuadro, cuadrado.")
	t.Locate(29, 2)
	t.Print("Si se ve alto, subí el valor; si se ve ancho, bajalo.")
	a.testOut = t
	a.leaveLine()
	a.mode = modeOutput
}
