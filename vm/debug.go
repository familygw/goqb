package vm

import (
	"errors"
	"sort"
)

// ErrPaused indica que el depurador detuvo el programa (breakpoint, paso,
// watchpoint o STOP). Se puede continuar llamando a Run.
var ErrPaused = errors.New("Paused")

var errEvalDone = errors.New("eval done")

// Modos de ejecución del depurador.
const (
	DebugRun      = iota
	DebugStepInto // F8: detenerse en la próxima sentencia
	DebugStepOver // F10: próxima sentencia sin entrar en procedimientos
	DebugStepOut  // hasta volver al llamador
)

// Watchpoint detiene el programa cuando su expresión es verdadera.
type Watchpoint struct {
	Entry int // código compilado de la expresión (termina en OpEvalDone)
	Proc  int // procedimiento donde se evalúa (-1: nivel de módulo)
}

// Debugger controla la ejecución paso a paso. Se asigna a VM.Debug.
type Debugger struct {
	Breaks      map[int]bool // pcs de inicio de sentencia
	Watchpoints []Watchpoint
	Mode        int
	refDepth    int
	skipPC      int
	// History guarda los pcs de las últimas sentencias ejecutadas (si HistoryOn).
	HistoryOn bool
	History   []int
}

// NewDebugger crea un depurador vacío.
func NewDebugger() *Debugger { return &Debugger{Breaks: map[int]bool{}, skipPC: -1} }

const historySize = 20

// isStmtStart reporta si pc es el inicio de una sentencia.
func (p *Program) isStmtStart(pc int) bool {
	if p.stmtStart == nil || len(p.stmtStart) != len(p.Code) {
		p.stmtStart = make([]bool, len(p.Code))
		for _, s := range p.Stmts {
			if s.PC < len(p.stmtStart) {
				p.stmtStart[s.PC] = true
			}
		}
	}
	return pc >= 0 && pc < len(p.stmtStart) && p.stmtStart[pc]
}

// StmtPCs devuelve el pc de la primera sentencia de cada línea de un archivo.
func (p *Program) StmtPCs(file string) map[int]int {
	out := map[int]int{}
	for _, s := range p.Stmts {
		if s.File < len(p.Files) && p.Files[s.File] == file {
			if old, ok := out[s.Line]; !ok || s.PC < old {
				out[s.Line] = s.PC
			}
		}
	}
	return out
}

// ProcAt devuelve el índice del procedimiento cuyo código contiene pc (-1: módulo).
func (p *Program) ProcAt(pc int) int {
	for i, pr := range p.Procs {
		if pc >= pr.Start && pc < pr.End {
			return i
		}
	}
	return -1
}

// debugCheck decide si hay que detenerse antes de la sentencia en pc.
func (m *VM) debugCheck(pc int) bool {
	d := m.Debug
	if d == nil || m.inEval {
		return false
	}
	if pc == d.skipPC {
		d.skipPC = -1
		return false
	}
	if d.HistoryOn {
		d.History = append(d.History, pc)
		if len(d.History) > historySize {
			d.History = d.History[len(d.History)-historySize:]
		}
	}
	depth := len(m.frames)
	switch {
	case d.Breaks[pc]:
	case d.Mode == DebugStepInto:
	case d.Mode == DebugStepOver && depth <= d.refDepth:
	case d.Mode == DebugStepOut && depth < d.refDepth:
	default:
		if !m.watchpointHit() {
			return false
		}
	}
	d.Mode = DebugRun
	d.skipPC = pc
	return true
}

func (m *VM) watchpointHit() bool {
	cur := m.CurrentProc()
	for _, w := range m.Debug.Watchpoints {
		if w.Proc != cur {
			continue
		}
		v, err := m.Eval(w.Entry)
		if err == nil && v.K.IsNumeric() && v.IsTrue() {
			return true
		}
	}
	return false
}

// Step prepara la próxima ejecución en el modo dado (relativo al frame actual).
func (m *VM) Step(mode int) {
	if m.Debug == nil {
		m.Debug = NewDebugger()
	}
	m.Debug.Mode = mode
	m.Debug.refDepth = len(m.frames)
}

