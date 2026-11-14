package ide

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"goqb/runtime/termrun"
)

type harness struct {
	t   *testing.T
	scr tcell.SimulationScreen
	app *App
	end chan struct{}
}

func newHarness(t *testing.T, w, h int, path string) *harness {
	t.Helper()
	scr := tcell.NewSimulationScreen("UTF-8")
	if err := scr.Init(); err != nil {
		t.Fatal(err)
	}
	scr.SetSize(w, h)
	app, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	hs := &harness{t: t, scr: scr, app: app, end: make(chan struct{})}
	go func() {
		app.RunOn(scr)
		close(hs.end)
	}()
	hs.wait()
	return hs
}

func (h *harness) wait() { time.Sleep(60 * time.Millisecond) }

func (h *harness) key(k tcell.Key, r rune, mod tcell.ModMask) {
	h.scr.InjectKey(k, r, mod)
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		switch r {
		case '\n':
			h.key(tcell.KeyEnter, 0, 0)
		default:
			h.key(tcell.KeyRune, r, 0)
		}
		time.Sleep(2 * time.Millisecond)
	}
	h.wait()
}

func (h *harness) text() string {
	cells, w, hh := h.scr.GetContents()
	var b strings.Builder
	for y := 0; y < hh; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (h *harness) close() {
	h.scr.Fini()
	select {
	case <-h.end:
	case <-time.After(2 * time.Second):
		h.t.Error("el IDE no terminó")
	}
}

func (h *harness) expect(substr string) {
	h.t.Helper()
	if !strings.Contains(h.text(), substr) {
		h.t.Fatalf("no aparece %q en pantalla:\n%s", substr, h.text())
	}
}

func TestIDEEditRunAndSubs(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)

	h := newHarness(t, 80, 25, "")
	defer h.close()
	h.expect("Untitled")
	h.expect("Immediate")
	h.expect("File")

	// formateo automático al salir de la línea
	h.typeText("cls\nfor i=1 to 3:print i*10;:next\nsaludar \"mundo\"\n")
	h.expect("FOR i = 1 TO 3: PRINT i * 10;: NEXT")
	h.expect("CLS")

	// escribir "sub Saludar(q$)" crea un procedimiento en su propia vista
	// (el identificador conserva la primera grafía escrita: "saludar")
	h.typeText("sub Saludar(q$)\n")
	h.expect("Untitled:saludar")
	h.expect("SUB saludar (q$)")
	h.expect("END SUB")
	h.typeText("print \"Hola, \";q$\n")

	// F5: ejecutar y ver la salida
	h.key(tcell.KeyF5, 0, 0)
	time.Sleep(300 * time.Millisecond)
	h.expect(" 10  20  30")
	h.expect("Hola, mundo")
	h.expect("Press any key to continue")
	h.key(tcell.KeyRune, ' ', 0)
	h.wait()
	h.expect("Immediate")

	// F2: lista de procedimientos
	h.key(tcell.KeyF2, 0, 0)
	h.wait()
	h.expect("Edit in Active")
	h.expect("  saludar")
	h.key(tcell.KeyUp, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.wait()
	h.expect(" Untitled ")

	// error de sintaxis al salir de una línea
	h.key(tcell.KeyHome, 0, tcell.ModCtrl)
	h.typeText("x = (1 +\n")
	h.expect("Expected: expression")
	h.key(tcell.KeyEnter, 0, 0)
	h.wait()

	// Immediate: F6 y una sentencia
	h.key(tcell.KeyF6, 0, 0)
	h.typeText("print 6*7\n")
	time.Sleep(200 * time.Millisecond)
	h.expect(" 42")
	h.key(tcell.KeyRune, ' ', 0)
	h.wait()

	// el texto armado lleva el DECLARE automático
	lines, _ := h.app.doc.Assemble()
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "DECLARE SUB saludar (q$)") {
		t.Errorf("falta DECLARE automático:\n%s", all)
	}
}

