package parser

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"goqb/lang/ast"
)

// sx convierte una expresión a notación S para comparar precedencias.
func sx(e ast.Expr) string {
	switch x := e.(type) {
	case nil:
		return "_"
	case *ast.NumberLit:
		if x.IsFloat {
			return fmt.Sprint(x.Float)
		}
		return fmt.Sprint(x.Int)
	case *ast.StringLit:
		return fmt.Sprintf("%q", x.Value)
	case *ast.Ident:
		return x.Name
	case *ast.Call:
		parts := []string{x.Name}
		for _, a := range x.Args {
			parts = append(parts, sx(a))
		}
		if x.Empty {
			parts = append(parts, "()")
		}
		return "[" + strings.Join(parts, " ") + "]"
	case *ast.FieldAccess:
		return sx(x.X) + "." + x.Field
	case *ast.Unary:
		return "(" + x.Op + " " + sx(x.X) + ")"
	case *ast.Binary:
		return "(" + x.Op + " " + sx(x.L) + " " + sx(x.R) + ")"
	case *ast.Paren:
		return sx(x.X)
	case *ast.FileNum:
		return "#" + sx(x.X)
	}
	return fmt.Sprintf("<%T>", e)
}

func TestPrecedence(t *testing.T) {
	cases := []struct{ src, want string }{
		{"-2 ^ 2", "(- (^ 2 2))"},
		{"2 ^ 3 ^ 2", "(^ (^ 2 3) 2)"},
		{"2 ^ -1", "(^ 2 (- 1))"},
		{"1 + 2 * 3 MOD 4 \\ 5", "(+ 1 (MOD (* 2 3) (\\ 4 5)))"},
		{"a OR b AND NOT c = d", "(OR a (AND b (NOT (= c d))))"},
		{"a IMP b EQV c XOR d", "(IMP a (EQV b (XOR c d)))"},
		{"a$ + b$ = c$", "(= (+ a$ b$) c$)"},
		{"-a * b", "(* (- a) b)"},
		{"x(1, 2).campo + f!(a)", "(+ [x 1 2].campo [f! a])"},
		{"TIMER - t#", "(- [TIMER] t#)"},
		{"LEN(a$) + SCREEN(1, 2)", "(+ [LEN a$] [SCREEN 1 2])"},
		{"&HFF + 1.5D2", "(+ 255 150)"},
		{"x = NOT y", "(= x (NOT y))"},
		{"(1 + 2) * 3", "(* (+ 1 2) 3)"},
		{"lista(i).NAME", "[lista i].NAME"},
	}
	for _, c := range cases {
		e, errs := ParseExpr(c.src)
		if len(errs) > 0 {
			t.Errorf("%q: errores %v", c.src, errs)
			continue
		}
		if got := sx(e); got != c.want {
			t.Errorf("%q\n got: %s\nwant: %s", c.src, got, c.want)
		}
	}
}

func parseOK(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	f, errs := ParseFile("t.bas", []byte(src))
	if len(errs) > 0 {
		t.Fatalf("errores inesperados en %q: %v", src, errs)
	}
	var out []ast.Stmt
	for _, s := range f.Stmts {
		if _, ok := s.(*ast.Comment); !ok {
			out = append(out, s)
		}
	}
	return out
}

func one[T ast.Stmt](t *testing.T, src string) T {
	t.Helper()
	st := parseOK(t, src)
	if len(st) != 1 {
		t.Fatalf("%q: se esperaba 1 sentencia, hay %d:\n%s", src, len(st), ast.Dump(st))
	}
	s, ok := st[0].(T)
	if !ok {
		t.Fatalf("%q: tipo %T", src, st[0])
	}
	return s
}

func TestAssignAndCall(t *testing.T) {
	a := one[*ast.Assign](t, "a(1, 2) = 3")
	if sx(a.Target) != "[a 1 2]" {
		t.Errorf("destino: %s", sx(a.Target))
	}
	one[*ast.Assign](t, "LET x.y = 1")
	c := one[*ast.CallStmt](t, "Dibujar x, (y), 3")
	if len(c.Args) != 3 || c.Explicit {
		t.Errorf("call implícito: %s", ast.Dump(c))
	}
	c = one[*ast.CallStmt](t, "CALL Ordenar(lista(), 10)")
	if sx(c.Args[0]) != "[lista ()]" {
		t.Errorf("array completo: %s", sx(c.Args[0]))
	}
	c = one[*ast.CallStmt](t, "CALL INTERRUPT(&H33, regs, regs)")
	if c.Name != "INTERRUPT" || len(c.Args) != 3 {
		t.Errorf("CALL INTERRUPT: %s", ast.Dump(c))
	}
	m := one[*ast.MidAssign](t, `MID$(s$, 2) = "x"`)
	if m.Length != nil {
		t.Errorf("MID$ sin largo")
	}
}

