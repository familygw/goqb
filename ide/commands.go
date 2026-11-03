package ide

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"goqb/compiler"
	"goqb/lang/ast"
	"goqb/lang/check"
	"goqb/lang/cp437"
	"goqb/lang/format"
	"goqb/lang/parser"
	"goqb/lang/source"
	"goqb/runtime/sound"
	"goqb/vm"
)

const version = "0.3.0"

func (a *App) buildMenus() {
	always := func() bool { return true }
	running := func() bool { return a.canContinue }
	never := func() bool { return false }
	a.menu = MenuBar{open: -1, Menus: []*Menu{
		{Title: "&File", Items: []*MenuItem{
			{Label: "&New Program", Action: a.newProgram},
			{Label: "&Open Program...", Action: a.openDialog},
			{Label: "&Save", Shortcut: "Ctrl+S", Action: func() { a.save(false, nil) }},
			{Label: "Save &As...", Action: func() { a.save(true, nil) }},
			{Sep: true},
			{Label: "E&xit", Action: a.exit},
		}},
		{Title: "&Edit", Items: []*MenuItem{
			{Label: "&Undo", Shortcut: "Ctrl+Z", Action: func() { a.activeEditor().undo() }},
			{Sep: true},
			{Label: "Cu&t", Shortcut: "Shift+Del", Action: func() { a.activeEditor().cutSel() }},
			{Label: "&Copy", Shortcut: "Ctrl+Ins", Action: func() { a.activeEditor().copySel() }},
			{Label: "&Paste", Shortcut: "Shift+Ins", Action: func() { a.activeEditor().paste() }},
			{Label: "Cl&ear", Shortcut: "Del", Action: func() { a.activeEditor().deleteSelection() }},
			{Sep: true},
			{Label: "New &SUB...", Action: func() { a.newProcDialog("SUB") }},
			{Label: "New &FUNCTION...", Action: func() { a.newProcDialog("FUNCTION") }},
		}},
		{Title: "&View", Items: []*MenuItem{
			{Label: "&SUBs...", Shortcut: "F2", Action: a.subsDialog},
			{Label: "&Next SUB", Shortcut: "Shift+F2", Action: func() { a.nextView(1) }},
			{Label: "&Output Screen", Shortcut: "F4", Action: func() { a.mode = modeOutput }},
		}},
		{Title: "&Search", Items: []*MenuItem{
			{Label: "&Find...", Action: a.findDialog},
			{Label: "&Repeat Last Find", Shortcut: "F3", Action: a.findNext},
			{Label: "&Change...", Action: a.changeDialog},
		}},
		{Title: "&Run", Items: []*MenuItem{
			{Label: "&Start", Shortcut: "Shift+F5", Action: func() { a.start(true) }},
			{Label: "&Restart", Action: a.restart},
			{Label: "Co&ntinue", Shortcut: "F5", Action: func() { a.start(false) }, Enabled: running},
			{Label: "Modify &COMMAND$...", Action: a.commandDialog},
			{Sep: true},
			{Label: "Make E&XE File...", Enabled: never},
		}},
		{Title: "&Debug", Items: []*MenuItem{
			{Label: "Add &Watch...", Enabled: never},
			{Label: "&Step", Shortcut: "F8", Enabled: never},
			{Label: "&Procedure Step", Shortcut: "F10", Enabled: never},
			{Label: "Toggle &Breakpoint", Shortcut: "F9", Enabled: never},
			{Label: "Set &Next Statement", Enabled: never},
		}},
		{Title: "&Options", Items: []*MenuItem{
			{Label: "&Syntax Checking", Action: func() { a.syntaxCheck = !a.syntaxCheck }, Checked: func() bool { return a.syntaxCheck }, Enabled: always},
			{Label: "&Auto Format", Action: func() { a.autoFormat = !a.autoFormat }, Checked: func() bool { return a.autoFormat }, Enabled: always},
			{Label: "Syntax &Coloring", Action: a.toggleColoring, Checked: func() bool { return a.ed.Colorize != nil }, Enabled: always},
			{Label: "&Line Numbers", Action: func() { a.lineNumbers = !a.lineNumbers }, Checked: func() bool { return a.lineNumbers }, Enabled: always},
		}},
		{Title: "&Help", Items: []*MenuItem{
			{Label: "&Keyboard...", Shortcut: "F1", Action: a.help},
			{Label: "&About...", Action: a.about},
		}},
	}}
}

