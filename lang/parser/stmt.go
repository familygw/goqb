package parser

import (
	"strings"

	"goqb/lang/ast"
	"goqb/lang/token"
)

// Sentencias "PALABRA [args]" con sintaxis uniforme.
var simpleStmts = map[string]bool{
	"BEEP": true, "BLOAD": true, "BSAVE": true, "CHAIN": true, "CHDIR": true,
	"CLEAR": true, "CLOSE": true, "CLS": true, "COLOR": true, "DRAW": true,
	"ENVIRON": true, "ERASE": true, "ERROR": true, "FILES": true, "IOCTL": true,
	"KILL": true, "LOCATE": true, "MKDIR": true, "OUT": true, "PCOPY": true,
	"POKE": true, "RANDOMIZE": true, "READ": true, "RESET": true, "RMDIR": true,
	"RUN": true, "SCREEN": true, "SEEK": true, "SHELL": true, "SLEEP": true,
	"SOUND": true, "STOP": true, "SWAP": true, "SYSTEM": true, "TROFF": true,
	"TRON": true, "WAIT": true,
}

func (p *Parser) parseStmt() ast.Stmt {
	t := p.cur()
	switch t.Kind {
	case token.Ident:
		return p.parseIdentStmt()
	case token.Keyword:
	default:
		p.fail("Expected: statement")
	}

	kw := t.Text
	if simpleStmts[kw] {
		p.next()
		return &ast.Simple{P: t.Pos, Name: kw, Args: p.parseArgList()}
	}

	switch kw {
	case "LET":
		p.next()
		target := p.parseLValue()
		p.expect("=")
		return &ast.Assign{P: t.Pos, Let: true, Target: target, Value: p.parseExpr()}
	case "MID$":
		return p.parseMidAssign()
	case "DATE$", "TIME$":
		p.next()
		p.expect("=")
		return &ast.Simple{P: t.Pos, Name: kw, Args: []ast.Expr{p.parseExpr()}}
	case "LSET", "RSET":
		p.next()
		target := p.parseLValue()
		p.expect("=")
		return &ast.SetStr{P: t.Pos, Right: kw == "RSET", Target: target, Value: p.parseExpr()}
	case "CALL", "CALLS":
		return p.parseCall()
	case "PRINT", "LPRINT":
		return p.parsePrint()
	case "WRITE":
		return p.parseWrite()
	case "INPUT":
		return p.parseInput(false)
	case "LINE":
		if p.peek(1).Is("INPUT") {
			p.next()
			return p.parseInput(true)
		}
		return p.parseLine()
	case "IF":
		return p.parseIf()
	case "SELECT":
		return p.parseSelect()
	case "FOR":
		return p.parseFor()
	case "DO":
		return p.parseDo()
	case "WHILE":
		return p.parseWhile()
	case "GOTO", "GOSUB":
		p.next()
		return &ast.Jump{P: t.Pos, Kind: kw, Target: p.parseLabelRef()}
	case "RETURN", "RESTORE":
		p.next()
		j := &ast.Jump{P: t.Pos, Kind: kw}
		if !p.atEnd() {
			j.Target = p.parseLabelRef()
		}
		return j
	case "RESUME":
		p.next()
		r := &ast.Resume{P: t.Pos}
		if p.accept("NEXT") {
			r.Next = true
		} else if !p.atEnd() {
			r.Target = p.parseLabelRef()
		}
		return r
	case "ON":
		return p.parseOn()
	case "TIMER", "PEN", "PLAY", "STRIG", "UEVENT", "COM", "KEY":
		if s := p.parseEventCtl(); s != nil {
			return s
		}
		if kw == "PLAY" {
			p.next()
			return &ast.Simple{P: t.Pos, Name: kw, Args: p.parseArgList()}
		}
		if kw == "KEY" {
			return p.parseKey()
		}
		p.fail("Expected: ON, OFF or STOP")
	case "EXIT":
		p.next()
		w := p.cur()
		switch {
		case w.Is("FOR"), w.Is("DO"), w.Is("SUB"), w.Is("FUNCTION"), w.Is("DEF"):
			p.next()
			p.checkExit(w)
			return &ast.Exit{P: t.Pos, What: w.Text}
		}
		p.fail("Expected: DEF, DO, FOR, FUNCTION or SUB")
	case "END":
		return p.parseEnd()
	case "DIM", "REDIM", "STATIC", "SHARED", "COMMON":
		return p.parseDim()
	case "CONST":
		return p.parseConst()
	case "DEFINT", "DEFLNG", "DEFSNG", "DEFDBL", "DEFSTR":
		return p.parseDefType()
	case "TYPE":
		return p.parseType()
	case "DECLARE":
		return p.parseDeclare()
	case "SUB", "FUNCTION":
		return p.parseProc()
	case "DEF":
		if p.peek(1).Is("SEG") {
			p.next()
			p.next()
			s := &ast.Simple{P: t.Pos, Name: "DEF SEG"}
			if p.accept("=") {
				s.Args = []ast.Expr{p.parseExpr()}
			}
			return s
		}
		return p.parseDefFn()
	case "DATA":
		return p.parseData()
	case "OPEN":
		return p.parseOpen()
	case "FIELD":
		return p.parseField()
	case "GET", "PUT":
		return p.parseGetPut()
	case "LOCK", "UNLOCK":
		return p.parseLock()
	case "NAME":
		p.next()
		old := p.parseExpr()
		p.expect("AS")
		return &ast.Simple{P: t.Pos, Name: "NAME", Args: []ast.Expr{old, p.parseExpr()}}
	case "OPTION":
		p.next()
		p.expect("BASE")
		n := p.cur()
		if n.Kind != token.Int || (n.Text != "0" && n.Text != "1") {
			p.fail("Expected: 0 or 1")
		}
		p.next()
		return &ast.Simple{P: t.Pos, Name: "OPTION BASE", Args: []ast.Expr{p.numberLit(n)}}
	case "WIDTH":
		p.next()
		if p.accept("LPRINT") {
			return &ast.Simple{P: t.Pos, Name: "WIDTH LPRINT", Args: []ast.Expr{p.parseExpr()}}
		}
		return &ast.Simple{P: t.Pos, Name: "WIDTH", Args: p.parseArgList()}
	case "PALETTE":
		p.next()
		if p.accept("USING") {
			return &ast.Simple{P: t.Pos, Name: "PALETTE USING", Args: []ast.Expr{p.parseExpr()}}
		}
		return &ast.Simple{P: t.Pos, Name: "PALETTE", Args: p.parseArgList()}
	case "PSET", "PRESET":
		return p.parsePSet()
	case "CIRCLE":
		return p.parseCircle()
	case "PAINT":
		return p.parsePaint()
	case "VIEW":
		return p.parseView()
	case "WINDOW":
		return p.parseWindow()

	// Terminadores sin bloque abierto.
	case "NEXT":
		p.fail("NEXT without FOR")
	case "LOOP":
		p.fail("LOOP without DO")
	case "WEND":
		p.fail("WEND without WHILE")
	case "ELSE":
		p.fail("ELSE without IF")
	case "ELSEIF":
		p.fail("ELSEIF without IF")
	case "CASE":
		p.fail("CASE without SELECT CASE")
	}
	p.fail("Expected: statement")
	return nil
}

