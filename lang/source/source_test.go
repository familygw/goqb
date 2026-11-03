package source_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goqb/lang/ast"
	"goqb/lang/check"
	"goqb/lang/source"
)

func write(t *testing.T, dir, name, src string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func load(t *testing.T, path string) (*source.Program, string) {
	t.Helper()
	prog, diags := (&source.Loader{}).Load(path)
	diags = append(diags, check.Program(prog)...)
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.String())
	}
	return prog, strings.Join(msgs, "\n")
}

func TestIncludeCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "qb.bi", "TYPE RegType\n ax AS INTEGER\nEND TYPE\nfin:\n")
	main := write(t, dir, "main.bas", "'$INCLUDE: 'QB.BI'\nDIM r AS RegType\nGOTO fin\n")
	prog, msgs := load(t, main)
	if msgs != "" {
		t.Fatalf("diagnósticos inesperados:\n%s", msgs)
	}
	m := prog.Modules[0].Stmts[1].(*ast.Meta)
	if m.Resolved == "" || len(m.Stmts) == 0 {
		t.Errorf("include no resuelto: %+v", m)
	}
}

func TestIncludeErrors(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.bi", "'$INCLUDE: 'b.bi'\n")
	write(t, dir, "b.bi", "'$INCLUDE: 'a.bi'\nx = (\n")
	main := write(t, dir, "main.bas", "'$INCLUDE: 'a.bi'\n'$INCLUDE: 'nada.bi'\n")
	_, msgs := load(t, main)
	for _, want := range []string{"Recursive $INCLUDE: a.bi", "File not found: nada.bi", "b.bi:2:6: Expected: expression"} {
		if !strings.Contains(msgs, want) {
			t.Errorf("falta %q en:\n%s", want, msgs)
		}
	}
}

func TestMakProject(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "MAIN.BAS", "DECLARE SUB Uno ()\nUno\n")
	write(t, dir, "otro.bas", "SUB Uno\nEND SUB\n")
	write(t, dir, "dup.bas", "SUB uno\nEND SUB\n")
	mak := write(t, dir, "prog.mak", "main\r\nOTRO.BAS\r\ndup.bas\r\n")
	prog, msgs := load(t, mak)
	if len(prog.Modules) != 3 {
		t.Fatalf("módulos: %d", len(prog.Modules))
	}
	if !strings.Contains(msgs, "Duplicate definition: uno") {
		t.Errorf("duplicado entre módulos no detectado:\n%s", msgs)
	}
}

func TestLabelScopes(t *testing.T) {
	dir := t.TempDir()
	src := `ON ERROR GOTO handler
GOTO adentro
handler: RESUME NEXT
uno:
uno:
SUB s
  adentro: GOTO adentro
  GOSUB handler
END SUB
`
	_, msgs := load(t, write(t, dir, "m.bas", src))
	for _, want := range []string{
		"m.bas:2:1: Label not defined: ADENTRO",
		"m.bas:5:1: Duplicate label: UNO",
		"m.bas:8:3: Label not defined: HANDLER",
	} {
		if !strings.Contains(msgs, want) {
			t.Errorf("falta %q en:\n%s", want, msgs)
		}
	}
	if strings.Count(msgs, "\n")+1 != 3 {
		t.Errorf("se esperaban exactamente 3 diagnósticos:\n%s", msgs)
	}
}
