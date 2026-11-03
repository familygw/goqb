package parser

import (
	"strings"

	"goqb/lang/ast"
	"goqb/lang/token"
)

// ------------------------------------------------------------- IF

func (p *Parser) parseIf() ast.Stmt {
	t := p.next()
	s := &ast.If{P: t.Pos, Cond: p.parseExpr()}

	if p.is("GOTO") { // IF c GOTO 100 [ELSE ...]
		g := p.next()
		s.SingleLine = true
		s.Then = []ast.Stmt{&ast.Jump{P: g.Pos, Kind: "GOTO", Target: p.parseLabelRef()}}
		p.parseSingleElse(s)
		return s
	}
	p.expect("THEN")

	if k := p.cur().Kind; k != token.Newline && k != token.EOF && k != token.Comment {
		s.SingleLine = true
		s.Then = p.parseSingleLine()
		p.parseSingleElse(s)
		return s
	}

	// IF de bloque
	body, ok := p.parseBody("IF", t.Pos, "Block IF without END IF")
	s.Then = body
	if !ok {
		return s
	}
	for p.is("ELSEIF") {
		e := p.next()
		ei := ast.ElseIf{P: e.Pos, Cond: p.parseExpr()}
		p.expect("THEN")
		p.expectEnd()
		ei.Body, ok = p.parseBody("IF", t.Pos, "Block IF without END IF")
		s.ElseIfs = append(s.ElseIfs, ei)
		if !ok {
			return s
		}
	}
	if p.is("ELSE") {
		p.next()
		s.HasElse = true
		s.Else, ok = p.parseBody("IF", t.Pos, "Block IF without END IF")
		if !ok {
			return s
		}
		for p.is("ELSE") || p.is("ELSEIF") { // ELSE duplicado o ELSEIF tras ELSE
			p.errAt(p.cur().Pos, p.cur().Text+" without IF")
			p.syncLine()
			var more []ast.Stmt
			more, ok = p.parseBody("IF", t.Pos, "Block IF without END IF")
			s.Else = append(s.Else, more...)
			if !ok {
				return s
			}
		}
	}
	p.expect("END")
	p.expect("IF")
	return s
}

// parseSingleLine analiza la parte THEN/ELSE de un IF de una línea.
func (p *Parser) parseSingleLine() []ast.Stmt {
	if t := p.cur(); t.Kind == token.Int { // IF c THEN 100
		return []ast.Stmt{&ast.Jump{P: t.Pos, Kind: "GOTO", Target: p.parseLabelRef()}}
	}
	p.singleIf++
	defer func() { p.singleIf-- }()
	var out []ast.Stmt
	for {
		if s := p.parseStmt(); s != nil {
			out = append(out, s)
		}
		if !p.accept(":") {
			break
		}
		if p.atEnd() {
			break
		}
	}
	if p.pendingNext == 0 && !p.atEnd() {
		p.fail("Expected: end-of-statement")
	}
	return out
}

func (p *Parser) parseSingleElse(s *ast.If) {
	if p.accept("ELSE") {
		s.HasElse = true
		s.Else = p.parseSingleLine()
	}
}

// ------------------------------------------------------------- SELECT CASE

func (p *Parser) parseSelect() ast.Stmt {
	t := p.next()
	p.expect("CASE")
	s := &ast.SelectCase{P: t.Pos, X: p.parseExpr()}
	p.expectEnd()

	pre, ok := p.parseBody("SELECT", t.Pos, "SELECT without END SELECT")
	for _, st := range pre {
		if _, isC := st.(*ast.Comment); !isC {
			p.errAt(st.Position(), "Statements illegal between SELECT CASE and CASE")
			break
		}
	}
	if !ok {
		return s
	}
	for p.is("CASE") {
		c := p.next()
		cl := ast.CaseClause{P: c.Pos}
		if p.accept("ELSE") {
			cl.Else = true
		} else {
			cl.Tests = p.parseCaseTests()
		}
		cl.Body, ok = p.parseBody("SELECT", t.Pos, "SELECT without END SELECT")
		s.Cases = append(s.Cases, cl)
		if !ok {
			return s
		}
	}
	p.expect("END")
	p.expect("SELECT")
	return s
}