func TestLabels(t *testing.T) {
	st := parseOK(t, "10 PRINT\ninicio: CLS\n20\nGOTO inicio")
	if l := st[0].(*ast.Label); !l.Numeric || l.Name != "10" {
		t.Errorf("número de línea: %+v", l)
	}
	if l := st[2].(*ast.Label); l.Numeric || l.Name != "INICIO" {
		t.Errorf("etiqueta: %+v", l)
	}
	if j := st[5].(*ast.Jump); j.Target != "INICIO" {
		t.Errorf("GOTO: %+v", j)
	}
}

func TestSingleLineIf(t *testing.T) {
	s := one[*ast.If](t, `IF a THEN PRINT 1: PRINT 2 ELSE PRINT 3: PRINT 4`)
	if !s.SingleLine || len(s.Then) != 2 || len(s.Else) != 2 {
		t.Errorf("IF de una línea: %s", ast.Dump(s))
	}
	s = one[*ast.If](t, `IF a THEN IF b THEN x = 1 ELSE x = 2`)
	inner := s.Then[0].(*ast.If)
	if s.HasElse || !inner.HasElse {
		t.Errorf("ELSE debe ligarse al IF más cercano")
	}
	s = one[*ast.If](t, `IF a THEN 100 ELSE 200`)
	if s.Then[0].(*ast.Jump).Target != "100" || s.Else[0].(*ast.Jump).Target != "200" {
		t.Errorf("GOTO implícito: %s", ast.Dump(s))
	}
	s = one[*ast.If](t, `IF a GOTO fin`)
	if s.Then[0].(*ast.Jump).Target != "FIN" {
		t.Errorf("IF GOTO")
	}
}

func TestBlockIf(t *testing.T) {
	s := one[*ast.If](t, "IF a THEN\n x=1\nELSEIF b THEN\n x=2\nELSEIF c THEN\nELSE\n x=3\nEND IF")
	if s.SingleLine || len(s.ElseIfs) != 2 || !s.HasElse {
		t.Errorf("IF de bloque: %s", ast.Dump(s))
	}
}

func TestSelect(t *testing.T) {
	s := one[*ast.SelectCase](t, "SELECT CASE x\n' c\nCASE 1, 2 TO 4, IS >= 10, < 0\n y=1\nCASE ELSE\nEND SELECT")
	if len(s.Cases) != 2 {
		t.Fatalf("casos: %d", len(s.Cases))
	}
	tests := s.Cases[0].Tests
	kinds := []string{}
	for _, ct := range tests {
		kinds = append(kinds, ct.Kind+ct.Op)
	}
	if strings.Join(kinds, ",") != "VALUE,RANGE,IS>=,IS<" {
		t.Errorf("pruebas: %v", kinds)
	}
	if !s.Cases[1].Else {
		t.Errorf("CASE ELSE")
	}
}

func TestLoops(t *testing.T) {
	st := parseOK(t, "FOR i = 1 TO 3\nFOR j = 1 TO 2 STEP -1\nNEXT j, i\nPRINT")
	if len(st) != 2 {
		t.Fatalf("NEXT j, i: %s", ast.Dump(st))
	}
	outer := st[0].(*ast.For)
	inner := outer.Body[0].(*ast.For)
	if sx(outer.NextVar) != "i" || sx(inner.NextVar) != "j" || sx(inner.Step) != "(- 1)" {
		t.Errorf("FOR anidados: %s", ast.Dump(outer))
	}
	d := one[*ast.DoLoop](t, "DO\nLOOP UNTIL x > 3")
	if !d.PostUntil || d.PreCond != nil {
		t.Errorf("DO..LOOP UNTIL")
	}
	d = one[*ast.DoLoop](t, "DO WHILE x: x = x - 1: LOOP")
	if d.PreCond == nil || len(d.Body) != 1 {
		t.Errorf("DO WHILE en una línea: %s", ast.Dump(d))
	}
	one[*ast.While](t, "WHILE a: WEND")
}

func TestPrintInput(t *testing.T) {
	p := one[*ast.Print](t, `PRINT #1, USING "##.#"; a; b, c`)
	if p.File == nil || p.Using == nil || len(p.Items) != 3 || p.Items[1].Sep != ',' {
		t.Errorf("PRINT: %s", ast.Dump(p))
	}
	p = one[*ast.Print](t, `PRINT "a" x; TAB(10);`)
	if len(p.Items) != 3 || p.Items[2].Sep != ';' {
		t.Errorf("PRINT yuxtapuesto: %s", ast.Dump(p))
	}
	p = one[*ast.Print](t, `PRINT`)
	if len(p.Items) != 0 {
		t.Errorf("PRINT vacío")
	}
	in := one[*ast.Input](t, `INPUT ; "Edad: ", e%, n$`)
	if !in.NoNewline || in.Prompt.Value != "Edad: " || in.PromptSep != ',' || len(in.Vars) != 2 {
		t.Errorf("INPUT: %s", ast.Dump(in))
	}
	in = one[*ast.Input](t, `LINE INPUT #2, l$`)
	if !in.Line || in.File == nil {
		t.Errorf("LINE INPUT #")
	}
}

