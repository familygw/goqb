// Package compiler traduce el AST de QuickBASIC 4.5 a bytecode de la VM.
//
// Hace el análisis semántico en el mismo recorrido: resolución de
// variables según sufijo/DEFtype/AS, arrays vs. funciones, registros,
// constantes, verificación de tipos y de cantidad de argumentos.
//
// Pasadas:
//  0. TYPEs (pueden venir de $INCLUDE).
//  1. Firmas de SUB/FUNCTION (DECLARE y definiciones), con el DEFtype
//     vigente en el punto de la definición.
//  2. Código de nivel de módulo, luego cada procedimiento.
package compiler

import (
	"strings"

	"goqb/lang/ast"
	"goqb/lang/source"
	"goqb/vm"
)

// Type es el tipo estático de una expresión o variable.
type Type struct {
	K     vm.Kind
	Rec   *vm.RecordType
	Fixed int // STRING * n
}

func (t Type) String() string {
	if t.K == vm.KRecord && t.Rec != nil {
		return t.Rec.Name
	}
	return t.K.String()
}

// zero devuelve el valor inicial de una variable del tipo t.
func (t Type) zero() vm.Value {
	switch t.K {
	case vm.KRecord:
		return vm.RecordValue(t.Rec)
	case vm.KString:
		if t.Fixed > 0 {
			return vm.FixedStr(t.Fixed)
		}
		return vm.Str("")
	case vm.KInt, vm.KLong, vm.KSingle, vm.KDouble:
		return vm.Value{K: t.K}
	}
	return vm.Value{}
}

var suffixKind = map[byte]vm.Kind{'%': vm.KInt, '&': vm.KLong, '!': vm.KSingle, '#': vm.KDouble, '$': vm.KString}
var kindSuffix = map[vm.Kind]byte{vm.KInt: '%', vm.KLong: '&', vm.KSingle: '!', vm.KDouble: '#', vm.KString: '$', vm.KRecord: '@'}

type symKind int

const (
	symVar symKind = iota
	symArray
	symConst
)

// Symbol es una variable, array o constante.
type Symbol struct {
	Kind   symKind
	Name   string
	Type   Type
	Global bool
	Slot   int
	Dims   int // arrays: cantidad de dimensiones (-1 si no se conoce)
	Value  vm.Value
	Shared bool // DIM SHARED a nivel de módulo
}

type labelRef struct {
	at    int // índice de instrucción
	field byte
	tbl   int // para tablas de ON GOTO (field == 't')
	idx   int
	label string
	pos   ast.Pos
	file  int
}

type implicitArr struct {
	sym  *Symbol
	dims int
}

// scope es el ámbito de un módulo, SUB/FUNCTION o DEF FN.
type scope struct {
	module   *scope // nil si es el ámbito de módulo
	vars     map[string]*Symbol
	asDecl   map[string]string // nombre base (+"()") → clave del símbolo declarado con AS
	defTypes [26]vm.Kind
	labels   map[string]int
	dataLbl  map[string]int
	fixups   []labelRef
	dataFix  []labelRef
	proc     *procInfo
	locals   []vm.Value
	static   bool // SUB ... STATIC: las variables locales viven en globales
	isDefFn  bool
	implicit []implicitArr
	optBase  int
	dynamic  bool // $DYNAMIC
}

func newScope(module *scope) *scope {
	s := &scope{module: module, vars: map[string]*Symbol{}, asDecl: map[string]string{},
		labels: map[string]int{}, dataLbl: map[string]int{}}
	for i := range s.defTypes {
		s.defTypes[i] = vm.KSingle
	}
	if module != nil {
		s.defTypes = module.defTypes
		s.optBase = module.optBase
		s.dynamic = module.dynamic
	}
	return s
}

type paramInfo struct {
	Name    string
	Type    Type
	IsArray bool
}

type procInfo struct {
	Name     string
	Key      string
	IsFunc   bool
	Ret      Type
	Params   []paramInfo
	Index    int
	Def      *ast.Proc
	DefFn    *ast.DefFn
	File     int
	defTypes [26]vm.Kind
	mod      *modState
	declared bool
}