var relOps = map[string]bool{"=": true, "<>": true, "<": true, ">": true, "<=": true, ">=": true}

func (p *Parser) parseCaseTests() []ast.CaseTest {
	var out []ast.CaseTest
	for {
		var ct ast.CaseTest
		hasIs := p.accept("IS")
		if op := p.cur(); op.Kind == token.Op && relOps[op.Text] {
			p.next()
			ct = ast.CaseTest{Kind: "IS", Op: op.Text, X: p.parseExpr()}
		} else if hasIs {
			p.fail("Expected: relational operator")
		} else {
			ct = ast.CaseTest{Kind: "VALUE", X: p.parseExpr()}
			if p.accept("TO") {
				ct.Kind, ct.Y = "RANGE", p.parseExpr()
			}
		}
		out = append(out, ct)
		if !p.accept(",") {
			return out
		}
	}
}

// ------------------------------------------------------------- bucles

func (p *Parser) parseFor() ast.Stmt {
	t := p.next()
	s := &ast.For{P: t.Pos}
	v := p.cur()
	if v.Kind != token.Ident || p.peek(1).Is("(") {
		p.fail("Expected: simple variable")
	}
	p.next()
	s.Var = &ast.Ident{P: v.Pos, Name: v.Text, Suffix: v.Suffix}
	p.expect("=")
	s.From = p.parseExpr()
	p.expect("TO")
	s.To = p.parseExpr()
	if p.accept("STEP") {
		s.Step = p.parseExpr()
	}
	p.expectEnd()

	body, ok := p.parseBody("FOR", t.Pos, "FOR without NEXT")
	s.Body = body
	if !ok {
		return s
	}
	if p.pendingNext > 0 { // cerrado por un NEXT j, i anterior
		p.pendingNext--
	} else {
		p.expect("NEXT")
		if p.atEnd() {
			return s
		}
	}
	nv := p.cur()
	if nv.Kind != token.Ident {
		p.fail("Expected: variable")
	}
	p.next()
	s.NextVar = &ast.Ident{P: nv.Pos, Name: nv.Text, Suffix: nv.Suffix}
	if !strings.EqualFold(nv.Text, v.Text) {
		p.errAt(nv.Pos, "NEXT without FOR")
	}
	if p.accept(",") {
		p.pendingNext++
	}
	return s
}

func (p *Parser) parseDo() ast.Stmt {
	t := p.next()
	s := &ast.DoLoop{P: t.Pos}
	if p.accept("WHILE") {
		s.PreCond = p.parseExpr()
	} else if p.accept("UNTIL") {
		s.PreUntil, s.PreCond = true, p.parseExpr()
	}
	p.expectEnd()
	body, ok := p.parseBody("DO", t.Pos, "DO without LOOP")
	s.Body = body
	if !ok {
		return s
	}
	p.expect("LOOP")
	if p.is("WHILE") || p.is("UNTIL") {
		if s.PreCond != nil {
			p.fail("Expected: end-of-statement")
		}
		u := p.next()
		s.PostUntil, s.PostCond = u.Text == "UNTIL", p.parseExpr()
	}
	return s
}

func (p *Parser) parseWhile() ast.Stmt {
	t := p.next()
	s := &ast.While{P: t.Pos, Cond: p.parseExpr()}
	p.expectEnd()
	body, ok := p.parseBody("WHILE", t.Pos, "WHILE without WEND")
	s.Body = body
	if ok {
		p.expect("WEND")
	}
	return s
}

// ------------------------------------------------------------- procedimientos

func (p *Parser) parseProc() ast.Stmt {
	t := p.next()
	if len(p.blocks) > 0 || p.singleIf > 0 {
		p.errAt(t.Pos, "SUB or FUNCTION not allowed inside a block or procedure")
	}
	s := &ast.Proc{P: t.Pos, Function: t.Text == "FUNCTION"}
	n := p.cur()
	if n.Kind != token.Ident {
		p.fail("Expected: identifier")
	}
	p.next()
	if !s.Function && n.Suffix != 0 {
		p.errAt(n.Pos, "SUB name cannot have a type suffix")
	}
	s.Name, s.Suffix = n.Text, n.Suffix
	if p.is("(") {
		s.Params = p.parseParams()
	}
	s.Static = p.accept("STATIC")
	p.expectEnd()

	missing := t.Text + " without END " + t.Text
	body, ok := p.parseBody(t.Text, t.Pos, missing)
	s.Body = body
	if ok {
		s.EndPos = p.cur().Pos
		p.next()
		p.next()
	}
	return s
}

