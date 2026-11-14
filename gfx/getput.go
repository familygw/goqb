package gfx

// Get implementa GET (x1,y1)-(x2,y2): devuelve la imagen en el formato de
// los arrays de QB: ancho en bits (2 bytes), alto (2 bytes) y filas
// empaquetadas (alineadas a byte, MSB primero).
func (s *Screen) Get(step1 bool, x1, y1 float64, step2 bool, x2, y2 float64) ([]byte, error) {
	px1, py1 := s.point(step1, x1, y1)
	px2, py2 := s.point(step2, x2, y2)
	if px1 > px2 {
		px1, px2 = px2, px1
	}
	if py1 > py2 {
		py1, py2 = py2, py1
	}
	if px1 < 0 || py1 < 0 || px2 >= s.M.W || py2 >= s.M.H {
		return nil, ErrIllegal
	}
	w, h, bpp := px2-px1+1, py2-py1+1, s.M.BPP
	rowBytes := (w*bpp + 7) / 8
	out := make([]byte, 4+h*rowBytes)
	wb := w * bpp
	out[0], out[1], out[2], out[3] = byte(wb), byte(wb>>8), byte(h), byte(h>>8)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := s.get(px1+x, py1+y)
			bit := x * bpp
			off := 4 + y*rowBytes + bit/8
			shift := 8 - bpp - bit%8
			out[off] |= byte(c<<uint(shift)) & (byte(1<<uint(bpp)-1) << uint(shift))
		}
	}
	return out, nil
}

// Put implementa PUT (x,y), imagen, acción (PSET, PRESET, AND, OR, XOR).
func (s *Screen) Put(step bool, x, y float64, img []byte, action string) error {
	if len(img) < 4 {
		return ErrIllegal
	}
	bpp := s.M.BPP
	wb := int(img[0]) | int(img[1])<<8
	h := int(img[2]) | int(img[3])<<8
	w := wb / bpp
	rowBytes := (w*bpp + 7) / 8
	if w <= 0 || h <= 0 || len(img) < 4+h*rowBytes {
		return ErrIllegal
	}
	px, py := s.point(step, x, y)
	if px < 0 || py < 0 || px+w > s.M.W || py+h > s.M.H {
		return ErrIllegal
	}
	mask := 1<<uint(bpp) - 1
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			bit := xx * bpp
			b := int(img[4+yy*rowBytes+bit/8])
			c := b >> uint(8-bpp-bit%8) & mask
			old := s.get(px+xx, py+yy)
			switch action {
			case "PSET":
			case "PRESET":
				c = ^c & mask
			case "AND":
				c &= old
			case "OR":
				c |= old
			default: // XOR
				c ^= old
			}
			s.set(px+xx, py+yy, c)
		}
	}
	s.touch()
	return nil
}
