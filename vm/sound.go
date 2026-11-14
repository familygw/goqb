package vm

import (
	"math"
	"strings"
	"sync"
	"time"
)

// SoundDevice reproduce tonos en cola (SOUND, PLAY, BEEP).
type SoundDevice interface {
	// Tone encola un tono; freq 0 es silencio. vol en 0..1.
	Tone(freq float64, dur time.Duration)
	// Pending devuelve cuántas notas faltan reproducir (incluida la actual).
	Pending() int
	// Stop vacía la cola y silencia.
	Stop()
	// Real indica si hay salida de audio (si no, BEEP usa la campana de la terminal).
	Real() bool
}

// SilentSound respeta los tiempos de la música sin emitir sonido.
type SilentSound struct {
	mu   sync.Mutex
	ends []time.Time
}

func (s *SilentSound) Tone(freq float64, dur time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	start := time.Now()
	if n := len(s.ends); n > 0 && s.ends[n-1].After(start) {
		start = s.ends[n-1]
	}
	s.ends = append(s.ends, start.Add(dur))
}

func (s *SilentSound) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	i := 0
	for i < len(s.ends) && !s.ends[i].After(now) {
		i++
	}
	s.ends = s.ends[i:]
	return len(s.ends)
}

func (s *SilentSound) Stop() {
	s.mu.Lock()
	s.ends = nil
	s.mu.Unlock()
}

func (s *SilentSound) Real() bool { return false }

// ------------------------------------------------------------- PLAY

type musicState struct {
	octave     int
	length     int
	tempo      int
	articulate float64 // fracción sonora de cada nota: MN 7/8, ML 1, MS 3/4
	background bool
}

func newMusic() musicState {
	return musicState{octave: 4, length: 4, tempo: 120, articulate: 7.0 / 8}
}

// noteFreq: la octava 3 empieza en el do central; el La de la octava 3 es 440 Hz.
func noteFreq(n int) float64 { // n = octava*12 + semitono (do = 0)
	return 440 * math.Pow(2, float64(n-(3*12+9))/12)
}

// queueTone encola un tono y, en primer plano, espera a que la cola se vacíe.
func (m *VM) queueTone(freq float64, dur time.Duration, wait bool) error {
	// el búfer de música de QB admite 32 notas: si está lleno, se espera
	for m.Sound.Pending() >= 32 {
		if err := m.soundSleep(); err != nil {
			return err
		}
	}
	m.Sound.Tone(freq, dur)
	if wait {
		return m.waitSound()
	}
	return nil
}

func (m *VM) soundSleep() error {
	if m.stop.Load() {
		m.Sound.Stop()
		return errStopped
	}
	time.Sleep(5 * time.Millisecond)
	return nil
}

func (m *VM) waitSound() error {
	for m.Sound.Pending() > 0 {
		if err := m.soundSleep(); err != nil {
			return err
		}
	}
	return nil
}

// play interpreta el macrolenguaje musical de PLAY.
func (m *VM) play(s string) error {
	st := &m.music
	s = strings.ToUpper(s)
	i := 0
	num := func() (int, bool) {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return 0, false
		}
		n := 0
		for _, c := range s[start:i] {
			n = n*10 + int(c-'0')
		}
		return n, true
	}
	dots := func() float64 {
		f, add := 1.0, 0.5
		for i < len(s) && s[i] == '.' {
			f += add
			add /= 2
			i++
		}
		return f
	}
	whole := func() float64 { return 240.0 / float64(st.tempo) } // segundos por redonda
	sound := func(n int, length int, mult float64) error {
		secs := whole() / float64(length) * mult
		total := time.Duration(secs * float64(time.Second))
		on := time.Duration(float64(total) * st.articulate)
		if n < 0 { // silencio
			return m.queueTone(0, total, false)
		}
		if err := m.queueTone(noteFreq(n), on, false); err != nil {
			return err
		}
		if total > on {
			return m.queueTone(0, total-on, false)
		}
		return nil
	}
	semis := map[byte]int{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	for i < len(s) {
		c := s[i]
		i++
		switch {
		case c == ' ' || c == ';':
		case c >= 'A' && c <= 'G':
			n := st.octave*12 + semis[c]
			if i < len(s) {
				switch s[i] {
				case '#', '+':
					n++
					i++
				case '-':
					n--
					i++
				}
			}
			length := st.length
			if l, ok := num(); ok {
				if l < 1 || l > 64 {
					return ErrIllegalCall
				}
				length = l
			}
			if err := sound(n, length, dots()); err != nil {
				return err
			}
		case c == 'N':
			n, ok := num()
			if !ok || n > 84 {
				return ErrIllegalCall
			}
			note := n - 1
			if n == 0 {
				note = -1
			}
			if err := sound(note, st.length, dots()); err != nil {
				return err
			}
		case c == 'O':
			n, ok := num()
			if !ok || n > 6 {
				return ErrIllegalCall
			}
			st.octave = n
		case c == '<':
			if st.octave > 0 {
				st.octave--
			}
		case c == '>':
			if st.octave < 6 {
				st.octave++
			}
		case c == 'L':
			n, ok := num()
			if !ok || n < 1 || n > 64 {
				return ErrIllegalCall
			}
			st.length = n
		case c == 'P' || c == 'R':
			n, ok := num()
			if !ok {
				n = st.length
			}
			if n < 1 || n > 64 {
				return ErrIllegalCall
			}
			if err := sound(-1, n, dots()); err != nil {
				return err
			}
		case c == 'T':
			n, ok := num()
			if !ok || n < 32 || n > 255 {
				return ErrIllegalCall
			}
			st.tempo = n
		case c == 'M':
			if i >= len(s) {
				return ErrIllegalCall
			}
			switch s[i] {
			case 'N':
				st.articulate = 7.0 / 8
			case 'L':
				st.articulate = 1
			case 'S':
				st.articulate = 3.0 / 4
			case 'F':
				st.background = false
			case 'B':
				st.background = true
			default:
				return ErrIllegalCall
			}
			i++
		case c == 'X' || c == '=':
			return ErrIllegalCall // VARPTR$: no soportado
		default:
			return ErrIllegalCall
		}
	}
	if !st.background {
		return m.waitSound()
	}
	return nil
}

func init() {
	regStmt("SOUND", func(m *VM, a []Value) error {
		f := a[0].Num()
		d := a[1].Num()
		if d < 0 || d > 65535 || f != 0 && (f < 37 || f > 32767) {
			return ErrIllegalCall
		}
		if d == 0 {
			m.Sound.Stop()
			return nil
		}
		dur := time.Duration(d / 18.2 * float64(time.Second))
		if f >= 32767 {
			f = 0
		}
		return m.queueTone(f, dur, !m.music.background)
	})
	regStmt("PLAY", func(m *VM, a []Value) error {
		if a[0].K != KString {
			return ErrTypeMismatch
		}
		return m.play(a[0].S)
	})
}