func (p *Parser) parseParams() []ast.Param {
	p.expect("(")
	var out []ast.Param
	if p.accept(")") {
		return out
	}
	for {
		pr := ast.Param{P: p.cur().Pos}
		pr.ByVal = p.accept("BYVAL")
		pr.Seg = p.accept("SEG")
		n := p.cur()
		if n.Kind != token.Ident {
			p.fail("Expected: parameter")
		}
		p.next()
		pr.Name, pr.Suffix = n.Text, n.Suffix
		if p.accept("(") {
			p.expect(")")
			pr.IsArray = true
		}
		if p.accept("AS") {
			pr.Type = p.parseTypeRef(false)
		}
		out = append(out, pr)
		if !p.accept(",") {
			break
		}
	}
	p.expect(")")
	return out
}

func (p *Parser) parseDeclare() ast.Stmt {
	t := p.next()
	k := p.cur()
	if !k.Is("SUB") && !k.Is("FUNCTION") {
		p.fail("Expected: SUB or FUNCTION")
	}
	p.next()
	n := p.cur()
	if n.Kind != token.Ident {
		p.fail("Expected: identifier")
	}
	p.next()
	d := &ast.Declare{P: t.Pos, Function: k.Text == "FUNCTION", Name: n.Text}
	d.CDecl = p.accept("CDECL")
	if p.accept("ALIAS") {
		a := p.cur()
		if a.Kind != token.String {
			p.fail("Expected: string")
		}
		p.next()
		d.Alias = a.Text
	}
	if p.is("(") {
		d.HasParens = true
		d.Params = p.parseParams()
	}
	return d
}

func (p *Parser) parseDefFn() ast.Stmt {
	t := p.next()
	if p.inProc() {
		p.errAt(t.Pos, "DEF FN not allowed inside a procedure")
	}
	n := p.cur()
	if n.Kind != token.Ident || !strings.HasPrefix(strings.ToUpper(n.Text), "FN") {
		p.fail("Expected: FNname")
	}
	p.next()
	d := &ast.DefFn{P: t.Pos, Name: n.Text}
	if p.is("(") {
		d.Params = p.parseParams()
	}
	if p.accept("=") {
		d.Expr = p.parseExpr()
		return d
	}
	p.expectEnd()
	body, ok := p.parseBody("DEF", t.Pos, "DEF without END DEF")
	d.Body = body
	if ok {
		p.next()
		p.next()
	}
	return d
}

// ------------------------------------------------------------- tipos y declaraciones

// parseTypeRef lee el tipo después de AS. fixed permite STRING * n.
func (p *Parser) parseTypeRef(fixed bool) *ast.TypeRef {
	t := p.cur()
	r := &ast.TypeRef{P: t.Pos}
	switch {
	case t.Is("INTEGER"), t.Is("LONG"), t.Is("SINGLE"), t.Is("DOUBLE"), t.Is("ANY"):
		p.next()
		r.Name = t.Text
	case t.Is("STRING"):
		p.next()
		r.Name = "STRING"
		if p.is("*") {
			if !fixed {
				p.fail("Expected: end-of-statement")
			}
			p.next()
			r.FixedLen = p.parsePrimary()
		}
	case t.Kind == token.Ident && t.Suffix == 0:
		p.next()
		r.Name = strings.ToUpper(t.Text)
	default:
		p.fail("Expected: INTEGER, LONG, SINGLE, DOUBLE, STRING or type name")
	}
	return r
}

func (p *Parser) parseDim() ast.Stmt {
	t := p.next()
	d := &ast.Dim{P: t.Pos, Kind: t.Text}
	if t.Text == "DIM" || t.Text == "REDIM" || t.Text == "COMMON" {
		d.Shared = p.accept("SHARED")
	}
	if t.Text == "COMMON" && p.accept("/") {
		b := p.cur()
		if b.Kind != token.Ident {
			p.fail("Expected: block name")
		}
		p.next()
		d.Block = strings.ToUpper(b.Text)
		p.expect("/")
	}
	for {
		d.Vars = append(d.Vars, p.parseDimVar())
		if !p.accept(",") {
			return d
		}
	}
}

