package compiler

import (
	"strings"

	"goqb/lang/ast"
	"goqb/vm"
)

// callExpr compila nombre(args): elemento de array, FUNCTION, DEF FN o intrínseca.
func (c *Compiler) callExpr(x *ast.Call) Type {
	up := strings.ToUpper(x.Name)
	if !x.Keyword {
		if s := c.lookup(x.Name, true, false); s != nil {
			t, ok := c.addr(x)
			if ok {
				c.emit(vm.OpLoadInd, 0, 0)
			}
			return t
		}
		if p := c.procs[baseName(x.Name)]; p != nil {
			if !p.IsFunc {
				c.errAt(x.P, "Syntax error")
			}
			return c.callProc(p, x.Args, x.P, true)
		}
	}
	if up == "TAB" || up == "SPC" {
		c.errAt(x.P, "Syntax error")
		return Type{K: vm.KString}
	}
	if up == "LEN" && len(x.Args) == 1 {
		if t, ok := c.lenOf(x.Args[0]); ok {
			return t
		}
	}
	if _, ok := vm.BuiltinIndex[up]; ok {
		return c.builtin(up, x.Args, x.P)
	}
	if x.Keyword {
		c.errAt(x.P, "Syntax error")
		return Type{K: vm.KInt}
	}
	if strings.HasPrefix(up, "FN") {
		c.errAt(x.P, "Function not defined: "+x.Name)
		return c.nameType(x.Name)
	}
	t, ok := c.addr(x) // array implícito
	if ok {
		c.emit(vm.OpLoadInd, 0, 0)
	}
	return t
}

// lenOf resuelve LEN(variable) para variables no string (tamaño en bytes).
func (c *Compiler) lenOf(e ast.Expr) (Type, bool) {
	if !c.isLValue(e) {
		return Type{}, false
	}
	// Evaluamos el tipo sin emitir código: compilamos en un buffer descartable.
	save := len(c.prog.Code)
	saveConsts := len(c.prog.Consts)
	t, ok := c.addr(e)
	c.prog.Code = c.prog.Code[:save]
	c.prog.Consts = c.prog.Consts[:saveConsts]
	if !ok || t.K == vm.KString && t.Fixed == 0 {
		return Type{}, false
	}
	c.pushConst(vm.Int(int64(vm.SizeOf(t.zero()))))
	return Type{K: vm.KInt}, true
}

// builtin compila una llamada a función intrínseca.
func (c *Compiler) builtin(name string, args []ast.Expr, pos ast.Pos) Type {
	idx := vm.BuiltinIndex[name]
	b := vm.Builtins[idx]
	req, opt, _ := strings.Cut(b.Args, "|")
	if len(args) < len(req) || len(args) > len(req)+len(opt) {
		c.errAt(pos, "Argument-count mismatch")
		return Type{K: b.Ret}
	}
	spec := req + opt
	var first Type
	for i, a := range args {
		var t Type
		switch spec[i] {
		case 'a':
			if _, ok := c.arrayRef(a); !ok {
				c.errAt(a.Position(), "Array not defined")
			}
			continue
		case 'v':
			if !c.isLValue(a) {
				c.errAt(a.Position(), "Variable required")
				c.pushNone()
				continue
			}
			t, _ = c.addr(a)
			continue
		case 'f':
			if fn, ok := a.(*ast.FileNum); ok {
				a = fn.X
			}
			t = c.expr(a)
			if !t.K.IsNumeric() {
				c.errAt(a.Position(), "Type mismatch")
			}
		case 'n':
			t = c.expr(a)
			if !t.K.IsNumeric() {
				c.errAt(a.Position(), "Type mismatch")
			}
		case 's':
			t = c.expr(a)
			if t.K != vm.KString {
				c.errAt(a.Position(), "Type mismatch")
			}
		default:
			t = c.expr(a)
		}
		if i == 0 {
			first = t
		}
	}
	c.emit(vm.OpBuiltin, int32(idx), int32(len(args)))
	switch {
	case b.SameKind:
		return first
	case b.FloatKind:
		if first.K == vm.KDouble {
			return first
		}
		return Type{K: vm.KSingle}
	}
	return Type{K: b.Ret}
}

// callProc compila la llamada a un SUB/FUNCTION/DEF FN con sus argumentos.
func (c *Compiler) callProc(p *procInfo, args []ast.Expr, pos ast.Pos, isExpr bool) Type {
	if isExpr && !p.IsFunc {
		c.errAt(pos, "Syntax error")
	}
	if p.Def == nil && p.DefFn == nil {
		c.errAt(pos, "Subprogram not defined: "+p.Name)
		return p.Ret
	}
	if len(args) != len(p.Params) {
		c.errAt(pos, "Argument-count mismatch")
		return p.Ret
	}
	for i, a := range args {
		pr := p.Params[i]
		c.passArg(pr, a, p.DefFn != nil)
	}
	at := c.emit(vm.OpCall, int32(p.Index), int32(len(args)))
	if p.Index < 0 { // procedimiento aún no compilado: se parchea al final
		c.pendingCalls = append(c.pendingCalls, pendingCall{at, p})
	}
	return p.Ret
}

