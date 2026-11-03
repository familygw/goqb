package lexer

import (
	"strings"
	"testing"

	"goqb/lang/token"
)

// render devuelve los tokens como "KIND:texto" separados por espacios.
func render(src string) string {
	var parts []string
	for _, t := range Tokenize([]byte(src)) {
		switch t.Kind {
		case token.EOF:
			return strings.Join(parts, " ")
		case token.Newline:
			parts = append(parts, "NL")
		default:
			s := t.Kind.String() + ":" + t.Text
			if t.Suffix != 0 && t.Kind != token.Ident {
				s += string(t.Suffix)
			}
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func TestTokens(t *testing.T) {
	cases := []struct{ src, want string }{
		{`PRINT "hola"; x%`, `KEYWORD:PRINT STRING:hola OP:; IDENT:x%`},
		{`print#1, a$`, `KEYWORD:PRINT OP:# INT:1 OP:, IDENT:a$`},
		{`GET#2,,r`, `KEYWORD:GET OP:# INT:2 OP:, OP:, IDENT:r`},
		{`x = &HFF + &O17 + &777 + &HFFFF&`, `IDENT:x OP:= INT:&HFF OP:+ INT:&O17 OP:+ INT:&777 OP:+ INT:&HFFFF&`},
		{`1.5D3 2.5E-2 .5 10& 3! 7#`, `FLOAT:1.5D3 FLOAT:2.5E-2 FLOAT:.5 INT:10& FLOAT:3! FLOAT:7#`},
		{`a <= b >< c => d =< e`, `IDENT:a OP:<= IDENT:b OP:<> IDENT:c OP:>= IDENT:d OP:<= IDENT:e`},
		{`? x`, `KEYWORD:PRINT IDENT:x`},
		{`MID$(a$, 1) = DATE$`, `KEYWORD:MID$ OP:( IDENT:a$ OP:, INT:1 OP:) OP:= KEYWORD:DATE$`},
		{`s$ = STRING$(3, 42) + INPUT$(1)`, `IDENT:s$ OP:= IDENT:STRING$ OP:( INT:3 OP:, INT:42 OP:) OP:+ IDENT:INPUT$ OP:( INT:1 OP:)`},
		{`j.pos.x = 1`, `IDENT:j.pos.x OP:= INT:1`},
		{`x = 1 ' comentario`, `IDENT:x OP:= INT:1 COMMENT: comentario`},
		{`REM todo esto`, `COMMENT:todo esto`},
		{`DATA 1, "a,b", hola mundo: PRINT`, `KEYWORD:DATA DATA:1, "a,b", hola mundo OP:: KEYWORD:PRINT`},
		{"A\r\nB\rC", `IDENT:A NL IDENT:B NL IDENT:C`},
		{`"sin cerrar`, `STRING:sin cerrar`},
		{"X\x1aBASURA", `IDENT:X`},
	}
	for _, c := range cases {
		if got := render(c.src); got != c.want {
			t.Errorf("%q\n got: %s\nwant: %s", c.src, got, c.want)
		}
	}
}

func TestLineStart(t *testing.T) {
	toks := Tokenize([]byte("10 PRINT\nfin: END"))
	if !toks[0].LineStart || toks[1].LineStart {
		t.Errorf("LineStart incorrecto en la primera línea")
	}
	if !toks[3].LineStart || toks[3].Text != "fin" {
		t.Errorf("LineStart incorrecto en la segunda línea: %+v", toks[3])
	}
}

func TestMeta(t *testing.T) {
	toks := Tokenize([]byte("'$INCLUDE: 'qb.bi'\nREM $DYNAMIC\n' $STATIC $DYNAMIC\n' $NOPE"))
	var metas []string
	for _, tk := range toks {
		for _, m := range tk.Meta {
			metas = append(metas, m.Name+"="+m.Arg)
		}
	}
	want := "INCLUDE=qb.bi DYNAMIC= STATIC= DYNAMIC="
	if got := strings.Join(metas, " "); got != want {
		t.Errorf("metacomandos: got %q want %q", got, want)
	}
}

func TestPositions(t *testing.T) {
	toks := Tokenize([]byte("A = 1\n  PRINT"))
	p := toks[4].Pos // PRINT
	if p.Line != 2 || p.Col != 3 {
		t.Errorf("posición de PRINT: %v", p)
	}
}