func (p *Parser) parseDimVar() ast.DimVar {
	n := p.cur()
	if n.Kind != token.Ident {
		p.fail("Expected: variable")
	}
	p.next()
	v := ast.DimVar{P: n.Pos, Name: n.Text, Suffix: n.Suffix}
	if p.accept("(") {
		v.IsArray = true
		if !p.accept(")") {
			for {
				b := ast.Bound{Hi: p.parseExpr()}
				if p.accept("TO") {
					b.Lo, b.Hi = b.Hi, p.parseExpr()
				}
				v.Bounds = append(v.Bounds, b)
				if !p.accept(",") {
					break
				}
			}
			p.expect(")")
		}
	}
	if p.accept("AS") {
		if n.Suffix != 0 {
			p.errAt(n.Pos, "AS clause not allowed with type suffix")
		}
		v.Type = p.parseTypeRef(true)
	}
	return v
}

func (p *Parser) parseConst() ast.Stmt {
	t := p.next()
	c := &ast.Const{P: t.Pos}
	for {
		n := p.cur()
		if n.Kind != token.Ident {
			p.fail("Expected: identifier")
		}
		p.next()
		p.expect("=")
		c.Items = append(c.Items, ast.ConstItem{P: n.Pos, Name: n.Text, Value: p.parseExpr()})
		if !p.accept(",") {
			return c
		}
	}
}

var defTypes = map[string]string{
	"DEFINT": "INTEGER", "DEFLNG": "LONG", "DEFSNG": "SINGLE", "DEFDBL": "DOUBLE", "DEFSTR": "STRING",
}

func (p *Parser) parseDefType() ast.Stmt {
	t := p.next()
	d := &ast.DefType{P: t.Pos, Type: defTypes[t.Text]}
	letter := func() byte {
		l := p.cur()
		if l.Kind != token.Ident || len(l.Text) != 1 {
			p.fail("Expected: letter")
		}
		p.next()
		return strings.ToUpper(l.Text)[0]
	}
	for {
		r := ast.LetterRange{From: letter()}
		r.To = r.From
		if p.accept("-") {
			r.To = letter()
		}
		d.Ranges = append(d.Ranges, r)
		if !p.accept(",") {
			return d
		}
	}
}

func (p *Parser) parseType() ast.Stmt {
	t := p.next()
	if len(p.blocks) > 0 {
		p.errAt(t.Pos, "TYPE not allowed inside a block or procedure")
	}
	n := p.cur()
	if n.Kind != token.Ident || n.Suffix != 0 {
		p.fail("Expected: identifier")
	}
	p.next()
	d := &ast.TypeDecl{P: t.Pos, Name: strings.ToUpper(n.Text)}
	p.expectEnd()
	for {
		c := p.cur()
		switch {
		case c.Kind == token.EOF:
			p.errAt(t.Pos, "TYPE without END TYPE")
			return d
		case c.Kind == token.Newline, c.Kind == token.Comment, c.Is(":"):
			p.next()
			continue
		case p.isEnd("TYPE"):
			p.next()
			p.next()
			if len(d.Fields) == 0 {
				p.errAt(t.Pos, "Element not defined")
			}
			return d
		}
		func() {
			defer p.recoverStmt()
			f := p.cur()
			if f.Kind != token.Ident || !p.peek(1).Is("AS") {
				p.fail("Statement illegal in TYPE block")
			}
			if f.Suffix != 0 {
				p.errAt(f.Pos, "AS clause not allowed with type suffix")
			}
			p.next()
			p.next()
			tr := p.parseTypeRef(true)
			if tr.Name == "ANY" {
				p.errAt(tr.P, "Expected: INTEGER, LONG, SINGLE, DOUBLE, STRING or type name")
			}
			if tr.Name == "STRING" && tr.FixedLen == nil {
				p.errAt(tr.P, "Expected: STRING * length")
			}
			d.Fields = append(d.Fields, ast.TypeField{P: f.Pos, Name: f.Text, Type: *tr})
			p.expectEnd()
		}()
	}
}
