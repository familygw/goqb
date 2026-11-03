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

// runProgram compila y ejecuta un .BAS con consola de flujo.
func runProgram(t *testing.T, path string, input string) (string, error) {
	t.Helper()
	abs, _ := filepath.Abs(path)
	prog, diags := (&source.Loader{}).Load(abs)
	diags = append(diags, check.Program(prog)...)
	if len(diags) > 0 {
		t.Fatalf("errores de carga: %v", diags)
	}
	p, _, diags := compiler.Compile(prog)
	if len(diags) > 0 {
		t.Fatalf("errores de compilación: %v", diags)
	}
	var out bytes.Buffer
	con := vm.NewPlainConsole(strings.NewReader(input), &out)
	con.EchoInput = true
	// los programas crean archivos: ejecutar en un directorio temporal
	wd, _ := os.Getwd()
	os.Chdir(t.TempDir())
	defer os.Chdir(wd)
	err := vm.New(p, con).Run()
	return out.String(), err
}

// TestGolden ejecuta testdata/run/*.bas y compara con *.out.
// Para regenerar: GOQB_UPDATE=1 go test ./compiler -run Golden
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("../testdata/run/*.bas")
	if len(files) == 0 {
		t.Skip("sin programas de prueba")
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".bas")
		t.Run(name, func(t *testing.T) {
			in, _ := os.ReadFile(strings.TrimSuffix(f, ".bas") + ".in")
			got, err := runProgram(t, f, string(in))
			if err != nil {
				got += "ERROR: " + err.Error() + "\n"
			}
			golden := strings.TrimSuffix(f, ".bas") + ".out"
			if os.Getenv("GOQB_UPDATE") != "" {
				os.WriteFile(golden, []byte(got), 0o644)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("falta %s (GOQB_UPDATE=1 para generarlo)", golden)
			}
			if got != string(want) {
				t.Errorf("salida distinta.\n--- obtenida ---\n%s\n--- esperada ---\n%s", got, want)
			}
		})
	}
}