type modState struct {
	f     *ast.File
	file  int
	scope *scope
}

type loopCtx struct {
	kind  string // FOR, DO
	exits []int  // saltos a parchear con la salida
}

// Compiler mantiene el estado de compilación de un programa.
type Compiler struct {
	prog    *vm.Program
	diags   []source.Diag
	types   map[string]*vm.RecordType
	procs   map[string]*procInfo
	plist   []*procInfo
	common  map[string][]int
	mods    []*modState
	files   map[string]int
	noneIdx int

	mod          *modState
	sc           *scope
	file         int
	erl          int
	loops        []loopCtx
	pendingCalls []pendingCall
	commonCount  map[string]int
}

// Compile compila un programa (uno o varios módulos).
func Compile(prog *source.Program) (*vm.Program, *Compiler, []source.Diag) {
	c := &Compiler{
		prog:   &vm.Program{GlobalNames: map[string]int{}},
		types:  map[string]*vm.RecordType{},
		procs:  map[string]*procInfo{},
		common: map[string][]int{},
		files:  map[string]int{},

		commonCount: map[string]int{},
	}
	c.noneIdx = c.constIdx(vm.Value{})
	for _, f := range prog.Modules {
		m := &modState{f: f, file: c.fileIdx(f.Name)}
		m.scope = newScope(nil)
		c.mods = append(c.mods, m)
	}
	// Pasada 0 y 1
	for _, m := range c.mods {
		c.mod, c.sc, c.file = m, m.scope, m.file
		c.collect(m.f.Stmts, true)
	}
	for _, m := range c.mods { // restablecer DEFtype/OPTION BASE para la pasada 2
		def := newScope(nil)
		m.scope.defTypes, m.scope.optBase = def.defTypes, 0
	}
	// Pasada 2: código de módulo
	start := c.emit(vm.OpJmp, 0, 0) // a los prólogos
	bodyStart := len(c.prog.Code)
	for i, m := range c.mods {
		c.mod, c.sc, c.file, c.erl = m, m.scope, m.file, 0
		c.stmts(m.f.Stmts)
		if i == 0 {
			c.emit(vm.OpEnd, 0, 0)
		}
	}
	// Procedimientos
	for _, p := range c.plist {
		if p.Def != nil {
			c.compileProc(p)
		}
	}
	for _, pc := range c.pendingCalls {
		c.prog.Code[pc.at].A = int32(pc.p.Index)
	}
	c.pendingCalls = nil
	// Prólogo global: arrays implícitos de los módulos
	c.prog.Entry = len(c.prog.Code)
	c.patchA(start, c.prog.Entry)
	for _, m := range c.mods {
		c.sc = m.scope
		c.prologue(m.scope)
	}
	c.emit(vm.OpJmp, int32(bodyStart), 0)
	c.prog.Entry = start
	for _, m := range c.mods {
		c.resolveLabels(m.scope)
	}
	sortStmts(c.prog.Stmts)
	return c.prog, c, c.diags
}

func (c *Compiler) fileIdx(name string) int {
	if i, ok := c.files[name]; ok {
		return i
	}
	c.files[name] = len(c.prog.Files)
	c.prog.Files = append(c.prog.Files, name)
	return c.files[name]
}

func (c *Compiler) errAt(pos ast.Pos, msg string) {
	c.diags = append(c.diags, source.Diag{File: c.prog.Files[c.file], Pos: pos, Msg: msg})
}

// ------------------------------------------------------------- emisión

func (c *Compiler) emit(op vm.Op, a, b int32) int {
	c.prog.Code = append(c.prog.Code, vm.Instr{Op: op, A: a, B: b})
	return len(c.prog.Code) - 1
}

func (c *Compiler) here() int { return len(c.prog.Code) }

func (c *Compiler) patchA(at, target int) { c.prog.Code[at].A = int32(target) }

func (c *Compiler) constIdx(v vm.Value) int {
	c.prog.Consts = append(c.prog.Consts, v)
	return len(c.prog.Consts) - 1
}

