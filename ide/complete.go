package ide

import (
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"

	"goqb/lang/format"
	"goqb/lang/lexer"
	"goqb/lang/token"
	"goqb/runtime/termrun"
	"goqb/vm"
)

// Autocompletado al estilo IntelliSense: mientras se escribe una palabra se
// sugieren palabras clave, funciones intrínsecas, SUB/FUNCTION del programa,
// constantes y variables. Tab acepta; Enter acepta solo si se eligió una
// opción con las flechas (si no, termina la línea como siempre); Esc cierra.
// Ctrl+Espacio abre la lista en cualquier momento. Además, la barra de estado
// muestra la sintaxis de la sentencia o función que se está escribiendo.

const (
	acMinChars = 2 // letras escritas antes de abrir la lista sola
	acMaxRows  = 8
)

var (
	stACItem   = termrun.Style(0, 7)
	stACSel    = termrun.Style(15, 0)
	stACKind   = termrun.Style(8, 7)
	stACKindS  = termrun.Style(7, 0)
	stACDetail = termrun.Style(0, 3)
)

type acItem struct {
	label string
	kind  string // palabra clave, función, SUB, FUNCTION, CONST, variable
}

type autocomplete struct {
	open    bool
	chosen  bool // el usuario movió la selección
	items   []acItem
	sel     int
	top     int
	row     int // fila y columna donde empieza la palabra
	start   int
	symRev  int // versión del documento de los símbolos en caché
	symbols []acItem
	hint    string // sintaxis de lo que se está escribiendo (barra de estado)
}

// staticItems: palabras clave, intrínsecas y temas de ayuda.
var staticItems = func() []acItem {
	seen := map[string]bool{}
	var out []acItem
	add := func(name, kind string) {
		if name == "" || seen[name] || strings.HasPrefix(name, "_") {
			return
		}
		seen[name] = true
		out = append(out, acItem{name, kind})
	}
	for _, b := range vm.Builtins {
		add(b.Name, "función")
	}
	for _, k := range token.Keywords() {
		add(k, "palabra clave")
	}
	for k := range helpTopics {
		add(k, "palabra clave")
	}
	return out
}()

// docSymbols recorre el documento y junta procedimientos, constantes y variables.
func (a *App) docSymbols() []acItem {
	ac := &a.ac
	if ac.symbols != nil && ac.symRev == a.doc.rev {
		return ac.symbols
	}
	seen := map[string]bool{}
	var out []acItem
	add := func(name, kind string) {
		u := strings.ToUpper(name)
		if seen[u] {
			return
		}
		seen[u] = true
		out = append(out, acItem{name, kind})
	}
	for _, v := range a.doc.Views {
		if v.Name != "" {
			add(v.Name, v.Kind)
		}
	}
	for _, v := range a.doc.Views {
		for _, l := range v.Lines {
			toks := lexer.Tokenize([]byte(l))
			inDef := format.DefTypeTokens(toks)
			for i, t := range toks {
				switch {
				case inDef[i]: // letras de DEFINT A-Z
				case t.Kind == token.Keyword && t.Text == "CONST" && i+1 < len(toks) && toks[i+1].Kind == token.Ident:
					add(toks[i+1].Text, "CONST")
				case i == 0 && t.Kind == token.Keyword && (t.Text == "SUB" || t.Text == "FUNCTION") && len(toks) > 1 && toks[1].Kind == token.Ident:
					add(toks[1].Text, t.Text)
				case t.Kind == token.Keyword && t.Text == "DECLARE" && i+2 < len(toks) && toks[i+2].Kind == token.Ident:
					add(toks[i+2].Text, toks[i+1].Text)
				case t.Kind == token.Ident:
					if _, builtin := vm.BuiltinIndex[strings.ToUpper(t.Text)]; !builtin && !token.IsKeyword(strings.ToUpper(t.Text)) {
						add(t.Text, "variable")
					}
				}
			}
		}
	}
	ac.symbols, ac.symRev = out, a.doc.rev
	return out
}

