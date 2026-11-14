package compiler

import (
	"errors"
	"strings"

	"goqb/lang/ast"
	"goqb/vm"
)

var errNotConst = errors.New("Invalid constant")

// literal convierte un literal numérico a su Value tipado según las reglas de QB.
func literal(n *ast.NumberLit) (vm.Value, error) {
	switch n.Suffix {
	case '%':
		if n.IsFloat {
			return vm.ToInt(vm.Double(n.Float))
		}
		if n.Radix != 10 {
			if n.Int > 0xFFFF {
				return vm.Value{}, vm.ErrOverflow
			}
			return vm.Int(int64(int16(uint16(n.Int)))), nil
		}
		return vm.ToInt(vm.Long(n.Int))
	case '&':
		if n.Radix != 10 {
			if n.Int > 0xFFFFFFFF {
				return vm.Value{}, vm.ErrOverflow
			}
			return vm.Long(int64(int32(uint32(n.Int)))), nil
		}
		if n.IsFloat {
			return vm.ToLong(vm.Double(n.Float))
		}
		return vm.ToLong(vm.Double(float64(n.Int)))
	case '!':
		if n.IsFloat {
			return vm.ToSingle(vm.Double(n.Float))
		}
		return vm.ToSingle(vm.Double(float64(n.Int)))
	case '#':
		if n.IsFloat {
			return vm.Double(n.Float), nil
		}
		return vm.Double(float64(n.Int)), nil
	}
	if n.Radix != 10 {
		switch {
		case n.Int <= 0xFFFF:
			return vm.Int(int64(int16(uint16(n.Int)))), nil
		case n.Int <= 0xFFFFFFFF:
			return vm.Long(int64(int32(uint32(n.Int)))), nil
		}
		return vm.Value{}, vm.ErrOverflow
	}
	if !n.IsFloat {
		switch {
		case n.Int >= -32768 && n.Int <= 32767:
			return vm.Int(n.Int), nil
		case n.Int >= -2147483648 && n.Int <= 2147483647:
			return vm.Long(n.Int), nil
		}
		return vm.Double(float64(n.Int)), nil
	}
	// Sin sufijo: DOUBLE si tiene exponente D o más de 7 dígitos significativos.
	if strings.ContainsAny(n.Text, "Dd") || sigDigits(n.Text) > 7 {
		return vm.Double(n.Float), nil
	}
	return vm.ToSingle(vm.Double(n.Float))
}

func sigDigits(s string) int {
	if i := strings.IndexAny(s, "EeDd"); i >= 0 {
		s = s[:i]
	}
	s = strings.Replace(s, ".", "", 1)
	s = strings.TrimLeft(s, "0")
	return len(s)
}

func typeOf(v vm.Value) Type { return Type{K: v.K} }

// ------------------------------------------------------------- constantes

// evalConst evalúa una expresión constante (CONST, STRING * n).
func (c *Compiler) evalConst(e ast.Expr) (vm.Value, error) {
	switch x := e.(type) {
	case *ast.NumberLit:
		return literal(x)
	case *ast.StringLit:
		return vm.Str(x.Value), nil
	case *ast.Paren:
		return c.evalConst(x.X)
	case *ast.Ident:
		if s := c.lookup(x.Name, false, false); s != nil && s.Kind == symConst {
			return s.Value, nil
		}
		return vm.Value{}, errNotConst
	case *ast.Unary:
		v, err := c.evalConst(x.X)
		if err != nil {
			return v, err
		}
		switch x.Op {
		case "-":
			return vm.Neg(v)
		case "+":
			return v, nil
		case "NOT":
			return vm.Not(v)
		}
	case *ast.Binary:
		a, err := c.evalConst(x.L)
		if err != nil {
			return a, err
		}
		b, err := c.evalConst(x.R)
		if err != nil {
			return b, err
		}
		return binaryValue(x.Op, a, b)
	}
	return vm.Value{}, errNotConst
}

