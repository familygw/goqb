package parser

import (
	"math"
	"strconv"
	"strings"

	"goqb/lang/ast"
	"goqb/lang/token"
)

// Precedencia de QB 4.5, de menor a mayor:
//   IMP < EQV < XOR < OR < AND < NOT < relacionales < + - < MOD < \ < * / < negación < ^
// ^ es asociativo a izquierda (2^3^2 = 64) y liga más que el menos unario (-2^2 = -4).

var binLevels = [][]string{
	{"IMP"},
	{"EQV"},
	{"XOR"},
	{"OR"},
	{"AND"},
	nil, // NOT (unario), manejado aparte
	{"=", "<>", "<", ">", "<=", ">="},
	{"+", "-"},
	{"MOD"},
	{"\\"},
	{"*", "/"},
}

const notLevel = 5

func (p *Parser) parseExpr() ast.Expr { return p.parseLevel(0) }

func (p *Parser) parseLevel(lvl int) ast.Expr {
	if lvl == len(binLevels) {
		return p.parseUnary()
	}
	if lvl == notLevel {
		if t := p.cur(); t.Is("NOT") {
			p.next()
			return &ast.Unary{P: t.Pos, Op: "NOT", X: p.parseLevel(notLevel)}
		}
		return p.parseLevel(lvl + 1)
	}
	left := p.parseLevel(lvl + 1)
	for {
		t := p.cur()
		if !matchOp(t, binLevels[lvl]) {
			return left
		}
		p.next()
		right := p.parseLevel(lvl + 1) // a AND NOT b: el nivel siguiente acepta NOT
		left = &ast.Binary{P: t.Pos, Op: t.Text, L: left, R: right}
	}
}

func matchOp(t token.Token, ops []string) bool {
	if t.Kind != token.Op && t.Kind != token.Keyword {
		return false
	}
	for _, o := range ops {
		if t.Text == o {
			return true
		}
	}
	return false
}

// parseUnary: negación (menor precedencia que ^, mayor que * /).
func (p *Parser) parseUnary() ast.Expr {
	t := p.cur()
	if t.Is("-") || t.Is("+") {
		p.next()
		return &ast.Unary{P: t.Pos, Op: t.Text, X: p.parseUnary()}
	}
	if t.Is("NOT") { // a = NOT b: QB lo acepta en posición de operando
		p.next()
		return &ast.Unary{P: t.Pos, Op: "NOT", X: p.parseLevel(notLevel)}
	}
	return p.parsePower()
}

func (p *Parser) parsePower() ast.Expr {
	left := p.parsePostfix()
	for p.is("^") {
		t := p.next()
		var right ast.Expr
		if u := p.cur(); u.Is("-") || u.Is("+") { // 2 ^ -1
			p.next()
			right = &ast.Unary{P: u.Pos, Op: u.Text, X: p.parsePostfix()}
		} else {
			right = p.parsePostfix()
		}
		left = &ast.Binary{P: t.Pos, Op: "^", L: left, R: right}
	}
	return left
}

// parsePostfix: primario seguido de .campo (sólo tras paréntesis).
func (p *Parser) parsePostfix() ast.Expr {
	x := p.parsePrimary()
	for p.is(".") {
		if _, ok := x.(*ast.Call); !ok {
			break
		}
		dot := p.next()
		f := p.cur()
		if f.Kind != token.Ident && f.Kind != token.Keyword { // a(1).name: NAME es reservada
			p.fail("Expected: identifier")
		}
		p.next()
		x = &ast.FieldAccess{P: dot.Pos, X: x, Field: f.Text}
	}
	return x
}

// Palabras reservadas que también son funciones en expresiones.
var keywordFuncs = map[string]bool{
	"TIMER": true, "DATE$": true, "TIME$": true, "SCREEN": true, "PLAY": true,
	"PEN": true, "STRIG": true, "LEN": true, "SEEK": true, "MID$": true,
}