// ------------------------------------------------------------- salida de línea

// Mensajes de estructura de bloques: no se informan al editar una línea.
func blockMessage(msg string) bool {
	for _, s := range []string{"without", "not within", "not allowed inside", "illegal between", "illegal in TYPE"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// lineExit formatea la línea, crea procedimientos y verifica la sintaxis.
func (a *App) lineExit(row int) {
	v := a.doc.Views[a.cur]
	if row >= len(v.Lines) {
		return
	}
	line := v.Lines[row]
	if a.autoFormat {
		line = format.Line(line, a.doc.Names)
		v.Lines[row] = line
	}
	// SUB/FUNCTION nuevos: se mueven a su propia vista, como en QB.
	if kind, name := procHeader(line); kind != "" {
		if a.cur > 0 && row == 0 {
			v.Name, v.Kind = name, kind // renombrar el procedimiento actual
			if n := len(v.Lines) - 1; n > 0 && isProcEnd(v.Lines[n], oppositeKind(kind)) {
				v.Lines[n] = "END " + kind
			}
		} else {
			if len(v.Lines) > 1 {
				v.Lines = append(v.Lines[:row], v.Lines[row+1:]...)
			} else {
				v.Lines[0] = ""
			}
			idx := a.doc.FindView(name)
			if idx < 0 {
				idx = a.doc.AddProc(kind, name, strings.TrimSpace(line))
			}
			a.doc.Touch()
			a.cur = idx
			a.ed.setView(a.doc.Views[idx])
			a.ed.ensureVisible()
			return
		}
	}
	if !a.syntaxCheck {
		return
	}
	text := strings.Join(v.Lines, "\n")
	_, errs := parser.ParseFile("", cp437.Encode(text))
	for _, e := range errs {
		if e.Pos.Line == row+1 && !blockMessage(e.Msg) {
			col := e.Pos.Col - 1
			a.pushDialog(&Dialog{Lines: []string{e.Msg}, Buttons: []string{"OK", "Help"},
				OnClose: func(d *Dialog, b int) bool {
					if b == 1 {
						a.help()
					}
					v.Row, v.Col = row, max(0, col)
					a.ed.ensureVisible()
					return true
				}})
			return
		}
	}
}

func oppositeKind(k string) string {
	if k == "SUB" {
		return "FUNCTION"
	}
	return "SUB"
}

// ------------------------------------------------------------- compilar y ejecutar

func (a *App) mainPath() string {
	if a.doc.Path != "" {
		p, _ := filepath.Abs(a.doc.Path)
		return p
	}
	wd, _ := os.Getwd()
	return filepath.Join(wd, "UNTITLED.BAS")
}

// compile compila el documento. Si quiet, no muestra errores.
func (a *App) compile(quiet bool) bool {
	src, locs := a.doc.Source()
	main := a.mainPath()
	ld := &source.Loader{ReadFile: func(p string) ([]byte, error) {
		if p == main {
			return src, nil
		}
		b, err := os.ReadFile(p)
		return cp437.ToCP437(b), err
	}}
	prog, diags := ld.Load(main)
	if len(diags) == 0 {
		diags = check.Program(prog)
	}
	var p *vm.Program
	var comp *compiler.Compiler
	if len(diags) == 0 {
		p, comp, diags = compiler.Compile(prog)
	}
	if len(diags) > 0 {
		if !quiet {
			d := diags[0]
			a.gotoDiag(d.File, d.Pos.Line, d.Pos.Col, locs)
			a.message("", d.Msg)
		}
		return false
	}
	a.prog, a.comp, a.locs, a.progRev = p, comp, locs, a.doc.rev
	return true
}

// gotoDiag ubica el cursor en la línea del documento que corresponde a file:line.
func (a *App) gotoDiag(file string, line, col int, locs []Loc) bool {
	if file != a.mainPath() || line < 1 || line > len(locs) {
		return false
	}
	loc := locs[line-1]
	a.switchView(loc.View)
	v := a.doc.Views[loc.View]
	v.Row, v.Col = loc.Line, max(0, col-1)
	a.ed.ensureVisible()
	return true
}

// start ejecuta el programa: restart = Shift+F5 (desde el principio); si no, F5 continúa.
func (a *App) start(restart bool) {
	a.leaveLine()
	if len(a.dialogs) > 0 {
		return
	}
	if !restart && a.canContinue && a.m != nil && a.progRev == a.doc.rev {
		a.curStmt = Loc{View: -1}
		a.out.Rearm()
		a.begin(func() error { return a.m.Run() }, false)
		return
	}
	if !a.compile(false) {
		return
	}
	a.out.Reset()
	a.m = vm.New(a.prog, a.out)
	a.m.Command = a.command
	a.m.Sound = sound.Device()
	a.curStmt = Loc{View: -1}
	a.begin(func() error { return a.m.Run() }, false)
}

func (a *App) restart() {
	a.canContinue = false
	a.m = nil
	a.curStmt = Loc{View: -1}
	a.status = "Programa reiniciado"
}

func (a *App) begin(run func() error, fromImm bool) {
	a.mode = modeRun
	a.fromImm = fromImm
	a.canContinue = false
	go func() { a.vmDone <- run() }()
}

// finished procesa el fin de la ejecución.
func (a *App) finished(err error) {
	if err == nil {
		a.mode = modeOutput // "Press any key to continue"
		return
	}
	a.mode = modeEdit
	var re *vm.RuntimeError
	if !errors.As(err, &re) {
		a.message("", err.Error())
		return
	}
	if errors.Is(re.Err, vm.ErrBreak) {
		a.canContinue = !a.fromImm
		if a.gotoDiag(re.File, re.Line, re.Col, a.locs) {
			a.curStmt = Loc{View: a.cur, Line: a.doc.Views[a.cur].Row}
		}
		a.status = "Interrumpido: F5 continúa, Shift+F5 reinicia"
		return
	}
	if !a.fromImm {
		a.gotoDiag(re.File, re.Line, re.Col, a.locs)
	}
	a.message("", re.Err.Error())
}

// immediateEnter ejecuta la línea actual de la ventana Immediate.
func (a *App) immediateEnter() {
	v := a.imv
	line := strings.TrimSpace(v.Lines[v.Row])
	// la línea siguiente queda lista para escribir
	if v.Row == len(v.Lines)-1 {
		v.Lines = append(v.Lines, "")
		if len(v.Lines) > 10 {
			v.Lines = v.Lines[1:]
			v.Row--
		}
	}
	v.Row++
	v.Col = 0
	a.imEd.lineDirty = false
	a.imEd.ensureVisible()
	if line == "" {
		return
	}
	f, errs := parser.ParseFile("Immediate", cp437.Encode(line))
	if len(errs) > 0 {
		a.message("", errs[0].Msg)
		return
	}
	if a.comp == nil || a.progRev != a.doc.rev || a.m == nil {
		if !a.compile(true) {
			// programa con errores: Immediate trabaja sobre un programa vacío
			empty := &source.Program{Modules: []*ast.File{{Name: "Immediate"}}}
			a.prog, a.comp, _ = compiler.Compile(empty)
			a.progRev = -1
		}
		a.m = vm.New(a.prog, a.out)
		a.m.Command = a.command
		a.m.Sound = sound.Device()
	}
	entry, diags := a.comp.Immediate(f.Stmts)
	if len(diags) > 0 {
		a.message("", diags[0].Msg)
		return
	}
	a.out.Rearm()
	a.begin(func() error { return a.m.Continue(a.prog, entry) }, true)
}

// ------------------------------------------------------------- diálogos

func (a *App) subsDialog() {
	order := a.sortedViews()
	items := make([]string, len(order))
	sel := 0
	for i, vi := range order {
		if vi == 0 {
			items[i] = a.doc.Title()
		} else {
			items[i] = "  " + a.doc.Views[vi].Name
		}
		if vi == a.cur {
			sel = i
		}
	}
	d := &Dialog{Title: "SUBs", List: &ListBox{Label: "Choose program item to edit", Items: items, Sel: sel, Height: min(12, len(items)+1), Width: 40},
		Buttons: []string{"Edit in Active", "Delete", "Cancel"}}
	d.OnClose = func(d *Dialog, b int) bool {
		vi := order[d.List.Sel]
		switch b {
		case 0:
			a.switchView(vi)
		case 1:
			if vi == 0 {
				a.message("", "No se puede borrar el código de módulo")
				return true
			}
			name := a.doc.Views[vi].Name
			a.pushDialog(&Dialog{Lines: []string{"¿Borrar el procedimiento " + name + "?"}, Buttons: []string{"OK", "Cancel"},
				OnClose: func(_ *Dialog, b int) bool {
					if b == 0 {
						a.doc.Views = append(a.doc.Views[:vi], a.doc.Views[vi+1:]...)
						a.doc.Touch()
						a.cur = 0
						a.ed.setView(a.doc.Views[0])
					}
					return true
				}})
		}
		return true
	}
	a.pushDialog(d)
}

func (a *App) newProcDialog(kind string) {
	in := &TextInput{Label: "Name:", Width: 30}
	a.pushDialog(&Dialog{Title: "New " + kind, Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			name := strings.TrimSpace(d.InputText())
			if b != 0 || name == "" {
				return true
			}
			if _, n := procHeader(kind + " " + name); n == "" {
				a.message("", "Nombre inválido: "+name)
				return true
			}
			idx := a.doc.FindView(name)
			if idx < 0 {
				idx = a.doc.AddProc(kind, name, "")
			}
			a.switchView(idx)
			return true
		}})
}

func (a *App) commandDialog() {
	in := &TextInput{Label: "Command Line:", Width: 40}
	in.set(a.command)
	a.pushDialog(&Dialog{Title: "Modify COMMAND$", Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b == 0 {
				a.command = d.InputText()
			}
			return true
		}})
}