func (p *Parser) checkExit(w token.Token) {
	ok := false
	switch w.Text {
	case "FOR", "DO":
		ok = p.inBlock(w.Text)
	case "SUB", "FUNCTION", "DEF":
		ok = p.inBlock(w.Text)
	}
	if !ok {
		p.errAt(w.Pos, "EXIT "+w.Text+" not within "+w.Text)
	}
}

// parseIdentStmt: asignación implícita o llamada a SUB sin CALL.
func (p *Parser) parseIdentStmt() ast.Stmt {
	t := p.cur()
	// FN no puede ser destino; SUB sin argumentos: "Foo"
	if p.peek(1).Is("=") || p.peek(1).Is("(") || p.peek(1).Is(".") {
		save := p.i
		target := p.parseLValue()
		if p.accept("=") {
			return &ast.Assign{P: t.Pos, Target: target, Value: p.parseExpr()}
		}
		p.i = save
	}
	// Llamada sin CALL: Nombre arg1, arg2
	p.next()
	c := &ast.CallStmt{P: t.Pos, Name: t.Text}
	if !p.atEnd() {
		c.Args = p.parseExprList()
	}
	return c
}

// parseLValue: variable, elemento de array o campo de registro.
func (p *Parser) parseLValue() ast.Expr {
	t := p.cur()
	if t.Kind != token.Ident {
		p.fail("Expected: variable")
	}
	return p.parsePostfix()
}

func (p *Parser) parseMidAssign() ast.Stmt {
	t := p.next()
	p.expect("(")
	m := &ast.MidAssign{P: t.Pos, Target: p.parseLValue()}
	p.expect(",")
	m.Start = p.parseExpr()
	if p.accept(",") {
		m.Length = p.parseExpr()
	}
	p.expect(")")
	p.expect("=")
	m.Value = p.parseExpr()
	return m
}

func (p *Parser) parseCall() ast.Stmt {
	t := p.next()
	n := p.cur()
	if n.Kind != token.Ident {
		p.fail("Expected: identifier")
	}
	p.next()
	c := &ast.CallStmt{P: t.Pos, Name: n.Text, Explicit: true, Calls: t.Text == "CALLS"}
	if p.is("(") {
		c.Args, _ = p.parseCallArgs()
	}
	return c
}

