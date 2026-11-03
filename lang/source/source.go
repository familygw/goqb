// Package source carga programas QB desde disco: módulos .BAS, proyectos
// .MAK y resolución de metacomandos $INCLUDE.
//
// Los nombres de archivo se resuelven sin distinguir mayúsculas, porque los
// programas DOS suelen referirse a 'QB.BI' cuando en disco está qb.bi.
package source

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goqb/lang/ast"
	"goqb/lang/cp437"
	"goqb/lang/parser"
	"goqb/lang/token"
)

// Diag es un diagnóstico asociado a un archivo.
type Diag struct {
	File string
	Pos  token.Pos
	Msg  string
}

func (d Diag) String() string {
	if d.Pos.Line == 0 {
		return fmt.Sprintf("%s: %s", d.File, d.Msg)
	}
	return fmt.Sprintf("%s:%d:%d: %s", d.File, d.Pos.Line, d.Pos.Col, d.Msg)
}

// MaxIncludeDepth limita el anidamiento de $INCLUDE.
const MaxIncludeDepth = 16

// Program es un conjunto de módulos (uno, o varios si viene de un .MAK).
type Program struct {
	Modules []*ast.File
}

// Loader carga archivos y resuelve includes.
type Loader struct {
	// IncludePath son directorios adicionales donde buscar archivos $INCLUDE
	// (equivalente a la opción "Include path" de QB 4.5).
	IncludePath []string
	// ReadFile permite inyectar un sistema de archivos (tests, buffers del IDE).
	ReadFile func(string) ([]byte, error)

	diags []Diag
}

func (l *Loader) read(path string) ([]byte, error) {
	if l.ReadFile != nil {
		return l.ReadFile(path)
	}
	b, err := os.ReadFile(path)
	return cp437.ToCP437(b), err // los fuentes en UTF-8 se pasan a CP437
}

// Load carga un .BAS o un .MAK.
func (l *Loader) Load(path string) (*Program, []Diag) {
	l.diags = nil
	prog := &Program{}
	if strings.EqualFold(filepath.Ext(path), ".mak") {
		mods, err := l.readMak(path)
		if err != nil {
			l.diags = append(l.diags, Diag{File: path, Msg: err.Error()})
			return prog, l.diags
		}
		for _, m := range mods {
			if f := l.loadModule(m); f != nil {
				prog.Modules = append(prog.Modules, f)
			}
		}
		return prog, l.diags
	}
	if f := l.loadModule(path); f != nil {
		prog.Modules = append(prog.Modules, f)
	}
	return prog, l.diags
}

// readMak lee un archivo de proyecto QB 4.5: un módulo por línea, el
// primero es el módulo principal.
func (l *Loader) readMak(path string) ([]string, error) {
	src, err := l.read(path)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	var mods []string
	sc := bufio.NewScanner(bytes.NewReader(src))
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if name == "" {
			continue
		}
		if filepath.Ext(name) == "" {
			name += ".BAS"
		}
		if !filepath.IsAbs(name) {
			name = filepath.Join(dir, name)
		}
		mods = append(mods, resolveCase(name))
	}
	if len(mods) == 0 {
		return nil, fmt.Errorf("empty project file")
	}
	return mods, sc.Err()
}

func (l *Loader) loadModule(path string) *ast.File {
	src, err := l.read(path)
	if err != nil {
		l.diags = append(l.diags, Diag{File: path, Msg: "File not found"})
		return nil
	}
	f := l.parse(path, src)
	f.Stmts = l.expand(path, f.Stmts, []string{abs(path)})
	return f
}

func (l *Loader) parse(path string, src []byte) *ast.File {
	f, errs := parser.ParseFile(path, src)
	for _, e := range errs {
		l.diags = append(l.diags, Diag{File: path, Pos: e.Pos, Msg: e.Msg})
	}
	return f
}

// expand sustituye cada $INCLUDE por las sentencias del archivo incluido,
// envueltas en un nodo Include para conservar el origen.
func (l *Loader) expand(from string, stmts []ast.Stmt, stack []string) []ast.Stmt {
	out := make([]ast.Stmt, 0, len(stmts))
	for _, s := range stmts {
		m, ok := s.(*ast.Meta)
		if !ok || m.Name != "INCLUDE" {
			if p, ok := s.(*ast.Proc); ok {
				p.Body = l.expand(from, p.Body, stack)
			}
			out = append(out, s)
			continue
		}
		out = append(out, s)
		if m.Arg == "" {
			l.diags = append(l.diags, Diag{File: from, Pos: m.P, Msg: "Expected: '$INCLUDE: 'filename'"})
			continue
		}
		path, err := l.findInclude(from, m.Arg)
		if err != nil {
			l.diags = append(l.diags, Diag{File: from, Pos: m.P, Msg: "File not found: " + m.Arg})
			continue
		}
		ap := abs(path)
		for _, x := range stack {
			if x == ap {
				l.diags = append(l.diags, Diag{File: from, Pos: m.P, Msg: "Recursive $INCLUDE: " + m.Arg})
				ap = ""
				break
			}
		}
		if ap == "" {
			continue
		}
		if len(stack) >= MaxIncludeDepth {
			l.diags = append(l.diags, Diag{File: from, Pos: m.P, Msg: "Too many nested $INCLUDE files"})
			continue
		}
		src, _ := l.read(path)
		inc := l.parse(path, src)
		m.Resolved = path
		m.Stmts = l.expand(path, inc.Stmts, append(stack, ap))
	}
	return out
}

func (l *Loader) findInclude(from, name string) (string, error) {
	dirs := append([]string{filepath.Dir(from), "."}, l.IncludePath...)
	if filepath.IsAbs(name) {
		dirs = []string{""}
	}
	for _, d := range dirs {
		p := resolveCase(filepath.Join(d, filepath.FromSlash(strings.ReplaceAll(name, `\`, "/"))))
		if _, err := l.read(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

// resolveCase busca path en disco sin distinguir mayúsculas en el nombre final.
func resolveCase(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	dir, base := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return path
	}
	for _, e := range ents {
		if strings.EqualFold(e.Name(), base) {
			return filepath.Join(dir, e.Name())
		}
	}
	return path
}

func abs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return strings.ToLower(a)
}
