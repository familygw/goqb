package ide

import (
	"reflect"
	"strings"
	"testing"
)

const sample = "' Programa de prueba\r\nDEFINT A-Z\r\nCLS\r\nSaludar \"mundo\"\r\nPRINT Doble(21)\r\n\r\nSUB Saludar (q$)\r\n  PRINT \"Hola, \"; q$\r\nEND SUB\r\n\r\nFUNCTION Doble (x) STATIC\r\n  Doble = x * 2\r\nEND FUNCTION\r\n"

func TestSplitAndAssemble(t *testing.T) {
	d := ParseDocument([]byte(sample))
	if len(d.Views) != 3 || d.Views[1].Name != "Saludar" || d.Views[2].Kind != "FUNCTION" {
		t.Fatalf("vistas: %+v", d.Views)
	}
	if !d.CRLF || d.UTF8 {
		t.Errorf("formato detectado incorrecto")
	}
	lines, locs := d.Assemble()
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"' Programa de prueba\nDECLARE SUB Saludar (q$)\nDECLARE FUNCTION Doble (x)\nDEFINT A-Z",
		"END SUB\n\nFUNCTION Doble (x) STATIC",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("falta %q en:\n%s", want, text)
		}
	}
	if len(lines) != len(locs) {
		t.Fatalf("mapa de líneas inconsistente")
	}
	// la línea "Doble = x * 2" debe mapear a la vista 2, línea 1
	for i, l := range lines {
		if strings.Contains(l, "Doble = x * 2") && (locs[i] != Loc{2, 1}) {
			t.Errorf("loc de %q: %+v", l, locs[i])
		}
	}
	// guardar y volver a cargar no duplica los DECLARE
	d2 := ParseDocument(d.Bytes())
	l2, _ := d2.Assemble()
	if strings.Count(strings.Join(l2, "\n"), "DECLARE SUB Saludar") != 1 {
		t.Errorf("DECLARE duplicado:\n%s", strings.Join(l2, "\n"))
	}
}

func TestUTF8RoundTrip(t *testing.T) {
	d := ParseDocument([]byte("PRINT \"ñandú\"\n"))
	if !d.UTF8 || d.CRLF {
		t.Fatalf("debería detectarse UTF-8 con LF")
	}
	if string(d.Bytes()) != "PRINT \"ñandú\"\n" {
		t.Errorf("ida y vuelta: %q", d.Bytes())
	}
	src, _ := d.Source()
	if !strings.Contains(string(src), "\xa4and\xa3") { // ñ=0xA4, ú=0xA3 en CP437
		t.Errorf("fuente para compilar no está en CP437: %q", src)
	}
}

// TestCommentBeforeSub: los comentarios antes de un SUB van a su vista y el
// archivo se vuelve a guardar igual.
func TestCommentBeforeSub(t *testing.T) {
	src := "CLS\r\nDECLARE SUB Caja (w AS INTEGER)\r\n\r\nCaja 10\r\n\r\n' dibuja una caja\r\n' de ancho w\r\n\r\nSUB Caja (w AS INTEGER)\r\n  PRINT w\r\nEND SUB\r\n"
	d := ParseDocument([]byte(src))
	if len(d.Views) != 2 {
		t.Fatalf("%d vistas", len(d.Views))
	}
	if got := d.Views[0].Lines; !reflect.DeepEqual(got, []string{"CLS", "DECLARE SUB Caja (w AS INTEGER)", "", "Caja 10"}) {
		t.Errorf("módulo: %q", got)
	}
	v := d.Views[1]
	if v.Lines[0] != "' dibuja una caja" || v.headerRow() != 3 {
		t.Errorf("vista del SUB: %q (encabezado en %d)", v.Lines, v.headerRow())
	}
	if got := string(d.Bytes()); got != src {
		t.Errorf("al guardar cambió:\n%q\n%q", got, src)
	}
	if h := procsHint(d); !strings.Contains(h, "SUB Caja") {
		t.Errorf("aviso: %q", h)
	}
	// sin comentarios, las líneas vacías quedan en el módulo y el SUB empieza en el encabezado
	d = ParseDocument([]byte("PRINT 1\r\n\r\nSUB A\r\nEND SUB\r\n"))
	if d.Views[1].Lines[0] != "SUB A" || d.Views[1].headerRow() != 0 {
		t.Errorf("sin comentarios: %q", d.Views[1].Lines)
	}
}