func (c *Compiler) pushConst(v vm.Value) { c.emit(vm.OpConst, int32(c.constIdx(v)), 0) }
func (c *Compiler) pushNone()            { c.emit(vm.OpConst, int32(c.noneIdx), 0) }

func (c *Compiler) stmt(name string, nargs int) {
	i, ok := vm.StmtIndex[name]
	if !ok {
		panic("compiler: sentencia intrínseca desconocida " + name)
	}
	c.emit(vm.OpStmt, int32(i), int32(nargs))
}

func (c *Compiler) unsupported(name string) {
	c.emit(vm.OpUnsupported, int32(c.constIdx(vm.Str(name))), 0)
}

// ------------------------------------------------------------- pasada 0/1

// collect registra TYPEs y firmas de procedimientos.
func (c *Compiler) collect(stmts []ast.Stmt, module bool) {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ast.Meta:
			if s.Resolved != "" {
				save := c.file
				c.file = c.fileIdx(s.Resolved)
				c.collect(s.Stmts, module)
				c.file = save
			}
		case *ast.TypeDecl:
			c.defineType(s)
		case *ast.DefType:
			c.applyDefType(s)
		case *ast.Simple:
			if s.Name == "OPTION BASE" {
				c.sc.optBase = int(s.Args[0].(*ast.NumberLit).Int)
			}
		case *ast.Declare:
			c.declareProc(s.Name, s.Function, s.Params, nil, s.P, true)
		case *ast.Proc:
			p := c.declareProc(s.Name, s.Function, s.Params, s, s.P, false)
			if p != nil {
				p.defTypes = c.sc.defTypes
				p.mod = c.mod
				p.File = c.file
			}
		}
	}
}

func (c *Compiler) applyDefType(s *ast.DefType) {
	k := map[string]vm.Kind{"INTEGER": vm.KInt, "LONG": vm.KLong, "SINGLE": vm.KSingle, "DOUBLE": vm.KDouble, "STRING": vm.KString}[s.Type]
	for _, r := range s.Ranges {
		for l := r.From; l <= r.To; l++ {
			c.sc.defTypes[l-'A'] = k
		}
	}
}

func (c *Compiler) defineType(s *ast.TypeDecl) {
	if _, dup := c.types[s.Name]; dup {
		return // ya reportado por check
	}
	rt := &vm.RecordType{Name: s.Name}
	for _, f := range s.Fields {
		t, ok := c.typeRef(&f.Type, f.P)
		if !ok {
			continue
		}
		z := t.zero()
		size := vm.SizeOf(z)
		rt.Fields = append(rt.Fields, vm.FieldDef{Name: strings.ToUpper(f.Name), Proto: z, Size: size})
		rt.Size += size
	}
	c.types[s.Name] = rt
}

// typeRef convierte una cláusula AS en un Type.
func (c *Compiler) typeRef(r *ast.TypeRef, pos ast.Pos) (Type, bool) {
	switch r.Name {
	case "INTEGER":
		return Type{K: vm.KInt}, true
	case "LONG":
		return Type{K: vm.KLong}, true
	case "SINGLE":
		return Type{K: vm.KSingle}, true
	case "DOUBLE":
		return Type{K: vm.KDouble}, true
	case "STRING":
		t := Type{K: vm.KString}
		if r.FixedLen != nil {
			v, err := c.evalConst(r.FixedLen)
			if err != nil || !v.K.IsNumeric() || v.Num() < 1 || v.Num() > 32767 {
				c.errAt(pos, "Illegal number")
				return t, false
			}
			t.Fixed = int(v.Num())
		}
		return t, true
	case "ANY":
		return Type{K: vm.KNone}, true
	}
	rt, ok := c.types[r.Name]
	if !ok {
		c.errAt(pos, "Type not defined: "+r.Name)
		return Type{}, false
	}
	return Type{K: vm.KRecord, Rec: rt}, true
}

func baseName(name string) string { return strings.ToUpper(strings.TrimRight(name, "%&!#$")) }