func isIdentRune(r rune) bool {
	return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '.'
}

// wordBeforeCursor devuelve el comienzo y el texto de la palabra que termina en el cursor.
func wordBeforeCursor(l []rune, col int) (int, string) {
	col = min(col, len(l))
	s := col
	if s > 0 && strings.ContainsRune("$%&!#", l[s-1]) {
		s--
	}
	for s > 0 && isIdentRune(l[s-1]) {
		s--
	}
	if s < col && !(l[s] >= 'A' && l[s] <= 'Z' || l[s] >= 'a' && l[s] <= 'z') {
		return col, ""
	}
	return s, string(l[s:col])
}

// inCode reporta si la columna está fuera de strings y comentarios.
func inCode(l []rune, col int) bool {
	quote := false
	for i := 0; i < min(col, len(l)); i++ {
		switch {
		case l[i] == '"':
			quote = !quote
		case !quote && l[i] == '\'':
			return false
		case !quote && (i == 0 || !isIdentRune(l[i-1])) && i+3 <= len(l) && strings.EqualFold(string(l[i:i+3]), "REM") &&
			(i+3 == len(l) || !isIdentRune(l[i+3])):
			return false
		}
	}
	return !quote
}

// refreshAutocomplete recalcula la lista para la palabra bajo el cursor.
// force: abrirla aunque la palabra sea corta (Ctrl+Espacio).
func (a *App) refreshAutocomplete(force bool) {
	ac := &a.ac
	v := a.doc.Views[a.cur]
	if v.Row >= len(v.Lines) {
		ac.open = false
		return
	}
	l := []rune(v.Lines[v.Row])
	start, word := wordBeforeCursor(l, v.Col)
	if !inCode(l, v.Col) || !force && len([]rune(word)) < acMinChars && !ac.open || !force && word == "" {
		ac.open = false
		return
	}
	up := strings.ToUpper(word)
	var pre, sub []acItem
	consider := func(list []acItem) {
		for _, it := range list {
			u := strings.ToUpper(it.label)
			if u == up && it.kind == "variable" { // la propia palabra que se está escribiendo
				continue
			}
			switch {
			case up == "" || strings.HasPrefix(u, up):
				pre = append(pre, it)
			case len(up) >= 2 && strings.Contains(u, up):
				sub = append(sub, it)
			}
		}
	}
	consider(a.docSymbols())
	consider(staticItems)
	// orden: al comienzo de una sentencia van primero las sentencias y los
	// SUB; dentro de una expresión, variables, constantes y funciones
	stmt := atStatementStart(l, start)
	rank := func(it acItem) int {
		switch it.kind {
		case "palabra clave":
			if _, documented := helpTopics[strings.ToUpper(it.label)]; stmt && documented {
				return 0 // sentencias (las que tienen ayuda) antes que RANDOM, ACCESS...
			}
			if stmt {
				return 2
			}
			return 3
		case "SUB":
			if stmt {
				return 1
			}
			return 4
		case "variable", "CONST":
			if stmt {
				return 2
			}
			return 0
		}
		if stmt { // funciones
			return 3
		}
		return 1
	}
	byLabel := func(s []acItem) {
		sort.SliceStable(s, func(i, j int) bool {
			if ri, rj := rank(s[i]), rank(s[j]); ri != rj {
				return ri < rj
			}
			return strings.ToUpper(s[i].label) < strings.ToUpper(s[j].label)
		})
	}
	byLabel(pre)
	byLabel(sub)
	items := dedupe(append(pre, sub...))
	// no mostrar si lo único que queda es exactamente lo escrito
	if len(items) == 0 || len(items) == 1 && strings.EqualFold(items[0].label, word) {
		ac.open = false
		return
	}
	prevSel := ""
	if ac.open && ac.chosen && ac.sel < len(ac.items) {
		prevSel = ac.items[ac.sel].label
	}
	ac.items, ac.row, ac.start, ac.open = items, v.Row, start, true
	ac.sel, ac.top = 0, 0
	if prevSel != "" {
		for i, it := range items {
			if it.label == prevSel {
				ac.sel = i
			}
		}
	} else {
		ac.chosen = false
	}
	ac.scroll()
}

