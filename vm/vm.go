package vm

import (
	"errors"
	"sync/atomic"
)

const maxFrames = 4096

type gosubRet struct {
	pc int
	ev int // evento cuyo manejador se está ejecutando (-1: GOSUB común)
}

type frame struct {
	proc   *Proc
	ret    int
	locals []Value
	base   int
	gosub  []gosubRet
}

// VM ejecuta un Program.
type VM struct {
	Prog    *Program
	Con     Console
	Globals []Value
	Command string // COMMAND$

	stack  []Value
	frames []frame
	pc     int
	errPC  int

	handler    int // pc del manejador de ON ERROR (-1: ninguno)
	inHandler  bool
	errCode    int
	erl        int
	resumePC   int
	resumeNext int

	dataPtr int
	seed    uint32
	lastRnd float64
	defSeg  int

	// Sound reproduce SOUND/PLAY/BEEP (por defecto, silencioso respetando los tiempos).
	Sound     SoundDevice
	music     musicState
	ev        events
	evPending atomic.Bool

	files   map[int]*qfile
	out     printer
	using   usingState
	started bool
	ended   bool

	stop atomic.Bool

	// Debug, si no es nil, habilita breakpoints y ejecución paso a paso.
	Debug  *Debugger
	inEval bool

	mem *memory // modelo de memoria de 16 bits (PEEK/POKE/VARPTR/CALL ABSOLUTE)

	// Loader carga los programas de CHAIN y RUN "archivo" (nil: no soportado).
	Loader Loader
	// OnChain, si no es nil, se llama cuando CHAIN o RUN cambian el programa.
	OnChain func(*Program)
}

// New crea una VM lista para ejecutar prog sobre con.
func New(prog *Program, con Console) *VM {
	m := &VM{Prog: prog, Con: con, handler: -1, seed: 0x50000, files: map[int]*qfile{}, Sound: &SilentSound{}, Loader: DefaultLoader}
	m.Reset()
	return m
}

// Reset reinicia variables, archivos y estado (RUN / CLEAR).
func (m *VM) Reset() {
	m.Globals = make([]Value, len(m.Prog.Globals))
	for i, g := range m.Prog.Globals {
		m.Globals[i] = g.Clone()
	}
	m.stack = m.stack[:0]
	m.frames = []frame{{ret: -1}}
	m.pc = m.Prog.Entry
	m.handler, m.inHandler, m.errCode, m.erl = -1, false, 0, 0
	m.dataPtr = 0
	m.defSeg, m.mem = dgroupSeg, nil
	m.closeAll()
	m.out = nil
	m.started, m.ended = false, false
	m.music = newMusic()
	m.initEvents()
}

// Stop solicita detener el programa (Ctrl+Break). Es seguro llamarlo desde otra goroutine.
func (m *VM) Stop() { m.stop.Store(true) }

// Ended reporta si el programa terminó (END o fin del código).
func (m *VM) Ended() bool { return m.ended }

// PC devuelve el contador de programa actual.
func (m *VM) PC() int { return m.pc }

// Run ejecuta hasta END, un error no atrapado o una interrupción.
// Tras ErrBreak se puede continuar llamando a Run otra vez.
func (m *VM) Run() error {
	m.stop.Store(false)
	m.started = true
	defer m.flushFiles()
	defer func() {
		if !m.music.background || m.stop.Load() || !m.ended {
			m.Sound.Stop()
		}
	}()
	for {
		err := m.exec()
		switch {
		case err == nil, errors.Is(err, errEnd):
			m.ended = true
			m.Con.Flush()
			return nil
		case errors.Is(err, errStopped):
			return m.runtimeError(ErrBreak)
		case errors.Is(err, errPaused):
			return m.runtimeError(ErrPaused)
		case errors.Is(err, errEvalDone):
			return nil
		case errors.Is(err, errRestart):
			continue
		}
		var qe *Error
		if errors.As(err, &qe) && m.handler >= 0 && !m.inHandler {
			m.trap(qe)
			continue
		}
		return m.runtimeError(err)
	}
}

func (m *VM) runtimeError(err error) *RuntimeError {
	re := &RuntimeError{Err: err, PC: m.errPC}
	if i := m.Prog.StmtAt(m.errPC); i >= 0 {
		s := m.Prog.Stmts[i]
		re.Line, re.Col = s.Line, s.Col
		if s.File < len(m.Prog.Files) {
			re.File = m.Prog.Files[s.File]
		}
	}
	return re
}

