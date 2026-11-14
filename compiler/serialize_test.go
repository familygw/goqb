package compiler_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goqb/compiler"
	"goqb/lang/check"
	"goqb/lang/source"
	"goqb/vm"
)

// TestSerializeRoundTrip ejecuta cada programa de testdata/run tras
// serializarlo y deserializarlo, y compara con la salida esperada.
func TestSerializeRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("../testdata/run/*.bas")
	if len(files) == 0 {
		t.Skip("sin programas de prueba")
	}
	for _, f := range files {
		f := f
		t.Run(filepath.Base(f), func(t *testing.T) {
			want, err := os.ReadFile(strings.TrimSuffix(f, ".bas") + ".out")
			if err != nil {
				t.Skip("sin .out")
			}
			input, _ := os.ReadFile(strings.TrimSuffix(f, ".bas") + ".in")
			abs, _ := filepath.Abs(f)
			prog, diags := (&source.Loader{}).Load(abs)
			diags = append(diags, check.Program(prog)...)
			p, _, d2 := compiler.Compile(prog)
			if len(diags)+len(d2) > 0 {
				t.Fatalf("errores: %v %v", diags, d2)
			}
			data, err := vm.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			q, err := vm.Unmarshal(data)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			con := vm.NewPlainConsole(bytes.NewReader(input), &out)
			con.EchoInput = true
			wd, _ := os.Getwd()
			os.Chdir(t.TempDir())
			defer os.Chdir(wd)
			got := ""
			if err := vm.New(q, con).Run(); err != nil {
				got = "ERROR: " + err.Error() + "\n"
			}
			if out.String()+got != string(want) {
				t.Errorf("salida distinta tras serializar:\n%s%s", out.String(), got)
			}
		})
	}
}
