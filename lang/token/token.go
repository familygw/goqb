// Package token define los tokens de QuickBASIC 4.5.
package token

import (
	"fmt"
	"sort"
	"strings"
)

// Kind clasifica un token.
type Kind int

const (
	EOF     Kind = iota
	Newline      // fin de línea física
	Ident        // identificador, con sufijo de tipo opcional (% & ! # $)
	Int          // literal entero (decimal, &H, &O)
	Float        // literal de punto flotante
	String       // literal "..."
	Keyword      // palabra reservada (Text en mayúsculas)
	Op           // operador o puntuación
	Comment      // ' ... o REM ...; puede contener metacomandos
	DataRaw      // texto crudo después de DATA hasta ':' o fin de línea
	Illegal      // carácter no reconocido
)

func (k Kind) String() string {
	switch k {
	case EOF:
		return "EOF"
	case Newline:
		return "NEWLINE"
	case Ident:
		return "IDENT"
	case Int:
		return "INT"
	case Float:
		return "FLOAT"
	case String:
		return "STRING"
	case Keyword:
		return "KEYWORD"
	case Op:
		return "OP"
	case Comment:
		return "COMMENT"
	case DataRaw:
		return "DATA"
	case Illegal:
		return "ILLEGAL"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Pos es una posición 1-based en el archivo fuente.
type Pos struct {
	Line int
	Col  int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Meta es un metacomando embebido en un comentario ('$INCLUDE: 'x.bi').
type Meta struct {
	Name string // INCLUDE, DYNAMIC, STATIC (mayúsculas, sin $)
	Arg  string // argumento de $INCLUDE
	Pos  Pos
}

// Token es la unidad léxica.
type Token struct {
	Kind Kind
	// Text es el texto tal como aparece en la fuente, salvo:
	//   Keyword: en mayúsculas.
	//   String:  sin comillas.
	//   Comment: sin la marca (' o REM).
	Text string
	Pos  Pos
	// Suffix es el sufijo de tipo de un Ident o literal numérico (0 si no tiene).
	Suffix byte
	// LineStart indica que es el primer token de la línea física.
	LineStart bool
	// Rem indica que un Comment proviene de REM (y no de ').
	Rem  bool
	Meta []Meta
}

// Upper devuelve el texto en mayúsculas (útil para identificadores).
func (t Token) Upper() string { return strings.ToUpper(t.Text) }

// Is reporta si el token es la palabra clave u operador dado.
func (t Token) Is(s string) bool {
	return (t.Kind == Keyword || t.Kind == Op) && t.Text == s
}

func (t Token) String() string {
	switch t.Kind {
	case EOF, Newline:
		return t.Kind.String()
	case String:
		return fmt.Sprintf("%s %q", t.Kind, t.Text)
	}
	return fmt.Sprintf("%s %s", t.Kind, t.Text)
}

// keywords son las palabras reservadas de QuickBASIC 4.5 que el lexer
// emite como Keyword. Las funciones intrínsecas comunes (LEFT$, CHR$, ABS,
// INKEY$...) se emiten como Ident y se resuelven en el análisis semántico,
// salvo las que además tienen sintaxis de sentencia (TIMER, SCREEN, PLAY...).
var keywords = map[string]bool{}

func init() {
	for _, k := range strings.Fields(`
ACCESS ALIAS AND ANY APPEND AS BASE BEEP BINARY BLOAD BSAVE BYVAL
CALL CALLS CASE CDECL CHAIN CHDIR CIRCLE CLEAR CLOSE CLS COLOR COM COMMON
CONST DATA DATE$ DECLARE DEF DEFDBL DEFINT DEFLNG DEFSNG DEFSTR DIM DO
DOUBLE DRAW ELSE ELSEIF END ENVIRON EQV ERASE ERROR EXIT FIELD FILES FOR
FUNCTION GET GOSUB GOTO IF IMP INPUT INTEGER IOCTL IS KEY KILL LEN LET
LINE LOCATE LOCK LONG LOOP LPRINT LSET MID$ MKDIR MOD NAME NEXT NOT OFF ON
OPEN OPTION OR OUT OUTPUT PAINT PALETTE PCOPY PEN PLAY POKE PRESET PRINT
PSET PUT RANDOM RANDOMIZE READ REDIM REM RESET RESTORE RESUME RETURN
RMDIR RSET RUN SCREEN SEEK SEG SELECT SHARED SHELL SINGLE SLEEP SOUND
STATIC STEP STOP STRIG STRING SUB SWAP SYSTEM THEN TIME$ TIMER TO TROFF
TRON TYPE UEVENT UNLOCK UNTIL USING VIEW WAIT WEND WHILE WIDTH WINDOW
WRITE XOR`) {
		keywords[k] = true
	}
}

// Keywords devuelve las palabras reservadas, ordenadas.
func Keywords() []string {
	out := make([]string, 0, len(keywords))
	for k := range keywords {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsKeyword reporta si s (en mayúsculas) es palabra reservada.
func IsKeyword(s string) bool { return keywords[s] }

// IsTypeSuffix reporta si c es un sufijo de tipo de QB.
func IsTypeSuffix(c byte) bool {
	return c == '%' || c == '&' || c == '!' || c == '#' || c == '$'
}