func (a *App) help() {
	a.message("Teclado",
		"F1 Ayuda          F2 Lista de SUBs     Shift+F2 SUB siguiente",
		"F3 Repetir búsqueda   F4 Pantalla de salida",
		"F5 Ejecutar/Continuar   Shift+F5 Ejecutar desde el inicio",
		"F6 Editor/Immediate   F10 o Alt+letra: menús   Ctrl+Break: detener",
		"",
		"Edición: Shift+flechas selecciona · Shift+Del corta · Ctrl+Ins copia",
		"Shift+Ins pega · Ctrl+Y borra la línea · Ctrl+Z deshace · Ins sobrescribe",
		"",
		"En macOS: activá \"Option como Meta\" en la terminal para usar Alt.")
}

func (a *App) about() {
	a.message("About", "goqb "+version, "Entorno QuickBASIC 4.5 escrito en Go", "")
}

// ------------------------------------------------------------- archivos

// confirmDiscard pregunta si guardar cambios; then se ejecuta si se puede seguir.
func (a *App) confirmDiscard(then func()) {
	if !a.doc.Dirty {
		then()
		return
	}
	a.pushDialog(&Dialog{Lines: []string{"Hay cambios sin guardar.", "¿Guardarlos ahora?"}, Buttons: []string{"Yes", "No", "Cancel"},
		OnClose: func(_ *Dialog, b int) bool {
			switch b {
			case 0:
				a.save(false, then)
			case 1:
				then()
			}
			return true
		}})
}