var binOps = map[string]vm.Op{
	"+": vm.OpAdd, "-": vm.OpSub, "*": vm.OpMul, "/": vm.OpDiv, "\\": vm.OpIDiv, "MOD": vm.OpMod,
	"^": vm.OpPow, "=": vm.OpEq, "<>": vm.OpNe, "<": vm.OpLt, ">": vm.OpGt, "<=": vm.OpLe, ">=": vm.OpGe,
	"AND": vm.OpAnd, "OR": vm.OpOr, "XOR": vm.OpXor, "EQV": vm.OpEqv, "IMP": vm.OpImp,
}

func binaryValue(op string, a, b vm.Value) (vm.Value, error) {
	switch op {
	case "+":
		return vm.Add(a, b)
	case "-":
		return vm.Sub(a, b)
	case "*":
		return vm.Mul(a, b)
	case "/":
		return vm.Div(a, b)
	case "\\":
		return vm.IDiv(a, b)
	case "MOD":
		return vm.Mod(a, b)
	case "^":
		return vm.Pow(a, b)
	case "AND", "OR", "XOR", "EQV", "IMP":
		return vm.Logic(binOps[op], a, b)
	}
	r, err := vm.Compare(a, b)
	if err != nil {
		return vm.Value{}, err
	}
	var t bool
	switch op {
	case "=":
		t = r == 0
	case "<>":
		t = r != 0
	case "<":
		t = r < 0
	case ">":
		t = r > 0
	case "<=":
		t = r <= 0
	case ">=":
		t = r >= 0
	}
	return vm.Bool(t), nil
}

// ------------------------------------------------------------- expresiones

func numericResult(a, b vm.Kind) vm.Kind {
	if a > b {
		return a
	}
	return b
}

// expr compila una expresión y devuelve su tipo estático.
func (c *Compiler) expr(e ast.Expr) Type {
	switch x := e.(type) {
	case *ast.NumberLit:
		v, err := literal(x)
		if err != nil {
			c.errAt(x.P, "Overflow")
			v = vm.Int(0)
		}
		c.pushConst(v)
		return typeOf(v)
	case *ast.StringLit:
		c.pushConst(vm.Str(x.Value))
		return Type{K: vm.KString}
	case *ast.Paren:
		return c.expr(x.X)
	case *ast.Ident:
		return c.identExpr(x)
	case *ast.Call:
		return c.callExpr(x)
	case *ast.FieldAccess:
		t, ok := c.addr(x)
		if !ok {
			return Type{K: vm.KSingle}
		}
		c.emit(vm.OpLoadInd, 0, 0)
		return t
	case *ast.Unary:
		t := c.expr(x.X)
		switch x.Op {
		case "-":
			if !t.K.IsNumeric() {
				c.errAt(x.P, "Type mismatch")
			}
			c.emit(vm.OpNeg, 0, 0)
			return t
		case "+":
			return t
		case "NOT":
			if !t.K.IsNumeric() {
				c.errAt(x.P, "Type mismatch")
			}
			c.emit(vm.OpNot, 0, 0)
			if t.K == vm.KInt {
				return t
			}
			return Type{K: vm.KLong}
		}
		c.errAt(x.P, "Syntax error")
		return t
	case *ast.Binary:
		return c.binary(x)
	case *ast.FileNum:
		c.errAt(x.P, "Syntax error")
		return c.expr(x.X)
	}
	c.errAt(e.Position(), "Syntax error")
	return Type{K: vm.KSingle}
}

func (c *Compiler) binary(x *ast.Binary) Type {
	// Plegado de constantes
	if v, err := c.evalConst(x); err == nil {
		c.pushConst(v)
		return typeOf(v)
	}
	lt := c.expr(x.L)
	rt := c.expr(x.R)
	op := binOps[x.Op]
	isStr := lt.K == vm.KString || rt.K == vm.KString
	if isStr && lt.K != rt.K || (lt.K == vm.KRecord || rt.K == vm.KRecord) {
		c.errAt(x.P, "Type mismatch")
	}
	c.emit(op, 0, 0)
	switch x.Op {
	case "+":
		if isStr {
			return Type{K: vm.KString}
		}
		return Type{K: numericResult(lt.K, rt.K)}
	case "-", "*":
		c.numOnly(x, isStr)
		return Type{K: numericResult(lt.K, rt.K)}
	case "/", "^":
		c.numOnly(x, isStr)
		if lt.K == vm.KDouble || rt.K == vm.KDouble {
			return Type{K: vm.KDouble}
		}
		return Type{K: vm.KSingle}
	case "\\", "MOD", "AND", "OR", "XOR", "EQV", "IMP":
		c.numOnly(x, isStr)
		if lt.K == vm.KInt && rt.K == vm.KInt {
			return Type{K: vm.KInt}
		}
		return Type{K: vm.KLong}
	}
	return Type{K: vm.KInt} // relacionales
}