func dedupe(items []acItem) []acItem {
	seen := map[string]bool{}
	out := items[:0]
	for _, it := range items {
		u := strings.ToUpper(it.label)
		if !seen[u] {
			seen[u] = true
			out = append(out, it)
		}
	}
	return out
}

func (ac *autocomplete) scroll() {
	if ac.sel < ac.top {
		ac.top = ac.sel
	}
	if ac.sel >= ac.top+acMaxRows {
		ac.top = ac.sel - acMaxRows + 1
	}
}

// accept reemplaza la palabra por la opción elegida.
func (a *App) acceptAutocomplete() {
	ac := &a.ac
	if !ac.open || ac.sel >= len(ac.items) {
		return
	}
	it := ac.items[ac.sel]
	ac.open = false
	v := a.doc.Views[a.cur]
	if v.Row != ac.row {
		return
	}
	l := []rune(v.Lines[v.Row])
	end := min(v.Col, len(l))
	if ac.start > end {
		return
	}
	a.ed.pushUndo("complete")
	nl := append(append(append([]rune{}, l[:ac.start]...), []rune(it.label)...), l[end:]...)
	v.Col = ac.start + len([]rune(it.label))
	a.ed.setLine(v.Row, nl)
	a.ed.ensureVisible()
	a.updateHint()
}

// autocompleteKey atiende las teclas mientras la lista está abierta.
// Devuelve true si consumió la tecla.
func (a *App) autocompleteKey(e *tcell.EventKey) bool {
	ac := &a.ac
	if e.Key() == tcell.KeyCtrlSpace || e.Key() == tcell.KeyRune && e.Rune() == ' ' && e.Modifiers()&tcell.ModCtrl != 0 {
		a.refreshAutocomplete(true)
		return true
	}
	if !ac.open {
		return false
	}
	n := len(ac.items)
	switch e.Key() {
	case tcell.KeyUp:
		ac.sel = (ac.sel - 1 + n) % n
		ac.chosen = true
	case tcell.KeyDown:
		ac.sel = (ac.sel + 1) % n
		ac.chosen = true
	case tcell.KeyPgUp:
		ac.sel = max(0, ac.sel-acMaxRows)
		ac.chosen = true
	case tcell.KeyPgDn:
		ac.sel = min(n-1, ac.sel+acMaxRows)
		ac.chosen = true
	case tcell.KeyTab:
		a.acceptAutocomplete()
	case tcell.KeyEnter:
		if !ac.chosen {
			ac.open = false
			return false // Enter normal: termina la línea
		}
		a.acceptAutocomplete()
	case tcell.KeyEsc:
		ac.open = false
	default:
		return false
	}
	ac.scroll()
	return true
}