func suffixOf(name string) byte {
	if name != "" && strings.IndexByte("%&!#$", name[len(name)-1]) >= 0 {
		return name[len(name)-1]
	}
	return 0
}

// defaultType es el tipo implícito de un nombre sin sufijo en el ámbito actual.
func (c *Compiler) defaultType(name string) Type {
	b := baseName(name)
	if b == "" || b[0] < 'A' || b[0] > 'Z' {
		return Type{K: vm.KSingle}
	}
	return Type{K: c.sc.defTypes[b[0]-'A']}
}

// nameType: tipo por sufijo o DEFtype.
func (c *Compiler) nameType(name string) Type {
	if k, ok := suffixKind[suffixOf(name)]; ok {
		return Type{K: k}
	}
	return c.defaultType(name)
}

func (c *Compiler) declareProc(name string, isFunc bool, params []ast.Param, def *ast.Proc, pos ast.Pos, isDecl bool) *procInfo {
	key := baseName(name)
	p := c.procs[key]
	if p == nil {
		p = &procInfo{Name: name, Key: key, IsFunc: isFunc, Index: -1}
		c.procs[key] = p
		c.plist = append(c.plist, p)
	} else if p.IsFunc != isFunc {
		c.errAt(pos, "Duplicate definition: "+name)
		return nil
	}
	if isDecl && p.Def != nil {
		return p // la definición manda
	}
	p.Params = p.Params[:0]
	for _, pr := range params {
		t := c.nameType(pr.Name)
		if pr.Type != nil {
			if tt, ok := c.typeRef(pr.Type, pr.P); ok {
				t = tt
			}
		}
		p.Params = append(p.Params, paramInfo{Name: pr.Name, Type: t, IsArray: pr.IsArray})
	}
	if isFunc {
		p.Ret = c.nameType(name)
	}
	if def != nil {
		p.Def = def
		p.Name = name
	}
	p.declared = p.declared || isDecl
	return p
}

// ------------------------------------------------------------- símbolos

func (c *Compiler) newSlot(sc *scope, proto vm.Value, name string) (bool, int) {
	if sc.module == nil || sc.static {
		c.prog.Globals = append(c.prog.Globals, proto)
		idx := len(c.prog.Globals) - 1
		if sc.module == nil {
			c.prog.GlobalNames[name] = idx
		}
		return true, idx
	}
	sc.locals = append(sc.locals, proto)
	return false, len(sc.locals) - 1
}

func varKey(base string, t Type, array bool) string {
	k := base + string(kindSuffix[t.K])
	if array {
		k += "()"
	}
	return k
}

// lookup busca una variable o array según las reglas de QB.
// create: crear la variable implícitamente si no existe.
func (c *Compiler) lookup(name string, array bool, create bool) *Symbol {
	base := baseName(name)
	sfx := suffixOf(name)
	arr := ""
	if array {
		arr = "()"
	}
	scopes := []*scope{c.sc}
	if c.sc.module != nil {
		scopes = append(scopes, c.sc.module)
	}
	for i, sc := range scopes {
		visible := func(s *Symbol) bool {
			return i == 0 || s.Shared || s.Kind == symConst || c.sc.isDefFn
		}
		if key, ok := sc.asDecl[base+arr]; ok {
			if s := sc.vars[key]; s != nil && visible(s) {
				if sfx == 0 || suffixKind[sfx] == s.Type.K {
					return s
				}
			}
		}
		if !array {
			if s := sc.vars[base]; s != nil && s.Kind == symConst && (sfx == 0 || suffixKind[sfx] == s.Type.K) {
				if visible(s) {
					return s
				}
			}
		}
		t := c.nameType(name) // el DEFtype del ámbito actual decide el tipo implícito
		if s := sc.vars[varKey(base, t, array)]; s != nil && visible(s) {
			return s
		}
	}
	if !create {
		return nil
	}
	target := c.sc
	if c.sc.isDefFn { // en DEF FN, las variables no parámetro son del módulo
		target = c.sc.module
	}
	return c.declare(target, name, c.nameType(name), array, false, -1)
}