// trap transfiere el control al manejador de ON ERROR.
func (m *VM) trap(e *Error) {
	m.errCode = e.Code
	i := m.Prog.StmtAt(m.errPC)
	m.resumePC, m.resumeNext = m.errPC, m.errPC
	if i >= 0 {
		m.resumePC = m.Prog.Stmts[i].PC
		m.erl = m.Prog.Stmts[i].ERL
		if i+1 < len(m.Prog.Stmts) {
			m.resumeNext = m.Prog.Stmts[i+1].PC
		} else {
			m.resumeNext = len(m.Prog.Code)
		}
	}
	m.stack = m.stack[:m.frames[len(m.frames)-1].base]
	m.inHandler = true
	m.pc = m.handler
}

func (m *VM) push(v Value) { m.stack = append(m.stack, v) }

func (m *VM) pop() Value {
	n := len(m.stack) - 1
	v := m.stack[n]
	m.stack = m.stack[:n]
	return v
}

func ptr(p *Value) Value { return Value{K: KPtr, P: p} }

// load devuelve el valor de una celda siguiendo referencias.
func load(v *Value) Value { return *v.Deref() }

func (m *VM) exec() error {
	code := m.Prog.Code
	count := 0
	for {
		if m.pc < 0 || m.pc >= len(code) {
			return errEnd
		}
		in := code[m.pc]
		m.errPC = m.pc
		m.pc++
		count++
		if count&1023 == 0 && m.stop.Load() {
			m.pc--
			return errStopped
		}
		var err error
		switch in.Op {
		case OpNop:
		case OpConst:
			m.push(m.Prog.Consts[in.A])
		case OpPop:
			m.pop()
		case OpLoadG:
			m.push(load(&m.Globals[in.A]))
		case OpStoreG:
			err = Assign(&m.Globals[in.A], m.pop())
		case OpLoadL:
			m.push(load(&m.frames[len(m.frames)-1].locals[in.A]))
		case OpStoreL:
			err = Assign(&m.frames[len(m.frames)-1].locals[in.A], m.pop())
		case OpAddrG:
			m.push(ptr(m.Globals[in.A].Deref()))
		case OpAddrL:
			m.push(ptr(m.frames[len(m.frames)-1].locals[in.A].Deref()))
		case OpIndex:
			err = m.index(int(in.A))
		case OpField:
			r := m.pop().P.Deref()
			if r.K != KRecord {
				err = ErrTypeMismatch
				break
			}
			m.push(ptr(&r.R.F[in.A]))
		case OpLoadInd:
			r := m.pop()
			m.push(load(r.P))
		case OpStoreInd:
			v := m.pop()
			r := m.pop()
			err = Assign(r.P, v)
		case OpConv:
			n := len(m.stack) - 1
			m.stack[n], err = Convert(m.stack[n], Kind(in.A))
		case OpTemp:
			v := m.pop().Clone()
			m.push(ptr(&v))
		case OpTempK:
			v, e := Convert(m.pop(), Kind(in.A))
			if e != nil {
				err = e
				break
			}
			m.push(ptr(&v))

		case OpAdd, OpSub, OpMul, OpDiv, OpIDiv, OpMod, OpPow:
			b := m.pop()
			a := m.pop()
			var r Value
			switch in.Op {
			case OpAdd:
				r, err = Add(a, b)
			case OpSub:
				r, err = Sub(a, b)
			case OpMul:
				r, err = Mul(a, b)
			case OpDiv:
				r, err = Div(a, b)
			case OpIDiv:
				r, err = IDiv(a, b)
			case OpMod:
				r, err = Mod(a, b)
			case OpPow:
				r, err = Pow(a, b)
			}
			m.push(r)
		case OpNeg:
			n := len(m.stack) - 1
			m.stack[n], err = Neg(m.stack[n])
		case OpEq, OpNe, OpLt, OpGt, OpLe, OpGe:
			b := m.pop()
			a := m.pop()
			c, e := Compare(a, b)
			if e != nil {
				err = e
				break
			}
			var r bool
			switch in.Op {
			case OpEq:
				r = c == 0
			case OpNe:
				r = c != 0
			case OpLt:
				r = c < 0
			case OpGt:
				r = c > 0
			case OpLe:
				r = c <= 0
			case OpGe:
				r = c >= 0
			}
			m.push(Bool(r))
		case OpNot:
			n := len(m.stack) - 1
			m.stack[n], err = Not(m.stack[n])
		case OpAnd, OpOr, OpXor, OpEqv, OpImp:
			b := m.pop()
			a := m.pop()
			var r Value
			r, err = Logic(in.Op, a, b)
			m.push(r)

		case OpJmp:
			m.pc = int(in.A)
		case OpJmpF:
			v := m.pop()
			if !v.K.IsNumeric() {
				err = ErrTypeMismatch
			} else if !v.IsTrue() {
				m.pc = int(in.A)
			}
		case OpJmpT:
			v := m.pop()
			if !v.K.IsNumeric() {
				err = ErrTypeMismatch
			} else if v.IsTrue() {
				m.pc = int(in.A)
			}
		case OpGosub:
			f := &m.frames[len(m.frames)-1]
			if len(f.gosub) > 10000 {
				err = ErrStackSpace
				break
			}
			f.gosub = append(f.gosub, gosubRet{m.pc, -1})
			m.pc = int(in.A)
		case OpReturn:
			f := &m.frames[len(m.frames)-1]
			if len(f.gosub) == 0 {
				err = ErrReturnNoGosub
				break
			}
			g := f.gosub[len(f.gosub)-1]
			f.gosub = f.gosub[:len(f.gosub)-1]
			ret := g.pc
			if g.ev >= 0 {
				m.ev.ev[g.ev].active = false
			}
			if in.A >= 0 {
				ret = int(in.A)
			}
			m.pc = ret
		case OpOnJump:
			v, e := ToLong(m.pop())
			if e != nil {
				err = e
				break
			}
			t := m.Prog.Tables[in.A]
			if v.I < 0 || v.I > 255 {
				err = ErrIllegalCall
				break
			}
			if v.I == 0 || int(v.I) > len(t) {
				break // fuera de rango: sigue con la próxima sentencia
			}
			if in.B == 1 {
				f := &m.frames[len(m.frames)-1]
				f.gosub = append(f.gosub, gosubRet{m.pc, -1})
			}
			m.pc = t[v.I-1]
		case OpCall:
			err = m.call(m.Prog.Procs[in.A], int(in.B))
		case OpRet:
			f := m.frames[len(m.frames)-1]
			m.frames = m.frames[:len(m.frames)-1]
			m.stack = m.stack[:f.base]
			m.pc = f.ret
			if f.proc.IsFunc {
				m.push(load(&f.locals[f.proc.RetSlot]).Clone())
			}
		case OpBuiltin:
			n := int(in.B)
			args := m.stack[len(m.stack)-n:]
			var r Value
			r, err = Builtins[in.A].Fn(m, args)
			m.stack = m.stack[:len(m.stack)-n]
			m.push(r)
		case OpStmt:
			n := int(in.B)
			base := len(m.stack) - n
			err = Statements[in.A].Fn(m, m.stack[base:])
			if len(m.stack) >= base { // RUN y CHAIN vacían la pila
				m.stack = m.stack[:base]
			}
		case OpForCmp:
			step := m.pop()
			lim := m.pop()
			v := m.pop()
			c, e := Compare(v, lim)
			if e != nil {
				err = e
				break
			}
			if step.Num() >= 0 {
				m.push(Bool(c <= 0))
			} else {
				m.push(Bool(c >= 0))
			}
		case OpOnError:
			m.handler = int(in.A)
			if in.A < 0 && m.inHandler {
				// ON ERROR GOTO 0 dentro del manejador: el error se vuelve fatal
				m.inHandler = false
				err = &Error{m.errCode}
			}
		case OpResume:
			if !m.inHandler {
				err = ErrResumeNoError
				break
			}
			m.inHandler, m.errCode = false, 0
			switch in.A {
			case 0:
				m.pc = m.resumePC
			case 1:
				m.pc = m.resumeNext
			case 2:
				m.frames = m.frames[:1]
				m.stack = m.stack[:0]
				m.pc = int(in.B)
			}
		case OpRead:
			err = m.readData(m.pop().P)
		case OpRestore:
			m.dataPtr = int(in.A)
		case OpDim:
			err = m.dim(int(in.A), int(in.B))
		case OpErase:
			slot := m.pop().P.Deref()
			if slot.K == KArray && slot.A != nil {
				if slot.A.Static {
					slot.A.Clear()
				} else {
					slot.A = nil
				}
			}
		case OpSwap:
			b := m.pop().P.Deref()
			a := m.pop().P.Deref()
			if a.K != b.K || (a.K == KString && a.I != b.I) || (a.K == KRecord && a.R.T != b.R.T) {
				err = ErrTypeMismatch
				break
			}
			*a, *b = *b, *a
		case OpEnd:
			m.closeAll()
			return errEnd
		case OpStop: // STOP: pausa, se puede continuar en la sentencia siguiente
			return errPaused
		case OpUnsupported:
			err = ErrAdvancedFeature
		case OpEvt:
			if m.Debug != nil && m.Prog.isStmtStart(m.pc-1) && m.debugCheck(m.pc-1) {
				m.pc--
				return errPaused
			}
			if m.ev.enabled > 0 && !m.inEval {
				m.checkEvents()
			}
		case OpEvalDone:
			return errEvalDone
		case OpError:
			v, e := ToInt(m.pop())
			if e != nil {
				err = e
			} else if v.I < 1 || v.I > 255 {
				err = ErrIllegalCall
			} else {
				err = &Error{int(v.I)}
			}
		default:
			err = &Error{51}
		}
		if err != nil {
			return err
		}
	}
}

