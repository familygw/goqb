package ide

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"goqb/compiler"
	"goqb/runtime/loader"
	"goqb/runtime/screen"
	"goqb/runtime/termrun"
	"goqb/vm"
)

type mode int

const (
	modeEdit   mode = iota
	modeRun         // el programa se está ejecutando (pantalla de salida)
	modeOutput      // mostrando la pantalla de salida hasta una tecla (F4 o fin del programa)
)

type layout struct {
	tooSmall bool
	compact  bool
	w, h     int
	frame    rect // ventana del editor (incluye marco)
	edit     rect // contenido del editor
	immFrame rect
	imm      rect
	hasImm   bool
	panel    rect // panel lateral de SUBs (contenido)
	hasPanel bool
	watch    rect // ventana de watches (alto 0 si no hay)
}

// App es el IDE.
type App struct {
	scr  tcell.Screen
	c    canvas
	doc  *Document
	cur  int // vista activa
	ed   Editor
	imv  *View
	imEd Editor
	imm  bool // foco en la ventana Immediate

	menu       MenuBar
	dialogs    []*Dialog
	clip       string
	clipColumn bool
	status     string
	lay        layout
	quit       bool
	events     chan tcell.Event
	pasting    bool
	pasteBuf   []rune

	syntaxCheck bool
	lineNumbers bool
	autoFormat  bool
	includePath []string // directorios para $INCLUDE (config.json)
	lastFind    string
	command     string

	// ejecución
	mode        mode
	out         *screen.Text
	testOut     *screen.Text // patrón de prueba de Graphics Aspect (mientras se muestra)
	ac          autocomplete // lista de sugerencias mientras se escribe
	fullView    bool         // todo el programa en una sola vista (View → Full Program)
	m           *vm.VM
	comp        *compiler.Compiler
	prog        *vm.Program
	progRev     int
	locs        []Loc
	vmDone      chan error
	canContinue bool
	fromImm     bool
	curStmt     Loc // sentencia donde se detuvo (View -1: ninguna)

	pres *termrun.Presenter

	breaks     map[bpKey]bool
	watches    []*watch
	historyOn  bool
	tempBreak  bpKey
	tempHad    bool
	runStart   time.Time
	outShown   bool
	immBreak   bool // la sentencia Immediate se ejecuta sobre un programa detenido
	lastClick  time.Time
	lastClickX int
	lastClickY int
	panelItems []progItem
}

// New crea el IDE; si path no es vacío abre ese archivo.
func New(path string) (*App, error) {
	a := &App{syntaxCheck: true, autoFormat: true, lineNumbers: true, vmDone: make(chan error, 1), curStmt: Loc{View: -1}}
	a.doc = NewDocument()
	vm.DefaultLoader = func(name string) (*vm.Program, error) { // CHAIN y RUN "archivo"
		dir := ""
		if a.doc.Path != "" {
			dir = filepath.Dir(a.doc.Path)
		}
		return loader.New(a.includePath, dir)(name)
	}
	if path != "" {
		d, err := LoadDocument(path)
		if err != nil {
			d = NewDocument()
			d.Path = path // archivo nuevo
		}
		a.doc = d
		a.status = procsHint(d)
	}
	a.out = screen.New(80, 25)
	a.imv = &View{Lines: []string{""}}
	a.ed.clipboard = &a.clip
	a.ed.clipColumn = &a.clipColumn
	a.loadConfig()
	a.applyViewMode()
	if a.fullView {
		a.status = ""
	}
	a.imEd.clipboard = &a.clip
	a.imEd.clipColumn = &a.clipColumn
	a.ed.OnLineExit = a.lineExit
	a.ed.OnChange = func() { a.doc.Touch(); a.curStmt = Loc{View: -1} }
	a.breaks = map[bpKey]bool{}
	a.ed.IsBreak = a.isBreak
	a.ed.setView(a.doc.Views[0])
	a.imEd.setView(a.imv)
	a.buildMenus()
	return a, nil
}

// Run inicia el IDE en la terminal y bloquea hasta salir.
func (a *App) Run() error {
	termrun.ProbeCellAspect() // antes de que tcell tome la entrada
	scr, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := scr.Init(); err != nil {
		return err
	}
	defer scr.Fini()
	return a.RunOn(scr)
}