// declare crea un símbolo en sc. asClause indica que el tipo vino de AS.
func (c *Compiler) declare(sc *scope, name string, t Type, array, asClause bool, dims int) *Symbol {
	base := baseName(name)
	key := varKey(base, t, array)
	if s := sc.vars[key]; s != nil {
		return s
	}
	var proto vm.Value
	kind := symVar
	if array {
		kind = symArray
		proto = vm.Value{K: vm.KArray}
	} else {
		proto = t.zero()
	}
	internal := key
	if sc.module != nil && sc.proc != nil {
		internal = sc.proc.Key + "." + key
	}
	g, slot := c.newSlot(sc, proto, internal)
	s := &Symbol{Kind: kind, Name: name, Type: t, Global: g, Slot: slot, Dims: dims}
	sc.vars[key] = s
	if asClause || t.K == vm.KRecord {
		arr := ""
		if array {
			arr = "()"
		}
		sc.asDecl[base+arr] = key
	}
	return s
}

// temp reserva una variable oculta del tipo t en el ámbito actual.
func (c *Compiler) temp(t Type) *Symbol {
	sc := c.sc
	g, slot := c.newSlot(sc, t.zero(), "")
	return &Symbol{Kind: symVar, Type: t, Global: g, Slot: slot}
}

func (c *Compiler) loadSym(s *Symbol) {
	if s.Global {
		c.emit(vm.OpLoadG, int32(s.Slot), 0)
	} else {
		c.emit(vm.OpLoadL, int32(s.Slot), 0)
	}
}

func (c *Compiler) storeSym(s *Symbol) {
	if s.Global {
		c.emit(vm.OpStoreG, int32(s.Slot), 0)
	} else {
		c.emit(vm.OpStoreL, int32(s.Slot), 0)
	}
}

func (c *Compiler) addrSym(s *Symbol) {
	if s.Global {
		c.emit(vm.OpAddrG, int32(s.Slot), 0)
	} else {
		c.emit(vm.OpAddrL, int32(s.Slot), 0)
	}
}

// ------------------------------------------------------------- etiquetas

func (c *Compiler) defineLabel(name string, pos ast.Pos) {
	c.sc.labels[name] = c.here()
	mod := c.mod.scope
	if c.sc == mod {
		mod.dataLbl[name] = len(c.prog.Data)
	}
}

func (c *Compiler) refLabel(sc *scope, at int, field byte, label string, pos ast.Pos) {
	sc.fixups = append(sc.fixups, labelRef{at: at, field: field, label: label, pos: pos, file: c.file})
}

func (c *Compiler) resolveLabels(sc *scope) {
	for _, f := range sc.fixups {
		pc, ok := sc.labels[f.label]
		if !ok {
			c.diags = append(c.diags, source.Diag{File: c.prog.Files[f.file], Pos: f.pos, Msg: "Label not defined: " + f.label})
			continue
		}
		switch f.field {
		case 'A':
			c.prog.Code[f.at].A = int32(pc)
		case 'B':
			c.prog.Code[f.at].B = int32(pc)
		case 't':
			c.prog.Tables[f.tbl][f.idx] = pc
		case 'c': // RUN línea: el pc va en una constante
			c.prog.Consts[-1-f.at] = vm.Long(int64(pc))
		}
	}
	for _, f := range sc.dataFix {
		idx, ok := sc.dataLbl[f.label]
		if !ok {
			c.diags = append(c.diags, source.Diag{File: c.prog.Files[f.file], Pos: f.pos, Msg: "Label not defined: " + f.label})
			continue
		}
		c.prog.Code[f.at].A = int32(idx)
	}
	sc.fixups, sc.dataFix = nil, nil
}

// prologue dimensiona los arrays implícitos (0..10) de un ámbito.
func (c *Compiler) prologue(sc *scope) {
	for _, ia := range sc.implicit {
		c.addrSym(ia.sym)
		for i := 0; i < ia.dims; i++ {
			c.pushConst(vm.Int(int64(sc.optBase)))
			c.pushConst(vm.Int(10))
		}
		c.emit(vm.OpDim, int32(c.constIdx(ia.sym.Type.zero())), int32(ia.dims|vm.DimStatic))
	}
	sc.implicit = nil
}

