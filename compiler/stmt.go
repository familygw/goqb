package compiler

import (
	"strconv"
	"strings"

	"goqb/lang/ast"
	"goqb/vm"
)

func (c *Compiler) stmts(list []ast.Stmt) {
	for _, s := range list {
		c.stmt1(s)
	}
}

// markEnd marca el cierre de un bloque (NEXT, LOOP, WEND, END SUB) como
// sentencia, para que el depurador se detenga ahí como en QB.
func (c *Compiler) markEnd(pos ast.Pos) {
	if pos.Line > 0 {
		c.mark(pos)
	}
}

// mark registra el inicio de una sentencia (para errores, RESUME y ERL).
func (c *Compiler) mark(pos ast.Pos) {
	c.prog.Stmts = append(c.prog.Stmts, vm.StmtInfo{PC: c.here(), File: c.file, Line: pos.Line, Col: pos.Col, ERL: c.erl})
	c.emit(vm.OpEvt, 0, 0)
}

func (c *Compiler) stmt1(st ast.Stmt) {
	switch s := st.(type) {
	case *ast.Comment, *ast.TypeDecl, *ast.Declare:
		return
	case *ast.Proc:
		return // se compila aparte
	case *ast.Label:
		c.defineLabel(s.Name, s.P)
		if s.Numeric {
			c.erl, _ = strconv.Atoi(s.Name)
		}
		return
	case *ast.Meta:
		switch s.Name {
		case "DYNAMIC":
			c.sc.dynamic = true
		case "STATIC":
			c.sc.dynamic = false
		case "INCLUDE":
			if s.Resolved != "" {
				save := c.file
				c.file = c.fileIdx(s.Resolved)
				c.stmts(s.Stmts)
				c.file = save
			}
		}
		return
	case *ast.DefType:
		c.applyDefType(s)
		return
	case *ast.Data:
		for _, it := range s.Items {
			c.prog.Data = append(c.prog.Data, vm.DataItem{Text: it.Text, Quoted: it.Quoted})
		}
		return
	}

	c.mark(st.Position())
	switch s := st.(type) {
	case *ast.Simple:
		c.simple(s)
	case *ast.Assign:
		c.store(s.Target, s.Value, s.P)
	case *ast.MidAssign:
		if t, ok := c.addr(s.Target); ok && t.K != vm.KString {
			c.errAt(s.P, "Type mismatch")
		}
		c.numArg(s.Start)
		if s.Length != nil {
			c.numArg(s.Length)
		} else {
			c.pushNone()
		}
		c.strArg(s.Value)
		c.stmt("MID_ASSIGN", 4)
	case *ast.SetStr:
		if t, ok := c.addr(s.Target); ok && t.K != vm.KString {
			c.errAt(s.P, "Type mismatch")
		}
		c.strArg(s.Value)
		if s.Right {
			c.stmt("RSET", 2)
		} else {
			c.stmt("LSET", 2)
		}
	case *ast.CallStmt:
		c.callStmt(s)
	case *ast.Print:
		c.print(s)
	case *ast.Write:
		c.write(s)
	case *ast.Input:
		c.input(s)
	case *ast.If:
		c.ifStmt(s)
	case *ast.SelectCase:
		c.selectStmt(s)
	case *ast.For:
		c.forStmt(s)
	case *ast.DoLoop:
		c.doStmt(s)
	case *ast.While:
		top := c.here()
		c.emit(vm.OpEvt, 0, 0) // eventos también en el salto de vuelta del bucle
		c.cond(s.Cond)
		exit := c.emit(vm.OpJmpF, 0, 0)
		c.stmts(s.Body)
		c.markEnd(s.WendPos)
		c.emit(vm.OpJmp, int32(top), 0)
		c.patchA(exit, c.here())
	case *ast.Jump:
		c.jump(s)
	case *ast.OnJump:
		if t := c.expr(s.X); !t.K.IsNumeric() {
			c.errAt(s.P, "Type mismatch")
		}
		tbl := len(c.prog.Tables)
		c.prog.Tables = append(c.prog.Tables, make([]int, len(s.Targets)))
		for i, t := range s.Targets {
			c.sc.fixups = append(c.sc.fixups, labelRef{field: 't', tbl: tbl, idx: i, label: t, pos: s.P, file: c.file})
		}
		gosub := int32(0)
		if s.Gosub {
			gosub = 1
		}
		c.emit(vm.OpOnJump, int32(tbl), gosub)
	case *ast.OnError:
		if s.Target == "0" {
			c.emit(vm.OpOnError, -1, 0)
			break
		}
		at := c.emit(vm.OpOnError, 0, 0)
		c.refLabel(c.mod.scope, at, 'A', s.Target, s.P)
	case *ast.Resume:
		switch {
		case s.Next:
			c.emit(vm.OpResume, 1, 0)
		case s.Target == "" || s.Target == "0":
			c.emit(vm.OpResume, 0, 0)
		default:
			at := c.emit(vm.OpResume, 2, 0)
			c.refLabel(c.mod.scope, at, 'B', s.Target, s.P)
		}
	case *ast.OnEvent:
		c.pushConst(vm.Str(s.Event))
		c.optNum(s.Arg)
		if s.Target == "0" {
			c.pushConst(vm.Long(-1))
		} else {
			at := c.emit(vm.OpConst, 0, 0)
			idx := c.constIdx(vm.Long(-1))
			c.prog.Code[at].A = int32(idx)
			// el pc del manejador (etiqueta de nivel de módulo) se resuelve al final
			c.mod.scope.fixups = append(c.mod.scope.fixups, labelRef{at: -1 - idx, field: 'c', label: s.Target, pos: s.P, file: c.file})
		}
		c.stmt("ON_EVENT", 3)
	case *ast.EventCtl:
		c.pushConst(vm.Str(s.Event))
		c.optNum(s.Arg)
		c.pushConst(vm.Str(s.Action))
		c.stmt("EVENT_CTL", 3)
	case *ast.Exit:
		c.exit(s)
	case *ast.Dim:
		c.dim(s)
	case *ast.Const:
		for _, it := range s.Items {
			v, err := c.evalConst(it.Value)
			if err != nil {
				c.errAt(it.P, "Invalid constant")
				continue
			}
			if k, ok := suffixKind[suffixOf(it.Name)]; ok {
				if v, err = vm.Convert(v, k); err != nil {
					c.errAt(it.P, err.Error())
					continue
				}
			}
			base := baseName(it.Name)
			if old := c.sc.vars[base]; old != nil {
				c.errAt(it.P, "Duplicate definition")
				continue
			}
			c.sc.vars[base] = &Symbol{Kind: symConst, Name: it.Name, Type: typeOf(v), Value: v}
		}
	case *ast.DefFn:
		c.defFn(s)
	case *ast.Open:
		c.open(s)
	case *ast.Field:
		c.fileArg(s.File)
		for _, f := range s.Fields {
			c.numArg(f.Width)
			if t, ok := c.addr(f.Var); ok && t.K != vm.KString {
				c.errAt(s.P, "Type mismatch")
			}
		}
		c.stmt("FIELD", 1+2*len(s.Fields))
	case *ast.FileIO:
		c.fileArg(s.File)
		if s.Record != nil {
			c.numArg(s.Record)
		} else {
			c.pushNone()
		}
		if s.Var != nil {
			c.addr(s.Var)
		} else {
			c.pushNone()
		}
		if s.Put {
			c.stmt("PUT", 3)
		} else {
			c.stmt("GET", 3)
		}
	case *ast.Lock:
		// un solo proceso: LOCK/UNLOCK no tienen efecto
	case *ast.View:
		if s.Print {
			if s.Top != nil {
				c.numArg(s.Top)
				c.numArg(s.Bottom)
				c.stmt("VIEW_PRINT", 2)
			} else {
				c.stmt("VIEW_PRINT", 0)
			}
			break
		}
		c.view(s)
	case *ast.PSet, *ast.Line, *ast.Circle, *ast.Paint, *ast.GfxGet, *ast.GfxPut, *ast.Window:
		c.graphics(st)
	default:
		c.errAt(st.Position(), "Statement not implemented")
	}
}