// RunOn ejecuta el IDE sobre una pantalla ya inicializada (también se usa en tests).
func (a *App) RunOn(scr tcell.Screen) error {
	a.scr = scr
	a.c = canvas{scr}
	scr.EnableMouse()
	scr.EnablePaste()
	scr.SetStyle(stDesk)
	a.out.OnBeep = func() { scr.Beep() }

	a.events = make(chan tcell.Event, 128)
	go func() {
		for {
			ev := scr.PollEvent()
			if ev == nil {
				close(a.events)
				return
			}
			a.events <- ev
		}
	}()
	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()
	var lastOut uint64
	a.draw()
	for !a.quit {
		select {
		case ev, ok := <-a.events:
			if !ok {
				return nil
			}
			a.handle(ev)
			a.draw()
		case err := <-a.vmDone:
			a.finished(err)
			a.draw()
		case <-tick.C:
			if a.mode == modeRun {
				// la pantalla de salida aparece recién si la ejecución dura algo
				// (al depurar paso a paso no parpadea)
				if v := a.out.Version(); v != lastOut || !a.outShown && time.Since(a.runStart) > outDelay {
					lastOut = v
					a.draw()
				}
			}
		}
	}
	if a.m != nil && a.mode == modeRun {
		a.m.Stop()
		a.out.Interrupt()
	}
	return nil
}

// ------------------------------------------------------------- layout

func (a *App) computeLayout() {
	w, h := a.scr.Size()
	l := layout{w: w, h: h}
	if w < 40 || h < 12 {
		l.tooSmall = true
		a.lay = l
		return
	}
	l.compact = w < 80 || h < 20
	ew := w
	if w >= 100 && h >= 30 {
		pw := 26
		l.hasPanel = true
		l.panel = rect{w - pw + 1, 2, pw - 2, h - 4}
		ew = w - pw
	}
	bottom := h - 2 // última fila del área de ventanas (la fila h-1 es la barra de estado)
	if !l.compact {
		l.hasImm = true
		l.immFrame = rect{0, h - 4, ew, 3}
		l.imm = rect{1, h - 3, ew - 2, 2}
		bottom = h - 5
	}
	// ventana de watches arriba del editor, como en QB
	wy := min(len(a.watches), maxWatches)
	if bottom-wy < 4 {
		wy = 0
	}
	l.watch = rect{0, 1, ew, wy}
	l.frame = rect{0, 1 + wy, ew, bottom - wy}
	l.edit = rect{1, 2 + wy, ew - 2, bottom - wy - 2}
	a.menu.compact = l.compact
	a.lay = l
}

// ------------------------------------------------------------- dibujo

func (a *App) draw() {
	a.computeLayout()
	scr, c, l := a.scr, a.c, a.lay
	scr.HideCursor()
	if l.tooSmall {
		c.fill(rect{0, 0, l.w, l.h}, ' ', stDesk)
		c.center(0, l.h/2, l.w, "goqb: terminal muy chica", stDesk)
		c.center(0, l.h/2+1, l.w, "(mínimo 40×12)", stDesk)
		scr.Show()
		return
	}
	if a.mode == modeRun && !a.outShown && time.Since(a.runStart) < outDelay {
		// todavía en el editor (paso del depurador)
	} else if a.mode != modeEdit {
		a.outShown = true
		a.drawOutput()
		return
	}
	if a.pres != nil {
		a.pres.Reset() // borrar imágenes del modo gráfico al volver al editor
	}
	c.fill(rect{0, 0, l.w, l.h}, ' ', stDesk)
	if l.watch.h > 0 {
		a.drawWatches()
	}
	cx, cy := a.drawEditorWindow()
	if v := a.doc.Views[a.cur]; a.ac.open && (a.imm || v.Row != a.ac.row || v.Col < a.ac.start) {
		a.ac.open = false
	}
	if a.imm {
		a.ac.hint = ""
	}
	acX, acY := cx, cy
	if l.hasImm {
		if x, y := a.drawImmediate(); x >= 0 {
			cx, cy = x, y
		}
	}
	if l.hasPanel {
		a.drawPanel()
	}
	a.drawStatus()
	if !a.imm && !a.menu.Active() && len(a.dialogs) == 0 {
		a.drawAutocomplete(acX, acY)
	}
	a.menu.Draw(c, l.w)
	if a.menu.Active() {
		cx = -1
	}
	if n := len(a.dialogs); n > 0 {
		cx, cy = a.dialogs[n-1].Draw(c, l.w, l.h)
	}
	if cx >= 0 {
		scr.ShowCursor(cx, cy)
		if a.ed.overwrite && !a.imm {
			scr.SetCursorStyle(tcell.CursorStyleSteadyBlock)
		} else {
			scr.SetCursorStyle(tcell.CursorStyleDefault)
		}
	}
	scr.Show()
}

