// Package lexer convierte código fuente QuickBASIC 4.5 en tokens.
//
// El lexer es orientado a líneas, como QB: cada fin de línea produce un
// token Newline. No hay continuación de línea (QB 4.5 no la tiene).
// El texto se trata como bytes (las fuentes DOS suelen estar en CP437);
// la decodificación para mostrar es responsabilidad del IDE.
package lexer

import (
	"strings"

	"goqb/lang/token"
)

// Lexer produce tokens bajo demanda.
type Lexer struct {
	src       []byte
	off       int
	line, col int
	lineStart bool // el próximo token es el primero de la línea
	afterData bool // el próximo token es el texto crudo de DATA
}

// New crea un lexer para src.
func New(src []byte) *Lexer {
	return &Lexer{src: src, line: 1, col: 1, lineStart: true}
}

// Tokenize devuelve todos los tokens de src, terminando en EOF.
func Tokenize(src []byte) []token.Token {
	lx := New(src)
	var out []token.Token
	for {
		t := lx.Next()
		out = append(out, t)
		if t.Kind == token.EOF {
			return out
		}
	}
}

func (l *Lexer) peek(n int) byte {
	if l.off+n < len(l.src) {
		return l.src[l.off+n]
	}
	return 0
}

func (l *Lexer) advance() byte {
	c := l.src[l.off]
	l.off++
	l.col++
	return c
}

func (l *Lexer) eof() bool { return l.off >= len(l.src) }

// atEOL reporta si la posición actual es fin de línea o de archivo.
func (l *Lexer) atEOL() bool {
	if l.eof() {
		return true
	}
	c := l.src[l.off]
	return c == '\n' || c == '\r' || c == 0x1A
}

func (l *Lexer) skipBlanks() {
	for !l.eof() {
		c := l.src[l.off]
		if c == ' ' || c == '\t' || c == '\f' || c == 0 {
			l.advance()
			continue
		}
		break
	}
}

// Next devuelve el siguiente token.
func (l *Lexer) Next() token.Token {
	if l.afterData {
		l.afterData = false
		return l.scanDataRaw()
	}
	l.skipBlanks()
	start := token.Pos{Line: l.line, Col: l.col}
	ls := l.lineStart
	mk := func(k token.Kind, text string) token.Token {
		l.lineStart = false
		return token.Token{Kind: k, Text: text, Pos: start, LineStart: ls}
	}

	if l.eof() || l.src[l.off] == 0x1A { // 0x1A: marca EOF de DOS
		l.off = len(l.src)
		return token.Token{Kind: token.EOF, Pos: start, LineStart: ls}
	}

	c := l.src[l.off]
	switch {
	case c == '\r' || c == '\n':
		l.advance()
		if c == '\r' && l.peek(0) == '\n' {
			l.off++
		}
		l.line++
		l.col = 1
		l.lineStart = true
		return token.Token{Kind: token.Newline, Pos: start, LineStart: ls}

	case c == '\'':
		l.advance()
		text := l.restOfLine()
		t := mk(token.Comment, text)
		t.Meta = parseMeta(text, start)
		return t

	case c == '"':
		l.advance()
		s := l.off
		for !l.atEOL() && l.src[l.off] != '"' {
			l.advance()
		}
		text := string(l.src[s:l.off])
		if !l.atEOL() {
			l.advance() // comilla de cierre; QB tolera strings sin cerrar al final de la línea
		}
		return mk(token.String, text)

	case isDigit(c) || (c == '.' && isDigit(l.peek(1))):
		return l.scanNumber(start, ls)

	case c == '&' && (isHexPrefix(l.peek(1)) || isOctDigit(l.peek(1))):
		return l.scanRadix(start, ls)

	case isLetter(c):
		return l.scanWord(start, ls)
	}

	// Operadores y puntuación.
	l.advance()
	switch c {
	case '<':
		if n := l.peek(0); n == '=' || n == '>' {
			l.advance()
			return mk(token.Op, "<"+string(n))
		}
		return mk(token.Op, "<")
	case '>':
		switch l.peek(0) {
		case '=':
			l.advance()
			return mk(token.Op, ">=")
		case '<': // >< es sinónimo de <> en QB
			l.advance()
			return mk(token.Op, "<>")
		}
		return mk(token.Op, ">")
	case '=':
		switch l.peek(0) {
		case '<': // =< → <=
			l.advance()
			return mk(token.Op, "<=")
		case '>': // => → >=
			l.advance()
			return mk(token.Op, ">=")
		}
		return mk(token.Op, "=")
	case '?': // abreviatura de PRINT
		return mk(token.Keyword, "PRINT")
	case '+', '-', '*', '/', '\\', '^', '(', ')', ',', ';', ':', '#', '.':
		return mk(token.Op, string(c))
	}
	return mk(token.Illegal, string(c))
}

func (l *Lexer) restOfLine() string {
	s := l.off
	for !l.atEOL() {
		l.advance()
	}
	return string(l.src[s:l.off])
}