func (c *Compiler) numArg(e ast.Expr) {
	if fn, ok := e.(*ast.FileNum); ok {
		e = fn.X
	}
	if t := c.expr(e); !t.K.IsNumeric() {
		c.errAt(e.Position(), "Type mismatch")
	}
}

func (c *Compiler) strArg(e ast.Expr) {
	if t := c.expr(e); t.K != vm.KString {
		c.errAt(e.Position(), "Type mismatch")
	}
}

func (c *Compiler) fileArg(e ast.Expr) { c.numArg(e) }

// cond compila una condición numérica.
func (c *Compiler) cond(e ast.Expr) {
	if t := c.expr(e); !t.K.IsNumeric() {
		c.errAt(e.Position(), "Type mismatch")
	}
}

// ------------------------------------------------------------- sentencias simples

func (c *Compiler) simple(s *ast.Simple) {
	args := func() int {
		for _, a := range s.Args {
			if a == nil {
				c.pushNone()
				continue
			}
			if fn, ok := a.(*ast.FileNum); ok {
				a = fn.X
			}
			c.expr(a)
		}
		return len(s.Args)
	}
	switch s.Name {
	case "END", "SYSTEM":
		c.emit(vm.OpEnd, 0, 0)
	case "STOP":
		c.emit(vm.OpStop, 0, 0)
	case "CLS", "COLOR", "LOCATE", "BEEP", "SLEEP", "RANDOMIZE", "KILL", "MKDIR", "RMDIR",
		"CHDIR", "FILES", "SHELL", "ENVIRON", "RESET", "POKE", "SCREEN", "NAME", "SEEK", "CLOSE":
		c.stmt(s.Name, args())
	case "CLEAR":
		c.stmt("CLEAR", 0)
	case "WIDTH":
		if len(s.Args) > 0 {
			if _, ok := s.Args[0].(*ast.FileNum); ok {
				c.stmt("WIDTH_FILE", args())
				return
			}
		}
		c.stmt("WIDTH", args())
	case "DEF SEG":
		c.stmt("DEF_SEG", args())
	case "DRAW":
		c.strArg(s.Args[0])
		c.stmt("DRAW", 1)
	case "PCOPY":
		if len(s.Args) != 2 || s.Args[0] == nil || s.Args[1] == nil {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.stmt("PCOPY", args())
	case "PALETTE":
		if len(s.Args) == 1 || len(s.Args) > 2 {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.stmt("PALETTE", args())
	case "PALETTE USING":
		c.arrayElemArg(s.Args[0], s.P)
		c.stmt("PALETTE_USING", 2)
	case "SOUND":
		if len(s.Args) != 2 || s.Args[0] == nil || s.Args[1] == nil {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.stmt("SOUND", args())
	case "PLAY":
		if len(s.Args) != 1 || s.Args[0] == nil {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.strArg(s.Args[0])
		c.stmt("PLAY", 1)
	case "KEY":
		c.numArg(s.Args[0])
		c.strArg(s.Args[1])
		c.stmt("KEY_DEF", 2)
	case "KEY ON", "KEY OFF", "KEY LIST",
		"TRON", "TROFF", "DATE$", "TIME$", "WIDTH LPRINT":
		c.stmt("NOP", args()) // línea de teclas de función y TRON: sin efecto en la terminal
	case "OPTION BASE":
		c.sc.optBase = int(s.Args[0].(*ast.NumberLit).Int)
	case "CHAIN":
		if len(s.Args) != 1 || s.Args[0] == nil {
			c.errAt(s.P, "Expected: expression")
			return
		}
		c.strArg(s.Args[0])
		c.stmt("CHAIN", 1)
	case "OUT", "WAIT":
		if len(s.Args) < 2 || s.Args[0] == nil || s.Args[1] == nil {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.stmt(s.Name, args())
	case "BSAVE", "BLOAD":
		if len(s.Args) == 0 || s.Args[0] == nil || s.Name == "BSAVE" && (len(s.Args) != 3 || s.Args[1] == nil || s.Args[2] == nil) {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		c.strArg(s.Args[0])
		for _, a := range s.Args[1:] {
			if a == nil {
				c.pushNone()
			} else {
				c.numArg(a)
			}
		}
		c.stmt(s.Name, len(s.Args))
	case "IOCTL":
		c.unsupported(s.Name)
	case "ERASE":
		for _, a := range s.Args {
			if _, ok := c.arrayRef(a); !ok {
				c.errAt(s.P, "Array not defined")
				continue
			}
			c.emit(vm.OpErase, 0, 0)
		}
	case "SWAP":
		if len(s.Args) != 2 || s.Args[0] == nil || s.Args[1] == nil {
			c.errAt(s.P, "Expected: variable, variable")
			return
		}
		t1, _ := c.addr(s.Args[0])
		t2, _ := c.addr(s.Args[1])
		if t1.K != t2.K {
			c.errAt(s.P, "Type mismatch")
		}
		c.emit(vm.OpSwap, 0, 0)
	case "READ":
		for _, a := range s.Args {
			if a == nil {
				continue
			}
			c.addr(a)
			c.emit(vm.OpRead, 0, 0)
		}
	case "ERROR":
		c.numArg(s.Args[0])
		c.emit(vm.OpError, 0, 0)
	case "RUN":
		if len(s.Args) == 1 {
			if n, ok := s.Args[0].(*ast.NumberLit); ok && !n.IsFloat {
				at := c.emit(vm.OpConst, 0, 0)
				idx := c.constIdx(vm.Long(0))
				c.prog.Code[at].A = int32(idx)
				// el pc de la etiqueta se resuelve al final
				c.mod.scope.fixups = append(c.mod.scope.fixups, labelRef{at: -1 - idx, field: 'c', label: n.Text, pos: s.P, file: c.file})
				c.stmt("RUN", 1)
				return
			}
		}
		c.stmt("RUN", args())
	default:
		c.errAt(s.P, "Statement not implemented: "+s.Name)
	}
}

// ------------------------------------------------------------- CALL

func (c *Compiler) callStmt(s *ast.CallStmt) {
	key := baseName(s.Name)
	p := c.procs[key]
	if up := strings.ToUpper(s.Name); (p == nil || p.Def == nil) && (up == "ABSOLUTE" || up == "INTERRUPT" || up == "INTERRUPTX") {
		c.machineCall(up, s)
		return
	}
	if p == nil {
		c.errAt(s.P, "Subprogram not defined: "+s.Name)
		return
	}
	if p.IsFunc {
		c.errAt(s.P, "Syntax error")
		return
	}
	c.callProc(p, s.Args, s.P, false)
}

// machineCall compila CALL ABSOLUTE([args,] offset) y CALL INTERRUPT[X](n, inregs, outregs).
func (c *Compiler) machineCall(name string, s *ast.CallStmt) {
	for _, a := range s.Args {
		if a == nil {
			c.errAt(s.P, "Syntax error")
			return
		}
	}
	if name == "ABSOLUTE" {
		if len(s.Args) == 0 {
			c.errAt(s.P, "Argument-count mismatch")
			return
		}
		for _, a := range s.Args[:len(s.Args)-1] {
			if c.isLValue(a) {
				c.addr(a)
			} else {
				c.expr(a)
				c.emit(vm.OpTemp, 0, 0)
			}
		}
		c.numArg(s.Args[len(s.Args)-1])
		c.stmt("ABSOLUTE", len(s.Args))
		return
	}
	if len(s.Args) != 3 {
		c.errAt(s.P, "Argument-count mismatch")
		return
	}
	c.numArg(s.Args[0])
	for _, a := range s.Args[1:] {
		t, ok := c.addr(a)
		if ok && t.K != vm.KRecord {
			c.errAt(a.Position(), "Parameter type mismatch")
		}
	}
	c.stmt("INTERRUPT", 3)
}

// ------------------------------------------------------------- PRINT / WRITE / INPUT

func (c *Compiler) print(s *ast.Print) {
	redirect := false
	switch {
	case s.LPrint:
		c.stmt("PRINT_LPT", 0)
		redirect = true
	case s.File != nil:
		c.fileArg(s.File)
		c.stmt("PRINT_FILE", 1)
		redirect = true
	}
	newline := len(s.Items) == 0 || s.Items[len(s.Items)-1].Sep == 0
	if s.Using != nil {
		c.strArg(s.Using)
		c.stmt("USING_BEGIN", 1)
		for _, it := range s.Items {
			if it.X != nil {
				c.expr(it.X)
				c.stmt("USING_VAL", 1)
			}
		}
		c.pushConst(vm.Bool(newline))
		c.stmt("USING_END", 1)
	} else {
		for _, it := range s.Items {
			if it.X != nil {
				if call, ok := it.X.(*ast.Call); ok && c.isTabSpc(call) {
					c.numArg(call.Args[0])
					c.stmt("PRINT_"+strings.ToUpper(call.Name), 1)
				} else {
					t := c.expr(it.X)
					if t.K == vm.KRecord {
						c.errAt(it.X.Position(), "Type mismatch")
					}
					c.stmt("PRINT_VAL", 1)
				}
			}
			if it.Sep == ',' {
				c.stmt("PRINT_COMMA", 0)
			}
		}
		if newline {
			c.stmt("PRINT_NL", 0)
		}
	}
	if redirect {
		c.stmt("PRINT_DONE", 0)
	}
}

func (c *Compiler) isTabSpc(call *ast.Call) bool {
	up := strings.ToUpper(call.Name)
	if (up != "TAB" && up != "SPC") || len(call.Args) != 1 {
		return false
	}
	return c.lookup(call.Name, true, false) == nil && c.procs[up] == nil
}

func (c *Compiler) write(s *ast.Write) {
	if s.File != nil {
		c.fileArg(s.File)
		c.stmt("PRINT_FILE", 1)
	}
	if len(s.Args) == 0 {
		c.stmt("WRITE_NL", 0)
	}
	for i, a := range s.Args {
		c.expr(a)
		c.pushConst(vm.Bool(i == len(s.Args)-1))
		c.stmt("WRITE_VAL", 2)
	}
	if s.File != nil {
		c.stmt("PRINT_DONE", 0)
	}
}

func (c *Compiler) input(s *ast.Input) {
	if s.File != nil {
		c.fileArg(s.File)
		for _, v := range s.Vars {
			c.addr(v)
		}
		if s.Line {
			c.stmt("LINE_INPUT_FILE", 2)
		} else {
			c.stmt("INPUT_FILE", 1+len(s.Vars))
		}
		return
	}
	flags := 0
	if s.NoNewline {
		flags |= vm.InputNoNewline
	}
	if !s.Line && (s.Prompt == nil || s.PromptSep == ';') {
		flags |= vm.InputQuestion
	}
	c.pushConst(vm.Int(int64(flags)))
	prompt := ""
	if s.Prompt != nil {
		prompt = s.Prompt.Value
	}
	c.pushConst(vm.Str(prompt))
	for _, v := range s.Vars {
		t, ok := c.addr(v)
		if ok && s.Line && t.K != vm.KString {
			c.errAt(s.P, "Type mismatch")
		}
	}
	if s.Line {
		c.stmt("LINE_INPUT", 3)
	} else {
		c.stmt("INPUT", 2+len(s.Vars))
	}
}

var openModes = map[string]int{"INPUT": vm.ModeInput, "OUTPUT": vm.ModeOutput, "APPEND": vm.ModeAppend, "RANDOM": vm.ModeRandom, "BINARY": vm.ModeBinary, "": vm.ModeRandom}

func (c *Compiler) open(s *ast.Open) {
	if s.Legacy {
		c.strArg(s.LegacyMode)
		c.fileArg(s.Num)
		c.strArg(s.File)
		if s.Len != nil {
			c.numArg(s.Len)
		} else {
			c.pushNone()
		}
		c.stmt("OPEN_LEGACY", 4)
		return
	}
	c.strArg(s.File)
	c.pushConst(vm.Int(int64(openModes[s.Mode])))
	c.fileArg(s.Num)
	if s.Len != nil {
		c.numArg(s.Len)
	} else {
		c.pushNone()
	}
	c.stmt("OPEN", 4)
}

// ------------------------------------------------------------- control de flujo

func (c *Compiler) ifStmt(s *ast.If) {
	var ends []int
	c.cond(s.Cond)
	next := c.emit(vm.OpJmpF, 0, 0)
	c.stmts(s.Then)
	for _, ei := range s.ElseIfs {
		ends = append(ends, c.emit(vm.OpJmp, 0, 0))
		c.patchA(next, c.here())
		c.mark(ei.P)
		c.cond(ei.Cond)
		next = c.emit(vm.OpJmpF, 0, 0)
		c.stmts(ei.Body)
	}
	if s.HasElse {
		ends = append(ends, c.emit(vm.OpJmp, 0, 0))
		c.patchA(next, c.here())
		c.stmts(s.Else)
	} else {
		c.patchA(next, c.here())
	}
	for _, e := range ends {
		c.patchA(e, c.here())
	}
}

func (c *Compiler) selectStmt(s *ast.SelectCase) {
	t := c.expr(s.X)
	if t.K == vm.KRecord {
		c.errAt(s.P, "Type mismatch")
		return
	}
	tmp := c.temp(Type{K: t.K})
	c.storeSym(tmp)
	var ends []int
	for _, cl := range s.Cases {
		if cl.Else {
			c.stmts(cl.Body)
			break
		}
		var toBody []int
		var nextCase int
		for _, ct := range cl.Tests {
			check := func(e ast.Expr) {
				if et := c.expr(e); (et.K == vm.KString) != (t.K == vm.KString) {
					c.errAt(e.Position(), "Type mismatch")
				}
			}
			switch ct.Kind {
			case "VALUE":
				c.loadSym(tmp)
				check(ct.X)
				c.emit(vm.OpEq, 0, 0)
				toBody = append(toBody, c.emit(vm.OpJmpT, 0, 0))
			case "IS":
				c.loadSym(tmp)
				check(ct.X)
				c.emit(binOps[ct.Op], 0, 0)
				toBody = append(toBody, c.emit(vm.OpJmpT, 0, 0))
			case "RANGE":
				c.loadSym(tmp)
				check(ct.X)
				c.emit(vm.OpGe, 0, 0)
				skip := c.emit(vm.OpJmpF, 0, 0)
				c.loadSym(tmp)
				check(ct.Y)
				c.emit(vm.OpLe, 0, 0)
				toBody = append(toBody, c.emit(vm.OpJmpT, 0, 0))
				c.patchA(skip, c.here())
			}
		}
		nextCase = c.emit(vm.OpJmp, 0, 0)
		for _, j := range toBody {
			c.patchA(j, c.here())
		}
		c.stmts(cl.Body)
		ends = append(ends, c.emit(vm.OpJmp, 0, 0))
		c.patchA(nextCase, c.here())
	}
	for _, e := range ends {
		c.patchA(e, c.here())
	}
}

func (c *Compiler) forStmt(s *ast.For) {
	v := s.Var.(*ast.Ident)
	sym := c.lookup(v.Name, false, true)
	if sym.Kind != symVar || !sym.Type.K.IsNumeric() {
		c.errAt(s.P, "Type mismatch")
		return
	}
	vt := Type{K: sym.Type.K}
	c.store(s.Var, s.From, s.P)
	lim := c.temp(vt)
	if t := c.expr(s.To); !t.K.IsNumeric() {
		c.errAt(s.P, "Type mismatch")
	}
	c.storeSym(lim)
	step := c.temp(vt)
	if s.Step != nil {
		if t := c.expr(s.Step); !t.K.IsNumeric() {
			c.errAt(s.P, "Type mismatch")
		}
	} else {
		c.pushConst(vm.Int(1))
	}
	c.storeSym(step)

	top := c.here()
	c.emit(vm.OpEvt, 0, 0) // eventos también en el salto de vuelta del bucle
	c.loadSym(sym)
	c.loadSym(lim)
	c.loadSym(step)
	c.emit(vm.OpForCmp, 0, 0)
	exit := c.emit(vm.OpJmpF, 0, 0)
	c.loops = append(c.loops, loopCtx{kind: "FOR"})
	c.stmts(s.Body)
	c.markEnd(s.NextPos)
	c.loadSym(sym)
	c.loadSym(step)
	c.emit(vm.OpAdd, 0, 0)
	c.storeSym(sym)
	c.emit(vm.OpJmp, int32(top), 0)
	c.patchA(exit, c.here())
	c.closeLoop()
}

func (c *Compiler) doStmt(s *ast.DoLoop) {
	top := c.here()
	c.emit(vm.OpEvt, 0, 0) // eventos también en el salto de vuelta del bucle
	exit := -1
	if s.PreCond != nil {
		c.cond(s.PreCond)
		if s.PreUntil {
			exit = c.emit(vm.OpJmpT, 0, 0)
		} else {
			exit = c.emit(vm.OpJmpF, 0, 0)
		}
	}
	c.loops = append(c.loops, loopCtx{kind: "DO"})
	c.stmts(s.Body)
	c.markEnd(s.LoopPos)
	if s.PostCond != nil {
		c.cond(s.PostCond)
		if s.PostUntil {
			c.emit(vm.OpJmpF, int32(top), 0)
		} else {
			c.emit(vm.OpJmpT, int32(top), 0)
		}
	} else {
		c.emit(vm.OpJmp, int32(top), 0)
	}
	if exit >= 0 {
		c.patchA(exit, c.here())
	}
	c.closeLoop()
}

func (c *Compiler) closeLoop() {
	l := c.loops[len(c.loops)-1]
	c.loops = c.loops[:len(c.loops)-1]
	for _, e := range l.exits {
		c.patchA(e, c.here())
	}
}

func (c *Compiler) exit(s *ast.Exit) {
	switch s.What {
	case "FOR", "DO":
		for i := len(c.loops) - 1; i >= 0; i-- {
			if c.loops[i].kind == s.What {
				c.loops[i].exits = append(c.loops[i].exits, c.emit(vm.OpJmp, 0, 0))
				return
			}
		}
		c.errAt(s.P, "EXIT "+s.What+" not within "+s.What)
	default:
		if c.sc.proc == nil {
			c.errAt(s.P, "EXIT "+s.What+" not within "+s.What)
			return
		}
		c.emit(vm.OpRet, 0, 0)
	}
}

func (c *Compiler) jump(s *ast.Jump) {
	switch s.Kind {
	case "GOTO":
		at := c.emit(vm.OpJmp, 0, 0)
		c.refLabel(c.sc, at, 'A', s.Target, s.P)
	case "GOSUB":
		at := c.emit(vm.OpGosub, 0, 0)
		c.refLabel(c.sc, at, 'A', s.Target, s.P)
	case "RETURN":
		at := c.emit(vm.OpReturn, -1, 0)
		if s.Target != "" {
			c.refLabel(c.sc, at, 'A', s.Target, s.P)
		}
	case "RESTORE":
		at := c.emit(vm.OpRestore, 0, 0)
		if s.Target != "" {
			c.mod.scope.dataFix = append(c.mod.scope.dataFix, labelRef{at: at, label: s.Target, pos: s.P, file: c.file})
		}
	}
}

// ------------------------------------------------------------- DIM y compañía

func (c *Compiler) dim(s *ast.Dim) {
	for _, v := range s.Vars {
		t := c.nameType(v.Name)
		asClause := v.Type != nil
		if asClause {
			var ok bool
			if t, ok = c.typeRef(v.Type, v.P); !ok {
				continue
			}
			if t.K == vm.KNone {
				c.errAt(v.P, "Syntax error")
				continue
			}
		}
		switch s.Kind {
		case "SHARED": // dentro de SUB: alias de la variable del módulo
			if c.sc.module == nil {
				c.errAt(s.P, "Illegal outside of SUB/FUNCTION")
				continue
			}
			mod := c.sc.module
			save := c.sc
			c.sc = mod
			g := c.lookupOrDeclare(v, t, asClause)
			c.sc = save
			c.alias(v, g, asClause)
			continue
		case "STATIC":
			if c.sc.module == nil {
				c.errAt(s.P, "Illegal outside of SUB/FUNCTION")
				continue
			}
			saveStatic := c.sc.static
			c.sc.static = true
			sym := c.declare(c.sc, v.Name, t, v.IsArray, asClause, len(v.Bounds))
			c.sc.static = saveStatic
			if v.IsArray && len(v.Bounds) > 0 {
				c.dimArray(sym, v, true, false, s)
			}
			continue
		case "COMMON":
			c.common1(s, v, t, asClause)
			continue
		}
		// DIM / REDIM
		sc := c.sc
		var sym *Symbol
		if s.Shared && sc.module != nil {
			c.errAt(s.P, "Illegal in procedure")
		}
		if s.Kind == "REDIM" {
			sym = c.lookup(c.nameWithType(v, t, asClause), true, false)
		}
		if sym == nil {
			sym = c.declare(sc, v.Name, t, v.IsArray, asClause, len(v.Bounds))
		}
		if s.Shared {
			sym.Shared = true
		}
		if v.IsArray {
			if len(v.Bounds) == 0 {
				continue
			}
			sym.Dims = len(v.Bounds)
			static := s.Kind == "DIM" && !sc.dynamic && (sc.module == nil || sc.static) && c.constBounds(v.Bounds)
			c.dimArray(sym, v, static, s.Kind == "REDIM", s)
		}
	}
}

func (c *Compiler) nameWithType(v ast.DimVar, t Type, asClause bool) string {
	if asClause || v.Suffix != 0 {
		if sfx, ok := kindSuffix[t.K]; ok && sfx != '@' && v.Suffix == 0 {
			return v.Name + string(sfx)
		}
	}
	return v.Name
}

func (c *Compiler) constBounds(bs []ast.Bound) bool {
	for _, b := range bs {
		if _, err := c.evalConst(b.Hi); err != nil {
			return false
		}
		if b.Lo != nil {
			if _, err := c.evalConst(b.Lo); err != nil {
				return false
			}
		}
	}
	return true
}

func (c *Compiler) dimArray(sym *Symbol, v ast.DimVar, static, redim bool, s *ast.Dim) {
	c.addrSym(sym)
	for _, b := range v.Bounds {
		if b.Lo != nil {
			c.numArg(b.Lo)
		} else {
			c.pushConst(vm.Int(int64(c.sc.optBase)))
		}
		c.numArg(b.Hi)
	}
	flags := len(v.Bounds)
	if static {
		flags |= vm.DimStatic
	}
	if redim {
		flags |= vm.DimRedim
	}
	c.emit(vm.OpDim, int32(c.constIdx(sym.Type.zero())), int32(flags))
}

func (c *Compiler) lookupOrDeclare(v ast.DimVar, t Type, asClause bool) *Symbol {
	if s := c.lookup(c.nameWithType(v, t, asClause), v.IsArray, false); s != nil {
		return s
	}
	return c.declare(c.sc, v.Name, t, v.IsArray, asClause, len(v.Bounds))
}

// alias hace visible en el SUB actual una variable del módulo (SHARED).
func (c *Compiler) alias(v ast.DimVar, g *Symbol, asClause bool) {
	key := varKey(baseName(v.Name), g.Type, v.IsArray)
	c.sc.vars[key] = g
	if asClause || g.Type.K == vm.KRecord {
		arr := ""
		if v.IsArray {
			arr = "()"
		}
		c.sc.asDecl[baseName(v.Name)+arr] = key
	}
}

func (c *Compiler) common1(s *ast.Dim, v ast.DimVar, t Type, asClause bool) {
	if c.sc.module != nil {
		c.errAt(s.P, "Illegal in procedure")
		return
	}
	block := c.common[s.Block]
	idx := c.commonCount[c.commonKey(s.Block)]
	var sym *Symbol
	if idx < len(block) {
		// la variable ocupa el mismo slot que en el primer módulo
		sym = &Symbol{Kind: symVar, Name: v.Name, Type: t, Global: true, Slot: block[idx]}
		if v.IsArray {
			sym.Kind = symArray
		}
		key := varKey(baseName(v.Name), t, v.IsArray)
		c.sc.vars[key] = sym
		if asClause {
			arr := ""
			if v.IsArray {
				arr = "()"
			}
			c.sc.asDecl[baseName(v.Name)+arr] = key
		}
	} else {
		sym = c.declare(c.sc, v.Name, t, v.IsArray, asClause, -1)
		c.common[s.Block] = append(c.common[s.Block], sym.Slot)
	}
	if s.Shared {
		sym.Shared = true
	}
	c.commonCount[c.commonKey(s.Block)]++
}

func (c *Compiler) commonKey(block string) string {
	return strconv.Itoa(c.mod.file) + "/" + block
}

// ------------------------------------------------------------- gráficos

func (c *Compiler) flag(b bool) { c.pushConst(vm.Bool(b)) }

func (c *Compiler) coord(p ast.Coord) {
	c.flag(p.Step)
	c.numArg(p.X)
	c.numArg(p.Y)
}

func (c *Compiler) optNum(e ast.Expr) {
	if e == nil {
		c.pushNone()
		return
	}
	c.numArg(e)
}

// arrayElemArg empuja la referencia a un array y el índice inicial
// (arr → desde el primer elemento; arr(i) → desde i).
func (c *Compiler) arrayElemArg(e ast.Expr, pos ast.Pos) {
	switch x := e.(type) {
	case *ast.Ident:
		if s := c.lookup(x.Name, true, false); s != nil {
			c.addrSym(s)
			c.pushNone()
			return
		}
	case *ast.Call:
		if s := c.lookup(x.Name, true, false); s != nil && len(x.Args) <= 1 {
			c.addrSym(s)
			if len(x.Args) == 1 {
				c.numArg(x.Args[0])
			} else {
				c.pushNone()
			}
			return
		}
	}
	c.errAt(pos, "Array not defined")
	c.pushNone()
	c.pushNone()
}

func (c *Compiler) graphics(st ast.Stmt) {
	switch s := st.(type) {
	case *ast.PSet:
		c.flag(s.Preset)
		c.coord(s.At)
		c.optNum(s.Color)
		c.stmt("PSET", 5)
	case *ast.Line:
		c.flag(s.From != nil)
		if s.From != nil {
			c.coord(*s.From)
		} else {
			c.flag(false)
			c.pushConst(vm.Int(0))
			c.pushConst(vm.Int(0))
		}
		c.coord(s.To)
		c.optNum(s.Color)
		box := map[string]int64{"": 0, "B": 1, "BF": 2}[s.Box]
		c.pushConst(vm.Int(box))
		c.optNum(s.Style)
		c.stmt("LINE", 10)
	case *ast.Circle:
		c.coord(s.At)
		c.numArg(s.Radius)
		c.optNum(s.Color)
		c.optNum(s.Start)
		c.optNum(s.End)
		c.optNum(s.Aspect)
		c.stmt("CIRCLE", 8)
	case *ast.Paint:
		c.coord(s.At)
		if s.Fill != nil {
			c.expr(s.Fill) // número o string de mosaico
		} else {
			c.pushNone()
		}
		c.optNum(s.Border)
		c.stmt("PAINT", 5)
	case *ast.GfxGet:
		c.coord(s.From)
		c.coord(s.To)
		c.arrayElemArg(s.Array, s.P)
		c.stmt("GFX_GET", 8)
	case *ast.GfxPut:
		c.coord(s.At)
		c.arrayElemArg(s.Array, s.P)
		action := s.Action
		if action == "" {
			action = "XOR"
		}
		c.pushConst(vm.Str(action))
		c.stmt("GFX_PUT", 6)
	case *ast.Window:
		c.flag(s.From != nil)
		c.flag(s.Screen)
		c.rectArgs(s.From, s.To)
		c.stmt("WINDOW", 6)
	}
}

func (c *Compiler) rectArgs(from, to *ast.Coord) {
	if from == nil {
		for i := 0; i < 4; i++ {
			c.pushConst(vm.Int(0))
		}
		return
	}
	c.numArg(from.X)
	c.numArg(from.Y)
	c.numArg(to.X)
	c.numArg(to.Y)
}

func (c *Compiler) view(s *ast.View) {
	c.flag(s.From != nil)
	c.flag(s.Screen)
	c.rectArgs(s.From, s.To)
	c.optNum(s.Color)
	c.optNum(s.Border)
	c.stmt("VIEW", 8)
}
