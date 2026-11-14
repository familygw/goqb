package gfx

import "testing"

// TestAllModes: SCREEN 1-4 y 7-13 existen, 5 y 6 no (como en QB 4.5).
func TestAllModes(t *testing.T) {
	for n := 1; n <= 13; n++ {
		s, err := New(n)
		if n == 5 || n == 6 {
			if err == nil {
				t.Errorf("SCREEN %d no debería existir", n)
			}
			continue
		}
		if err != nil {
			t.Fatalf("SCREEN %d: %v", n, err)
		}
		// dibujar y escribir en la última celda de texto no debe fallar
		s.Line(true, false, 0, 0, false, float64(s.M.W-1), float64(s.M.H-1), s.Foreground(), 1, -1)
		s.Circle(false, float64(s.M.W/2), float64(s.M.H/2), float64(s.M.H/3), s.Foreground(), 0, 0, 0, false, false, false)
		s.DrawChar(s.M.Cols, s.M.Rows, 'X', s.Foreground(), 0)
		if s.Point(0, 0) != s.Foreground() {
			t.Errorf("SCREEN %d: el recuadro no se dibujó", n)
		}
	}
}

func TestHercules(t *testing.T) {
	s, _ := New(3)
	if s.M.CharW() != 9 || s.M.Pages != 2 {
		t.Fatalf("modo 3: celda %d, páginas %d", s.M.CharW(), s.M.Pages)
	}
	if s.Color(1, -1) != ErrIllegal {
		t.Error("COLOR debería ser inválido en SCREEN 3")
	}
	// 'Ä' (C4h, línea horizontal de recuadro) ocupa también la novena columna
	s.DrawChar(1, 1, 0xC4, 1, 0)
	p, any8 := s.pix(), false
	for y := 0; y < 14; y++ {
		if p[y*720+8] == 1 {
			any8 = true
		}
	}
	if !any8 {
		t.Error("la novena columna de los caracteres de recuadro debería repetir la octava")
	}
	// la columna 80 empieza en x = 711
	s.DrawChar(80, 2, 0xDB, 1, 0)
	if p[(14+3)*720+711] != 1 || p[(14+3)*720+718] != 1 {
		t.Error("la columna 80 no quedó en x = 711..719")
	}
}

func TestOlivetti(t *testing.T) {
	s, _ := New(4)
	if s.M.W != 640 || s.M.H != 400 || s.M.Colors != 2 {
		t.Fatalf("modo 4: %+v", s.M)
	}
	if err := s.Color(4, -1); err != nil {
		t.Fatal(err)
	}
	if s.pal[1] != CGA16[4] || s.pal[0] != 0 {
		t.Errorf("COLOR 4 en SCREEN 4: paleta %06X %06X", s.pal[0], s.pal[1])
	}
	if s.Color(1, 2) != ErrIllegal {
		t.Error("SCREEN 4 no acepta color de fondo")
	}
}
