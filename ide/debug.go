package ide

import (
	"strings"
	"unicode"

	"goqb/lang/cp437"
	"goqb/lang/parser"
	"goqb/runtime/sound"
	"goqb/vm"
)

// bpKey identifica una línea de una vista.
type bpKey struct {
	view *View
	line int
}

// watch es una expresión de la ventana de watches (o un watchpoint).
type watch struct {
	expr  string
	proc  string // procedimiento donde se agregó ("" = nivel de módulo)
	point bool   // watchpoint: detener cuando sea verdadera
	value string
}

const maxWatches = 6

// paused reporta si hay una sesión de depuración detenida que se puede continuar.
func (a *App) paused() bool {
	return a.canContinue && a.m != nil && a.progRev == a.doc.rev
}

// procIndex devuelve el índice del procedimiento compilado con ese nombre (-1: módulo).
func (a *App) procIndex(name string) int {
	if name == "" || a.prog == nil {
		return -1
	}
	k := strings.ToUpper(strings.TrimRight(name, "%&!#$"))
	for i, p := range a.prog.Procs {
		if strings.ToUpper(strings.TrimRight(p.Name, "%&!#$")) == k {
			return i
		}
	}
	return -1
}

// viewLines devuelve, para cada (vista, línea), la línea del texto armado.
func (a *App) viewLines() map[Loc]int {
	rev := map[Loc]int{}
	for i, l := range a.locs {
		rev[l] = i + 1 // la última aparición es la línea real (no un DECLARE generado)
	}
	return rev
}

// syncDebugger crea el depurador y traduce breakpoints y watchpoints a pcs.
func (a *App) syncDebugger() {
	if a.m == nil {
		return
	}
	if a.m.Debug == nil {
		a.m.Debug = vm.NewDebugger()
	}
	d := a.m.Debug
	d.Breaks = map[int]bool{}
	pcs := a.prog.StmtPCs(a.mainPath())
	rev := a.viewLines()
	for k := range a.breaks {
		vi := a.viewIndex(k.view)
		if vi < 0 {
			delete(a.breaks, k)
			continue
		}
		if line, ok := rev[Loc{vi, k.line}]; ok {
			if pc, ok := pcs[line]; ok {
				d.Breaks[pc] = true
			}
		}
	}
	d.Watchpoints = nil
	for _, w := range a.watches {
		if !w.point {
			continue
		}
		p := a.procIndex(w.proc)
		if entry, diags := a.comp.CompileEval(w.expr, p); len(diags) == 0 {
			d.Watchpoints = append(d.Watchpoints, vm.Watchpoint{Entry: entry, Proc: p})
		}
	}
	d.HistoryOn = a.historyOn
}

func (a *App) viewIndex(v *View) int {
	for i, x := range a.doc.Views {
		if x == v {
			return i
		}
	}
	return -1
}

// isBreak lo usa el editor para resaltar las líneas con breakpoint.
func (a *App) isBreak(row int) bool {
	return a.breaks[bpKey{a.doc.Views[a.cur], row}]
}

// toggleBreakpoint es F9.
func (a *App) toggleBreakpoint() {
	a.leaveLine()
	v := a.doc.Views[a.cur]
	k := bpKey{v, v.Row}
	if a.breaks[k] {
		delete(a.breaks, k)
	} else {
		a.breaks[k] = true
	}
	a.syncDebugger()
}

func (a *App) clearBreakpoints() {
	a.breaks = map[bpKey]bool{}
	a.syncDebugger()
}

// debugRun continúa o inicia el programa en el modo de paso dado.
func (a *App) debugRun(mode int) {
	a.leaveLine()
	if len(a.dialogs) > 0 {
		return
	}
	if !a.paused() {
		if !a.compile(false) {
			return
		}
		a.out.Reset()
		a.m = vm.New(a.prog, a.out)
		a.m.Command = a.command
		a.m.Sound = sound.Device()
	}
	a.syncDebugger()
	a.m.Step(mode)
	a.curStmt = Loc{View: -1}
	a.out.Rearm()
	a.begin(func() error { return a.m.Run() }, false)
}

// runToCursor es F7: ejecutar hasta la línea del cursor.
func (a *App) runToCursor() {
	a.leaveLine()
	v := a.doc.Views[a.cur]
	k := bpKey{v, v.Row}
	had := a.breaks[k]
	a.breaks[k] = true
	a.tempBreak, a.tempHad = k, had
	a.debugRun(vm.DebugRun)
}