// afterEditorKey actualiza la lista y la ayuda de sintaxis después de una tecla del editor.
func (a *App) afterEditorKey(e *tcell.EventKey) {
	ac := &a.ac
	switch e.Key() {
	case tcell.KeyRune:
		r := e.Rune()
		if isIdentRune(r) || strings.ContainsRune("$%&!#", r) {
			a.refreshAutocomplete(false)
		} else {
			ac.open = false
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if ac.open {
			a.refreshAutocomplete(false)
		}
	default:
		ac.open = false
	}
	a.updateHint()
}

// updateHint busca la función con paréntesis abierto más cercana al cursor o,
// si no hay, la primera palabra de la sentencia, y deja su sintaxis en hint.
func (a *App) updateHint() {
	ac := &a.ac
	ac.hint = ""
	v := a.doc.Views[a.cur]
	if v.Row >= len(v.Lines) {
		return
	}
	l := []rune(v.Lines[v.Row])
	col := min(v.Col, len(l))
	if !inCode(l, col) {
		return
	}
	// función con el paréntesis todavía abierto
	depth := 0
	for i := col - 1; i >= 0; i-- {
		switch l[i] {
		case ')':
			depth++
		case '(':
			if depth > 0 {
				depth--
				continue
			}
			if _, w := wordBeforeCursor(l, i); w != "" {
				if t, ok := helpTopics[strings.ToUpper(w)]; ok {
					ac.hint = firstLine(t.syntax)
					return
				}
			}
		case ':':
			if depth == 0 {
				i = -1
			}
		}
	}
	// primera palabra de la sentencia (después del último ':')
	st := 0
	quote := false
	for i := 0; i < col; i++ {
		if l[i] == '"' {
			quote = !quote
		} else if l[i] == ':' && !quote {
			st = i + 1
		}
	}
	for st < col && l[st] == ' ' {
		st++
	}
	e := st
	for e < len(l) && (isIdentRune(l[e]) || l[e] == '$') {
		e++
	}
	if e > st {
		if t, ok := helpTopics[strings.ToUpper(string(l[st:e]))]; ok {
			ac.hint = firstLine(t.syntax)
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// drawAutocomplete dibuja la lista junto al cursor (cx, cy) y, al costado,
// la sintaxis de la opción elegida.
func (a *App) drawAutocomplete(cx, cy int) {
	ac := &a.ac
	if !ac.open || cx < 0 || len(ac.items) == 0 {
		return
	}
	c, l := a.c, a.lay
	labelW, kindW := 0, 0
	for _, it := range ac.items {
		labelW = max(labelW, len([]rune(it.label)))
		kindW = max(kindW, len([]rune(it.kind)))
	}
	w := min(labelW+kindW+3, l.w-2)
	h := min(len(ac.items), acMaxRows)
	x := cx - (a.doc.Views[a.cur].Col - ac.start) - 1
	x = max(0, min(x, l.w-w-1))
	y := cy + 1
	if y+h > l.h-1 { // no entra abajo: arriba de la línea
		y = cy - h
	}
	if y < 1 {
		return
	}
	for i := 0; i < h; i++ {
		idx := ac.top + i
		it := ac.items[idx]
		st, kst := stACItem, stACKind
		if idx == ac.sel {
			st, kst = stACSel, stACKindS
		}
		c.hline(x, y+i, w, ' ', st)
		c.text(x+1, y+i, it.label, st, labelW)
		c.text(x+w-kindW-1, y+i, it.kind, kst, kindW)
	}
	if len(ac.items) > h { // indicador de desplazamiento
		c.put(x+w-1, y, '↑', stACKind)
		c.put(x+w-1, y+h-1, '↓', stACKind)
	}
	// detalle de la opción elegida
	t, ok := helpTopics[strings.ToUpper(ac.items[ac.sel].label)]
	if !ok {
		return
	}
	var lines []string
	for _, sl := range strings.Split(t.syntax, "\n") {
		lines = append(lines, wrap(sl, 44)...)
	}
	lines = append(lines, wrap(t.desc, 44)...)
	dw := 0
	for _, s := range lines {
		dw = max(dw, len([]rune(s)))
	}
	dw = min(dw+2, 48)
	dx := x + w + 1
	if dx+dw > l.w {
		dx = x - dw - 1
	}
	if dx < 0 {
		return
	}
	for i, s := range lines {
		if i >= max(h, 6) || y+i >= l.h-1 {
			break
		}
		c.hline(dx, y+i, dw, ' ', stACDetail)
		c.text(dx+1, y+i, s, stACDetail, dw-2)
	}
}

// atStatementStart reporta si la columna col es el comienzo de una sentencia
// (principio de línea, después de ':' o después de THEN/ELSE).
func atStatementStart(l []rune, col int) bool {
	i := col
	for i > 0 && l[i-1] == ' ' {
		i--
	}
	if i == 0 || l[i-1] == ':' {
		return true
	}
	e := i
	for i > 0 && isIdentRune(l[i-1]) {
		i--
	}
	w := strings.ToUpper(string(l[i:e]))
	return w == "THEN" || w == "ELSE"
}
