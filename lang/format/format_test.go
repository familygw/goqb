package format

import "testing"

func TestLine(t *testing.T) {
	names := Names{}
	names.Learn("DIM Contador AS INTEGER")
	cases := []struct{ in, want string }{
		{"print \"hola\";x", `PRINT "hola"; x`},
		{"x=a+b*-c", "x = a + b * -c"},
		{"if x>=1 then y=2 else y=-1", "IF x >= 1 THEN y = 2 ELSE y = -1"},
		{"s$=left$(a$,3)+mid$(a$,2)", "s$ = LEFT$(a$, 3) + MID$(a$, 2)"},
		{"contador=contador+1", "Contador = Contador + 1"},
		{"for i=1 to 10 step -2", "FOR i = 1 TO 10 STEP -2"},
		{"  print#1,a;b", "  PRINT #1, a; b"},
		{"10 cls:locate 1,1", "10 CLS: LOCATE 1, 1"},
		{"inicio:", "inicio:"},
		{"x = 1   ' comentario  ", "x = 1   ' comentario"},
		{"rem   nota", "REM   nota"},
		{"data 1,  2, hola mundo", "DATA 1,  2, hola mundo"},
		{"sub dibujar(a,b) static", "SUB dibujar (a, b) STATIC"},
		{"declare function f%(x)", "DECLARE FUNCTION f% (x)"},
		{"call dibujar(1,2)", "CALL dibujar(1, 2)"},
		{"t=timer:x=len(a$)", "t = TIMER: x = LEN(a$)"},
		{"p.x=q(1).y", "p.x = q(1).y"},
		{"a=not b and -1", "a = NOT b AND -1"},
		{"x = &hff + 1.5e3", "x = &HFF + 1.5E3"},
		{"?\"ñandú\"", `PRINT "ñandú"`},
		{"a = b @ c", "a = b @ c"}, // token ilegal: sin cambios
		{"print (a)", "PRINT (a)"},
	}
	for _, c := range cases {
		if got := Line(c.in, names); got != c.want {
			t.Errorf("%q\n got: %q\nwant: %q", c.in, got, c.want)
		}
	}
}

func TestDefTypeLetters(t *testing.T) {
	names := Names{}
	names.Learn("a = 1: z = 2") // variables a y z en minúsculas
	for in, want := range map[string]string{
		"defint a-z":             "DEFINT A-Z",
		"DEFINT A-Z":             "DEFINT A-Z",
		"defint a - c, x-z":      "DEFINT A-C, X-Z",
		"DEFSTR s: s = \"hola\"": "DEFSTR S: s = \"hola\"",
		"10 deflng i-n":          "10 DEFLNG I-N",
		"x = a - z":              "x = a - z",
	} {
		if got := Line(in, names); got != want {
			t.Errorf("Line(%q) = %q, se esperaba %q", in, got, want)
		}
	}
	n := Names{}
	n.Learn("DEFINT A-Z")
	if len(n) != 0 {
		t.Errorf("las letras de DEFINT no son variables: %v", n)
	}
}