// ------------------------------------------------------------- procedimientos

func (c *Compiler) compileProc(p *procInfo) {
	def := p.Def
	c.mod = p.mod
	c.file = p.File
	sc := newScope(p.mod.scope)
	sc.defTypes = p.defTypes
	sc.proc = p
	sc.static = def.Static
	c.sc = sc
	c.erl = 0

	vp := &vm.Proc{Name: p.Name, IsFunc: p.IsFunc, NParams: len(p.Params), File: p.File, Line: def.P.Line, EndLine: def.EndPos.Line}
	p.Index = len(c.prog.Procs)
	c.prog.Procs = append(c.prog.Procs, vp)

	// parámetros: siempre locales (llegan como referencias)
	for _, pr := range p.Params {
		sc.locals = append(sc.locals, vm.Value{K: vm.KPtr})
		key := varKey(baseName(pr.Name), pr.Type, pr.IsArray)
		kind := symVar
		if pr.IsArray {
			kind = symArray
		}
		s := &Symbol{Kind: kind, Name: pr.Name, Type: pr.Type, Slot: len(sc.locals) - 1, Dims: -1}
		sc.vars[key] = s
		if suffixOf(pr.Name) == 0 || pr.Type.K == vm.KRecord {
			arr := ""
			if pr.IsArray {
				arr = "()"
			}
			sc.asDecl[baseName(pr.Name)+arr] = key
		}
	}
	if p.IsFunc {
		sc.locals = append(sc.locals, p.Ret.zero())
		vp.RetSlot = len(sc.locals) - 1
		retSym := &Symbol{Kind: symVar, Name: p.Name, Type: p.Ret, Slot: vp.RetSlot}
		sc.vars[varKey(p.Key, p.Ret, false)] = retSym
		sc.asDecl[p.Key] = varKey(p.Key, p.Ret, false)
	}

	entry := c.emit(vm.OpJmp, 0, 0)
	body := c.here()
	c.stmts(def.Body)
	c.emit(vm.OpRet, 0, 0)
	pro := c.here()
	c.patchA(entry, pro)
	c.prologue(sc)
	c.emit(vm.OpJmp, int32(body), 0)
	vp.Entry = entry
	vp.Locals = sc.locals
	c.resolveLabels(sc)
}

// sortStmts ordena la tabla de sentencias por PC (inserción: casi ordenada).
func sortStmts(s []vm.StmtInfo) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j].PC < s[j-1].PC; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// Immediate compila sentencias de la ventana Immediate en el contexto del
// módulo principal (con acceso a sus variables) y devuelve el pc de entrada.
// El código se agrega al mismo Program.
func (c *Compiler) Immediate(stmts []ast.Stmt) (int, []source.Diag) {
	c.diags = nil
	m := c.mods[0]
	c.mod, c.sc, c.file, c.erl = m, m.scope, c.fileIdx("Immediate"), 0
	for _, s := range stmts {
		switch s.(type) {
		case *ast.Label, *ast.Proc, *ast.DefFn, *ast.TypeDecl, *ast.Data, *ast.Declare:
			c.errAt(s.Position(), "Illegal in direct mode")
			return 0, c.diags
		}
	}
	entry := c.emit(vm.OpJmp, 0, 0)
	body := c.here()
	c.stmts(stmts)
	c.emit(vm.OpJmp, -1, 0) // fin: pc fuera del código
	c.patchA(entry, c.here())
	c.prologue(m.scope)
	c.emit(vm.OpJmp, int32(body), 0)
	for _, pc := range c.pendingCalls {
		c.prog.Code[pc.at].A = int32(pc.p.Index)
	}
	c.pendingCalls = nil
	c.resolveLabels(m.scope)
	sortStmts(c.prog.Stmts)
	return entry, c.diags
}
