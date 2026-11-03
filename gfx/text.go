package gfx

// DrawChar dibuja un carácter CP437 en la celda de texto (col, row), 1-based,
// con el color de frente fg y fondo bg (como PRINT en modo gráfico).
func (s *Screen) DrawChar(col, row int, ch byte, fg, bg int) {
	ch8 := Font8x8[ch]
	h := s.M.CharH
	x0, y0 := (col-1)*8, (row-1)*h
	if x0+8 > s.M.W || y0+h > s.M.H {
		return
	}
	p := s.pix()
	n := uint8(s.M.Colors - 1)
	if s.M.Colors == 256 {
		n = 255
	}
	f, b := uint8(fg)&n, uint8(bg)&n
	for y := 0; y < h; y++ {
		bits := ch8[y*8/h]
		row := p[(y0+y)*s.M.W+x0:]
		for x := 0; x < 8; x++ {
			if bits&(0x80>>uint(x)) != 0 {
				row[x] = f
			} else {
				row[x] = b
			}
		}
	}
	s.touch()
}

// ScrollText desplaza una fila de texto hacia arriba las filas top..bottom (1-based).
func (s *Screen) ScrollText(top, bottom, bg int) {
	h, w := s.M.CharH, s.M.W
	p := s.pix()
	from, to := (top-1)*h, bottom*h
	if to > s.M.H {
		to = s.M.H
	}
	copy(p[from*w:], p[(from+h)*w:to*w])
	for i := (to - h) * w; i < to*w; i++ {
		p[i] = uint8(bg)
	}
	s.touch()
}

// ClearText borra las filas de texto top..bottom (1-based).
func (s *Screen) ClearText(top, bottom, bg int) {
	h, w := s.M.CharH, s.M.W
	p := s.pix()
	end := bottom * h * w
	if end > len(p) {
		end = len(p)
	}
	for i := (top - 1) * h * w; i < end; i++ {
		p[i] = uint8(bg)
	}
	s.touch()
}