func (a *App) viewTitle() string {
	v := a.doc.Views[a.cur]
	if a.cur == 0 {
		return a.doc.Title()
	}
	return a.doc.Title() + ":" + v.Name
}

func (a *App) drawEditorWindow() (int, int) {
	c, l := a.c, a.lay
	f := l.frame
	st := stFrame
	// marco superior con título
	c.put(f.x, f.y, '┌', st)
	c.hline(f.x+1, f.y, f.w-2, '─', st)
	c.put(f.x+f.w-1, f.y, '┐', st)
	tst := stTitleOff
	if !a.imm {
		tst = stTitle
	}
	c.center(f.x+1, f.y, f.w-2, " "+a.viewTitle()+" ", tst)
	if a.doc.Dirty {
		c.put(f.x+1, f.y, '*', st)
	}
	// laterales y barra de desplazamiento vertical
	v := a.doc.Views[a.cur]
	for y := l.edit.y; y < l.edit.y+l.edit.h; y++ {
		c.put(f.x, y, '│', st)
		c.put(f.x+f.w-1, y, '░', stScroll)
	}
	if l.edit.h >= 3 {
		c.put(f.x+f.w-1, l.edit.y, '↑', stScroll)
		c.put(f.x+f.w-1, l.edit.y+l.edit.h-1, '↓', stScroll)
		track := l.edit.h - 2
		if track > 0 {
			pos := 0
			if len(v.Lines) > 1 {
				pos = v.Row * (track - 1) / (len(v.Lines) - 1)
			}
			c.put(f.x+f.w-1, l.edit.y+1+pos, '■', stScroll)
		}
	}
	// barra horizontal (última fila de la ventana)
	hy := f.y + f.h - 1
	c.put(f.x, hy, '│', st)
	c.put(f.x+f.w-1, hy, '│', st)
	c.put(f.x+1, hy, '←', stScroll)
	c.hline(f.x+2, hy, f.w-4, '░', stScroll)
	c.put(f.x+f.w-2, hy, '→', stScroll)
	if f.w > 6 {
		pos := v.Left * (f.w - 5) / 255
		c.put(f.x+2+pos, hy, '■', stScroll)
	}
	cs := -1
	if a.curStmt.View == a.cur {
		cs = a.curStmt.Line
	}
	area := l.edit
	if a.lineNumbers {
		gw := a.gutterWidth(v)
		a.drawGutter(v, area, gw)
		area.x += gw
		area.w -= gw
	}
	return a.ed.Draw(c, area, !a.imm && len(a.dialogs) == 0, cs)
}

// gutterWidth es el ancho de la columna de números de línea (dígitos + separador).
func (a *App) gutterWidth(v *View) int {
	gw := len(strconv.Itoa(len(v.Lines))) + 1
	if gw < 4 {
		gw = 4
	}
	if gw > a.lay.edit.w/2 {
		gw = a.lay.edit.w / 2
	}
	return gw
}

// drawGutter dibuja los números de línea de la vista (relativos a la vista,
// como la posición de la barra de estado), con la línea actual resaltada.
func (a *App) drawGutter(v *View, area rect, gw int) {
	for y := 0; y < area.h; y++ {
		row := v.Top + y
		st := stGutter
		if row == v.Row {
			st = stGutterCur
		}
		a.c.hline(area.x, area.y+y, gw, ' ', st)
		if row < len(v.Lines) {
			n := strconv.Itoa(row + 1)
			a.c.text(area.x+gw-1-len(n), area.y+y, n, st, 0)
		}
	}
}