func (a *App) setDocument(d *Document) {
	a.doc = d
	a.cur = 0
	a.ed.setView(d.Views[0])
	a.m, a.comp, a.canContinue = nil, nil, false
	a.curStmt = Loc{View: -1}
}

func (a *App) newProgram() {
	a.confirmDiscard(func() { a.setDocument(NewDocument()) })
}

func (a *App) exit() {
	a.confirmDiscard(func() { a.quit = true })
}

// save guarda; asNew fuerza el diálogo "Save As". then se ejecuta al terminar bien.
func (a *App) save(asNew bool, then func()) {
	a.leaveLine()
	doSave := func(path string) {
		if err := a.doc.Save(path); err != nil {
			a.message("", "No se pudo guardar: "+err.Error())
			return
		}
		a.status = "Guardado " + a.doc.Path
		if then != nil {
			then()
		}
	}
	if a.doc.Path != "" && !asNew {
		doSave(a.doc.Path)
		return
	}
	in := &TextInput{Label: "File Name:", Width: 40}
	in.set(a.doc.Path)
	a.pushDialog(&Dialog{Title: "Save As", Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			name := strings.TrimSpace(d.InputText())
			if b == 0 && name != "" {
				doSave(name)
			}
			return true
		}})
}

// listDir devuelve los .BAS y subdirectorios del directorio actual.
func listDir() []string {
	ents, _ := os.ReadDir(".")
	var dirs, files []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, "["+e.Name()+"]")
		} else if ext := strings.ToUpper(filepath.Ext(e.Name())); ext == ".BAS" || ext == ".BI" {
			files = append(files, e.Name())
		}
	}
	sort.Strings(dirs)
	sort.Strings(files)
	return append(append([]string{"[..]"}, dirs...), files...)
}