func (m *VM) index(n int) error {
	idx := make([]int, n)
	for i := n - 1; i >= 0; i-- {
		v, err := ToLong(m.pop())
		if err != nil {
			return err
		}
		idx[i] = int(v.I)
	}
	slot := m.pop().P.Deref()
	if slot.K != KArray || slot.A == nil {
		return ErrSubscript
	}
	e, err := slot.A.Elem(idx)
	if err != nil {
		return err
	}
	m.push(ptr(e))
	return nil
}

func (m *VM) dim(protoIdx, arg int) error {
	nd := arg & 0xFF
	bounds := make([]int, 2*nd)
	for i := 2*nd - 1; i >= 0; i-- {
		v, err := ToLong(m.pop())
		if err != nil {
			return err
		}
		bounds[i] = int(v.I)
	}
	slot := m.pop().P.Deref()
	if slot.K == KArray && slot.A != nil && arg&DimRedim == 0 {
		return ErrDuplicate // "Array already dimensioned"
	}
	if slot.K == KArray && slot.A != nil && slot.A.Static {
		return ErrDuplicate
	}
	lo, hi := make([]int, nd), make([]int, nd)
	for i := 0; i < nd; i++ {
		lo[i], hi[i] = bounds[2*i], bounds[2*i+1]
	}
	a, err := NewArray(m.Prog.Consts[protoIdx], lo, hi)
	if err != nil {
		return err
	}
	a.Static = arg&DimStatic != 0
	*slot = Value{K: KArray, A: a}
	return nil
}

