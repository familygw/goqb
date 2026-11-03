// Package format normaliza líneas de código como el editor de QB 4.5:
// palabras reservadas en mayúsculas, mismo uso de mayúsculas para cada
// identificador en todo el módulo y espaciado canónico ("x = a + b",
// "PRINT a; b", "LEFT$(s$, 1)").
//
// La normalización es conservadora: si la línea tiene tokens ilegales o
// si el resultado no produce exactamente la misma secuencia de tokens,
// se devuelve la línea original.
package format

import (
	"strings"

	"goqb/lang/cp437"
	"goqb/lang/lexer"
	"goqb/lang/token"
	"goqb/vm"
)

// Names recuerda la grafía canónica de cada identificador (clave en mayúsculas, sin sufijo).
type Names map[string]string

// Learn registra los identificadores de una línea sin pisar los conocidos.
func (n Names) Learn(line string) {
	for _, t := range lexer.Tokenize(cp437.Encode(line)) {
		if t.Kind == token.Ident {
			k := key(t.Text)
			if _, ok := n[k]; !ok {
				n[k] = base(cp437.Decode([]byte(t.Text)))
			}
		}
	}
}

func key(s string) string  { return strings.ToUpper(base(s)) }
func base(s string) string { return strings.TrimRight(s, "%&!#$") }

// Palabras reservadas que funcionan como función: van pegadas al paréntesis.
var funcKeywords = map[string]bool{
	"TIMER": true, "SCREEN": true, "LEN": true, "SEEK": true, "MID$": true, "PLAY": true,
	"PEN": true, "STRIG": true, "DATE$": true, "TIME$": true,
}

var binaryOps = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "\\": true, "^": true, "=": true,
	"<": true, ">": true, "<=": true, ">=": true, "<>": true,
}

var wordOps = map[string]bool{"MOD": true, "AND": true, "OR": true, "XOR": true, "EQV": true, "IMP": true, "NOT": true}

// Line formatea una línea. names puede ser nil.
func Line(src string, names Names) string {
	raw := cp437.Encode(src)
	toks := lexer.Tokenize(raw)
	for _, t := range toks {
		if t.Kind == token.Illegal {
			return src
		}
	}
	indent := len(src) - len(strings.TrimLeft(src, " \t"))
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", indent))

	var prev token.Token
	have := false
	unaryCtx := true // el próximo + o - es unario
	for i, t := range toks {
		if t.Kind == token.EOF || t.Kind == token.Newline {
			break
		}
		text := tokenText(t, names)
		if have {
			if sp := space(prev, t, toks, i, unaryCtx, raw); sp != "" {
				b.WriteString(sp)
			}
		}
		b.WriteString(text)
		unaryCtx = startsOperand(t)
		prev, have = t, true
	}
	out := strings.TrimRight(b.String(), " ")
	if !same(raw, cp437.Encode(out)) {
		return src
	}
	return out
}

// tokenText devuelve el texto normalizado de un token (en Unicode).
func tokenText(t token.Token, names Names) string {
	switch t.Kind {
	case token.Keyword:
		return t.Text
	case token.Ident:
		txt := cp437.Decode([]byte(t.Text))
		up := strings.ToUpper(txt)
		if _, ok := vm.BuiltinIndex[up]; ok || up == "TAB" || up == "SPC" {
			return up // funciones intrínsecas: también son reservadas
		}
		if names != nil {
			k := key(txt)
			if c, ok := names[k]; ok {
				return c + txt[len(base(txt)):]
			}
			names[k] = base(txt)
		}
		return txt
	case token.String:
		var b strings.Builder
		b.WriteByte('"')
		b.WriteString(cp437.Decode([]byte(t.Text)))
		b.WriteByte('"')
		return b.String()
	case token.Comment:
		var b strings.Builder
		if t.Rem {
			b.WriteString("REM")
			if t.Text != "" {
				b.WriteByte(' ')
			}
		} else {
			b.WriteByte('\'')
		}
		b.WriteString(cp437.Decode([]byte(t.Text)))
		return b.String()
	case token.DataRaw:
		return cp437.Decode([]byte(t.Text))
	case token.Int, token.Float:
		s := strings.ToUpper(t.Text)
		if t.Suffix != 0 {
			s += string(t.Suffix)
		}
		return s
	}
	return t.Text
}