func (a *App) openDialog() {
	a.confirmDiscard(func() {
		in := &TextInput{Label: "File Name:", Width: 40}
		list := &ListBox{Items: listDir(), Height: 10, Width: 40}
		wd, _ := os.Getwd()
		d := &Dialog{Title: "Open Program", Lines: []string{wd}, Input: in, List: list, Buttons: []string{"OK", "Cancel"}}
		d.OnSelect = func(d *Dialog) {
			if it := d.List.Items[d.List.Sel]; !strings.HasPrefix(it, "[") {
				d.Input.set(it)
			}
		}
		d.OnActivate = func(d *Dialog) bool {
			it := d.List.Items[d.List.Sel]
			if strings.HasPrefix(it, "[") {
				os.Chdir(strings.Trim(it, "[]"))
				wd, _ := os.Getwd()
				d.Lines[0] = wd
				d.List.Items, d.List.Sel = listDir(), 0
				return false
			}
			d.Input.set(it)
			return true
		}
		d.OnClose = func(d *Dialog, b int) bool {
			name := strings.TrimSpace(d.InputText())
			if b != 0 || name == "" {
				return true
			}
			if fi, err := os.Stat(name); err == nil && fi.IsDir() {
				os.Chdir(name)
				wd, _ := os.Getwd()
				d.Lines[0] = wd
				d.List.Items, d.List.Sel = listDir(), 0
				d.Input.set("")
				return false
			}
			if filepath.Ext(name) == "" {
				if _, err := os.Stat(name + ".BAS"); err == nil {
					name += ".BAS"
				} else if _, err := os.Stat(name + ".bas"); err == nil {
					name += ".bas"
				}
			}
			doc, err := LoadDocument(name)
			if err != nil {
				a.message("", "File not found: "+name)
				return true
			}
			a.setDocument(doc)
			return true
		}
		a.pushDialog(d)
	})
}