func (p *Parser) parseEnd() ast.Stmt {
	t := p.next()
	w := p.cur()
	if w.Kind == token.Keyword {
		msg := map[string]string{
			"IF":       "END IF without block IF",
			"SELECT":   "END SELECT without SELECT CASE",
			"SUB":      "END SUB without SUB",
			"FUNCTION": "END FUNCTION without FUNCTION",
			"DEF":      "END DEF without DEF",
			"TYPE":     "END TYPE without TYPE",
		}[w.Text]
		if msg != "" {
			p.errAt(t.Pos, msg)
			panic(bailout{})
		}
	}
	return &ast.Simple{P: t.Pos, Name: "END"}
}

// ------------------------------------------------------------- eventos

// parseEventCtl reconoce EVENTO[(n)] {ON|OFF|STOP}. Devuelve nil si no aplica.
func (p *Parser) parseEventCtl() ast.Stmt {
	save := p.i
	t := p.next()
	var arg ast.Expr
	if p.is("(") && t.Text != "TIMER" && t.Text != "PEN" && t.Text != "UEVENT" {
		p.next()
		arg = p.parseExpr()
		p.expect(")")
	}
	if t.Text == "KEY" && arg == nil { // KEY ON/OFF: línea de teclas de función
		p.i = save
		return nil
	}
	if a := p.cur(); a.Is("ON") || a.Is("OFF") || a.Is("STOP") {
		p.next()
		return &ast.EventCtl{P: t.Pos, Event: t.Text, Arg: arg, Action: a.Text}
	}
	p.i = save
	return nil
}

func (p *Parser) parseKey() ast.Stmt {
	t := p.next()
	switch {
	case p.is("ON"), p.is("OFF"):
		a := p.next()
		return &ast.Simple{P: t.Pos, Name: "KEY " + a.Text}
	case p.cur().Kind == token.Ident && strings.EqualFold(p.cur().Text, "LIST"):
		p.next()
		return &ast.Simple{P: t.Pos, Name: "KEY LIST"}
	}
	n := p.parseExpr()
	p.expect(",")
	return &ast.Simple{P: t.Pos, Name: "KEY", Args: []ast.Expr{n, p.parseExpr()}}
}

func (p *Parser) parseOn() ast.Stmt {
	t := p.next()
	c := p.cur()
	switch {
	case c.Is("ERROR"):
		p.next()
		p.expect("GOTO")
		return &ast.OnError{P: t.Pos, Target: p.parseLabelRef()}
	case c.Is("TIMER"), c.Is("KEY"), c.Is("COM"), c.Is("PLAY"), c.Is("STRIG"), c.Is("PEN"), c.Is("UEVENT"):
		p.next()
		e := &ast.OnEvent{P: t.Pos, Event: c.Text}
		if c.Text != "PEN" && c.Text != "UEVENT" {
			p.expect("(")
			e.Arg = p.parseExpr()
			p.expect(")")
		}
		p.expect("GOSUB")
		e.Target = p.parseLabelRef()
		return e
	}
	o := &ast.OnJump{P: t.Pos, X: p.parseExpr()}
	switch {
	case p.accept("GOTO"):
	case p.accept("GOSUB"):
		o.Gosub = true
	default:
		p.fail("Expected: GOTO or GOSUB")
	}
	for {
		o.Targets = append(o.Targets, p.parseLabelRef())
		if !p.accept(",") {
			return o
		}
	}
}

// ------------------------------------------------------------- DATA

func (p *Parser) parseData() ast.Stmt {
	t := p.next()
	d := &ast.Data{P: t.Pos}
	if r := p.cur(); r.Kind == token.DataRaw {
		p.next()
		d.Raw = r.Text
		d.Items = splitData(r.Text)
	}
	return d
}

// splitData separa los ítems de DATA respetando comillas.
func splitData(raw string) []ast.DataItem {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []ast.DataItem
	i := 0
	for {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
			i++
		}
		if i < len(raw) && raw[i] == '"' {
			i++
			s := i
			for i < len(raw) && raw[i] != '"' {
				i++
			}
			items = append(items, ast.DataItem{Text: raw[s:i], Quoted: true})
			if i < len(raw) {
				i++
			}
			for i < len(raw) && raw[i] != ',' {
				i++
			}
		} else {
			s := i
			for i < len(raw) && raw[i] != ',' {
				i++
			}
			items = append(items, ast.DataItem{Text: strings.TrimSpace(raw[s:i])})
		}
		if i >= len(raw) {
			return items
		}
		i++ // coma
	}
}