func (c *Compiler) numOnly(x *ast.Binary, isStr bool) {
	if isStr {
		c.errAt(x.P, "Type mismatch")
	}
}

// Funciones intrínsecas que se pueden usar sin paréntesis.
var noArgBuiltins = map[string]bool{
	"INKEY$": true, "TIMER": true, "RND": true, "CSRLIN": true, "DATE$": true, "TIME$": true,
	"ERR": true, "ERL": true, "FREEFILE": true, "COMMAND$": true, "ERDEV": true, "ERDEV$": true,
}

func (c *Compiler) identExpr(x *ast.Ident) Type {
	// Campo de registro con nombre punteado: j.pos.x
	if strings.Contains(x.Name, ".") {
		if t, ok := c.dottedAddr(x); ok {
			c.emit(vm.OpLoadInd, 0, 0)
			return t
		}
	}
	s := c.lookup(x.Name, false, false)
	if s != nil && s.Kind == symConst {
		c.pushConst(s.Value)
		return s.Type
	}
	if s == nil || s.Kind != symVar {
		key := baseName(x.Name)
		// FUNCTION sin argumentos (fuera de su propio cuerpo, donde el nombre es el valor de retorno)
		if p := c.procs[key]; p != nil && p.IsFunc && (c.sc.proc != p || s == nil) {
			return c.callProc(p, nil, x.P, true)
		}
		up := strings.ToUpper(x.Name)
		if noArgBuiltins[up] && s == nil {
			return c.builtin(up, nil, x.P)
		}
	}
	if s == nil {
		s = c.lookup(x.Name, false, true)
	}
	c.loadSym(s)
	return s.Type
}

// ------------------------------------------------------------- direcciones

// isLValue reporta si e puede pasarse por referencia.
func (c *Compiler) isLValue(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		s := c.lookup(x.Name, false, false)
		if s != nil && s.Kind == symConst {
			return false
		}
		if p := c.procs[baseName(x.Name)]; p != nil && p.IsFunc && c.sc.proc != p {
			return false
		}
		if s == nil && noArgBuiltins[strings.ToUpper(x.Name)] {
			return false
		}
		return true
	case *ast.Call:
		if x.Keyword {
			return false
		}
		if s := c.lookup(x.Name, true, false); s != nil {
			return true
		}
		if c.procs[baseName(x.Name)] != nil || strings.HasPrefix(strings.ToUpper(x.Name), "FN") {
			return false
		}
		if _, ok := vm.BuiltinIndex[strings.ToUpper(x.Name)]; ok {
			return false
		}
		return true // array implícito
	case *ast.FieldAccess:
		return true
	}
	return false
}

// addr compila la dirección de un lvalue y devuelve su tipo.
func (c *Compiler) addr(e ast.Expr) (Type, bool) {
	switch x := e.(type) {
	case *ast.Ident:
		if strings.Contains(x.Name, ".") {
			if t, ok := c.dottedAddr(x); ok {
				return t, true
			}
		}
		s := c.lookup(x.Name, false, true)
		if s.Kind == symConst {
			c.errAt(x.P, "Duplicate definition")
			return s.Type, false
		}
		c.addrSym(s)
		return s.Type, true
	case *ast.Call:
		s := c.arraySym(x)
		if s == nil {
			c.errAt(x.P, "Array not defined")
			return Type{}, false
		}
		c.addrSym(s)
		for _, a := range x.Args {
			t := c.expr(a)
			if !t.K.IsNumeric() {
				c.errAt(a.Position(), "Type mismatch")
			}
		}
		if s.Dims >= 0 && s.Dims != len(x.Args) {
			c.errAt(x.P, "Wrong number of dimensions")
		}
		c.emit(vm.OpIndex, int32(len(x.Args)), 0)
		return s.Type, true
	case *ast.FieldAccess:
		t, ok := c.addr(x.X)
		if !ok {
			return t, false
		}
		return c.fieldChain(t, strings.Split(x.Field, "."), x.P)
	}
	c.errAt(e.Position(), "Variable required")
	return Type{}, false
}