func TestIDERuntimeErrorAndBreak(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/err.bas"
	os.WriteFile(path, []byte("PRINT \"a\"\r\nx = 1 / 0\r\n"), 0o644)
	h := newHarness(t, 80, 25, path)
	defer h.close()
	h.expect("ERR.BAS")
	h.key(tcell.KeyF5, 0, 0)
	time.Sleep(300 * time.Millisecond)
	h.expect("Division by zero")
	if v := h.app.doc.Views[0]; v.Row != 1 {
		t.Errorf("el cursor debería quedar en la línea del error, está en %d", v.Row)
	}
	h.key(tcell.KeyEnter, 0, 0)
	h.wait()

	// bucle infinito + Ctrl+Break + continuar
	h.app.doc.Views[0].Lines = []string{"DO", "  i = i + 1", "LOOP"}
	h.app.doc.Touch()
	h.key(tcell.KeyF5, 0, tcell.ModShift)
	time.Sleep(200 * time.Millisecond)
	h.key(tcell.KeyCtrlC, 0, 0)
	time.Sleep(200 * time.Millisecond)
	h.expect("Interrumpido")
	if !h.app.canContinue {
		t.Error("debería poder continuar tras Ctrl+Break")
	}
}

func TestIDEResponsive(t *testing.T) {
	h := newHarness(t, 60, 16, "")
	defer h.close()
	h.expect("F1 Help")
	if strings.Contains(h.text(), "Immediate") {
		t.Error("en modo compacto no debería verse Immediate")
	}
	h.scr.SetSize(30, 10)
	h.scr.PostEvent(tcell.NewEventResize(30, 10))
	h.wait()
	h.expect("terminal muy chica")
	h.scr.SetSize(120, 40)
	h.scr.PostEvent(tcell.NewEventResize(120, 40))
	h.wait()
	h.expect(" SUBs ")
	h.expect("Immediate")
}

func TestIDELineNumbers(t *testing.T) {
	h := newHarness(t, 80, 25, "")
	defer h.close()
	h.typeText("cls\nprint 1\nprint 2\n")
	for _, want := range []string{"│  1 CLS", "│  2 PRINT 1", "│  3 PRINT 2", "│  4 "} {
		h.expect(want)
	}
	// clic en el número de la línea 2: el cursor va al inicio de esa línea
	h.scr.InjectMouse(2, 3, tcell.Button1, 0)
	h.scr.InjectMouse(2, 3, tcell.ButtonNone, 0)
	h.wait()
	if v := h.app.doc.Views[0]; v.Row != 1 || v.Col != 0 {
		t.Errorf("cursor en %d:%d, se esperaba 1:0", v.Row, v.Col)
	}
	h.expect("00002:001")
	// Options → Line Numbers permite desactivarlos.
	h.key(tcell.KeyRune, 'o', tcell.ModAlt)
	h.key(tcell.KeyRune, 'l', 0)
	h.wait()
	if strings.Contains(h.text(), "│  1 CLS") {
		t.Fatal("los números de línea deberían poder desactivarse")
	}
}

func TestHighlight(t *testing.T) {
	st := highlight(`10 PRINT "hola"; LEFT$(a$, 2) ' nota`)
	check := func(col int, want tcell.Style, what string) {
		t.Helper()
		if st[col] != want {
			t.Errorf("%s (col %d): estilo incorrecto", what, col)
		}
	}
	check(0, hlLabel, "número de línea")
	check(3, hlKeyword, "PRINT")
	check(9, hlString, "string")
	check(17, hlBuiltin, "LEFT$")
	check(23, hlText, "variable")
	check(27, hlNumber, "número")
	check(30, hlComment, "comentario")
	check(35, hlComment, "fin del comentario")
}

func TestIDEGraphicsBlocks(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/g.bas"
	os.WriteFile(path, []byte("SCREEN 13\r\nLINE (0, 0)-(319, 199), 4, BF\r\nCIRCLE (160, 100), 60, 14\r\nDO: LOOP WHILE INKEY$ = \"\"\r\n"), 0o644)
	h := newHarness(t, 80, 25, path)
	defer h.close()
	h.key(tcell.KeyF5, 0, 0)
	time.Sleep(300 * time.Millisecond)
	h.expect("▀▀▀▀")
	cells, w, _ := h.scr.GetContents()
	// el centro debería ser rojo (atributo 4) y no negro
	c := cells[12*w+40]
	fg, _, _ := c.Style.Decompose()
	if fg != tcell.NewHexColor(0xAA0000) && fg != tcell.NewHexColor(0xFFFF55) {
		t.Errorf("color inesperado en el centro: %v", fg)
	}
	h.key(tcell.KeyRune, 'x', 0)
	time.Sleep(200 * time.Millisecond)
	h.key(tcell.KeyRune, ' ', 0)
	h.wait()
	h.expect("Immediate")
}

