package ide

import (
	"sort"
	"strings"
)

// Vista completa (View → Full Program): todo el archivo en un solo editor,
// como en los editores actuales. El panel SUBs y F2 saltan al procedimiento.
// Al desactivarla, el programa se vuelve a separar en vistas como en QB.

// progItem es una entrada del panel SUBs / F2: el módulo o un procedimiento.
type progItem struct {
	name string
	view int // vista donde está
	row  int // fila del encabezado (vista completa)
}

// programItems: el módulo primero y los procedimientos en orden alfabético.
func (a *App) programItems() []progItem {
	items := []progItem{{name: "", view: 0, row: -1}}
	var procs []progItem
	if a.fullView {
		for _, p := range a.doc.Views[0].ProcHeaders() {
			procs = append(procs, progItem{name: p.Name, view: 0, row: p.Row})
		}
	} else {
		for i := 1; i < len(a.doc.Views); i++ {
			procs = append(procs, progItem{name: a.doc.Views[i].Name, view: i, row: -1})
		}
	}
	sort.SliceStable(procs, func(i, j int) bool {
		return strings.ToUpper(procs[i].name) < strings.ToUpper(procs[j].name)
	})
	return append(items, procs...)
}

// currentItem es el índice en items del lugar donde está el cursor.
func (a *App) currentItem(items []progItem) int {
	if !a.fullView {
		for i, it := range items {
			if it.view == a.cur {
				return i
			}
		}
		return 0
	}
	v := a.doc.Views[0]
	best, bestRow := 0, -1
	for i, it := range items {
		if it.row < 0 || it.row > v.Row || it.row < bestRow {
			continue
		}
		// ¿el cursor está antes del END del procedimiento?
		kind, _ := procHeader(v.Lines[it.row])
		inside := true
		for r := it.row + 1; r < v.Row && r < len(v.Lines); r++ {
			if isProcEnd(v.Lines[r], kind) {
				inside = false
				break
			}
		}
		if inside {
			best, bestRow = i, it.row
		}
	}
	return best
}

// gotoItem abre la vista del ítem o, en la vista completa, lleva el cursor a su encabezado.
func (a *App) gotoItem(it progItem) {
	if !a.fullView {
		a.switchView(it.view)
		return
	}
	a.imm = false
	row := max(it.row, 0)
	a.ed.moveTo(row, 0, false)
	v := a.doc.Views[0]
	v.Top = row // el encabezado arriba de todo, como al abrir la vista en QB
	if v.Top > len(v.Lines)-1 {
		v.Top = len(v.Lines) - 1
	}
}

func (a *App) itemLabel(it progItem) string {
	if it.name == "" {
		return a.doc.Title()
	}
	return "  " + it.name
}

// setFullView activa o desactiva la vista completa conservando la posición del cursor.
func (a *App) setFullView(on bool) {
	if on == a.fullView {
		return
	}
	a.ed.leaveLine()
	a.ac.open = false
	a.clearBreakpoints() // las posiciones de los breakpoints cambian
	if on {
		v := a.doc.Views[a.cur]
		rowOf := a.doc.Merge()
		row, col := rowOf(a.cur, v.Row), v.Col
		a.fullView, a.cur = true, 0
		a.ed.setView(a.doc.Views[0])
		a.ed.moveTo(row, col, false)
	} else {
		v := a.doc.Views[0]
		row, col := v.Row, v.Col
		pos := a.doc.Split()
		a.fullView, a.cur = false, 0
		if row < len(pos) {
			a.cur = pos[row].View
			a.ed.setView(a.doc.Views[a.cur])
			a.ed.moveTo(pos[row].Line, col, false)
		} else {
			a.ed.setView(a.doc.Views[0])
		}
	}
	a.ed.ensureVisible()
	a.saveConfig()
}

// applyViewMode adapta un documento recién abierto al modo de vista elegido.
func (a *App) applyViewMode() {
	if a.fullView && len(a.doc.Views) > 1 {
		a.doc.Merge()
		a.cur = 0
		a.ed.setView(a.doc.Views[0])
	}
}

// fullViewProcExit: en la vista completa, al escribir SUB/FUNCTION se agrega
// su END si falta (en lugar de abrir una vista nueva).
func (a *App) fullViewProcExit(row int, kind string) {
	v := a.doc.Views[0]
	for r := row + 1; r < len(v.Lines); r++ {
		if isProcEnd(v.Lines[r], kind) {
			return
		}
		if k, _ := procHeader(v.Lines[r]); k != "" {
			break
		}
	}
	end := []string{"", "END " + kind}
	v.Lines = append(v.Lines[:row+1], append(end, v.Lines[row+1:]...)...)
	a.doc.Touch()
}
