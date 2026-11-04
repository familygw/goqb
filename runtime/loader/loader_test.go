package loader

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goqb/vm"
)

func TestChainRun(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		os.WriteFile(filepath.Join(dir, name), []byte(strings.ReplaceAll(src, "\n", "\r\n")), 0o644)
	}
	write("A.BAS", `COMMON a%, s$
DIM x(5)
a% = 5: s$ = "hola"
OPEN "f.txt" FOR OUTPUT AS #1
PRINT #1, "uno"
CHAIN "b"
PRINT "no llega"
`)
	write("b.bas", `COMMON n%, t$
PRINT n%; t$
PRINT #1, "dos"
ON ERROR GOTO falla
RUN "noexiste"
falla:
PRINT "ERR"; ERR
RESUME sigue
sigue:
RUN "C"
`)
	write("C.BAS", `PRINT "C"; n%
`)
	wd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(wd)
	p, err := New(nil)("A")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	m := vm.New(p, vm.NewPlainConsole(strings.NewReader(""), &out))
	m.Loader = New(nil)
	if err := m.Run(); err != nil {
		t.Fatal(err)
	}
	want := " 5 hola\nERR 53 \nC 0 \n"
	if out.String() != want {
		t.Errorf("salida %q, se esperaba %q", out.String(), want)
	}
	data, _ := os.ReadFile("f.txt")
	if string(data) != "uno\r\ndos\r\n" {
		t.Errorf("archivo %q", data)
	}
}
