package ide

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func editingTest(lines ...string) *Editor {
	clip, column := "", false
	e := &Editor{clipboard: &clip, clipColumn: &column}
	e.setView(&View{Lines: lines})
	return e
}

func arrow(e *Editor, key tcell.Key, mods tcell.ModMask) {
	e.HandleKey(tcell.NewEventKey(key, 0, mods))
}

func TestEditorCommentEndEnter(t *testing.T) {
	for _, tc := range []struct {
		name, line string
		indent     int
	}{
		{"box CP437", `    PRINT "┌"; STRING$(BoxWidth - 2, "─"); "┐";`, 4},
		{"acentos", `    PRINT "Información: año, día, crédito"`, 4},
		{"sin sangría", `PRINT "└────┘"`, 0},
		{"ASCII", `    PRINT "Box"`, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := editingTest(tc.line, "END SUB")
			// Comentar al inicio del código, Fn+derecha (End), Enter.
			arrow(e, tcell.KeyHome, 0)
			e.typeRune('\'')
			commented := e.v.Lines[0]
			arrow(e, tcell.KeyEnd, 0)
			arrow(e, tcell.KeyEnter, 0)
			want := []string{commented, strings.Repeat(" ", tc.indent), "END SUB"}
			if !reflect.DeepEqual(e.v.Lines, want) || e.v.Row != 1 || e.v.Col != tc.indent {
				t.Fatalf("Enter: %q, cursor %d:%d; esperado %q", e.v.Lines, e.v.Row, e.v.Col, want)
			}
			e.undo()
			if !reflect.DeepEqual(e.v.Lines, []string{commented, "END SUB"}) || e.v.Col != len([]rune(commented)) {
				t.Fatalf("undo Enter: %q, columna %d", e.v.Lines, e.v.Col)
			}
		})
	}
}

func TestEditorShiftSelection(t *testing.T) {
	e := editingTest("PRINT 1", "PRINT 2")
	arrow(e, tcell.KeyDown, tcell.ModShift)
	arrow(e, tcell.KeyRight, tcell.ModShift)
	if got := e.selectedText(); got != "PRINT 1\nP" {
		t.Fatalf("selección: %q", got)
	}
	e.typeRune('X')
	if !reflect.DeepEqual(e.v.Lines, []string{"XRINT 2"}) {
		t.Fatal(e.v.Lines)
	}
}

func TestEditorColumnEditing(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"delete", "", []string{"ade", "x", "ávw"}},
		{"replace", "Q", []string{"aQde", "xQ", "áQvw"}},
		{"multiline", "12\n34\n56", []string{"a12de", "x34", "á56vw"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := editingTest("abcde", "x", "áéuvw")
			e.moveTo(0, 1, false)
			mods := tcell.ModAlt | tcell.ModShift | tcell.ModMeta
			arrow(e, tcell.KeyRight, mods)
			arrow(e, tcell.KeyRight, mods)
			arrow(e, tcell.KeyDown, mods)
			arrow(e, tcell.KeyDown, mods)
			if got := e.selectedText(); got != "bc\n  \néu" {
				t.Fatalf("bloque: %q", got)
			}
			if tc.text == "" {
				e.deleteSelection()
			} else {
				e.insertText(tc.text)
			}
			if !reflect.DeepEqual(e.v.Lines, tc.want) {
				t.Fatalf("líneas: %q, esperado %q", e.v.Lines, tc.want)
			}
			e.undo()
			if !reflect.DeepEqual(e.v.Lines, []string{"abcde", "x", "áéuvw"}) {
				t.Fatalf("undo: %q", e.v.Lines)
			}
		})
	}
}

func TestEditorColumnClipboard(t *testing.T) {
	e := editingTest("abcde", "fghij", "klmno")
	// Selección hacia arriba y a la izquierda desde el menú de columnas.
	e.columnMode = true
	e.moveTo(1, 3, false)
	arrow(e, tcell.KeyLeft, tcell.ModShift)
	arrow(e, tcell.KeyLeft, tcell.ModShift)
	arrow(e, tcell.KeyUp, tcell.ModShift)
	e.cutSel()
	if *e.clipboard != "bc\ngh" || !*e.clipColumn {
		t.Fatalf("portapapeles: %q", *e.clipboard)
	}
	e.moveTo(2, 7, false)
	e.paste()
	if !reflect.DeepEqual(e.v.Lines, []string{"ade", "fij", "klmno  bc", "       gh"}) {
		t.Fatalf("pegado rectangular: %q", e.v.Lines)
	}
	e.undo()
	if !reflect.DeepEqual(e.v.Lines, []string{"ade", "fij", "klmno"}) {
		t.Fatalf("undo del pegado: %q", e.v.Lines)
	}
	e.undo()
	if !reflect.DeepEqual(e.v.Lines, []string{"abcde", "fghij", "klmno"}) {
		t.Fatalf("undo del corte: %q", e.v.Lines)
	}
}