// ------------------------------------------------------------- búsqueda

func (a *App) findDialog() {
	in := &TextInput{Label: "Find What:", Width: 40}
	in.set(a.lastFind)
	if t := a.ed.selectedText(); t != "" && !strings.Contains(t, "\n") {
		in.set(t)
	}
	a.pushDialog(&Dialog{Title: "Find", Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b == 0 && d.InputText() != "" {
				a.lastFind = d.InputText()
				a.findNext()
			}
			return true
		}})
}

// findNext busca lastFind desde el cursor, en todas las vistas (sin distinguir mayúsculas).
func (a *App) findNext() {
	if a.lastFind == "" {
		a.findDialog()
		return
	}
	needle := []rune(strings.ToUpper(a.lastFind))
	order := a.sortedViews()
	start := 0
	for k, vi := range order {
		if vi == a.cur {
			start = k
		}
	}
	v := a.doc.Views[a.cur]
	for n := 0; n <= len(order); n++ {
		vi := order[(start+n)%len(order)]
		lines := a.doc.Views[vi].Lines
		for r := 0; r < len(lines); r++ {
			row := r
			if n == 0 {
				row = v.Row + r
				if row >= len(lines) {
					break
				}
			}
			hay := []rune(strings.ToUpper(lines[row]))
			from := 0
			if n == 0 && row == v.Row {
				from = v.Col + 1
			}
			if i := indexRunes(hay, needle, from); i >= 0 {
				a.switchView(vi)
				fv := a.doc.Views[vi]
				fv.Row, fv.Col = row, i
				a.ed.sel, a.ed.selRow, a.ed.selCol = true, row, i
				fv.Col = i + len(needle)
				a.ed.ensureVisible()
				return
			}
		}
	}
	a.message("", "Match not found")
}

func indexRunes(hay, needle []rune, from int) int {
	for i := from; i+len(needle) <= len(hay); i++ {
		if string(hay[i:i+len(needle)]) == string(needle) {
			return i
		}
	}
	return -1
}

func (a *App) changeDialog() {
	find := &TextInput{Label: "Find What:", Width: 40}
	find.set(a.lastFind)
	a.pushDialog(&Dialog{Title: "Change", Input: find, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			what := d.InputText()
			if b != 0 || what == "" {
				return true
			}
			a.lastFind = what
			to := &TextInput{Label: "Change To:", Width: 40}
			a.pushDialog(&Dialog{Title: "Change", Lines: []string{"Cambiar todas las apariciones de \"" + what + "\""}, Input: to, Buttons: []string{"Change All", "Cancel"},
				OnClose: func(d *Dialog, b int) bool {
					if b != 0 {
						return true
					}
					n := 0
					for _, v := range a.doc.Views {
						for i, l := range v.Lines {
							if c := strings.Count(strings.ToUpper(l), strings.ToUpper(what)); c > 0 {
								v.Lines[i] = replaceFold(l, what, d.InputText())
								n += c
							}
						}
					}
					if n > 0 {
						a.doc.Touch()
					}
					a.status = "Cambios: " + strconv.Itoa(n)
					return true
				}})
			return true
		}})
}

func replaceFold(s, old, repl string) string {
	var b strings.Builder
	up, uo := strings.ToUpper(s), strings.ToUpper(old)
	i := 0
	for {
		j := strings.Index(up[i:], uo)
		if j < 0 {
			b.WriteString(s[i:])
			return b.String()
		}
		b.WriteString(s[i : i+j])
		b.WriteString(repl)
		i += j + len(old)
	}
}

func (a *App) toggleColoring() {
	if a.ed.Colorize == nil {
		a.ed.Colorize = highlight
	} else {
		a.ed.Colorize = nil
	}
}