// startsOperand: después de este token, un + o - sería unario.
func startsOperand(t token.Token) bool {
	switch t.Kind {
	case token.Ident, token.Int, token.Float, token.String:
		return false
	case token.Keyword:
		return !funcKeywords[t.Text]
	case token.Op:
		return t.Text != ")"
	}
	return true
}

func space(prev, t token.Token, toks []token.Token, i int, unary bool, raw []byte) string {
	pt, ct := prev.Text, t.Text
	isOp := func(x token.Token, s string) bool { return x.Kind == token.Op && x.Text == s }

	switch {
	case t.Kind == token.Comment:
		// conservar la separación original del comentario (mínimo 1)
		gap := t.Pos.Col - (prev.Pos.Col + tokLen(prev, raw))
		if gap < 1 {
			gap = 1
		}
		return strings.Repeat(" ", gap)
	case prev.Kind == token.Int && prev.LineStart && t.Kind != token.Op:
		return " " // número de línea
	case isOp(t, ":"):
		if prev.Kind == token.Ident && prev.LineStart {
			return "" // etiqueta:
		}
		return ""
	case isOp(prev, ":"):
		return " "
	case isOp(t, ",") || isOp(t, ";") || isOp(t, ")"):
		return ""
	case isOp(prev, ",") || isOp(prev, ";"):
		return " "
	case isOp(prev, "("):
		return ""
	case isOp(t, ".") || isOp(prev, "."):
		return ""
	case isOp(prev, "#"):
		return ""
	case isOp(t, "("):
		switch {
		case prev.Kind == token.Ident:
			if i >= 2 && (toks[i-2].Is("SUB") || toks[i-2].Is("FUNCTION") || toks[i-2].Is("DECLARE")) {
				return " "
			}
			if i >= 3 && toks[i-3].Is("DECLARE") {
				return " "
			}
			return ""
		case prev.Kind == token.Keyword && funcKeywords[pt]:
			return ""
		}
		return " "
	case (isOp(prev, "-") || isOp(prev, "+")) && prevWasUnary(toks, i-1):
		return ""
	case t.Kind == token.Op && binaryOps[ct]:
		return " "
	case prev.Kind == token.Op && binaryOps[pt]:
		return " "
	}
	return " "
}

// prevWasUnary decide si el + o - en la posición j es unario.
func prevWasUnary(toks []token.Token, j int) bool {
	if j == 0 {
		return true
	}
	return startsOperand(toks[j-1])
}

func tokLen(t token.Token, raw []byte) int {
	switch t.Kind {
	case token.String:
		n := len(t.Text) + 1
		if end := t.Pos.Col - 1 + n; end < len(raw) && raw[end] == '"' {
			n++
		}
		return n
	case token.Int, token.Float, token.Ident:
		n := len(t.Text)
		if t.Suffix != 0 && t.Kind != token.Ident {
			n++
		}
		return n
	case token.Keyword:
		return len(t.Text)
	}
	return len(t.Text)
}

// same compara dos líneas por su secuencia de tokens.
func same(a, b []byte) bool {
	ta, tb := lexer.Tokenize(a), lexer.Tokenize(b)
	if len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		x, y := ta[i], tb[i]
		if x.Kind != y.Kind || x.Suffix != y.Suffix {
			return false
		}
		switch x.Kind {
		case token.Ident:
			if !strings.EqualFold(x.Text, y.Text) {
				return false
			}
		case token.Comment, token.DataRaw:
			if strings.TrimSpace(x.Text) != strings.TrimSpace(y.Text) {
				return false
			}
		case token.Int, token.Float:
			if !strings.EqualFold(x.Text, y.Text) {
				return false
			}
		default:
			if x.Text != y.Text {
				return false
			}
		}
	}
	return true
}
