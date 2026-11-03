package gfx

import (
	"math"
	"strings"
)

// Draw interpreta el macrolenguaje de DRAW. Soporta U D L R E F G H M B N
// A TA C S P; los argumentos "=" + VARPTR$ no están soportados.
func (s *Screen) Draw(cmds string) error {
	cmds = strings.ToUpper(cmds)
	i := 0
	num := func() (float64, bool) {
		for i < len(cmds) && cmds[i] == ' ' {
			i++
		}
		start := i
		if i < len(cmds) && (cmds[i] == '-' || cmds[i] == '+') {
			i++
		}
		for i < len(cmds) && cmds[i] >= '0' && cmds[i] <= '9' {
			i++
		}
		if i == start || (i == start+1 && (cmds[start] == '-' || cmds[start] == '+')) {
			i = start
			return 0, false
		}
		v := 0.0
		neg := false
		for _, c := range cmds[start:i] {
			switch c {
			case '-':
				neg = true
			case '+':
			default:
				v = v*10 + float64(c-'0')
			}
		}
		if neg {
			v = -v
		}
		return v, true
	}
	noDraw, noMove := false, false
	c := s.fg
	px, py := s.physX(s.lx), s.physY(s.ly)
	moveRel := func(dx, dy float64) {
		scale := float64(s.drawScale) / 4
		dx, dy = dx*scale, dy*scale
		if s.drawAngle != 0 {
			a := float64(s.drawAngle) * math.Pi / 180
			dx, dy = dx*math.Cos(a)+dy*math.Sin(a), -dx*math.Sin(a)+dy*math.Cos(a)
		}
		nx, ny := px+dx, py+dy
		if !noDraw {
			s.lineRaw(round(px), round(py), round(nx), round(ny), c, 0xFFFF)
		}
		if !noMove {
			px, py = nx, ny
		}
		noDraw, noMove = false, false
	}
	for i < len(cmds) {
		ch := cmds[i]
		i++
		switch ch {
		case ' ', ';':
		case 'B':
			noDraw = true
		case 'N':
			noMove = true
		case 'U', 'D', 'L', 'R', 'E', 'F', 'G', 'H':
			n, ok := num()
			if !ok {
				n = 1
			}
			dirs := map[byte][2]float64{'U': {0, -1}, 'D': {0, 1}, 'L': {-1, 0}, 'R': {1, 0}, 'E': {1, -1}, 'F': {1, 1}, 'G': {-1, 1}, 'H': {-1, -1}}
			d := dirs[ch]
			moveRel(d[0]*n, d[1]*n)
		case 'M':
			for i < len(cmds) && cmds[i] == ' ' {
				i++
			}
			rel := i < len(cmds) && (cmds[i] == '+' || cmds[i] == '-')
			x, ok1 := num()
			if i < len(cmds) && cmds[i] == ',' {
				i++
			}
			y, ok2 := num()
			if !ok1 || !ok2 {
				return ErrIllegal
			}
			if rel {
				moveRel(x, y)
			} else {
				nx, ny := s.physX(x), s.physY(y)
				if !noDraw {
					s.lineRaw(round(px), round(py), round(nx), round(ny), c, 0xFFFF)
				}
				if !noMove {
					px, py = nx, ny
				}
				noDraw, noMove = false, false
			}
		case 'A':
			n, _ := num()
			if n < 0 || n > 3 {
				return ErrIllegal
			}
			s.drawAngle = int(n) * 90
		case 'T':
			if i < len(cmds) && cmds[i] == 'A' {
				i++
			}
			n, _ := num()
			if n < -360 || n > 360 {
				return ErrIllegal
			}
			s.drawAngle = int(n)
		case 'C':
			n, _ := num()
			c = int(n)
		case 'S':
			n, _ := num()
			if n < 1 || n > 255 {
				return ErrIllegal
			}
			s.drawScale = int(n)
		case 'P':
			f, _ := num()
			if i < len(cmds) && cmds[i] == ',' {
				i++
			}
			b, _ := num()
			s.lx, s.ly = s.logX(px), s.logY(py)
			s.Paint(false, s.lx, s.ly, int(f), nil, int(b))
		case 'X', '=':
			return ErrIllegal // VARPTR$: no soportado
		default:
			return ErrIllegal
		}
	}
	s.lx, s.ly = s.logX(px), s.logY(py)
	s.touch()
	return nil
}
