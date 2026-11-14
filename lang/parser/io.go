package parser

import (
	"strings"

	"goqb/lang/ast"
	"goqb/lang/token"
)

// ------------------------------------------------------------- consola y archivos

func (p *Parser) parsePrint() ast.Stmt {
	t := p.next()
	s := &ast.Print{P: t.Pos, LPrint: t.Text == "LPRINT"}
	if !s.LPrint && p.is("#") {
		s.File = p.parseFileNum()
		p.expect(",")
	}
	if p.accept("USING") {
		s.Using = p.parseExpr()
		p.expect(";")
	}
	for !p.atEnd() {
		if c := p.cur(); c.Is(";") || c.Is(",") {
			p.next()
			s.Items = append(s.Items, ast.PrintItem{Sep: c.Text[0]})
			continue
		}
		it := ast.PrintItem{X: p.parseExpr()}
		if c := p.cur(); c.Is(";") || c.Is(",") {
			p.next()
			it.Sep = c.Text[0]
		}
		s.Items = append(s.Items, it)
	}
	return s
}

func (p *Parser) parseWrite() ast.Stmt {
	t := p.next()
	s := &ast.Write{P: t.Pos}
	if p.is("#") {
		s.File = p.parseFileNum()
		if !p.accept(",") {
			p.expectEnd()
			return s
		}
	}
	if !p.atEnd() {
		s.Args = p.parseExprList()
	}
	return s
}

// parseInput analiza INPUT y LINE INPUT (LINE ya consumido si line).
func (p *Parser) parseInput(line bool) ast.Stmt {
	t := p.next() // INPUT
	s := &ast.Input{P: t.Pos, Line: line}
	if p.is("#") {
		s.File = p.parseFileNum()
		p.expect(",")
	} else {
		s.NoNewline = p.accept(";")
		if c := p.cur(); c.Kind == token.String && (p.peek(1).Is(";") || p.peek(1).Is(",")) {
			p.next()
			s.Prompt = &ast.StringLit{P: c.Pos, Value: c.Text}
			s.PromptSep = p.next().Text[0]
		}
	}
	for {
		s.Vars = append(s.Vars, p.parseLValue())
		if line || !p.accept(",") {
			return s
		}
	}
}

var openModes = map[string]bool{"INPUT": true, "OUTPUT": true, "APPEND": true, "RANDOM": true, "BINARY": true}

func (p *Parser) parseOpen() ast.Stmt {
	t := p.next()
	s := &ast.Open{P: t.Pos}
	first := p.parseExpr()
	if p.accept(",") { // OPEN "O", #1, "archivo"[, largo]
		s.Legacy, s.LegacyMode = true, first
		s.Num = p.parseFileNum()
		p.expect(",")
		s.File = p.parseExpr()
		if p.accept(",") {
			s.Len = p.parseExpr()
		}
		return s
	}
	s.File = first
	if p.accept("FOR") {
		m := p.cur()
		if m.Kind != token.Keyword || !openModes[m.Text] {
			p.fail("Expected: INPUT, OUTPUT, APPEND, RANDOM or BINARY")
		}
		p.next()
		s.Mode = m.Text
	}
	if p.accept("ACCESS") {
		s.Access = p.readWriteSpec()
	}
	switch {
	case p.accept("SHARED"):
		s.Lock = "SHARED"
	case p.accept("LOCK"):
		s.Lock = "LOCK " + p.readWriteSpec()
	}
	p.expect("AS")
	s.Num = p.parseFileNum()
	if p.accept("LEN") {
		p.expect("=")
		s.Len = p.parseExpr()
	}
	return s
}

// readWriteSpec lee READ | WRITE | READ WRITE.
func (p *Parser) readWriteSpec() string {
	switch {
	case p.accept("READ"):
		if p.accept("WRITE") {
			return "READ WRITE"
		}
		return "READ"
	case p.accept("WRITE"):
		return "WRITE"
	}
	p.fail("Expected: READ or WRITE")
	return ""
}

func (p *Parser) parseField() ast.Stmt {
	t := p.next()
	s := &ast.Field{P: t.Pos, File: p.parseFileNum()}
	for p.accept(",") {
		w := p.parseExpr()
		p.expect("AS")
		s.Fields = append(s.Fields, ast.FieldItem{Width: w, Var: p.parseLValue()})
	}
	if len(s.Fields) == 0 {
		p.fail("Expected: ,")
	}
	return s
}

func (p *Parser) parseGetPut() ast.Stmt {
	t := p.next()
	put := t.Text == "PUT"
	if p.is("(") || p.is("STEP") { // gráficos
		if put {
			s := &ast.GfxPut{P: t.Pos, At: p.parseCoord()}
			p.expect(",")
			s.Array = p.parseExpr()
			if p.accept(",") {
				a := p.cur()
				switch a.Text {
				case "PSET", "PRESET", "AND", "OR", "XOR":
					if a.Kind == token.Keyword {
						p.next()
						s.Action = a.Text
						return s
					}
				}
				p.fail("Expected: PSET, PRESET, AND, OR or XOR")
			}
			return s
		}
		s := &ast.GfxGet{P: t.Pos, From: p.parseCoord()}
		p.expect("-")
		s.To = p.parseCoord()
		p.expect(",")
		s.Array = p.parseExpr()
		return s
	}
	s := &ast.FileIO{P: t.Pos, Put: put, File: p.parseFileNum()}
	if p.accept(",") {
		if !p.is(",") && !p.atEnd() {
			s.Record = p.parseExpr()
		}
		if p.accept(",") {
			s.Var = p.parseLValue()
		}
	}
	return s
}

