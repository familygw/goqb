// Package sound reproduce SOUND/PLAY/BEEP con una onda cuadrada, como el
// parlante de la PC. En macOS y Windows usa oto (sin cgo); en el resto de
// las plataformas cae a un dispositivo silencioso que respeta los tiempos.
package sound

import (
	"math"
	"sync"
	"time"
)

const sampleRate = 44100

type note struct {
	freq  float64
	total int
	left  int
}

// Synth genera las muestras de la cola de notas (int16 mono little endian).
type Synth struct {
	mu    sync.Mutex
	queue []note
	phase float64
}

// Tone encola un tono (freq 0 = silencio).
func (s *Synth) Tone(freq float64, dur time.Duration) {
	n := int(dur.Seconds() * sampleRate)
	if n <= 0 {
		return
	}
	s.mu.Lock()
	s.queue = append(s.queue, note{freq, n, n})
	s.mu.Unlock()
}

// Pending devuelve las notas que faltan reproducir.
func (s *Synth) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.queue)
}

// Stop vacía la cola.
func (s *Synth) Stop() {
	s.mu.Lock()
	s.queue = nil
	s.mu.Unlock()
}

// Real indica que hay salida de audio.
func (s *Synth) Real() bool { return true }

const (
	amplitude = 0.18 * 32767
	ramp      = sampleRate / 500 // 2 ms de rampa para evitar clicks
)

// Read implementa io.Reader para el reproductor: produce silencio si no hay notas.
func (s *Synth) Read(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := len(p) / 2
	for i := 0; i < n; i++ {
		var v float64
		if len(s.queue) > 0 {
			nt := &s.queue[0]
			if nt.freq > 0 {
				pos := nt.total - nt.left
				env := 1.0
				if pos < ramp {
					env = float64(pos) / ramp
				} else if nt.left < ramp {
					env = float64(nt.left) / ramp
				}
				if math.Sin(s.phase) >= 0 {
					v = amplitude * env
				} else {
					v = -amplitude * env
				}
				s.phase += 2 * math.Pi * nt.freq / sampleRate
				if s.phase > 2*math.Pi {
					s.phase -= 2 * math.Pi
				}
			}
			nt.left--
			if nt.left <= 0 {
				s.queue = s.queue[1:]
			}
		}
		x := int16(v)
		p[2*i], p[2*i+1] = byte(x), byte(x>>8)
	}
	return n * 2, nil
}
