package vm

import (
	"sync"
	"time"
)

// Eventos que se pueden atrapar con ON <evento> GOSUB.
const (
	EvTimer  = 0
	EvKey    = 1 // EvKey+n para KEY(n), n = 1..31
	EvPlay   = 33
	EvPen    = 34
	EvStrig  = 35 // + n/2 (0..3)
	EvCom    = 39 // + n-1
	EvUEvent = 41
	numEvents
)

// Estados de un evento.
const (
	evOff = iota
	evOn
	evStop
)

type event struct {
	handler int // pc del manejador (-1: sin manejador)
	state   int
	pending bool
	active  bool // ejecutando su manejador
}

type events struct {
	ev       [numEvents]event
	enabled  int // eventos en ON/STOP (si es 0, no hay nada que revisar)
	interval time.Duration
	next     time.Time
	playN    int
	playWas  int

	mu      sync.Mutex
	keyHits uint64     // bits de teclas atrapadas (lo escribe el filtro de teclado)
	keyDefs [32]string // KEY n, CHR$(flags) + CHR$(scan) para n = 15..25
}

// KeyFilterer es una consola que permite interceptar teclas (ON KEY).
type KeyFilterer interface {
	SetKeyFilter(func(key string) bool)
}

func (m *VM) initEvents() {
	for i := range m.ev.ev {
		m.ev.ev[i] = event{handler: -1}
	}
	m.ev.enabled = 0
	if kf, ok := m.Con.(KeyFilterer); ok {
		kf.SetKeyFilter(m.keyFilter)
	}
}

func (m *VM) recount() {
	n := 0
	for _, e := range m.ev.ev {
		if e.state != evOff {
			n++
		}
	}
	m.ev.enabled = n
}

// Códigos de exploración de las teclas comunes (para KEY 15..25).
var scanCodes = func() map[byte]byte {
	t := map[byte]byte{27: 1, '\r': 28, ' ': 57, '\t': 15, '\b': 14}
	rows := []string{"1234567890-=", "QWERTYUIOP[]", "ASDFGHJKL;'`", "ZXCVBNM,./"}
	starts := []byte{2, 16, 30, 44}
	for r, row := range rows {
		for i := 0; i < len(row); i++ {
			t[row[i]] = starts[r] + byte(i)
			if row[i] >= 'A' && row[i] <= 'Z' {
				t[row[i]+32] = starts[r] + byte(i)
			}
		}
	}
	return t
}()

// keyNumber devuelve el número de KEY(n) que corresponde a una tecla (0 si ninguno).
func (m *VM) keyNumber(k string) int {
	switch k {
	case "\x00H":
		return 11
	case "\x00K":
		return 12
	case "\x00M":
		return 13
	case "\x00P":
		return 14
	case "\x00\x85":
		return 30
	case "\x00\x86":
		return 31
	}
	if len(k) == 2 && k[0] == 0 && k[1] >= 59 && k[1] <= 68 {
		return int(k[1]-59) + 1 // F1..F10
	}
	var scan byte
	if len(k) == 2 && k[0] == 0 {
		scan = k[1]
	} else if len(k) == 1 {
		scan = scanCodes[k[0]]
	}
	if scan == 0 {
		return 0
	}
	m.ev.mu.Lock()
	defer m.ev.mu.Unlock()
	for n := 15; n <= 25; n++ {
		if d := m.ev.keyDefs[n]; len(d) == 2 && d[1] == scan {
			return n
		}
	}
	return 0
}

// keyFilter se llama desde la goroutine de la interfaz con cada tecla;
// devuelve true si la tecla queda atrapada (no llega a INKEY$).
func (m *VM) keyFilter(k string) bool {
	n := m.keyNumber(k)
	if n == 0 {
		return false
	}
	m.ev.mu.Lock()
	defer m.ev.mu.Unlock()
	st := m.ev.ev[EvKey+n].state
	if st == evOff {
		return false
	}
	m.ev.keyHits |= 1 << uint(n)
	m.evPending.Store(true)
	return true
}

// checkEvents revisa y despacha eventos al inicio de cada sentencia.
func (m *VM) checkEvents() {
	e := &m.ev
	if e.enabled == 0 {
		return
	}
	if t := &e.ev[EvTimer]; t.state != evOff && e.interval > 0 && !time.Now().Before(e.next) {
		t.pending = true
		e.next = time.Now().Add(e.interval)
	}
	if p := &e.ev[EvPlay]; p.state != evOff && e.playN > 0 {
		n := m.Sound.Pending()
		if n < e.playN && e.playWas >= e.playN {
			p.pending = true
		}
		e.playWas = n
	}
	if m.evPending.Swap(false) {
		e.mu.Lock()
		hits := e.keyHits
		e.keyHits = 0
		e.mu.Unlock()
		for n := 1; n <= 31; n++ {
			if hits&(1<<uint(n)) != 0 {
				e.ev[EvKey+n].pending = true
			}
		}
	}
	for i := range e.ev {
		ev := &e.ev[i]
		if ev.pending && ev.state == evOn && !ev.active && ev.handler >= 0 {
			ev.pending, ev.active = false, true
			f := &m.frames[len(m.frames)-1]
			f.gosub = append(f.gosub, gosubRet{m.pc, i})
			m.pc = ev.handler
			return
		}
	}
}

