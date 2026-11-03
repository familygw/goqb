package screen

import "testing"

func TestBlockColorsAndBlink(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fg, bg int
		want   byte
	}{
		{"pared de Nibbles", 12, 12, 0x4c},
		{"serpiente de Nibbles", 14, 14, 0x6e},
		{"fondo brillante sin parpadeo", 7, 15, 0x77},
		{"parpadeo explícito", 30, 1, 0x9e},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(80, 25)
			s.Color(tc.fg, tc.bg, -1)
			s.Print(string([]byte{219}))
			cell := s.Snapshot().Cells[0]
			if cell.Ch != 219 || cell.Attr != tc.want {
				t.Fatalf("bloque: %+v; atributo esperado: %#02x", cell, tc.want)
			}
		})
	}
}
