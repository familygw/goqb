package sound

import (
	"testing"
	"time"
)

func TestSynth(t *testing.T) {
	s := &Synth{}
	s.Tone(440, 10*time.Millisecond)
	s.Tone(0, 10*time.Millisecond)
	if s.Pending() != 2 {
		t.Fatalf("pendientes: %d", s.Pending())
	}
	buf := make([]byte, 2*441) // 10 ms
	s.Read(buf)
	nonzero := 0
	for i := 0; i < len(buf); i += 2 {
		if buf[i] != 0 || buf[i+1] != 0 {
			nonzero++
		}
	}
	if nonzero < 300 || s.Pending() != 1 {
		t.Errorf("onda: %d muestras con sonido, %d pendientes", nonzero, s.Pending())
	}
	s.Read(buf)
	if s.Pending() != 0 {
		t.Error("la cola debería quedar vacía")
	}
}
