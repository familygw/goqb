package compiler_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"goqb/compiler"
	"goqb/lang/ast"
	"goqb/lang/parser"
	"goqb/lang/source"
	"goqb/runtime/screen"
	"goqb/vm"
)

func compileSrc(t *testing.T, src string) *vm.Program {
	t.Helper()
	f, errs := parser.ParseFile("t.bas", []byte(src))
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	p, _, diags := compiler.Compile(&source.Program{Modules: []*ast.File{f}})
	if len(diags) > 0 {
		t.Fatal(diags)
	}
	return p
}

func TestOnTimerAndPlay(t *testing.T) {
	p := compileSrc(t, `
ON TIMER(1) GOSUB Tic
TIMER ON
t0! = TIMER
DO WHILE ticks < 1 AND TIMER - t0! < 3: LOOP
TIMER OFF
PRINT "ticks"; ticks
t0! = TIMER
PLAY "T240 L8 CDEFG"
PRINT "play fg"; (TIMER - t0!) > .5
ON PLAY(2) GOSUB Musica
PLAY ON
PLAY "MB T255 L16 CDEFGAB"
t0! = TIMER
DO WHILE avisos = 0 AND TIMER - t0! < 3: LOOP
PRINT "on play"; avisos
SOUND 440, 2
END
Tic: ticks = ticks + 1: RETURN
Musica: avisos = avisos + 1: RETURN
`)
	var out bytes.Buffer
	con := vm.NewPlainConsole(strings.NewReader(""), &out)
	start := time.Now()
	if err := vm.New(p, con).Run(); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(strings.Fields(out.String()), " ")
	if got != "ticks 1 play fg-1 on play 1" {
		t.Errorf("salida: %q", got)
	}
	if el := time.Since(start); el > 4*time.Second {
		t.Errorf("tardó demasiado: %v", el)
	}
}

func TestOnKey(t *testing.T) {
	p := compileSrc(t, `
KEY 15, CHR$(0) + CHR$(31)
ON KEY(1) GOSUB F1
ON KEY(11) GOSUB Arriba
ON KEY(15) GOSUB LetraS
KEY(1) ON: KEY(11) ON: KEY(15) ON
DO
  k$ = INKEY$
  IF k$ = "q" THEN EXIT DO
  IF k$ <> "" THEN otras$ = otras$ + k$
LOOP
PRINT f; a; s; otras$
END
F1: f = f + 1: RETURN
Arriba: a = a + 1: RETURN
LetraS: s = s + 1: RETURN
`)
	txt := screen.New(80, 25)
	m := vm.New(p, txt)
	done := make(chan error, 1)
	go func() { done <- m.Run() }()
	for _, k := range []string{"\x00;", "x", "\x00H", "\x00H", "s", "y", "q"} {
		time.Sleep(20 * time.Millisecond)
		txt.PushKey(k)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		m.Stop()
		t.Fatal("el programa no terminó")
	}
	s := txt.Snapshot()
	line := make([]byte, s.Cols)
	for i := range line {
		line[i] = s.Cells[i].Ch
	}
	if got := strings.TrimSpace(string(line)); got != "1  2  1 xy" {
		t.Errorf("salida: %q", got)
	}
}