func TestFiles(t *testing.T) {
	o := one[*ast.Open](t, `OPEN f$ FOR RANDOM ACCESS READ WRITE LOCK WRITE AS #1 LEN = 128`)
	if o.Mode != "RANDOM" || o.Access != "READ WRITE" || o.Lock != "LOCK WRITE" || o.Len == nil {
		t.Errorf("OPEN: %s", ast.Dump(o))
	}
	o = one[*ast.Open](t, `OPEN "I", 2, "a.txt"`)
	if !o.Legacy {
		t.Errorf("OPEN legado")
	}
	g := one[*ast.FileIO](t, `GET #1, , reg`)
	if g.Record != nil || g.Var == nil {
		t.Errorf("GET sin registro: %s", ast.Dump(g))
	}
	one[*ast.Field](t, `FIELD 1, 10 AS a$, 20 AS b$`)
	l := one[*ast.Lock](t, `LOCK #1, TO 5`)
	if l.From != nil || !l.HasTo {
		t.Errorf("LOCK TO")
	}
}

func TestGraphics(t *testing.T) {
	l := one[*ast.Line](t, `LINE -(10, 20), , BF, &HFF00`)
	if l.From != nil || l.Color != nil || l.Box != "BF" || l.Style == nil {
		t.Errorf("LINE: %s", ast.Dump(l))
	}
	c := one[*ast.Circle](t, `CIRCLE STEP(0, 0), 10, , , , .5`)
	if !c.At.Step || c.Color != nil || c.Aspect == nil {
		t.Errorf("CIRCLE: %s", ast.Dump(c))
	}
	p := one[*ast.GfxPut](t, `PUT (1, 2), spr(0), PSET`)
	if p.Action != "PSET" {
		t.Errorf("PUT gráfico")
	}
	one[*ast.GfxGet](t, `GET (0, 0)-STEP(9, 9), spr`)
	v := one[*ast.View](t, `VIEW PRINT 2 TO 24`)
	if !v.Print || v.Top == nil {
		t.Errorf("VIEW PRINT")
	}
	w := one[*ast.Window](t, `WINDOW SCREEN (0, 0)-(1, 1)`)
	if !w.Screen || w.From == nil {
		t.Errorf("WINDOW")
	}
	s := one[*ast.Simple](t, `COLOR , 1`)
	if len(s.Args) != 2 || s.Args[0] != nil {
		t.Errorf("COLOR con omisión: %s", ast.Dump(s))
	}
	s = one[*ast.Simple](t, `LOCATE , , 1`)
	if len(s.Args) != 3 {
		t.Errorf("LOCATE: %s", ast.Dump(s))
	}
}

func TestEvents(t *testing.T) {
	e := one[*ast.OnEvent](t, `ON KEY(10) GOSUB Ayuda`)
	if e.Event != "KEY" || e.Target != "AYUDA" {
		t.Errorf("ON KEY: %s", ast.Dump(e))
	}
	one[*ast.OnEvent](t, `ON PEN GOSUB 100`)
	c := one[*ast.EventCtl](t, `KEY(1) STOP`)
	if c.Action != "STOP" || c.Arg == nil {
		t.Errorf("KEY(n) STOP")
	}
	if s := one[*ast.Simple](t, `KEY OFF`); s.Name != "KEY OFF" {
		t.Errorf("KEY OFF: %s", s.Name)
	}
	if s := one[*ast.Simple](t, `KEY 1, "LIST"`); s.Name != "KEY" || len(s.Args) != 2 {
		t.Errorf("KEY n, s$")
	}
	one[*ast.EventCtl](t, `TIMER ON`)
	if s := one[*ast.Simple](t, `PLAY "CDE"`); s.Name != "PLAY" {
		t.Errorf("PLAY")
	}
	o := one[*ast.OnJump](t, `ON n GOSUB 10, 20, uno`)
	if !o.Gosub || len(o.Targets) != 3 {
		t.Errorf("ON GOSUB")
	}
	one[*ast.OnError](t, `ON ERROR GOTO 0`)
	r := one[*ast.Resume](t, `RESUME NEXT`)
	if !r.Next {
		t.Errorf("RESUME NEXT")
	}
}

