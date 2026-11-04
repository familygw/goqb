package compiler_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"goqb/compiler"
	"goqb/lang/ast"
	"goqb/lang/parser"
	"goqb/lang/source"
	"goqb/vm"
)

const debugSrc = `DECLARE SUB Sumar (n)
total = 0
FOR i = 1 TO 3
    Sumar i
NEXT
PRINT "total"; total
STOP
PRINT "fin"
SUB Sumar (n)
    SHARED total
    doble = n * 2
    total = total + doble
END SUB
`

func debugVM(t *testing.T) (*vm.VM, *compiler.Compiler, *bytes.Buffer) {
	t.Helper()
	f, errs := parser.ParseFile("d.bas", []byte(debugSrc))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	p, c, diags := compiler.Compile(&source.Program{Modules: []*ast.File{f}})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	var out bytes.Buffer
	m := vm.New(p, vm.NewPlainConsole(strings.NewReader(""), &out))
	m.Debug = vm.NewDebugger()
	return m, c, &out
}

func expectPause(t *testing.T, m *vm.VM, line int) {
	t.Helper()
	err := m.Run()
	var re *vm.RuntimeError
	if !errors.As(err, &re) || !errors.Is(re.Err, vm.ErrPaused) {
		t.Fatalf("se esperaba una pausa, hubo %v", err)
	}
	if re.Line != line {
		t.Fatalf("pausa en la línea %d, se esperaba %d", re.Line, line)
	}
}

func eval(t *testing.T, m *vm.VM, c *compiler.Compiler, expr string) string {
	t.Helper()
	entry, diags := c.CompileEval(expr, m.CurrentProc())
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	v, err := m.Eval(entry)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(vm.FormatNumber(v))
}

func TestDebugger(t *testing.T) {
	m, c, out := debugVM(t)
	pcs := m.Prog.StmtPCs("d.bas")
	m.Debug.Breaks[pcs[12]] = true // total = total + doble

	expectPause(t, m, 12)
	if got := eval(t, m, c, "doble"); got != "2" {
		t.Errorf("doble = %s", got)
	}
	if st := m.CallStack(); len(st) != 2 || st[0].Proc != "Sumar" {
		t.Errorf("pila: %+v", st)
	}
	// segunda vuelta
	expectPause(t, m, 12)
	if got := eval(t, m, c, "n * 100 + total"); got != "202" {
		t.Errorf("expresión = %s", got)
	}
	// Immediate dentro del SUB detenido, sin perder la posibilidad de continuar
	f, _ := parser.ParseFile("i", []byte("total = total + 1000"))
	entry, diags := c.ImmediateIn(f.Stmts, m.CurrentProc())
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	if _, err := m.Eval(entry); err != nil {
		t.Fatal(err)
	}
	delete(m.Debug.Breaks, pcs[12])

	// paso a paso: 12 → 13 (END SUB) → vuelve al FOR
	m.Step(vm.DebugStepInto)
	expectPause(t, m, 13)
	m.Step(vm.DebugStepOver) // vuelve al llamador: NEXT
	expectPause(t, m, 5)
	m.Step(vm.DebugStepOver) // NEXT vuelve al cuerpo
	expectPause(t, m, 4)
	m.Step(vm.DebugStepOver) // Sumar 3 sin entrar
	expectPause(t, m, 5)

	// Set Next Statement: saltar el NEXT e ir directo al PRINT
	if err := m.SetNextStatement(pcs[6]); err != nil {
		t.Fatal(err)
	}
	if err := m.SetNextStatement(pcs[11]); err == nil {
		t.Error("no debería permitir saltar dentro de otro procedimiento")
	}
	expectPause(t, m, 7) // STOP
	if err := m.Run(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(strings.Fields(out.String()), " "); got != "total 1012 fin" {
		t.Errorf("salida: %q", got)
	}
}