type pendingCall struct {
	at int
	p  *procInfo
}

// passArg empuja un argumento: referencia para variables (BYREF por
// defecto en QB), temporal convertido para expresiones.
func (c *Compiler) passArg(pr paramInfo, a ast.Expr, byVal bool) {
	if pr.IsArray {
		s, ok := c.arrayRef(a)
		if !ok {
			c.errAt(a.Position(), "Array not defined")
			c.pushNone()
			return
		}
		if pr.Type.K != vm.KNone && s.Type.K != pr.Type.K {
			c.errAt(a.Position(), "Parameter type mismatch")
		}
		return
	}
	if !byVal && c.isLValue(a) {
		save := len(c.prog.Code)
		t, ok := c.addr(a)
		if ok && (pr.Type.K == vm.KNone || t.K == pr.Type.K && t.Rec == pr.Type.Rec) {
			return
		}
		// Tipo distinto: QB lo rechaza; pasamos una copia convertida.
		c.prog.Code = c.prog.Code[:save]
		if ok && (pr.Type.K == vm.KRecord || t.K == vm.KRecord) {
			c.errAt(a.Position(), "Parameter type mismatch")
		}
	}
	t := c.expr(a)
	switch {
	case pr.Type.K == vm.KString || pr.Type.K == vm.KRecord:
		if t.K != pr.Type.K {
			c.errAt(a.Position(), "Parameter type mismatch")
		}
		c.emit(vm.OpTemp, 0, 0)
	case pr.Type.K.IsNumeric():
		if !t.K.IsNumeric() {
			c.errAt(a.Position(), "Parameter type mismatch")
		}
		c.emit(vm.OpTempK, int32(pr.Type.K), 0)
	default:
		c.emit(vm.OpTemp, 0, 0)
	}
}

// defFn compila DEF FN en línea (con salto por encima del cuerpo).
func (c *Compiler) defFn(d *ast.DefFn) {
	key := baseName(d.Name)
	if c.sc.module != nil {
		c.errAt(d.P, "DEF FN not allowed in procedure")
		return
	}
	p := &procInfo{Name: d.Name, Key: key, IsFunc: true, DefFn: d, Ret: c.nameType(d.Name), Index: len(c.prog.Procs)}
	vp := &vm.Proc{Name: d.Name, IsFunc: true, NParams: len(d.Params), File: c.file, Line: d.P.Line}
	c.prog.Procs = append(c.prog.Procs, vp)

	saveSc := c.sc
	sc := newScope(saveSc)
	sc.isDefFn = true
	sc.proc = p
	for _, pr := range d.Params {
		t := c.nameType(pr.Name)
		if pr.Type != nil {
			if tt, ok := c.typeRef(pr.Type, pr.P); ok {
				t = tt
			}
		}
		p.Params = append(p.Params, paramInfo{Name: pr.Name, Type: t})
		sc.locals = append(sc.locals, vm.Value{K: vm.KPtr})
		key := varKey(baseName(pr.Name), t, false)
		sc.vars[key] = &Symbol{Kind: symVar, Name: pr.Name, Type: t, Slot: len(sc.locals) - 1}
		if suffixOf(pr.Name) == 0 {
			sc.asDecl[baseName(pr.Name)] = key
		}
	}
	sc.locals = append(sc.locals, p.Ret.zero())
	vp.RetSlot = len(sc.locals) - 1
	sc.vars[varKey(key, p.Ret, false)] = &Symbol{Kind: symVar, Name: d.Name, Type: p.Ret, Slot: vp.RetSlot}
	sc.asDecl[key] = varKey(key, p.Ret, false)

	jmp := c.emit(vm.OpJmp, 0, 0)
	vp.Entry = c.here()
	c.sc = sc
	if d.Expr != nil {
		t := c.expr(d.Expr)
		c.checkAssign(p.Ret, t, d.P)
		c.emit(vm.OpStoreL, int32(vp.RetSlot), 0)
	} else {
		c.stmts(d.Body)
	}
	c.emit(vm.OpRet, 0, 0)
	c.resolveLabels(sc)
	c.sc = saveSc
	vp.Locals = sc.locals
	vp.Start, vp.End = vp.Entry, c.here()
	c.procScopes[p.Index] = sc
	c.patchA(jmp, c.here())
	// Registrar después de compilar el cuerpo: DEF FN no es recursivo.
	c.procs[key] = p
}
