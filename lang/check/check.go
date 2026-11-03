// Package check hace las verificaciones estructurales posteriores al parseo
// que el IDE de QB 4.5 reporta al cargar o ejecutar un programa:
// etiquetas duplicadas o inexistentes, definiciones duplicadas de
// SUB/FUNCTION/TYPE/DEF FN y saltos fuera de su ámbito.
//
// El chequeo de tipos y resolución de símbolos completo llega con el
// compilador (fase 2).
package check

import (
	"strings"

	"goqb/lang/ast"
	"goqb/lang/source"
)

type scope struct {
	name   string // "" = nivel de módulo
	labels map[string]bool
	refs   []ref
}

type ref struct {
	file   string
	pos    ast.Pos
	target string
}

type checker struct {
	file   string
	module *scope
	diags  []source.Diag
	procs  map[string]string // nombre → archivo donde se definió
	types  map[string]bool
	fns    map[string]bool
}

// Program verifica todos los módulos de un programa.
func Program(prog *source.Program) []source.Diag {
	c := &checker{procs: map[string]string{}}
	for _, f := range prog.Modules {
		c.module = newScope("")
		c.types = map[string]bool{}
		c.fns = map[string]bool{}
		c.file = f.Name
		c.walk(f.Name, f.Stmts, c.module)
		c.resolve(c.module)
	}
	return c.diags
}

func newScope(name string) *scope { return &scope{name: name, labels: map[string]bool{}} }

func (c *checker) errAt(file string, pos ast.Pos, msg string) {
	c.diags = append(c.diags, source.Diag{File: file, Pos: pos, Msg: msg})
}

func (c *checker) resolve(s *scope) {
	for _, r := range s.refs {
		if !s.labels[r.target] {
			c.errAt(r.file, r.pos, "Label not defined: "+r.target)
		}
	}
}

func (c *checker) walk(file string, stmts []ast.Stmt, sc *scope) {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ast.Label:
			if sc.labels[s.Name] {
				c.errAt(file, s.P, "Duplicate label: "+s.Name)
			}
			sc.labels[s.Name] = true
		case *ast.Meta:
			if s.Resolved != "" {
				c.walk(s.Resolved, s.Stmts, sc)
			}
		case *ast.Jump:
			if s.Target == "" {
				continue
			}
			target := sc
			if s.Kind == "RESTORE" {
				target = c.module // DATA siempre vive a nivel de módulo
			}
			target.refs = append(target.refs, ref{file, s.P, s.Target})
		case *ast.OnJump:
			for _, t := range s.Targets {
				sc.refs = append(sc.refs, ref{file, s.P, t})
			}
		case *ast.Resume:
			if s.Target != "" && s.Target != "0" {
				c.module.refs = append(c.module.refs, ref{file, s.P, s.Target})
			}
		case *ast.OnError:
			// En QB 4.5 el manejador de errores debe estar a nivel de módulo.
			if s.Target != "0" {
				c.module.refs = append(c.module.refs, ref{file, s.P, s.Target})
			}
		case *ast.OnEvent:
			c.module.refs = append(c.module.refs, ref{file, s.P, s.Target})
		case *ast.If:
			c.walk(file, s.Then, sc)
			for _, e := range s.ElseIfs {
				c.walk(file, e.Body, sc)
			}
			c.walk(file, s.Else, sc)
		case *ast.SelectCase:
			for _, cl := range s.Cases {
				c.walk(file, cl.Body, sc)
			}
		case *ast.For:
			c.walk(file, s.Body, sc)
		case *ast.DoLoop:
			c.walk(file, s.Body, sc)
		case *ast.While:
			c.walk(file, s.Body, sc)
		case *ast.Proc:
			key := procKey(s.Name)
			if prev, dup := c.procs[key]; dup {
				c.errAt(file, s.P, "Duplicate definition: "+s.Name+" (already defined in "+prev+")")
			} else {
				c.procs[key] = file
			}
			ps := newScope(key)
			c.walk(file, s.Body, ps)
			c.resolve(ps)
		case *ast.DefFn:
			key := procKey(s.Name)
			if c.fns[key] {
				c.errAt(file, s.P, "Duplicate definition: "+s.Name)
			}
			c.fns[key] = true
			if s.Body != nil {
				fs := newScope(key)
				c.walk(file, s.Body, fs)
				c.resolve(fs)
			}
		case *ast.TypeDecl:
			if c.types[s.Name] {
				c.errAt(file, s.P, "Duplicate definition: TYPE "+s.Name)
			}
			c.types[s.Name] = true
			seen := map[string]bool{}
			for _, f := range s.Fields {
				k := strings.ToUpper(f.Name)
				if seen[k] {
					c.errAt(file, f.P, "Duplicate definition: "+f.Name)
				}
				seen[k] = true
			}
		}
	}
}

// procKey normaliza el nombre de un procedimiento (sin sufijo, mayúsculas).
func procKey(name string) string {
	return strings.ToUpper(strings.TrimRight(name, "%&!#$"))
}