func (a *App) drawImmediate() (int, int) {
	c, l := a.c, a.lay
	f := l.immFrame
	c.put(f.x, f.y, '├', stFrame)
	c.hline(f.x+1, f.y, f.w-2, '─', stFrame)
	c.put(f.x+f.w-1, f.y, '┤', stFrame)
	tst := stTitleOff
	if a.imm {
		tst = stTitle
	}
	c.center(f.x+1, f.y, f.w-2, " Immediate ", tst)
	for y := l.imm.y; y < l.imm.y+l.imm.h; y++ {
		c.put(f.x, y, '│', stFrame)
		c.put(f.x+f.w-1, y, '│', stFrame)
	}
	return a.imEd.Draw(c, l.imm, a.imm && len(a.dialogs) == 0, -1)
}

func (a *App) drawPanel() {
	c, l := a.c, a.lay
	p := l.panel
	box := rect{p.x - 1, p.y - 1, p.w + 2, p.h + 2}
	c.box(box, stFrame)
	c.center(box.x+1, box.y, box.w-2, " SUBs ", stTitleOff)
	a.panelItems = a.programItems()
	cur := a.currentItem(a.panelItems)
	for i, it := range a.panelItems {
		if i >= p.h {
			break
		}
		name := a.itemLabel(it)
		st := stEditor
		if i == cur {
			st = stEditorSel
		}
		c.text(p.x, p.y+i, pad(name, p.w), st, p.w)
	}
}

// sortedViews: el módulo primero y los procedimientos en orden alfabético (como F2 en QB).
func (a *App) sortedViews() []int {
	idx := []int{0}
	var procs []int
	for i := 1; i < len(a.doc.Views); i++ {
		procs = append(procs, i)
	}
	sort.Slice(procs, func(i, j int) bool {
		return strings.ToUpper(a.doc.Views[procs[i]].Name) < strings.ToUpper(a.doc.Views[procs[j]].Name)
	})
	return append(idx, procs...)
}

func (a *App) drawStatus() {
	c, l := a.c, a.lay
	y := l.h - 1
	c.hline(0, y, l.w, ' ', stStatus)
	if a.status != "" {
		c.text(1, y, a.status, stStatus, l.w-2)
	} else if a.ac.hint != "" {
		c.text(1, y, a.ac.hint, stStatus, l.w-14)
	} else {
		keys := "<F1=Help> <F2=SUBs> <F4=Output> <F5=Run> <F6=Window> <F10=Menu>"
		if l.compact {
			keys = "F1 Help  F2 SUBs  F5 Run  F10 Menu"
		}
		x := 1
		for _, part := range strings.SplitAfter(keys, " ") {
			st := stStatus
			if strings.HasPrefix(part, "<") || strings.HasPrefix(part, "F") {
				st = stStatusKey
			}
			x += c.text(x, y, part, st, 0)
		}
	}
	v := a.doc.Views[a.cur]
	row, col := v.Row, v.Col
	if a.imm {
		row, col = a.imv.Row, a.imv.Col
	}
	pos := fmt.Sprintf("%05d:%03d", row+1, col+1)
	if a.ed.overwrite {
		pos = "OVR " + pos
	}
	c.text(l.w-len(pos)-1, y, pos, stStatus, 0)
}

func (a *App) drawOutput() {
	if a.pres == nil {
		a.pres = termrun.NewPresenter(a.scr)
	}
	footer := ""
	if a.mode == modeOutput {
		footer = "Press any key to continue"
	}
	src := a.out
	if a.testOut != nil && a.mode == modeOutput {
		src = a.testOut
	}
	a.pres.Draw(src.Snapshot(), footer)
}

// ------------------------------------------------------------- eventos