func (l *Lexer) scanWord(start token.Pos, ls bool) token.Token {
	s := l.off
	for !l.eof() && (isLetter(l.src[l.off]) || isDigit(l.src[l.off]) || l.src[l.off] == '.') {
		l.advance()
	}
	word := string(l.src[s:l.off])
	up := strings.ToUpper(word)
	l.lineStart = false

	// Sufijo de tipo. Las palabras reservadas sólo aceptan '$' cuando la
	// forma con $ es en sí reservada (MID$, DATE$, TIME$); así PRINT#1 y
	// GET#1 se separan correctamente.
	if !l.eof() && token.IsTypeSuffix(l.src[l.off]) {
		sfx := l.src[l.off]
		switch {
		case token.IsKeyword(up + string(sfx)):
			l.advance()
			return token.Token{Kind: token.Keyword, Text: up + string(sfx), Pos: start, LineStart: ls}
		case token.IsKeyword(up) && sfx == '$':
			// función intrínseca cuyo nombre base es reservado: STRING$, INPUT$, IOCTL$
			l.advance()
			return token.Token{Kind: token.Ident, Text: word + "$", Pos: start, LineStart: ls, Suffix: '$'}
		case token.IsKeyword(up):
			// palabra reservada seguida de puntuación: no consumir
		case sfx == '&' && isHexPrefix(l.peek(1)):
			// A&H10 no existe en QB; dejamos & para el literal
		default:
			l.advance()
			return token.Token{Kind: token.Ident, Text: word + string(sfx), Pos: start, LineStart: ls, Suffix: sfx}
		}
	}

	if token.IsKeyword(up) {
		switch up {
		case "REM":
			if !l.atEOL() {
				l.advance() // separador después de REM
			}
			text := l.restOfLine()
			return token.Token{Kind: token.Comment, Text: text, Pos: start, LineStart: ls, Rem: true, Meta: parseMeta(text, start)}
		case "DATA":
			l.afterData = true
		}
		return token.Token{Kind: token.Keyword, Text: up, Pos: start, LineStart: ls}
	}
	return token.Token{Kind: token.Ident, Text: word, Pos: start, LineStart: ls}
}

func (l *Lexer) scanNumber(start token.Pos, ls bool) token.Token {
	s := l.off
	kind := token.Int
	for !l.eof() && isDigit(l.src[l.off]) {
		l.advance()
	}
	if l.peek(0) == '.' {
		kind = token.Float
		l.advance()
		for !l.eof() && isDigit(l.src[l.off]) {
			l.advance()
		}
	}
	if e := l.peek(0); e == 'E' || e == 'e' || e == 'D' || e == 'd' {
		n := 1
		if l.peek(1) == '+' || l.peek(1) == '-' {
			n = 2
		}
		if isDigit(l.peek(n)) {
			kind = token.Float
			for i := 0; i < n; i++ {
				l.advance()
			}
			for !l.eof() && isDigit(l.src[l.off]) {
				l.advance()
			}
		}
	}
	text := string(l.src[s:l.off])
	var sfx byte
	if c := l.peek(0); c == '%' || c == '&' || c == '!' || c == '#' {
		// "#" como sufijo sólo si no inicia un número de archivo (PRINT 1 #2 no es válido igualmente)
		sfx = c
		l.advance()
		if c == '!' || c == '#' {
			kind = token.Float
		}
	}
	l.lineStart = false
	return token.Token{Kind: kind, Text: text, Pos: start, LineStart: ls, Suffix: sfx}
}

func (l *Lexer) scanRadix(start token.Pos, ls bool) token.Token {
	s := l.off
	l.advance() // &
	switch l.peek(0) {
	case 'H', 'h':
		l.advance()
		for !l.eof() && isHexDigit(l.src[l.off]) {
			l.advance()
		}
	case 'O', 'o':
		l.advance()
		fallthrough
	default:
		for !l.eof() && isOctDigit(l.src[l.off]) {
			l.advance()
		}
	}
	text := string(l.src[s:l.off])
	var sfx byte
	if c := l.peek(0); c == '%' || c == '&' {
		sfx = c
		l.advance()
	}
	l.lineStart = false
	return token.Token{Kind: token.Int, Text: strings.ToUpper(text), Pos: start, LineStart: ls, Suffix: sfx}
}

// scanDataRaw toma el texto de DATA hasta ':' (fuera de comillas) o fin de línea.
func (l *Lexer) scanDataRaw() token.Token {
	l.skipBlanks()
	start := token.Pos{Line: l.line, Col: l.col}
	s := l.off
	inq := false
	for !l.atEOL() {
		c := l.src[l.off]
		if c == '"' {
			inq = !inq
		} else if c == ':' && !inq {
			break
		}
		l.advance()
	}
	l.lineStart = false
	return token.Token{Kind: token.DataRaw, Text: strings.TrimRight(string(l.src[s:l.off]), " \t"), Pos: start}
}

// parseMeta extrae metacomandos de un comentario: $INCLUDE: 'archivo', $DYNAMIC, $STATIC.
func parseMeta(text string, pos token.Pos) []token.Meta {
	var out []token.Meta
	i := 0
	skip := func() {
		for i < len(text) && (text[i] == ' ' || text[i] == '\t') {
			i++
		}
	}
	skip()
	for i < len(text) && text[i] == '$' {
		i++
		s := i
		for i < len(text) && isLetter(text[i]) {
			i++
		}
		name := strings.ToUpper(text[s:i])
		m := token.Meta{Name: name, Pos: pos}
		switch name {
		case "INCLUDE":
			skip()
			if i < len(text) && text[i] == ':' {
				i++
			}
			skip()
			if i < len(text) && text[i] == '\'' {
				i++
				s := i
				for i < len(text) && text[i] != '\'' {
					i++
				}
				m.Arg = strings.TrimSpace(text[s:i])
				if i < len(text) {
					i++
				}
			}
		case "DYNAMIC", "STATIC":
		default:
			return out // no es un metacomando: el resto es comentario
		}
		out = append(out, m)
		skip()
	}
	return out
}

func isLetter(c byte) bool   { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isOctDigit(c byte) bool { return c >= '0' && c <= '7' }
func isHexDigit(c byte) bool {
	return isDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
func isHexPrefix(c byte) bool { return c == 'H' || c == 'h' || c == 'O' || c == 'o' }