func TestDeclarations(t *testing.T) {
	d := one[*ast.Dim](t, `DIM SHARED a(1 TO 10, 5) AS INTEGER, b$, c AS STRING * 20`)
	if !d.Shared || len(d.Vars) != 3 || d.Vars[0].Bounds[0].Lo == nil || d.Vars[0].Bounds[1].Lo != nil {
		t.Errorf("DIM: %s", ast.Dump(d))
	}
	if d.Vars[2].Type.FixedLen == nil {
		t.Errorf("STRING * n")
	}
	d = one[*ast.Dim](t, `COMMON SHARED /blk/ x, y()`)
	if d.Block != "BLK" || !d.Vars[1].IsArray {
		t.Errorf("COMMON: %s", ast.Dump(d))
	}
	dt := one[*ast.DefType](t, `DEFINT A-C, X`)
	if len(dt.Ranges) != 2 || dt.Ranges[0].To != 'C' || dt.Ranges[1].From != 'X' {
		t.Errorf("DEFINT: %s", ast.Dump(dt))
	}
	td := one[*ast.TypeDecl](t, "TYPE Reg\n  a AS INTEGER ' c\n  n AS STRING * 8\nEND TYPE")
	if td.Name != "REG" || len(td.Fields) != 2 {
		t.Errorf("TYPE: %s", ast.Dump(td))
	}
	dc := one[*ast.Declare](t, `DECLARE FUNCTION Foo% CDECL ALIAS "_foo" (BYVAL a AS INTEGER, b() AS ANY)`)
	if !dc.Function || !dc.CDecl || dc.Alias != "_foo" || !dc.Params[0].ByVal || !dc.Params[1].IsArray {
		t.Errorf("DECLARE: %s", ast.Dump(dc))
	}
	pr := one[*ast.Proc](t, "FUNCTION Suma& (a&, b&) STATIC\n  Suma& = a& + b&\nEND FUNCTION")
	if !pr.Function || !pr.Static || len(pr.Params) != 2 || len(pr.Body) != 1 {
		t.Errorf("FUNCTION: %s", ast.Dump(pr))
	}
	fn := one[*ast.DefFn](t, `DEF FNcubo# (x#) = x# ^ 3`)
	if fn.Expr == nil {
		t.Errorf("DEF FN de una línea")
	}
	data := one[*ast.Data](t, `DATA "a, b", 12 , sin comillas,`)
	if len(data.Items) != 4 || data.Items[0].Text != "a, b" || !data.Items[0].Quoted || data.Items[2].Text != "sin comillas" {
		t.Errorf("DATA: %s", ast.Dump(data))
	}
}

func TestErrors(t *testing.T) {
	cases := []struct{ src, want string }{
		{"FOR i = 1 TO 2\nPRINT", "1:1: FOR without NEXT"},
		{"NEXT", "1:1: NEXT without FOR"},
		{"IF a THEN\nPRINT", "1:1: Block IF without END IF"},
		{"END IF", "1:1: END IF without block IF"},
		{"DO\n", "1:1: DO without LOOP"},
		{"WEND", "WEND without WHILE"},
		{"SELECT CASE x\nPRINT\nCASE 1\nEND SELECT", "2:1: Statements illegal between SELECT CASE and CASE"},
		{"x = (1 + ", "Expected: expression"},
		{"x = 1 2", "Expected: end-of-statement"},
		{"EXIT FOR", "EXIT FOR not within FOR"},
		{"SUB a\nSUB b\nEND SUB\nEND SUB", "SUB or FUNCTION not allowed"},
		{"TYPE t\nPRINT\nEND TYPE", "Statement illegal in TYPE block"},
		{"FOR i = 1 TO 2\nNEXT j", "NEXT without FOR"},
		{"OPTION BASE 2", "Expected: 0 or 1"},
		{"FOR i = 1 TO 2\nNEXT i, j", "NEXT without FOR"},
	}
	for _, c := range cases {
		_, errs := ParseFile("t.bas", []byte(c.src))
		var all []string
		for _, e := range errs {
			all = append(all, e.Error())
		}
		joined := strings.Join(all, " | ")
		if !strings.Contains(joined, c.want) {
			t.Errorf("%q\n got: %s\nwant: %s", c.src, joined, c.want)
		}
	}
}

// Un error no debe impedir reportar los siguientes (resincronización por línea).
func TestRecovery(t *testing.T) {
	_, errs := ParseFile("t.bas", []byte("x = (\nPRINT \"ok\"\ny = )\nFOR i = 1 TO 3\nNEXT i"))
	if len(errs) != 2 || errs[0].Pos.Line != 1 || errs[1].Pos.Line != 3 {
		t.Errorf("recuperación: %v", errs)
	}
}

func TestShowcase(t *testing.T) {
	src, err := os.ReadFile("../../testdata/showcase.bas")
	if err != nil {
		t.Skip(err)
	}
	_, errs := ParseFile("showcase.bas", src)
	for _, e := range errs {
		t.Error(e)
	}
}