// setNextStatement mueve la ejecución a la línea del cursor.
func (a *App) setNextStatement() {
	if !a.paused() {
		a.message("", "Cannot continue: el programa no está detenido")
		return
	}
	v := a.doc.Views[a.cur]
	line, ok := a.viewLines()[Loc{a.cur, v.Row}]
	if !ok {
		return
	}
	pc, ok := a.prog.StmtPCs(a.mainPath())[line]
	if !ok {
		a.message("", "No hay una sentencia ejecutable en esta línea")
		return
	}
	if err := a.m.SetNextStatement(pc); err != nil {
		a.message("", "Sólo se puede elegir una sentencia del procedimiento actual")
		return
	}
	a.curStmt = Loc{View: a.cur, Line: v.Row}
}

// pausedAt procesa una pausa del depurador o un Ctrl+Break.
func (a *App) pausedAt(re *vm.RuntimeError, msg string) {
	a.canContinue = true
	if a.tempBreak.view != nil {
		if !a.tempHad {
			delete(a.breaks, a.tempBreak)
		}
		a.tempBreak = bpKey{}
	}
	if a.gotoDiag(re.File, re.Line, re.Col, a.locs) {
		a.curStmt = Loc{View: a.cur, Line: a.doc.Views[a.cur].Row}
	}
	a.evalWatches()
	a.status = msg
}

// evalWatches recalcula los valores de la ventana de watches.
func (a *App) evalWatches() {
	cur := -2
	if a.paused() {
		cur = a.m.CurrentProc()
	}
	for _, w := range a.watches {
		p := a.procIndex(w.proc)
		switch {
		case cur == -2:
			w.value = "<Not available>"
		case p != cur:
			w.value = "<Not watchable>"
		default:
			w.value = a.evalExpr(w.expr, p)
			if w.point {
				w.value = "<" + strings.TrimSpace(w.value) + ">"
			}
		}
	}
}

// evalExpr evalúa una expresión en el procedimiento p del programa detenido.
func (a *App) evalExpr(expr string, p int) string {
	entry, diags := a.comp.CompileEval(expr, p)
	if len(diags) > 0 {
		return diags[0].Msg
	}
	v, err := a.m.Eval(entry)
	if err != nil {
		return err.Error()
	}
	switch {
	case v.K == vm.KString:
		var b strings.Builder
		b.WriteByte('"')
		b.WriteString(cp437.Decode([]byte(v.S)))
		b.WriteByte('"')
		return b.String()
	case v.K.IsNumeric():
		return vm.StrNumber(v)
	}
	return "<" + v.K.String() + ">"
}

// addWatchDialog es Debug → Add Watch / Watchpoint.
func (a *App) addWatchDialog(point bool) {
	title, label := "Add Watch", "Expression:"
	if point {
		title, label = "Watchpoint", "Expression (detener cuando sea verdadera):"
	}
	in := &TextInput{Label: label, Width: 40}
	in.set(a.wordAtCursor())
	a.pushDialog(&Dialog{Title: title, Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			expr := strings.TrimSpace(d.InputText())
			if b == 0 && expr != "" {
				a.addWatch(expr, point)
			}
			return true
		}})
}

func (a *App) addWatch(expr string, point bool) {
	if _, errs := parser.ParseExpr(string(cp437.Encode(expr))); len(errs) > 0 {
		a.message("", errs[0].Msg)
		return
	}
	if len(a.watches) >= maxWatches {
		a.message("", "Demasiados watches (máximo 6)")
		return
	}
	a.watches = append(a.watches, &watch{expr: expr, proc: a.doc.Views[a.cur].Name, point: point})
	a.syncDebugger()
	a.evalWatches()
}

func (a *App) deleteWatchDialog() {
	if len(a.watches) == 0 {
		return
	}
	var items []string
	for _, w := range a.watches {
		items = append(items, watchLabel(w))
	}
	a.pushDialog(&Dialog{Title: "Delete Watch", List: &ListBox{Items: items, Height: len(items), Width: 50},
		Buttons: []string{"Delete", "Delete All", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			switch b {
			case 0:
				i := d.List.Sel
				a.watches = append(a.watches[:i], a.watches[i+1:]...)
			case 1:
				a.watches = nil
			}
			a.syncDebugger()
			return true
		}})
}