func TestEditorColumnDraw(t *testing.T) {
	e := editingTest("abc", "", "def")
	e.columnMode = true
	e.moveTo(0, 1, false)
	arrow(e, tcell.KeyRight, tcell.ModShift)
	arrow(e, tcell.KeyDown, tcell.ModShift)
	arrow(e, tcell.KeyDown, tcell.ModShift)
	s := tcell.NewSimulationScreen("")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(5, 3)
	e.Draw(canvas{s}, rect{0, 0, 5, 3}, true, -1)
	for y := 0; y < 3; y++ {
		for x := 0; x < 5; x++ {
			_, _, st, _ := s.GetContent(x, y)
			if (st == stEditorSel) != (x == 1) {
				t.Fatalf("selección dibujada incorrecta en %d,%d", x, y)
			}
		}
	}
}

func TestIDEColumnSelectionMenu(t *testing.T) {
	a, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	a.doc.Views[0].Lines = []string{"abc", "def"}
	key := func(k tcell.Key, r rune, mods tcell.ModMask) {
		a.key(tcell.NewEventKey(k, r, mods))
	}
	// Alt+E, O: activa columnas sin requerir modificadores en las flechas.
	key(tcell.KeyRune, 'e', tcell.ModAlt)
	key(tcell.KeyRune, 'o', 0)
	if !a.ed.columnMode {
		t.Fatal("el menú no activó columnas")
	}
	key(tcell.KeyRight, 0, tcell.ModShift)
	key(tcell.KeyDown, 0, tcell.ModShift)
	key(tcell.KeyCtrlC, 0, 0)
	if a.clip != "a\nd" || !a.clipColumn {
		t.Fatalf("selección desde menú: %q", a.clip)
	}
	key(tcell.KeyEsc, 0, 0)
	key(tcell.KeyRune, 'e', tcell.ModAlt)
	key(tcell.KeyRune, 'o', 0)
	if a.ed.columnMode {
		t.Fatal("el menú no desactivó columnas")
	}
}

// TestEditorGoalColumn: al moverse en vertical el cursor vuelve a la
// columna en que empezó si la línea llega hasta ahí; si no, queda al final.
func TestEditorGoalColumn(t *testing.T) {
	e := editingTest(
		strings.Repeat("x", 60),
		"",
		"corta",
		strings.Repeat("y", 70),
		"",
	)
	e.v.Col = 53
	pos := func() [2]int { return [2]int{e.v.Row, e.v.Col} }
	steps := []struct {
		key  tcell.Key
		want [2]int
	}{
		{tcell.KeyDown, [2]int{1, 0}},
		{tcell.KeyDown, [2]int{2, 5}},
		{tcell.KeyDown, [2]int{3, 53}},
		{tcell.KeyDown, [2]int{4, 0}},
		{tcell.KeyUp, [2]int{3, 53}},
		{tcell.KeyUp, [2]int{2, 5}},
		{tcell.KeyUp, [2]int{1, 0}},
		{tcell.KeyUp, [2]int{0, 53}},
	}
	for i, s := range steps {
		arrow(e, s.key, 0)
		if pos() != s.want {
			t.Fatalf("paso %d: posición %v, se esperaba %v", i, pos(), s.want)
		}
	}
	// un movimiento horizontal fija una nueva columna
	arrow(e, tcell.KeyDown, 0)
	arrow(e, tcell.KeyDown, 0) // "corta", col 5
	arrow(e, tcell.KeyLeft, 0) // col 4
	arrow(e, tcell.KeyDown, 0)
	if pos() != [2]int{3, 4} {
		t.Fatalf("después de mover a la izquierda: %v", pos())
	}
	// con Shift también (selección)
	e.v.Row, e.v.Col, e.hasGoal = 3, 65, false
	arrow(e, tcell.KeyUp, tcell.ModShift)
	if pos() != [2]int{2, 5} || !e.sel {
		t.Fatalf("Shift+↑: %v sel=%v", pos(), e.sel)
	}
}
