package ide

import (
	"strings"

	"github.com/gdamore/tcell/v2"

	"goqb/lang/cp437"
	"goqb/lang/lexer"
	"goqb/lang/token"
	"goqb/runtime/termrun"
	"goqb/vm"
)

// Colores del coloreo de sintaxis (sobre el fondo azul del editor).
var (
	hlKeyword = termrun.Style(15, 1)
	hlBuiltin = termrun.Style(11, 1)
	hlString  = termrun.Style(14, 1)
	hlNumber  = termrun.Style(13, 1)
	hlComment = termrun.Style(3, 1)
	hlMeta    = termrun.Style(12, 1)
	hlLabel   = termrun.Style(12, 1)
	hlText    = termrun.Style(7, 1)
)

// highlight devuelve un estilo por runa de la línea. QB no tiene
// construcciones multilínea, así que cada línea se colorea sola.
// Encode a CP437 produce un byte por runa: las columnas coinciden.
func highlight(line string) []tcell.Style {
	n := len([]rune(line))
	out := make([]tcell.Style, n)
	for i := range out {
		out[i] = hlText
	}
	toks := lexer.Tokenize(cp437.Encode(line))
	for i, t := range toks {
		if t.Kind == token.EOF || t.Kind == token.Newline {
			break
		}
		start := t.Pos.Col - 1
		end := n
		if i+1 < len(toks) && toks[i+1].Kind != token.EOF && toks[i+1].Kind != token.Newline {
			end = toks[i+1].Pos.Col - 1
		}
		var st tcell.Style
		switch t.Kind {
		case token.Keyword:
			st = hlKeyword
		case token.Ident:
			up := strings.ToUpper(t.Text)
			if _, ok := vm.BuiltinIndex[up]; ok || up == "TAB" || up == "SPC" {
				st = hlBuiltin
			} else if t.LineStart && i+1 < len(toks) && toks[i+1].Is(":") {
				st = hlLabel // etiqueta:
			} else {
				continue
			}
		case token.String:
			st = hlString
		case token.Int, token.Float:
			st = hlNumber
			if t.LineStart {
				st = hlLabel // número de línea
			}
		case token.Comment:
			st = hlComment
			if len(t.Meta) > 0 {
				st = hlMeta
			}
			end = n
		case token.DataRaw:
			st = hlString
		default:
			continue
		}
		for j := start; j < end && j < n; j++ {
			if j >= 0 {
				out[j] = st
			}
		}
	}
	return out
}