// eventIndex convierte (tipo, argumento) en índice de evento.
func eventIndex(kind string, arg int) (int, error) {
	switch kind {
	case "TIMER":
		return EvTimer, nil
	case "KEY":
		if arg < 1 || arg > 31 || (arg > 25 && arg < 30) {
			return 0, ErrIllegalCall
		}
		return EvKey + arg, nil
	case "PLAY":
		return EvPlay, nil
	case "PEN":
		return EvPen, nil
	case "STRIG":
		if arg < 0 || arg > 6 || arg%2 != 0 {
			return 0, ErrIllegalCall
		}
		return EvStrig + arg/2, nil
	case "COM":
		if arg < 1 || arg > 2 {
			return 0, ErrIllegalCall
		}
		return EvCom + arg - 1, nil
	case "UEVENT":
		return EvUEvent, nil
	}
	return 0, ErrIllegalCall
}

func init() {
	// ON evento(arg) GOSUB etiqueta: tipo, arg|none, pc (-1 = GOSUB 0)
	regStmt("ON_EVENT", func(m *VM, a []Value) error {
		kind := a[0].S
		arg := 0
		if !isNone(a[1]) {
			v, err := intArg(a[1])
			if err != nil {
				return err
			}
			arg = v
		}
		idx := 0
		switch kind {
		case "TIMER":
			if arg < 1 || arg > 86400 {
				return ErrIllegalCall
			}
			m.ev.interval = time.Duration(arg) * time.Second
			m.ev.next = time.Now().Add(m.ev.interval)
		case "PLAY":
			if arg < 1 || arg > 32 {
				return ErrIllegalCall
			}
			m.ev.playN = arg
		default:
			var err error
			if idx, err = eventIndex(kind, arg); err != nil {
				return err
			}
		}
		if kind == "PLAY" {
			idx = EvPlay
		}
		pc, _ := intArg(a[2])
		m.ev.ev[idx].handler = pc
		return nil
	})
	// evento(arg) ON|OFF|STOP: tipo, arg|none, acción
	regStmt("EVENT_CTL", func(m *VM, a []Value) error {
		kind, action := a[0].S, a[2].S
		arg := 0
		if !isNone(a[1]) {
			v, err := intArg(a[1])
			if err != nil {
				return err
			}
			arg = v
		}
		var idxs []int
		if kind == "KEY" && arg == 0 { // KEY(0): todas las teclas
			for n := 1; n <= 31; n++ {
				if n <= 25 || n >= 30 {
					idxs = append(idxs, EvKey+n)
				}
			}
		} else {
			i, err := eventIndex(kind, arg)
			if err != nil {
				return err
			}
			idxs = []int{i}
		}
		m.ev.mu.Lock()
		for _, i := range idxs {
			ev := &m.ev.ev[i]
			switch action {
			case "ON":
				ev.state = evOn
				if i == EvTimer && m.ev.interval > 0 {
					m.ev.next = time.Now().Add(m.ev.interval)
				}
			case "OFF":
				ev.state, ev.pending = evOff, false
			case "STOP":
				ev.state = evStop
			}
		}
		m.ev.mu.Unlock()
		m.recount()
		return nil
	})
	// KEY n, s$: 1..10 macros de teclas de función (sin efecto); 15..25 teclas atrapables.
	regStmt("KEY_DEF", func(m *VM, a []Value) error {
		n, err := intArg(a[0])
		if err != nil {
			return err
		}
		if a[1].K != KString {
			return ErrTypeMismatch
		}
		switch {
		case n >= 1 && n <= 10, n == 30, n == 31:
			return nil
		case n >= 15 && n <= 25:
			if len(a[1].S) != 2 {
				return ErrIllegalCall
			}
			m.ev.mu.Lock()
			m.ev.keyDefs[n] = a[1].S
			m.ev.mu.Unlock()
			return nil
		}
		return ErrIllegalCall
	})
	regStmt("BEEP", func(m *VM, a []Value) error {
		if !m.Sound.Real() {
			m.Con.Beep()
			return nil
		}
		return m.queueTone(800, 250*time.Millisecond, true)
	})
}
