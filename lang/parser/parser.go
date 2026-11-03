// Package parser construye el AST de un módulo QuickBASIC 4.5.
//
// El parser es descendente recursivo y orientado a líneas. Ante un error
// lo registra y se resincroniza en el fin de línea, igual que el chequeo
// línea a línea del IDE original, por lo que reporta todos los errores
// de un archivo en una sola pasada.
package parser

import (
	"fmt"
	"strings"

	"goqb/lang/ast"
	"goqb/lang/lexer"
	"goqb/lang/token"
)

// Error es un error de sintaxis con posición.
type Error struct {
	Pos token.Pos
	Msg string
}

func (e Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

// Parser mantiene el estado del análisis de un archivo.
type Parser struct {
	toks   []token.Token
	i      int
	errs   []Error
	blocks []string // bloques abiertos: IF, SELECT, FOR, DO, WHILE, SUB, FUNCTION, DEF, SINGLEIF

	singleIf    int // profundidad de IF de una línea (ELSE termina sentencias)
	pendingNext int // variables restantes de un NEXT i, j, k
}

type bailout struct{}

// ParseFile analiza src y devuelve el módulo y los errores encontrados.
func ParseFile(name string, src []byte) (*ast.File, []Error) {
	p := &Parser{toks: lexer.Tokenize(src)}
	f := &ast.File{Name: name}
	f.Stmts = p.parseStmts()
	if p.pendingNext > 0 {
		p.errAt(p.cur().Pos, "NEXT without FOR")
	}
	return f, p.errs
}

// ParseExpr analiza una expresión aislada (útil para la ventana Immediate y tests).
func ParseExpr(src string) (ast.Expr, []Error) {
	p := &Parser{toks: lexer.Tokenize([]byte(src))}
	var e ast.Expr
	func() {
		defer p.recoverStmt()
		e = p.parseExpr()
		if !p.atEnd() {
			p.fail("Expected: end-of-statement")
		}
	}()
	return e, p.errs
}

// ------------------------------------------------------------- navegación

func (p *Parser) cur() token.Token { return p.toks[p.i] }

func (p *Parser) peek(n int) token.Token {
	if p.i+n < len(p.toks) {
		return p.toks[p.i+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *Parser) next() token.Token {
	t := p.toks[p.i]
	if t.Kind != token.EOF {
		p.i++
	}
	return t
}

func (p *Parser) is(s string) bool { return p.cur().Is(s) }

func (p *Parser) accept(s string) bool {
	if p.is(s) {
		p.next()
		return true
	}
	return false
}

func (p *Parser) expect(s string) token.Token {
	if !p.is(s) {
		p.fail("Expected: " + s)
	}
	return p.next()
}

// atEnd reporta si la sentencia actual terminó.
func (p *Parser) atEnd() bool {
	t := p.cur()
	switch t.Kind {
	case token.EOF, token.Newline, token.Comment:
		return true
	}
	if t.Is(":") {
		return true
	}
	return p.singleIf > 0 && t.Is("ELSE")
}

func (p *Parser) expectEnd() {
	if !p.atEnd() {
		p.fail("Expected: end-of-statement")
	}
}

// ------------------------------------------------------------- errores

func (p *Parser) errAt(pos token.Pos, msg string) {
	p.errs = append(p.errs, Error{Pos: pos, Msg: msg})
}

// fail registra un error en el token actual y aborta la sentencia.
func (p *Parser) fail(msg string) {
	p.errAt(p.cur().Pos, msg)
	panic(bailout{})
}

func (p *Parser) recoverStmt() {
	if r := recover(); r != nil {
		if _, ok := r.(bailout); !ok {
			panic(r)
		}
		p.syncLine()
	}
}

// syncLine descarta tokens hasta el fin de línea (sin consumirlo).
func (p *Parser) syncLine() {
	for {
		k := p.cur().Kind
		if k == token.Newline || k == token.EOF {
			return
		}
		p.next()
	}
}

// ------------------------------------------------------------- bloques

// terminates reporta si la sentencia actual cierra un bloque del tipo dado.
func (p *Parser) terminates(block string) bool {
	t := p.cur()
	if t.Kind != token.Keyword {
		return false
	}
	switch block {
	case "IF":
		return t.Text == "ELSEIF" || t.Text == "ELSE" || p.isEnd("IF")
	case "SELECT":
		return t.Text == "CASE" || p.isEnd("SELECT")
	case "FOR":
		return t.Text == "NEXT"
	case "DO":
		return t.Text == "LOOP"
	case "WHILE":
		return t.Text == "WEND"
	case "SUB", "FUNCTION", "DEF":
		return p.isEnd(block)
	}
	return false
}

func (p *Parser) isEnd(what string) bool {
	return p.is("END") && p.peek(1).Is(what)
}

// stopForBlocks reporta si la sentencia actual cierra algún bloque abierto.
func (p *Parser) stopForBlocks() bool {
	if p.pendingNext > 0 {
		for i := len(p.blocks) - 1; i >= 0; i-- {
			if p.blocks[i] == "SINGLEIF" {
				continue
			}
			if p.blocks[i] == "FOR" {
				return true
			}
			break
		}
		// NEXT i, j sin un FOR que cierre j
		p.errAt(p.cur().Pos, "NEXT without FOR")
		p.pendingNext = 0
		p.syncLine()
		return false
	}
	for i := len(p.blocks) - 1; i >= 0; i-- {
		if p.blocks[i] == "SINGLEIF" {
			continue
		}
		if p.terminates(p.blocks[i]) {
			return true
		}
	}
	return false
}

// closesOpenBlockOnNewLine reporta si el token actual inicia una sentencia
// (en otra línea o tras ':') que cierra un bloque abierto. Ocurre cuando un
// bloque interno quedó sin cerrar: el error ya se reportó y el terminador
// pertenece al bloque externo.
func (p *Parser) closesOpenBlockOnNewLine() bool {
	if !p.cur().LineStart && !(p.i > 0 && p.toks[p.i-1].Is(":")) {
		return false
	}
	for i := len(p.blocks) - 1; i >= 0; i-- {
		if p.blocks[i] != "SINGLEIF" && p.terminates(p.blocks[i]) {
			return true
		}
	}
	return false
}

func (p *Parser) push(b string) { p.blocks = append(p.blocks, b) }
func (p *Parser) pop()          { p.blocks = p.blocks[:len(p.blocks)-1] }

func (p *Parser) inBlock(b string) bool {
	for _, x := range p.blocks {
		if x == b {
			return true
		}
	}
	return false
}

// inProc reporta si estamos dentro de SUB, FUNCTION o DEF FN.
func (p *Parser) inProc() bool {
	return p.inBlock("SUB") || p.inBlock("FUNCTION") || p.inBlock("DEF")
}

// parseBody analiza sentencias del bloque b hasta su terminador.
// Devuelve false si el bloque quedó sin cerrar (EOF o cierre de un bloque externo),
// en cuyo caso ya se reportó missing en la posición de apertura.
func (p *Parser) parseBody(b string, open token.Pos, missing string) ([]ast.Stmt, bool) {
	p.push(b)
	body := p.parseStmts()
	p.pop()
	if p.pendingNext > 0 && b == "FOR" {
		return body, true
	}
	if p.cur().Kind == token.EOF || !p.terminates(b) {
		p.errAt(open, missing)
		return body, false
	}
	return body, true
}

// parseStmts analiza una secuencia de sentencias hasta EOF o el
// terminador de un bloque abierto.
func (p *Parser) parseStmts() []ast.Stmt {
	var out []ast.Stmt
	for {
		t := p.cur()
		switch {
		case t.Kind == token.EOF:
			return out
		case t.Kind == token.Newline || t.Is(":"):
			p.next()
			continue
		case t.Kind == token.Comment:
			p.next()
			out = append(out, commentStmts(t)...)
			continue
		}
		if t.LineStart {
			if lbl := p.parseLabel(); lbl != nil {
				out = append(out, lbl)
				continue
			}
		}
		if p.stopForBlocks() {
			return out
		}
		if k := p.cur().Kind; k == token.Newline || k == token.EOF {
			continue // stopForBlocks resincronizó tras un error
		}
		out = append(out, p.parseStmtSafe()...)
	}
}

func commentStmts(t token.Token) []ast.Stmt {
	out := []ast.Stmt{&ast.Comment{P: t.Pos, Text: t.Text, Rem: t.Rem}}
	for _, m := range t.Meta {
		out = append(out, &ast.Meta{P: m.Pos, Name: m.Name, Arg: m.Arg})
	}
	return out
}

// parseLabel reconoce un número de línea o "etiqueta:" al inicio de línea.
func (p *Parser) parseLabel() ast.Stmt {
	t := p.cur()
	switch {
	case t.Kind == token.Int && !strings.HasPrefix(t.Text, "&") && t.Suffix == 0:
		p.next()
		return &ast.Label{P: t.Pos, Name: t.Text, Numeric: true}
	case t.Kind == token.Ident && t.Suffix == 0 && p.peek(1).Is(":"):
		p.next()
		p.next()
		return &ast.Label{P: t.Pos, Name: strings.ToUpper(t.Text)}
	}
	return nil
}

// parseStmtSafe analiza una sentencia con recuperación de errores.
func (p *Parser) parseStmtSafe() (out []ast.Stmt) {
	start := p.i
	func() {
		defer p.recoverStmt()
		s := p.parseStmt()
		if s != nil {
			out = append(out, s)
		}
		if p.pendingNext == 0 && !p.atEnd() && !p.closesOpenBlockOnNewLine() {
			p.fail("Expected: end-of-statement")
		}
	}()
	if p.i == start { // garantía de progreso
		p.next()
	}
	return out
}

// parseLabelRef lee una referencia a etiqueta (número de línea o nombre).
func (p *Parser) parseLabelRef() string {
	t := p.cur()
	switch {
	case t.Kind == token.Int && !strings.HasPrefix(t.Text, "&"):
		p.next()
		return t.Text
	case t.Kind == token.Ident && t.Suffix == 0:
		p.next()
		return strings.ToUpper(t.Text)
	}
	p.fail("Expected: label or line number")
	return ""
}