func (m *VM) call(p *Proc, nargs int) error {
	if len(m.frames) >= maxFrames {
		return ErrStackSpace
	}
	locals := make([]Value, len(p.Locals))
	for i := range p.Locals {
		locals[i] = p.Locals[i].Clone()
	}
	base := len(m.stack) - nargs
	copy(locals, m.stack[base:])
	m.stack = m.stack[:base]
	m.frames = append(m.frames, frame{proc: p, ret: m.pc, locals: locals, base: base})
	m.pc = p.Entry
	return nil
}

// ------------------------------------------------------------- DATA

func (m *VM) readData(dst *Value) error {
	if m.dataPtr >= len(m.Prog.Data) {
		return ErrOutOfData
	}
	it := m.Prog.Data[m.dataPtr]
	m.dataPtr++
	dst = dst.Deref()
	if dst.K == KString {
		return Assign(dst, Str(it.Text))
	}
	if it.Quoted {
		return ErrSyntax
	}
	f, ok := ParseInputNumber(it.Text)
	if !ok {
		return ErrSyntax
	}
	return Assign(dst, Double(f))
}

// ------------------------------------------------------------- inspección (Immediate / depurador)

// Global devuelve el valor de una variable global por nombre interno.
func (m *VM) Global(name string) (Value, bool) {
	i, ok := m.Prog.GlobalNames[name]
	if !ok || i >= len(m.Globals) {
		return Value{}, false
	}
	return load(&m.Globals[i]), true
}

// Continue prepara la VM para ejecutar código agregado (ventana Immediate)
// a partir de entry, conservando variables y archivos.
func (m *VM) Continue(prog *Program, entry int) error {
	m.Prog = prog
	for len(m.Globals) < len(prog.Globals) {
		m.Globals = append(m.Globals, prog.Globals[len(m.Globals)].Clone())
	}
	m.frames = []frame{{ret: -1}}
	m.stack = m.stack[:0]
	m.pc = entry
	m.inHandler = false
	return m.Run()
}
