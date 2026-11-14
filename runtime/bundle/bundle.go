// Package bundle arma ejecutables independientes (Make EXE): copia un
// binario de runtime ("stub") y le anexa el programa compilado.
//
// Formato al final del archivo: [programa serializado][largo: 8 bytes LE]["GOQBEXE1"].
package bundle

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"goqb/vm"
)

const magic = "GOQBEXE1"

// ErrNoPayload indica que el ejecutable no trae un programa anexado.
var ErrNoPayload = errors.New("sin programa anexado")

// Read lee el programa anexado a un ejecutable.
func Read(path string) (*vm.Program, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < 16 {
		return nil, ErrNoPayload
	}
	tail := make([]byte, 16)
	if _, err := f.ReadAt(tail, st.Size()-16); err != nil {
		return nil, err
	}
	if string(tail[8:]) != magic {
		return nil, ErrNoPayload
	}
	n := int64(binary.LittleEndian.Uint64(tail[:8]))
	if n <= 0 || n > st.Size()-16 {
		return nil, ErrNoPayload
	}
	data := make([]byte, n)
	if _, err := f.ReadAt(data, st.Size()-16-n); err != nil {
		return nil, err
	}
	return vm.Unmarshal(data)
}

// Self devuelve el programa anexado al ejecutable actual, si lo hay.
func Self() (*vm.Program, bool) {
	exe, err := os.Executable()
	if err != nil {
		return nil, false
	}
	p, err := Read(exe)
	return p, err == nil
}

// stubSize devuelve el tamaño del binario sin el programa anexado (si lo tuviera).
func stubSize(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := st.Size()
	if size >= 16 {
		tail := make([]byte, 16)
		if _, err := f.ReadAt(tail, size-16); err == nil && string(tail[8:]) == magic {
			size -= 16 + int64(binary.LittleEndian.Uint64(tail[:8]))
		}
	}
	return size, nil
}

// Write crea out copiando stub y anexándole el programa.
func Write(stub, out string, p *vm.Program) error {
	data, err := vm.Marshal(p)
	if err != nil {
		return err
	}
	src, err := os.Open(stub)
	if err != nil {
		return err
	}
	defer src.Close()
	size, err := stubSize(src)
	if err != nil {
		return err
	}
	if abs(stub) == abs(out) {
		return fmt.Errorf("el ejecutable de salida no puede ser el mismo stub")
	}
	tmp := out + ".tmp"
	dst, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, io.NewSectionReader(src, 0, size)); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	var tail bytes.Buffer
	tail.Write(data)
	binary.Write(&tail, binary.LittleEndian, uint64(len(data)))
	tail.WriteString(magic)
	if _, err := dst.Write(tail.Bytes()); err != nil {
		dst.Close()
		os.Remove(tmp)
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, out)
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// Target es un sistema/arquitectura de destino ("darwin/arm64").
type Target struct{ OS, Arch string }

// Host es la plataforma actual.
func Host() Target { return Target{runtime.GOOS, runtime.GOARCH} }

// ParseTarget interpreta "os/arch" (vacío = la plataforma actual).
func ParseTarget(s string) (Target, error) {
	if s == "" {
		return Host(), nil
	}
	os_, arch, ok := strings.Cut(s, "/")
	if !ok || os_ == "" || arch == "" {
		return Target{}, fmt.Errorf("destino inválido %q (usar os/arch, por ejemplo windows/amd64)", s)
	}
	return Target{os_, arch}, nil
}

func (t Target) String() string { return t.OS + "/" + t.Arch }

// ExeName agrega .exe si el destino es Windows.
func (t Target) ExeName(name string) string {
	if t.OS == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

// FindStub busca el runtime para el destino: $GOQB_STUBS o junto al
// ejecutable actual (goqbrt-os-arch[.exe]); para la plataforma actual
// también sirve goqbrt o el propio ejecutable.
func FindStub(t Target) (string, error) {
	name := t.ExeName("goqbrt-" + t.OS + "-" + t.Arch)
	var dirs []string
	if d := os.Getenv("GOQB_STUBS"); d != "" {
		dirs = append(dirs, d)
	}
	exe, err := os.Executable()
	if err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		dirs = append(dirs, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "stubs"))
	}
	for _, d := range dirs {
		if p := filepath.Join(d, name); exists(p) {
			return p, nil
		}
	}
	if t == Host() {
		for _, d := range dirs {
			if p := filepath.Join(d, t.ExeName("goqbrt")); exists(p) {
				return p, nil
			}
		}
		if exe != "" {
			return exe, nil // el propio goqb/goqbc también funciona como runtime
		}
	}
	return "", fmt.Errorf("no hay runtime para %s: generalo con scripts/build-stubs.sh y dejalo en $GOQB_STUBS", t)
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