func TestIDEDebugger(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/dbg.bas"
	src := "DECLARE SUB Doble (n)\r\nFOR i = 1 TO 3\r\n    Doble i\r\nNEXT\r\nPRINT \"fin\"; total\r\nSUB Doble (n)\r\n    SHARED total\r\n    total = total + n * 2\r\nEND SUB\r\n"
	os.WriteFile(path, []byte(src), 0o644)
	h := newHarness(t, 100, 30, path)
	defer h.close()
	a := h.app

	// breakpoint en "total = total + n * 2" (vista del SUB, línea 2)
	vi := a.doc.FindView("Doble")
	a.switchView(vi)
	a.doc.Views[vi].Row = 2
	h.key(tcell.KeyF9, 0, 0)
	h.wait()
	if !a.isBreak(2) {
		t.Fatal("F9 no puso el breakpoint")
	}
	h.key(tcell.KeyF5, 0, 0)
	time.Sleep(300 * time.Millisecond)
	h.expect("Detenido")
	if a.curStmt.View != vi || a.curStmt.Line != 2 {
		t.Fatalf("sentencia actual: %+v", a.curStmt)
	}
	a.addWatch("n", false)
	h.key(tcell.KeyEsc, 0, 0) // forzar un redibujo
	h.wait()
	h.expect("Doble n:  1")

	// F8: paso a END SUB; F8 otra vez: vuelve al NEXT del módulo
	h.key(tcell.KeyF8, 0, 0)
	time.Sleep(200 * time.Millisecond)
	if a.curStmt.Line != 3 {
		t.Errorf("después de F8: %+v", a.curStmt)
	}
	h.key(tcell.KeyF8, 0, 0)
	time.Sleep(200 * time.Millisecond)
	if a.curStmt.View != 0 || a.curStmt.Line != 3 { // NEXT (líneas desde 0)
		t.Errorf("después del segundo F8: %+v", a.curStmt)
	}
	h.expect("<Not watchable>")

	// Immediate en el programa detenido, y se puede seguir
	h.key(tcell.KeyF6, 0, 0)
	h.typeText("total = total + 100\n")
	time.Sleep(200 * time.Millisecond)
	if !a.paused() {
		t.Fatal("después de Immediate debería poder continuar")
	}
	h.key(tcell.KeyF6, 0, 0)
	h.wait()
	a.clearBreakpoints()
	h.key(tcell.KeyF5, 0, 0)
	time.Sleep(300 * time.Millisecond)
	h.expect("fin 112")
}

func TestIDEKeywordHelp(t *testing.T) {
	h := newHarness(t, 100, 30, "")
	h.typeText("PRINT \"hola\"")
	h.key(tcell.KeyHome, 0, 0)
	h.key(tcell.KeyF1, 0, 0)
	h.wait()
	if txt := h.text(); !strings.Contains(txt, "Escribe en pantalla") {
		t.Errorf("no aparece la ayuda de PRINT:\n%s", txt)
	}
	h.key(tcell.KeyEscape, 0, 0)
	h.key(tcell.KeyEnd, 0, 0)
	h.key(tcell.KeyF1, 0, 0) // sobre el string: ayuda de teclado
	h.wait()
	if txt := h.text(); !strings.Contains(txt, "Shift+F2") {
		t.Errorf("no aparece la ayuda de teclado:\n%s", txt)
	}
}