func (p *Parser) parseLock() ast.Stmt {
	t := p.next()
	s := &ast.Lock{P: t.Pos, Unlock: t.Text == "UNLOCK", File: p.parseFileNum()}
	if p.accept(",") {
		if !p.is("TO") {
			s.From = p.parseExpr()
		}
		if p.accept("TO") {
			s.HasTo, s.To = true, p.parseExpr()
		}
	}
	return s
}

// ------------------------------------------------------------- gráficos

func (p *Parser) parseCoord() ast.Coord {
	c := ast.Coord{Step: p.accept("STEP")}
	p.expect("(")
	c.X = p.parseExpr()
	p.expect(",")
	c.Y = p.parseExpr()
	p.expect(")")
	return c
}

func (p *Parser) parsePSet() ast.Stmt {
	t := p.next()
	s := &ast.PSet{P: t.Pos, Preset: t.Text == "PRESET", At: p.parseCoord()}
	if p.accept(",") {
		s.Color = p.parseExpr()
	}
	return s
}

// parseLine: LINE [[STEP](x1,y1)]-[STEP](x2,y2)[,[color][,[B|BF][,estilo]]]
func (p *Parser) parseLine() ast.Stmt {
	t := p.next()
	s := &ast.Line{P: t.Pos}
	if !p.is("-") {
		c := p.parseCoord()
		s.From = &c
	}
	p.expect("-")
	s.To = p.parseCoord()
	if !p.accept(",") {
		return s
	}
	if !p.is(",") && !p.atEnd() {
		s.Color = p.parseExpr()
	}
	if !p.accept(",") {
		return s
	}
	if b := p.cur(); b.Kind == token.Ident {
		switch strings.ToUpper(b.Text) {
		case "B", "BF":
			p.next()
			s.Box = strings.ToUpper(b.Text)
		default:
			p.fail("Expected: B or BF")
		}
	}
	if p.accept(",") {
		s.Style = p.parseExpr()
	}
	return s
}

// optArgs lee ", [a] [, [b] ...]" y devuelve exactamente n posiciones.
func (p *Parser) optArgs(n int) []ast.Expr {
	out := make([]ast.Expr, n)
	if !p.accept(",") {
		return out
	}
	args := p.parseArgList()
	if len(args) == 0 {
		args = []ast.Expr{nil}
	}
	if len(args) > n {
		p.fail("Expected: end-of-statement")
	}
	copy(out, args)
	return out
}

// parseCircle: CIRCLE [STEP](x,y), r[, [color][, [inicio][, [fin][, aspecto]]]]
func (p *Parser) parseCircle() ast.Stmt {
	t := p.next()
	s := &ast.Circle{P: t.Pos, At: p.parseCoord()}
	p.expect(",")
	s.Radius = p.parseExpr()
	a := p.optArgs(4)
	s.Color, s.Start, s.End, s.Aspect = a[0], a[1], a[2], a[3]
	return s
}

// parsePaint: PAINT [STEP](x,y)[, [relleno][, [borde][, fondo]]]
func (p *Parser) parsePaint() ast.Stmt {
	t := p.next()
	s := &ast.Paint{P: t.Pos, At: p.parseCoord()}
	a := p.optArgs(3)
	s.Fill, s.Border, s.Background = a[0], a[1], a[2]
	return s
}

// parseView: VIEW PRINT [arriba TO abajo] | VIEW [[SCREEN] (x1,y1)-(x2,y2)[, [color][, borde]]]
func (p *Parser) parseView() ast.Stmt {
	t := p.next()
	s := &ast.View{P: t.Pos}
	if p.accept("PRINT") {
		s.Print = true
		if !p.atEnd() {
			s.Top = p.parseExpr()
			p.expect("TO")
			s.Bottom = p.parseExpr()
		}
		return s
	}
	if p.atEnd() {
		return s
	}
	s.Screen = p.accept("SCREEN")
	from := p.parseCoord()
	p.expect("-")
	to := p.parseCoord()
	s.From, s.To = &from, &to
	a := p.optArgs(2)
	s.Color, s.Border = a[0], a[1]
	return s
}

// parseWindow: WINDOW [[SCREEN] (x1,y1)-(x2,y2)]
func (p *Parser) parseWindow() ast.Stmt {
	t := p.next()
	s := &ast.Window{P: t.Pos}
	if p.atEnd() {
		return s
	}
	s.Screen = p.accept("SCREEN")
	from := p.parseCoord()
	p.expect("-")
	to := p.parseCoord()
	s.From, s.To = &from, &to
	return s
}
