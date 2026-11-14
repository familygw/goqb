// Package loader resuelve y carga los programas de CHAIN y RUN "archivo":
// fuentes .BAS (que se compilan al vuelo) o ejecutables hechos con Make EXE.
package loader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goqb/compiler"
	"goqb/lang/check"
	"goqb/lang/source"
	"goqb/runtime/bundle"
	"goqb/vm"
)

// New devuelve un vm.Loader que busca primero el nombre tal cual y, si no
// tiene extensión, prueba .BAS y luego el ejecutable (.EXE o sin extensión).
// inc son directorios adicionales para $INCLUDE; los nombres relativos se
// buscan en el directorio actual y después en dirs.
func New(inc []string, dirs ...string) vm.Loader {
	return func(name string) (*vm.Program, error) {
		name = strings.TrimSpace(name)
		path, ok := Resolve(name)
		for _, d := range dirs {
			if ok || filepath.IsAbs(name) || d == "" {
				break
			}
			path, ok = Resolve(filepath.Join(d, name))
		}
		if !ok {
			return nil, vm.ErrFileNotFound
		}
		if p, err := bundle.Read(path); err == nil {
			return p, nil
		}
		if isBinary(path) {
			return nil, vm.ErrBadFileMode
		}
		ld := &source.Loader{IncludePath: inc}
		prog, diags := ld.Load(path)
		if len(diags) == 0 {
			diags = check.Program(prog)
		}
		if len(diags) == 0 {
			var p *vm.Program
			p, _, diags = compiler.Compile(prog)
			if len(diags) == 0 {
				return p, nil
			}
		}
		return nil, fmt.Errorf("%s", diags[0])
	}
}

// Resolve busca el archivo de un CHAIN/RUN sin distinguir mayúsculas.
func Resolve(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	cands := []string{name}
	if filepath.Ext(name) == "" {
		cands = append(cands, name+".BAS", name+".EXE", name+".bas", name+".exe")
	}
	for _, c := range cands {
		if p, ok := find(c); ok {
			return p, true
		}
	}
	return "", false
}

// find busca path tal cual o, si no existe, ignorando mayúsculas en el nombre.
func find(path string) (string, bool) {
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path, true
	}
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(e.Name(), base) {
			return filepath.Join(dir, e.Name()), true
		}
	}
	return "", false
}

// isBinary detecta ejecutables (que no son fuentes BASIC).
func isBinary(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := f.Read(head)
	return bytes.IndexByte(head[:n], 0) >= 0
}