// CurrentProc devuelve el procedimiento del frame actual (-1: nivel de módulo).
func (m *VM) CurrentProc() int {
	f := m.frames[len(m.frames)-1]
	if f.proc == nil {
		return -1
	}
	for i, p := range m.Prog.Procs {
		if p == f.proc {
			return i
		}
	}
	return -1
}

// Frame describe un nivel de la pila de llamadas (para el menú Calls).
type Frame struct {
	Proc string // "" = nivel de módulo
	PC   int
}

// CallStack devuelve la pila de llamadas, del más reciente al más antiguo.
func (m *VM) CallStack() []Frame {
	var out []Frame
	pc := m.pc
	for i := len(m.frames) - 1; i >= 0; i-- {
		f := m.frames[i]
		name := ""
		if f.proc != nil {
			name = f.proc.Name
		}
		out = append(out, Frame{Proc: name, PC: pc})
		pc = f.ret - 1
	}
	return out
}

// SetNextStatement mueve la ejecución a pc (debe estar en el mismo procedimiento).
func (m *VM) SetNextStatement(pc int) error {
	if !m.Prog.isStmtStart(pc) {
		return ErrIllegalCall
	}
	if m.Prog.ProcAt(pc) != m.CurrentProc() {
		return ErrIllegalCall
	}
	m.pc = pc
	m.stack = m.stack[:m.frames[len(m.frames)-1].base]
	if m.Debug != nil {
		m.Debug.skipPC = pc
	}
	return nil
}

// Eval ejecuta código compilado por el depurador (termina en OpEvalDone) en
// el frame actual y devuelve el valor que deja en la pila.
func (m *VM) Eval(entry int) (Value, error) {
	savePC, saveErrPC, saveStack := m.pc, m.errPC, len(m.stack)
	saveEval, saveHandler := m.inEval, m.handler
	m.inEval = true
	m.handler = -1 // un error en la expresión no debe saltar al ON ERROR del programa
	m.growLocals()
	m.pc = entry
	err := m.exec()
	var v Value
	if errors.Is(err, errEvalDone) {
		err = nil
		if len(m.stack) > saveStack {
			v = m.pop()
		}
	}
	m.pc, m.errPC, m.inEval, m.handler = savePC, saveErrPC, saveEval, saveHandler
	m.stack = m.stack[:saveStack]
	if errors.Is(err, errEnd) {
		err = nil
	}
	return v, err
}

// growLocals agrega los locales que la ventana Immediate haya creado en el procedimiento actual.
func (m *VM) growLocals() {
	f := &m.frames[len(m.frames)-1]
	if f.proc == nil {
		for len(m.Globals) < len(m.Prog.Globals) {
			m.Globals = append(m.Globals, m.Prog.Globals[len(m.Globals)].Clone())
		}
		return
	}
	for len(f.locals) < len(f.proc.Locals) {
		f.locals = append(f.locals, f.proc.Locals[len(f.locals)].Clone())
	}
	for len(m.Globals) < len(m.Prog.Globals) {
		m.Globals = append(m.Globals, m.Prog.Globals[len(m.Globals)].Clone())
	}
}

// HistoryLines devuelve archivo y línea de cada sentencia del historial.
func (m *VM) HistoryLines() []StmtInfo {
	if m.Debug == nil {
		return nil
	}
	var out []StmtInfo
	for _, pc := range m.Debug.History {
		if i := m.Prog.StmtAt(pc); i >= 0 {
			out = append(out, m.Prog.Stmts[i])
		}
	}
	return out
}

// Position devuelve archivo, línea y columna de un pc.
func (m *VM) Position(pc int) (string, int, int) {
	i := m.Prog.StmtAt(pc)
	if i < 0 {
		return "", 0, 0
	}
	s := m.Prog.Stmts[i]
	f := ""
	if s.File < len(m.Prog.Files) {
		f = m.Prog.Files[s.File]
	}
	return f, s.Line, s.Col
}

// SortedBreaks devuelve los breakpoints ordenados (para mostrar).
func (d *Debugger) SortedBreaks() []int {
	var out []int
	for pc := range d.Breaks {
		out = append(out, pc)
	}
	sort.Ints(out)
	return out
}