func watchLabel(w *watch) string {
	var b strings.Builder
	if w.proc != "" {
		b.WriteString(w.proc)
		b.WriteByte(' ')
	}
	b.WriteString(w.expr)
	if w.point {
		b.WriteString(" (watchpoint)")
	}
	return b.String()
}

// wordAtCursor devuelve la selección o la palabra bajo el cursor.
func (a *App) wordAtCursor() string {
	if t := a.ed.selectedText(); t != "" && !strings.Contains(t, "\n") {
		return strings.TrimSpace(t)
	}
	v := a.doc.Views[a.cur]
	l := []rune(v.Lines[v.Row])
	if v.Col > len(l) {
		return ""
	}
	isW := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("$%&!#.", r)
	}
	s, e := v.Col, v.Col
	for s > 0 && isW(l[s-1]) {
		s--
	}
	for e < len(l) && isW(l[e]) {
		e++
	}
	// incluir índices de array: a(i)
	if e < len(l) && l[e] == '(' {
		depth := 0
		for j := e; j < len(l); j++ {
			if l[j] == '(' {
				depth++
			} else if l[j] == ')' {
				depth--
				if depth == 0 {
					e = j + 1
					break
				}
			}
		}
	}
	return string(l[s:e])
}

// instantWatch es Shift+F9: muestra el valor de la expresión bajo el cursor.
func (a *App) instantWatch() {
	expr := a.wordAtCursor()
	if expr == "" {
		a.addWatchDialog(false)
		return
	}
	val := "<Not available>"
	if a.paused() {
		val = a.evalExpr(expr, a.m.CurrentProc())
	}
	a.pushDialog(&Dialog{Title: "Instant Watch", Lines: []string{expr, val}, Buttons: []string{"Add Watch", "Cancel"},
		OnClose: func(_ *Dialog, b int) bool {
			if b == 0 {
				a.addWatch(expr, false)
			}
			return true
		}})
}

// callsDialog es el menú Calls: la pila de procedimientos activos.
func (a *App) callsDialog() {
	if !a.paused() {
		a.message("Calls", "El programa no está detenido")
		return
	}
	stack := a.m.CallStack()
	var items []string
	for _, f := range stack {
		name := f.Proc
		if name == "" {
			name = a.doc.Title()
		}
		_, line, _ := a.m.Position(f.PC)
		items = append(items, name+"  (línea "+itoa(line)+")")
	}
	a.pushDialog(&Dialog{Title: "Calls", List: &ListBox{Items: items, Height: min(10, len(items)), Width: 50},
		Buttons: []string{"Show", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b == 0 && d.List.Sel < len(stack) {
				f, line, col := a.m.Position(stack[d.List.Sel].PC)
				a.gotoDiag(f, line, col, a.locs)
			}
			return true
		}})
}

// historyDialog muestra las últimas sentencias ejecutadas (Debug → History On).
func (a *App) historyDialog() {
	if a.m == nil || a.m.Debug == nil || len(a.m.Debug.History) == 0 {
		a.message("History", "Sin historial: activá Debug → History On y ejecutá el programa")
		return
	}
	hist := a.m.HistoryLines()
	var items []string
	for i := len(hist) - 1; i >= 0; i-- {
		items = append(items, "línea "+itoa(hist[i].Line)+":  "+strings.TrimSpace(a.sourceLine(hist[i].Line)))
	}
	a.pushDialog(&Dialog{Title: "History (más reciente primero)", List: &ListBox{Items: items, Height: min(12, len(items)), Width: 60},
		Buttons: []string{"Show", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b == 0 {
				h := hist[len(hist)-1-d.List.Sel]
				a.gotoDiag(a.mainPath(), h.Line, h.Col, a.locs)
			}
			return true
		}})
}

// sourceLine devuelve el texto de una línea del programa armado.
func (a *App) sourceLine(line int) string {
	if line < 1 || line > len(a.locs) {
		return ""
	}
	l := a.locs[line-1]
	if l.View >= len(a.doc.Views) || l.Line >= len(a.doc.Views[l.View].Lines) {
		return ""
	}
	return a.doc.Views[l.View].Lines[l.Line]
}

func itoa(n int) string {
	var b []byte
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