func (a *App) handle(ev tcell.Event) {
	switch e := ev.(type) {
	case *tcell.EventResize:
		a.scr.Sync()
	case *tcell.EventPaste:
		if e.Start() {
			a.pasting, a.pasteBuf = true, a.pasteBuf[:0]
		} else {
			a.pasting = false
			if a.mode == modeEdit && len(a.dialogs) == 0 {
				a.activeEditor().insertText(strings.ReplaceAll(string(a.pasteBuf), "\r", "\n"))
			}
		}
	case *tcell.EventMouse:
		a.mouse(e)
	case *tcell.EventKey:
		if a.pasting {
			switch e.Key() {
			case tcell.KeyRune:
				a.pasteBuf = append(a.pasteBuf, e.Rune())
			case tcell.KeyEnter:
				a.pasteBuf = append(a.pasteBuf, '\n')
			case tcell.KeyTab:
				a.pasteBuf = append(a.pasteBuf, ' ')
			}
			return
		}
		a.key(e)
	}
}

func (a *App) activeEditor() *Editor {
	if a.imm {
		return &a.imEd
	}
	return &a.ed
}

func (a *App) key(e *tcell.EventKey) {
	switch a.mode {
	case modeRun:
		if termrun.IsBreak(e) {
			a.m.Stop()
			a.out.Interrupt()
			return
		}
		if k := termrun.KeyString(e); k != "" {
			a.out.PushKey(k)
		}
		return
	case modeOutput:
		a.mode, a.testOut = modeEdit, nil
		return
	}
	a.status = ""
	if n := len(a.dialogs); n > 0 {
		d := a.dialogs[n-1]
		if d.HandleKey(e) {
			a.popDialog(d)
		}
		return
	}
	if a.menu.Active() {
		a.menu.HandleKey(e)
		return
	}
	if !a.imm && a.autocompleteKey(e) {
		return
	}
	shift := e.Modifiers()&tcell.ModShift != 0
	ctrl := e.Modifiers()&tcell.ModCtrl != 0
	switch e.Key() {
	case tcell.KeyF10:
		if a.paused() || ctrl { // F10 = paso por procedimiento mientras se depura
			a.debugRun(vm.DebugStepOver)
			return
		}
		a.leaveLine()
		a.menu.Activate()
		return
	case tcell.KeyF8:
		if shift {
			a.debugRun(vm.DebugStepOut)
		} else {
			a.debugRun(vm.DebugStepInto)
		}
		return
	case tcell.KeyF20: // Shift+F8
		a.debugRun(vm.DebugStepOut)
		return
	case tcell.KeyF34: // Ctrl+F10
		a.debugRun(vm.DebugStepOver)
		return
	case tcell.KeyF7:
		a.runToCursor()
		return
	case tcell.KeyF9:
		if shift {
			a.instantWatch()
		} else {
			a.toggleBreakpoint()
		}
		return
	case tcell.KeyF21: // Shift+F9
		a.instantWatch()
		return
	case tcell.KeyF1:
		a.contextHelp()
		return
	case tcell.KeyF2:
		switch {
		case shift:
			a.nextView(1)
		case ctrl:
			a.nextView(-1)
		default:
			a.subsDialog()
		}
		return
	case tcell.KeyF14: // Shift+F2 en muchas terminales
		a.nextView(1)
		return
	case tcell.KeyF26: // Ctrl+F2
		a.nextView(-1)
		return
	case tcell.KeyF3:
		a.findNext()
		return
	case tcell.KeyF4:
		a.leaveLine()
		a.mode = modeOutput
		return
	case tcell.KeyF5:
		if shift {
			a.start(true)
		} else {
			a.start(false)
		}
		return
	case tcell.KeyF17: // Shift+F5
		a.start(true)
		return
	case tcell.KeyF6:
		a.switchWindow()
		return
	case tcell.KeyCtrlS:
		a.save(false, nil)
		return
	case tcell.KeyEsc:
		a.activeEditor().sel = false
		return
	case tcell.KeyRune:
		if e.Modifiers()&tcell.ModAlt != 0 {
			a.leaveLine()
			a.menu.OpenByHot(e.Rune())
			return
		}
	}
	if a.imm {
		if e.Key() == tcell.KeyEnter {
			a.immediateEnter()
			return
		}
		a.imEd.HandleKey(e)
		return
	}
	a.ed.HandleKey(e)
	a.afterEditorKey(e)
}