func TestIDEGraphicsAspect(t *testing.T) {
	defer func() { termrun.CellAspectOverride = 0 }()
	h := newHarness(t, 120, 40, "")
	h.key(tcell.KeyRune, 'o', tcell.ModAlt)
	h.wait()
	h.key(tcell.KeyRune, 'g', 0)
	h.wait()
	if !strings.Contains(h.text(), "Graphics Aspect") {
		t.Fatalf("no se abrió el diálogo:\n%s", h.text())
	}
	h.typeText("2.2\n")
	h.wait()
	if termrun.CellAspectOverride != 2.2 {
		t.Fatalf("CellAspectOverride = %v", termrun.CellAspectOverride)
	}
	if h.app.testOut == nil || h.app.mode != modeOutput {
		t.Fatal("no se mostró el patrón de prueba")
	}
	h.key(tcell.KeyEscape, 0, 0)
	h.wait()
	if h.app.testOut != nil {
		t.Error("el patrón de prueba debería cerrarse con una tecla")
	}
}

func TestIDEAutocomplete(t *testing.T) {
	h := newHarness(t, 100, 30, "")
	defer h.close()
	lines := func() []string { return h.app.doc.Views[h.app.cur].Lines }

	// palabra clave + Tab
	h.typeText("RAND")
	h.expect("RANDOMIZE")
	h.key(tcell.KeyTab, 0, 0)
	h.wait()
	if got := lines()[0]; got != "RANDOMIZE" {
		t.Fatalf("Tab: línea %q", got)
	}
	// la barra de estado muestra la sintaxis de la sentencia
	h.typeText(" ")
	h.expect("RANDOMIZE [semilla]")
	h.typeText("TIMER\n")

	// variable del programa; Enter sin elegir termina la línea normalmente
	h.typeText("contador = 1\ncon")
	if !h.app.ac.open {
		t.Fatal("la lista debería estar abierta")
	}
	h.expect("variable")
	h.key(tcell.KeyEnter, 0, 0)
	h.wait()
	if h.app.ac.open || !strings.EqualFold(strings.TrimSpace(lines()[2]), "con") {
		t.Fatalf("Enter sin elegir: abierta=%v línea %q", h.app.ac.open, lines()[2])
	}
	// con flechas se elige y Enter acepta
	h.typeText("conta")
	h.key(tcell.KeyDown, 0, 0)
	h.key(tcell.KeyUp, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.wait()
	if got := lines()[3]; got != "contador" {
		t.Fatalf("Enter tras elegir: línea %q", got)
	}
	// función intrínseca con paréntesis: sintaxis en la barra de estado
	h.typeText(" = LEFT$(")
	h.expect("LEFT$(s$, n)")
	h.typeText("x$, 1)")
	// Esc cierra la lista sin tocar el texto
	h.typeText("\nPRI")
	h.key(tcell.KeyEsc, 0, 0)
	h.wait()
	cur := func() string { v := h.app.doc.Views[h.app.cur]; return v.Lines[v.Row] }
	if h.app.ac.open || cur() != "PRI" {
		t.Fatalf("Esc: abierta=%v línea %q", h.app.ac.open, cur())
	}
	// dentro de un string no se sugiere nada
	h.typeText("NT \"PRI")
	if h.app.ac.open {
		t.Error("no debería sugerir dentro de un string")
	}
}

func TestIDEClickPastEnd(t *testing.T) {
	h := newHarness(t, 100, 30, "")
	defer h.close()
	h.typeText("PRINT 1\nPRINT 22\n")
	ed := h.app.ed
	h.scr.InjectMouse(ed.area.x+40, ed.area.y, tcell.Button1, 0)
	h.wait()
	h.scr.InjectMouse(ed.area.x+40, ed.area.y, tcell.ButtonNone, 0)
	h.wait()
	v := h.app.doc.Views[h.app.cur]
	if v.Row != 0 || v.Col != len("PRINT 1") {
		t.Fatalf("clic después del texto: fila %d col %d", v.Row, v.Col)
	}
	h.scr.InjectMouse(ed.area.x+2, ed.area.y+1, tcell.Button1, 0)
	h.wait()
	h.scr.InjectMouse(ed.area.x+2, ed.area.y+1, tcell.ButtonNone, 0)
	h.wait()
	if v.Row != 1 || v.Col != 2 {
		t.Fatalf("clic dentro del texto: fila %d col %d", v.Row, v.Col)
	}
}

func TestIDETabStops(t *testing.T) {
	h := newHarness(t, 100, 30, "")
	defer h.close()
	if h.app.ed.TabSize != 8 {
		t.Fatalf("tab por defecto %d", h.app.ed.TabSize)
	}
	h.key(tcell.KeyRune, 'o', tcell.ModAlt)
	h.wait()
	h.key(tcell.KeyRune, 't', 0)
	h.wait()
	h.expect("Tab Stops")
	for i := 0; i < 3; i++ {
		h.key(tcell.KeyBackspace2, 0, 0)
	}
	h.typeText("4\n")
	if h.app.ed.TabSize != 4 {
		t.Fatalf("TabSize = %d", h.app.ed.TabSize)
	}
	h.typeText("x")
	h.key(tcell.KeyTab, 0, 0)
	h.wait()
	if v := h.app.doc.Views[h.app.cur]; v.Col != 4 {
		t.Fatalf("Tab con 4: columna %d", v.Col)
	}
}

func TestIDEFullView(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/PRUEBA.BAS"
	src := "CLS\r\nDrawBox 3\r\nPRINT Doble(2)\r\n\r\n' dibuja\r\nSUB DrawBox (w AS INTEGER)\r\n  PRINT w\r\nEND SUB\r\n\r\nFUNCTION Doble (x)\r\n  Doble = x * 2\r\nEND FUNCTION\r\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, 120, 40, path)
	defer h.close()
	a := h.app
	if len(a.doc.Views) != 3 {
		t.Fatalf("%d vistas al abrir", len(a.doc.Views))
	}
	split, _ := a.doc.Assemble()

	// activar: todo en una vista, sin perder nada
	h.key(tcell.KeyRune, 'v', tcell.ModAlt)
	h.wait()
	h.key(tcell.KeyRune, 'f', 0)
	h.wait()
	if !a.fullView || len(a.doc.Views) != 1 {
		t.Fatalf("vista completa: %v, %d vistas", a.fullView, len(a.doc.Views))
	}
	full, _ := a.doc.Assemble()
	if strings.Join(full, "\n") != strings.Join(split, "\n") {
		t.Fatalf("el programa cambió al juntarlo:\n%s\n---\n%s", strings.Join(full, "\n"), strings.Join(split, "\n"))
	}
	h.expect("' dibuja")
	h.expect("FUNCTION Doble")
	// el panel lista los procedimientos y salta al encabezado
	items := a.programItems()
	if len(items) != 3 || items[1].name != "Doble" || items[2].name != "DrawBox" {
		t.Fatalf("ítems: %+v", items)
	}
	a.gotoItem(items[2])
	v := a.doc.Views[0]
	if !strings.HasPrefix(v.Lines[v.Row], "SUB DrawBox") || a.currentItem(a.programItems()) != 2 {
		t.Fatalf("salto a DrawBox: fila %d %q", v.Row, v.Lines[v.Row])
	}
	// escribir un SUB nuevo agrega su END SUB en la misma vista
	h.key(tcell.KeyEnd, 0, tcell.ModCtrl)
	h.typeText("\nSUB Nuevo\n")
	if len(a.doc.Views) != 1 {
		t.Fatal("no debería crear otra vista")
	}
	joined := strings.Join(a.doc.Views[0].Lines, "\n")
	if !strings.Contains(joined, "SUB Nuevo\n\nEND SUB") {
		t.Fatalf("falta END SUB:\n%s", joined)
	}
	// guardar en vista completa y volver a abrir: mismas vistas
	a.save(false, nil)
	h.wait()
	data, _ := os.ReadFile(path)
	d := ParseDocument(data)
	if len(d.Views) != 4 || d.FindView("Nuevo") < 0 {
		t.Fatalf("al guardar: %d vistas\n%s", len(d.Views), data)
	}
	// desactivar: vuelve a separar, con el cursor en la vista correspondiente
	a.gotoItem(a.programItems()[1]) // Doble
	a.setFullView(false)
	if a.fullView || len(a.doc.Views) != 4 || a.doc.Views[a.cur].Name != "Doble" {
		t.Fatalf("al separar: %d vistas, actual %q", len(a.doc.Views), a.doc.Views[a.cur].Name)
	}
}
