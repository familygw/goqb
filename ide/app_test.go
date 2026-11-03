package ide

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
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
