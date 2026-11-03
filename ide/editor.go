package ide

import (
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

const maxUndo = 200

// Editor edita una View. La columna del cursor puede pasar el fin de la
// línea (como en QB); al escribir allí se completa con espacios.
type Editor struct {
	v         *View
	overwrite bool
	sel       bool
	selRow    int
	selCol    int
	lineDirty bool
	lastEdit  string
	area      rect

	// OnLineExit se invoca al dejar una línea modificada (formato, chequeo
	// de sintaxis, creación de SUB). Puede modificar las líneas.
	OnLineExit func(row int)
	// OnChange se invoca con cada modificación del texto.
	OnChange  func()
	clipboard *string
	// Colorize, si no es nil, devuelve un estilo por runa de una línea.
	Colorize func(line string) []tcell.Style
}

func (e *Editor) setView(v *View) {
	e.v = v
	e.sel, e.lineDirty, e.lastEdit = false, false, ""
	e.clamp()
}

func (e *Editor) lines() []string { return e.v.Lines }

func (e *Editor) line(r int) []rune { return []rune(e.v.Lines[r]) }

func (e *Editor) setLine(r int, s []rune) {
	e.v.Lines[r] = strings.TrimRight(string(s), " ")
	if r == e.v.Row {
		e.lineDirty = true
	}
	e.changed()
}

func (e *Editor) changed() {
	if e.OnChange != nil {
		e.OnChange()
	}
}

func (e *Editor) clamp() {
	v := e.v
	if v.Row >= len(v.Lines) {
		v.Row = len(v.Lines) - 1
	}
	if v.Row < 0 {
		v.Row = 0
	}
	if v.Col < 0 {
		v.Col = 0
	}
	if v.Col > 255 {
		v.Col = 255
	}
}

// ensureVisible desplaza la vista para mostrar el cursor.
func (e *Editor) ensureVisible() {
	v, a := e.v, e.area
	if a.h <= 0 || a.w <= 0 {
		return
	}
	if v.Row < v.Top {
		v.Top = v.Row
	}
	if v.Row >= v.Top+a.h {
		v.Top = v.Row - a.h + 1
	}
	if v.Col < v.Left {
		v.Left = v.Col
	}
	if v.Col >= v.Left+a.w {
		v.Left = v.Col - a.w + 1
	}
}

// moveTo mueve el cursor y dispara OnLineExit si se cambia de línea.
func (e *Editor) moveTo(row, col int, extend bool) {
	v := e.v
	if extend && !e.sel {
		e.sel, e.selRow, e.selCol = true, v.Row, v.Col
	} else if !extend {
		e.sel = false
	}
	if row != v.Row {
		e.leaveLine()
	}
	if row < 0 {
		row = 0
	}
	if row >= len(v.Lines) {
		row = len(v.Lines) - 1
	}
	v.Row, v.Col = row, col
	e.clamp()
	e.lastEdit = ""
	e.ensureVisible()
}

// leaveLine procesa la línea actual si fue modificada.
func (e *Editor) leaveLine() {
	if e.lineDirty {
		e.lineDirty = false
		if e.OnLineExit != nil {
			e.OnLineExit(e.v.Row)
		}
	}
}

// pushUndo guarda el estado antes de una edición (agrupa ediciones del mismo tipo).
func (e *Editor) pushUndo(kind string) {
	if kind != "" && kind == e.lastEdit {
		return
	}
	e.lastEdit = kind
	v := e.v
	s := snapshot{lines: append([]string(nil), v.Lines...), row: v.Row, col: v.Col}
	v.undo = append(v.undo, s)
	if len(v.undo) > maxUndo {
		v.undo = v.undo[1:]
	}
}

func (e *Editor) undo() {
	v := e.v
	if len(v.undo) == 0 {
		return
	}
	s := v.undo[len(v.undo)-1]
	v.undo = v.undo[:len(v.undo)-1]
	v.Lines, v.Row, v.Col = s.lines, s.row, s.col
	e.sel, e.lastEdit, e.lineDirty = false, "", false
	e.changed()
	e.ensureVisible()
}

// ------------------------------------------------------------- selección

// selection devuelve la selección normalizada (inicio <= fin).
func (e *Editor) selection() (r1, c1, r2, c2 int, ok bool) {
	if !e.sel {
		return 0, 0, 0, 0, false
	}
	r1, c1, r2, c2 = e.selRow, e.selCol, e.v.Row, e.v.Col
	if r1 > r2 || r1 == r2 && c1 > c2 {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 == r2 && c1 == c2 {
		return 0, 0, 0, 0, false
	}
	return r1, c1, r2, c2, true
}

func sliceRunes(r []rune, from, to int) []rune {
	if from > len(r) {
		from = len(r)
	}
	if to > len(r) {
		to = len(r)
	}
	if from > to {
		from = to
	}
	return r[from:to]
}

func (e *Editor) selectedText() string {
	r1, c1, r2, c2, ok := e.selection()
	if !ok {
		return ""
	}
	if r1 == r2 {
		return string(sliceRunes(e.line(r1), c1, c2))
	}
	var b strings.Builder
	b.WriteString(string(sliceRunes(e.line(r1), c1, 1<<30)))
	for r := r1 + 1; r < r2; r++ {
		b.WriteByte('\n')
		b.WriteString(e.v.Lines[r])
	}
	b.WriteByte('\n')
	b.WriteString(string(sliceRunes(e.line(r2), 0, c2)))
	return b.String()
}

// deleteSelection borra la selección; devuelve false si no había.
func (e *Editor) deleteSelection() bool {
	r1, c1, r2, c2, ok := e.selection()
	if !ok {
		e.sel = false
		return false
	}
	e.pushUndo("del")
	head := sliceRunes(e.line(r1), 0, c1)
	if len(head) < c1 {
		head = append(head, []rune(strings.Repeat(" ", c1-len(head)))...)
	}
	tail := sliceRunes(e.line(r2), c2, 1<<30)
	v := e.v
	v.Lines = append(v.Lines[:r1+1], v.Lines[r2+1:]...)
	v.Row, v.Col = r1, c1
	e.setLine(r1, append(append([]rune{}, head...), tail...))
	e.sel = false
	e.ensureVisible()
	return true
}

// insertText inserta texto (posiblemente multilínea) en el cursor.
func (e *Editor) insertText(s string) {
	if s == "" {
		return
	}
	e.deleteSelection()
	e.pushUndo("paste")
	v := e.v
	parts := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	cur := e.line(v.Row)
	if len(cur) < v.Col {
		cur = append(cur, []rune(strings.Repeat(" ", v.Col-len(cur)))...)
	}
	head, tail := append([]rune{}, cur[:v.Col]...), append([]rune{}, cur[v.Col:]...)
	if len(parts) == 1 {
		e.setLine(v.Row, append(append(head, []rune(parts[0])...), tail...))
		v.Col += len([]rune(parts[0]))
		e.ensureVisible()
		return
	}
	e.leaveLine()
	newLines := make([]string, 0, len(parts))
	newLines = append(newLines, string(append(head, []rune(parts[0])...)))
	newLines = append(newLines, parts[1:len(parts)-1]...)
	last := []rune(parts[len(parts)-1])
	newLines = append(newLines, string(append(append([]rune{}, last...), tail...)))
	rest := append([]string(nil), v.Lines[v.Row+1:]...)
	v.Lines = append(append(v.Lines[:v.Row], newLines...), rest...)
	v.Row += len(parts) - 1
	v.Col = len(last)
	e.lineDirty = false
	e.changed()
	e.ensureVisible()
}

// ------------------------------------------------------------- edición

func (e *Editor) typeRune(r rune) {
	e.deleteSelection()
	e.pushUndo("type")
	v := e.v
	cur := e.line(v.Row)
	if len(cur) < v.Col {
		cur = append(cur, []rune(strings.Repeat(" ", v.Col-len(cur)))...)
	}
	if e.overwrite && v.Col < len(cur) {
		cur[v.Col] = r
	} else {
		cur = append(cur[:v.Col], append([]rune{r}, cur[v.Col:]...)...)
	}
	v.Col++
	e.setLine(v.Row, cur)
	e.ensureVisible()
}

func (e *Editor) enter() {
	e.deleteSelection()
	e.pushUndo("enter")
	v := e.v
	cur := e.line(v.Row)
	col := min(v.Col, len(cur))
	head, tail := cur[:col], cur[col:]
	indent := len(cur) - len(strings.TrimLeft(string(cur), " "))
	if strings.TrimSpace(string(cur)) == "" {
		indent = v.Col
	}
	if indent > col {
		indent = col
	}
	v.Lines[v.Row] = strings.TrimRight(string(head), " ")
	newLine := strings.Repeat(" ", indent) + strings.TrimLeft(string(tail), " ")
	rest := append([]string(nil), v.Lines[v.Row+1:]...)
	v.Lines = append(append(v.Lines[:v.Row+1], newLine), rest...)
	e.lineDirty = true
	e.leaveLine()
	v.Row++
	v.Col = indent
	e.changed()
	e.ensureVisible()
}

func (e *Editor) backspace() {
	if e.deleteSelection() {
		return
	}
	e.pushUndo("bs")
	v := e.v
	cur := e.line(v.Row)
	switch {
	case v.Col > len(cur):
		v.Col--
	case v.Col > 0:
		e.setLine(v.Row, append(cur[:v.Col-1], cur[v.Col:]...))
		v.Col--
	case v.Row > 0:
		prev := e.line(v.Row - 1)
		v.Lines = append(v.Lines[:v.Row], v.Lines[v.Row+1:]...)
		v.Row--
		v.Col = len(prev)
		e.setLine(v.Row, append(prev, cur...))
	}
	e.ensureVisible()
}

func (e *Editor) delete() {
	if e.deleteSelection() {
		return
	}
	e.pushUndo("del")
	v := e.v
	cur := e.line(v.Row)
	if v.Col < len(cur) {
		e.setLine(v.Row, append(cur[:v.Col], cur[v.Col+1:]...))
		return
	}
	if v.Row+1 < len(v.Lines) {
		if len(cur) < v.Col {
			cur = append(cur, []rune(strings.Repeat(" ", v.Col-len(cur)))...)
		}
		next := e.line(v.Row + 1)
		v.Lines = append(v.Lines[:v.Row+1], v.Lines[v.Row+2:]...)
		e.setLine(v.Row, append(cur, next...))
	}
}

// deleteLine es Ctrl+Y: corta la línea al portapapeles.
func (e *Editor) deleteLine() {
	e.pushUndo("")
	v := e.v
	*e.clipboard = v.Lines[v.Row] + "\n"
	if len(v.Lines) == 1 {
		v.Lines[0] = ""
	} else {
		v.Lines = append(v.Lines[:v.Row], v.Lines[v.Row+1:]...)
	}
	e.lineDirty = false
	e.sel = false
	e.clamp()
	e.changed()
	e.ensureVisible()
}

func (e *Editor) copySel() {
	if t := e.selectedText(); t != "" {
		*e.clipboard = t
	}
}

func (e *Editor) cutSel() {
	if t := e.selectedText(); t != "" {
		*e.clipboard = t
		e.deleteSelection()
	}
}

func (e *Editor) paste() { e.insertText(*e.clipboard) }

func isWord(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '$' || r == '_' || r == '.'
}

func (e *Editor) wordLeft() (int, int) {
	v := e.v
	r, c := v.Row, min(v.Col, len(e.line(v.Row)))
	if c == 0 {
		if r == 0 {
			return 0, 0
		}
		return r - 1, len(e.line(r - 1))
	}
	l := e.line(r)
	for c > 0 && !isWord(l[c-1]) {
		c--
	}
	for c > 0 && isWord(l[c-1]) {
		c--
	}
	return r, c
}

func (e *Editor) wordRight() (int, int) {
	v := e.v
	l := e.line(v.Row)
	c := v.Col
	if c >= len(l) {
		if v.Row+1 < len(v.Lines) {
			return v.Row + 1, 0
		}
		return v.Row, c
	}
	for c < len(l) && isWord(l[c]) {
		c++
	}
	for c < len(l) && !isWord(l[c]) {
		c++
	}
	return v.Row, c
}

// HandleKey procesa una tecla; devuelve false si no la usó.
func (e *Editor) HandleKey(ev *tcell.EventKey) bool {
	v := e.v
	shift := ev.Modifiers()&tcell.ModShift != 0
	ctrl := ev.Modifiers()&tcell.ModCtrl != 0
	switch ev.Key() {
	case tcell.KeyUp:
		if ctrl { // Ctrl+↑: desplaza sin mover el cursor
			e.Scroll(-1)
			break
		}
		e.moveTo(v.Row-1, v.Col, shift)
	case tcell.KeyDown:
		if ctrl {
			e.Scroll(1)
			break
		}
		e.moveTo(v.Row+1, v.Col, shift)
	case tcell.KeyLeft:
		if ctrl {
			r, c := e.wordLeft()
			e.moveTo(r, c, shift)
		} else if v.Col > 0 {
			e.moveTo(v.Row, v.Col-1, shift)
		}
	case tcell.KeyRight:
		if ctrl {
			r, c := e.wordRight()
			e.moveTo(r, c, shift)
		} else {
			e.moveTo(v.Row, v.Col+1, shift)
		}
	case tcell.KeyHome:
		if ctrl {
			e.moveTo(0, 0, shift)
			break
		}
		l := e.line(v.Row)
		ind := len(l) - len([]rune(strings.TrimLeft(string(l), " ")))
		if v.Col == ind {
			ind = 0
		}
		e.moveTo(v.Row, ind, shift)
	case tcell.KeyEnd:
		if ctrl {
			last := len(v.Lines) - 1
			e.moveTo(last, len(e.line(last)), shift)
			break
		}
		e.moveTo(v.Row, len(e.line(v.Row)), shift)
	case tcell.KeyPgUp:
		v.Top -= e.area.h
		if v.Top < 0 {
			v.Top = 0
		}
		e.moveTo(v.Row-e.area.h, v.Col, shift)
	case tcell.KeyPgDn:
		v.Top += e.area.h
		if v.Top > len(v.Lines)-1 {
			v.Top = max(0, len(v.Lines)-1)
		}
		e.moveTo(v.Row+e.area.h, v.Col, shift)
	case tcell.KeyEnter:
		e.enter()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		e.backspace()
	case tcell.KeyDelete:
		if shift {
			e.cutSel()
		} else {
			e.delete()
		}
	case tcell.KeyInsert:
		switch {
		case shift:
			e.paste()
		case ctrl:
			e.copySel()
		default:
			e.overwrite = !e.overwrite
		}
	case tcell.KeyTab:
		n := 8 - v.Col%8
		for i := 0; i < n; i++ {
			e.typeRune(' ')
		}
	case tcell.KeyCtrlY:
		e.deleteLine()
	case tcell.KeyCtrlX:
		e.cutSel()
	case tcell.KeyCtrlC:
		e.copySel()
	case tcell.KeyCtrlV:
		e.paste()
	case tcell.KeyCtrlZ:
		e.undo()
	case tcell.KeyRune:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			return false
		}
		e.typeRune(ev.Rune())
	default:
		return false
	}
	return true
}

// Click ubica el cursor en la posición de la pantalla (x, y).
func (e *Editor) Click(x, y int, extend bool) {
	if !e.area.contains(x, y) {
		return
	}
	row := e.v.Top + y - e.area.y
	col := e.v.Left + x - e.area.x
	if row >= len(e.v.Lines) {
		row = len(e.v.Lines) - 1
	}
	e.moveTo(row, col, extend)
}

// Scroll desplaza la vista n líneas.
func (e *Editor) Scroll(n int) {
	v := e.v
	v.Top += n
	if v.Top > len(v.Lines)-1 {
		v.Top = len(v.Lines) - 1
	}
	if v.Top < 0 {
		v.Top = 0
	}
}

// Draw dibuja el contenido en area; curStmt resalta una línea (-1 ninguna).
func (e *Editor) Draw(c canvas, area rect, focused bool, curStmt int) (int, int) {
	e.area = area
	v := e.v
	r1, c1, r2, c2, hasSel := e.selection()
	for y := 0; y < area.h; y++ {
		row := v.Top + y
		base := stEditor
		if row == curStmt {
			base = stCurStmt.Reverse(true)
		}
		var l []rune
		var styles []tcell.Style
		if row < len(v.Lines) {
			l = []rune(v.Lines[row])
			if e.Colorize != nil && row != curStmt {
				styles = e.Colorize(v.Lines[row])
			}
		}
		for x := 0; x < area.w; x++ {
			col := v.Left + x
			ch := ' '
			if col < len(l) {
				ch = l[col]
			}
			st := base
			if col < len(styles) {
				st = styles[col]
			}
			if hasSel && row >= r1 && row <= r2 {
				in := true
				if row == r1 && col < c1 || row == r2 && col >= c2 {
					in = false
				}
				if in {
					st = stEditorSel
				}
			}
			c.put(area.x+x, area.y+y, ch, st)
		}
	}
	if !focused {
		return -1, -1
	}
	return area.x + v.Col - v.Left, area.y + v.Row - v.Top
}