// dottedAddr resuelve a.b.c cuando a es una variable de tipo registro.
func (c *Compiler) dottedAddr(x *ast.Ident) (Type, bool) {
	parts := strings.Split(x.Name, ".")
	s := c.lookup(parts[0], false, false)
	if s == nil || s.Type.K != vm.KRecord {
		return Type{}, false
	}
	c.addrSym(s)
	return c.fieldChain(s.Type, parts[1:], x.P)
}

func (c *Compiler) fieldChain(t Type, fields []string, pos ast.Pos) (Type, bool) {
	for _, f := range fields {
		f = strings.ToUpper(strings.TrimRight(f, "%&!#$"))
		if t.K != vm.KRecord {
			c.errAt(pos, "Element not defined")
			return t, false
		}
		idx := -1
		for i, fd := range t.Rec.Fields {
			if fd.Name == f {
				idx = i
				break
			}
		}
		if idx < 0 {
			c.errAt(pos, "Element not defined: "+f)
			return t, false
		}
		c.emit(vm.OpField, int32(idx), 0)
		fd := t.Rec.Fields[idx]
		t = Type{K: fd.Proto.K}
		switch fd.Proto.K {
		case vm.KRecord:
			t.Rec = fd.Proto.R.T
		case vm.KString:
			t.Fixed = int(fd.Proto.I)
		}
	}
	return t, true
}

// arraySym resuelve el array de x (creándolo implícitamente si corresponde).
func (c *Compiler) arraySym(x *ast.Call) *Symbol {
	if s := c.lookup(x.Name, true, false); s != nil {
		return s
	}
	if x.Empty {
		return nil
	}
	s := c.lookup(x.Name, true, true)
	s.Dims = len(x.Args)
	sc := c.sc
	if sc.isDefFn {
		sc = sc.module
	}
	sc.implicit = append(sc.implicit, implicitArr{s, len(x.Args)})
	return s
}

// arrayRef compila la referencia a un array completo (arg "a()" o nombre).
func (c *Compiler) arrayRef(e ast.Expr) (*Symbol, bool) {
	var name string
	switch x := e.(type) {
	case *ast.Call:
		if !x.Empty {
			return nil, false
		}
		name = x.Name
	case *ast.Ident:
		name = x.Name
	default:
		return nil, false
	}
	s := c.lookup(name, true, false)
	if s == nil {
		return nil, false
	}
	c.addrSym(s)
	return s, true
}

// store asigna el valor de e a target.
func (c *Compiler) store(target ast.Expr, value ast.Expr, pos ast.Pos) {
	// Asignación directa a una variable simple (sin campos).
	if id, ok := target.(*ast.Ident); ok && !(strings.Contains(id.Name, ".") && c.isRecordPath(id.Name)) {
		s := c.lookup(id.Name, false, true)
		if s.Kind == symConst {
			c.errAt(pos, "Duplicate definition")
			return
		}
		vt := c.expr(value)
		c.checkAssign(s.Type, vt, pos)
		c.storeSym(s)
		return
	}
	t, ok := c.addr(target)
	if !ok {
		return
	}
	vt := c.expr(value)
	c.checkAssign(t, vt, pos)
	c.emit(vm.OpStoreInd, 0, 0)
}

func (c *Compiler) isRecordPath(name string) bool {
	parts := strings.Split(name, ".")
	s := c.lookup(parts[0], false, false)
	return s != nil && s.Type.K == vm.KRecord
}

func (c *Compiler) checkAssign(dst, src Type, pos ast.Pos) {
	switch {
	case dst.K == vm.KString && src.K != vm.KString,
		dst.K.IsNumeric() && !src.K.IsNumeric(),
		dst.K == vm.KRecord && (src.K != vm.KRecord || src.Rec != dst.Rec):
		c.errAt(pos, "Type mismatch")
	}
}