func (p *Parser) parsePrimary() ast.Expr {
	t := p.cur()
	switch t.Kind {
	case token.Int, token.Float:
		p.next()
		return p.numberLit(t)
	case token.String:
		p.next()
		return &ast.StringLit{P: t.Pos, Value: t.Text}
	case token.Ident:
		p.next()
		if p.is("(") {
			args, empty := p.parseCallArgs()
			return &ast.Call{P: t.Pos, Name: t.Text, Suffix: t.Suffix, Args: args, Empty: empty}
		}
		return &ast.Ident{P: t.Pos, Name: t.Text, Suffix: t.Suffix}
	case token.Keyword:
		if keywordFuncs[t.Text] {
			p.next()
			c := &ast.Call{P: t.Pos, Name: t.Text, Keyword: true}
			if strings.HasSuffix(t.Text, "$") {
				c.Suffix = '$'
			}
			if p.is("(") {
				c.Args, c.Empty = p.parseCallArgs()
			}
			return c
		}
	case token.Op:
		if t.Is("(") {
			p.next()
			x := p.parseExpr()
			p.expect(")")
			return &ast.Paren{P: t.Pos, X: x}
		}
	}
	p.fail("Expected: expression")
	return nil
}

// parseCallArgs analiza "(args)". Admite a() (array completo), #n,
// y los prefijos BYVAL/SEG de CALL.
func (p *Parser) parseCallArgs() (args []ast.Expr, empty bool) {
	p.expect("(")
	if p.accept(")") {
		return nil, true
	}
	for {
		args = append(args, p.parseArg())
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	return args, false
}

// parseArg: expresión, #n, BYVAL x, SEG x.
func (p *Parser) parseArg() ast.Expr {
	t := p.cur()
	switch {
	case t.Is("#"):
		p.next()
		return &ast.FileNum{P: t.Pos, X: p.parseExpr(), Hash: true}
	case t.Is("BYVAL"), t.Is("SEG"):
		p.next()
		return &ast.Unary{P: t.Pos, Op: t.Text, X: p.parseExpr()}
	}
	return p.parseExpr()
}

// parseFileNum lee [#]n.
func (p *Parser) parseFileNum() *ast.FileNum {
	t := p.cur()
	hash := p.accept("#")
	return &ast.FileNum{P: t.Pos, X: p.parseExpr(), Hash: hash}
}

// parseArgList lee una lista de argumentos separados por coma donde
// cualquiera puede omitirse: "COLOR , 1" → [nil, 1].
func (p *Parser) parseArgList() []ast.Expr {
	var args []ast.Expr
	if p.atEnd() {
		return nil
	}
	for {
		if p.is(",") {
			args = append(args, nil)
		} else {
			args = append(args, p.parseArg())
		}
		if !p.accept(",") {
			return args
		}
		if p.atEnd() { // coma final
			args = append(args, nil)
			return args
		}
	}
}

// parseExprList lee expr {, expr} (sin omisiones).
func (p *Parser) parseExprList() []ast.Expr {
	var out []ast.Expr
	for {
		out = append(out, p.parseArg())
		if !p.accept(",") {
			return out
		}
	}
}

func (p *Parser) numberLit(t token.Token) *ast.NumberLit {
	n := &ast.NumberLit{P: t.Pos, Text: t.Text, Suffix: t.Suffix, Radix: 10}
	switch {
	case strings.HasPrefix(t.Text, "&H"):
		n.Radix = 16
		v, err := strconv.ParseUint(t.Text[2:], 16, 64)
		if err != nil || len(t.Text) == 2 {
			p.errAt(t.Pos, "Overflow")
		}
		n.Int = int64(v)
	case strings.HasPrefix(t.Text, "&"):
		n.Radix = 8
		s := strings.TrimPrefix(strings.TrimPrefix(t.Text, "&O"), "&")
		v, err := strconv.ParseUint(s, 8, 64)
		if err != nil || s == "" {
			p.errAt(t.Pos, "Overflow")
		}
		n.Int = int64(v)
	case t.Kind == token.Float:
		n.IsFloat = true
		s := strings.Map(func(r rune) rune {
			if r == 'D' || r == 'd' {
				return 'E'
			}
			return r
		}, t.Text)
		v, err := strconv.ParseFloat(s, 64)
		if err != nil && !math.IsInf(v, 0) {
			p.errAt(t.Pos, "Overflow")
		}
		n.Float = v
	default:
		v, err := strconv.ParseInt(t.Text, 10, 64)
		if err != nil {
			// demasiado grande para entero: QB lo toma como DOUBLE
			f, _ := strconv.ParseFloat(t.Text, 64)
			n.IsFloat, n.Float = true, f
		} else {
			n.Int = v
		}
	}
	return n
}