func (a *App) mouse(e *tcell.EventMouse) {
	x, y := e.Position()
	btn := e.Buttons()
	if a.mode == modeOutput && btn&tcell.Button1 != 0 {
		a.testOut = nil
		a.mode = modeEdit
		return
	}
	if a.mode != modeEdit {
		return
	}
	switch {
	case btn&tcell.WheelUp != 0:
		a.activeEditor().Scroll(-3)
		return
	case btn&tcell.WheelDown != 0:
		a.activeEditor().Scroll(3)
		return
	case btn&tcell.Button1 == 0:
		return
	}
	now := time.Now()
	double := now.Sub(a.lastClick) < 400*time.Millisecond && x == a.lastClickX && y == a.lastClickY
	a.lastClick, a.lastClickX, a.lastClickY = now, x, y

	if n := len(a.dialogs); n > 0 {
		d := a.dialogs[n-1]
		if d.Click(x, y, double) {
			a.popDialog(d)
		}
		return
	}
	if a.menu.Active() || y == 0 {
		if y == 0 {
			a.leaveLine()
		}
		a.menu.Click(x, y)
		return
	}
	l := a.lay
	switch {
	case l.edit.contains(x, y):
		if a.imm {
			a.imEd.leaveLine()
			a.imm = false
		}
		if x < a.ed.area.x { // clic en los números de línea: inicio de esa línea
			x = a.ed.area.x - a.doc.Views[a.cur].Left
		}
		a.ac.open = false
		a.ed.Click(x, y, e.Modifiers()&tcell.ModShift != 0)
		a.updateHint()
	case l.hasImm && l.imm.contains(x, y):
		if !a.imm {
			a.ed.leaveLine()
			a.imm = true
		}
		a.imEd.Click(x, y, false)
	case l.hasPanel && l.panel.contains(x, y):
		i := y - l.panel.y
		if i >= 0 && i < len(a.panelItems) {
			a.gotoItem(a.panelItems[i])
		}
	case x == l.frame.x+l.frame.w-1 && y == l.edit.y:
		a.ed.Scroll(-1)
	case x == l.frame.x+l.frame.w-1 && y == l.edit.y+l.edit.h-1:
		a.ed.Scroll(1)
	}
}

// ------------------------------------------------------------- vistas

func (a *App) leaveLine() {
	if a.imm {
		a.imEd.leaveLine()
	} else {
		a.ed.leaveLine()
	}
}

func (a *App) switchView(i int) {
	if i < 0 || i >= len(a.doc.Views) {
		return
	}
	a.ed.leaveLine()
	if i >= len(a.doc.Views) { // la salida de línea pudo borrar vistas
		i = 0
	}
	a.cur = i
	a.imm = false
	a.ed.setView(a.doc.Views[i])
	a.ed.ensureVisible()
}

func (a *App) nextView(dir int) {
	items := a.programItems()
	k := a.currentItem(items)
	a.gotoItem(items[(k+dir+len(items))%len(items)])
}

func (a *App) switchWindow() {
	if !a.lay.hasImm {
		return
	}
	a.leaveLine()
	a.imm = !a.imm
}

func (a *App) pushDialog(d *Dialog) {
	a.leaveLine()
	a.dialogs = append(a.dialogs, d)
}

func (a *App) popDialog(d *Dialog) {
	for i, x := range a.dialogs {
		if x == d {
			a.dialogs = append(a.dialogs[:i], a.dialogs[i+1:]...)
			return
		}
	}
}

// message muestra un aviso con OK.
func (a *App) message(title string, lines ...string) {
	a.pushDialog(&Dialog{Title: title, Lines: lines, Buttons: []string{"OK"}})
}

// outDelay: tiempo que debe correr el programa antes de mostrar la pantalla de salida.
const outDelay = 150 * time.Millisecond

func (a *App) drawWatches() {
	r := a.lay.watch
	for i := 0; i < r.h && i < len(a.watches); i++ {
		w := a.watches[i]
		a.c.hline(r.x, r.y+i, r.w, ' ', stWatch)
		var b strings.Builder
		b.WriteString(watchLabel(w))
		b.WriteString(": ")
		b.WriteString(w.value)
		a.c.text(r.x+1, r.y+i, b.String(), stWatch, r.w-2)
	}
}
