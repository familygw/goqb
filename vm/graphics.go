package vm

import (
	"errors"

	"goqb/gfx"
)

// GraphicsConsole es una consola con modos gráficos (SCREEN 1..13).
type GraphicsConsole interface {
	// SetScreen cambia de modo (0 = texto) y de páginas activa/visible.
	SetScreen(mode, apage, vpage int) error
	// ScreenMode devuelve el modo actual.
	ScreenMode() int
	// WithGfx ejecuta fn sobre la pantalla gráfica (con el bloqueo necesario).
	WithGfx(fn func(g *gfx.Screen) error) error
}

func (m *VM) gfxCon() (GraphicsConsole, bool) {
	g, ok := m.Con.(GraphicsConsole)
	return g, ok
}

// withGfx ejecuta fn si hay modo gráfico activo; si no, Illegal function call.
func (m *VM) withGfx(fn func(g *gfx.Screen) error) error {
	gc, ok := m.gfxCon()
	if !ok {
		return ErrAdvancedFeature
	}
	if gc.ScreenMode() == 0 {
		return ErrIllegalCall
	}
	err := gc.WithGfx(fn)
	if errors.Is(err, gfx.ErrIllegal) {
		return ErrIllegalCall
	}
	return err
}

func num(v Value) float64 { return v.Num() }

func optNum(a []Value, i int) (float64, bool) {
	if i >= len(a) || isNone(a[i]) {
		return 0, false
	}
	return a[i].Num(), true
}

func optColor(a []Value, i int) (int, error) {
	if i >= len(a) || isNone(a[i]) {
		return -1, nil
	}
	c, err := intArg(a[i])
	if err != nil {
		return 0, err
	}
	if c < 0 {
		return 0, ErrIllegalCall
	}
	return c, nil
}

// arrayStart devuelve el array y el índice lineal de inicio para GET/PUT/PALETTE USING.
func arrayStart(ref Value, idx Value) (*Array, int, error) {
	slot := ref.P.Deref()
	if slot.K != KArray || slot.A == nil {
		return nil, 0, ErrSubscript
	}
	a := slot.A
	start := 0
	if !isNone(idx) {
		i, err := intArg(idx)
		if err != nil {
			return nil, 0, err
		}
		start = i - a.Lo[0]
	}
	if start < 0 || start >= len(a.Data) {
		return nil, 0, ErrSubscript
	}
	if !a.Proto.K.IsNumeric() {
		return nil, 0, ErrTypeMismatch
	}
	return a, start, nil
}

// readArrayBytes serializa los elementos desde start.
func readArrayBytes(a *Array, start int) []byte {
	var b []byte
	for _, v := range a.Data[start:] {
		b = encode(v, b)
	}
	return b
}

// writeArrayBytes guarda bytes en los elementos desde start.
func writeArrayBytes(a *Array, start int, data []byte) error {
	size := sizeOf(a.Proto)
	need := (len(data) + size - 1) / size
	if start+need > len(a.Data) {
		return ErrIllegalCall
	}
	buf := readArrayBytes(a, start)[:need*size]
	copy(buf, data)
	for i := 0; i < need; i++ {
		decode(&a.Data[start+i], buf[i*size:])
	}
	return nil
}

func init() {
	regStmt("PSET", func(m *VM, a []Value) error { // preset, step, x, y, color
		c, err := optColor(a, 4)
		if err != nil {
			return err
		}
		return m.withGfx(func(g *gfx.Screen) error {
			g.PSet(a[1].IsTrue(), num(a[2]), num(a[3]), c, a[0].IsTrue())
			return nil
		})
	})
	regStmt("LINE", func(m *VM, a []Value) error { // hasFrom, s1, x1, y1, s2, x2, y2, color, box, style
		c, err := optColor(a, 7)
		if err != nil {
			return err
		}
		box, _ := intArg(a[8])
		style := -1
		if !isNone(a[9]) {
			v, err := ToLong(a[9])
			if err != nil {
				return err
			}
			style = int(uint16(v.I))
		}
		return m.withGfx(func(g *gfx.Screen) error {
			g.Line(a[0].IsTrue(), a[1].IsTrue(), num(a[2]), num(a[3]), a[4].IsTrue(), num(a[5]), num(a[6]), c, box, style)
			return nil
		})
	})
	regStmt("CIRCLE", func(m *VM, a []Value) error { // step, x, y, r, color, start, end, aspect
		c, err := optColor(a, 4)
		if err != nil {
			return err
		}
		st, hs := optNum(a, 5)
		en, he := optNum(a, 6)
		as, ha := optNum(a, 7)
		return m.withGfx(func(g *gfx.Screen) error {
			return g.Circle(a[0].IsTrue(), num(a[1]), num(a[2]), num(a[3]), c, st, en, as, hs, he, ha)
		})
	})
	regStmt("PAINT", func(m *VM, a []Value) error { // step, x, y, fill(num|string), border
		fill, tile := -1, []byte(nil)
		if !isNone(a[3]) {
			if a[3].K == KString {
				if a[3].S == "" || len(a[3].S) > 64 {
					return ErrIllegalCall
				}
				tile = []byte(a[3].S)
			} else {
				var err error
				if fill, err = optColor(a, 3); err != nil {
					return err
				}
			}
		}
		border, err := optColor(a, 4)
		if err != nil {
			return err
		}
		return m.withGfx(func(g *gfx.Screen) error {
			g.Paint(a[0].IsTrue(), num(a[1]), num(a[2]), fill, tile, border)
			return nil
		})
	})
	regStmt("DRAW", func(m *VM, a []Value) error {
		if a[0].K != KString {
			return ErrTypeMismatch
		}
		return m.withGfx(func(g *gfx.Screen) error { return g.Draw(a[0].S) })
	})
	regStmt("VIEW", func(m *VM, a []Value) error { // set, abs, x1, y1, x2, y2, fill, border
		fill, err := optColor(a, 6)
		if err != nil {
			return err
		}
		border, err := optColor(a, 7)
		if err != nil {
			return err
		}
		return m.withGfx(func(g *gfx.Screen) error {
			r := func(v Value) int { x, _ := ToLong(v); return int(x.I) }
			if !a[0].IsTrue() {
				return g.View(false, false, 0, 0, 0, 0, -1, -1)
			}
			return g.View(true, a[1].IsTrue(), r(a[2]), r(a[3]), r(a[4]), r(a[5]), fill, border)
		})
	})
	regStmt("WINDOW", func(m *VM, a []Value) error { // set, screen, x1, y1, x2, y2
		return m.withGfx(func(g *gfx.Screen) error {
			return g.Window(a[0].IsTrue(), a[1].IsTrue(), num(a[2]), num(a[3]), num(a[4]), num(a[5]))
		})
	})
	regStmt("PALETTE", func(m *VM, a []Value) error { // attr, color (ambos omitidos: restablecer)
		gc, ok := m.gfxCon()
		if !ok || gc.ScreenMode() == 0 {
			return nil // en modo texto la paleta no tiene efecto visible aquí
		}
		if len(a) == 0 || isNone(a[0]) {
			return m.withGfx(func(g *gfx.Screen) error { g.ResetPalette(); return nil })
		}
		attr, err := intArg(a[0])
		if err != nil {
			return err
		}
		col, err := ToLong(a[1])
		if err != nil {
			return err
		}
		return m.withGfx(func(g *gfx.Screen) error { return g.Palette(attr, col.I) })
	})
	regStmt("PALETTE_USING", func(m *VM, a []Value) error { // arrayRef, idx
		arr, start, err := arrayStart(a[0], a[1])
		if err != nil {
			return err
		}
		return m.withGfx(func(g *gfx.Screen) error {
			for i := 0; i < g.M.Colors && start+i < len(arr.Data) && i < 256; i++ {
				c := int64(arr.Data[start+i].Num())
				if c == -1 {
					continue
				}
				if err := g.Palette(i, c); err != nil {
					return err
				}
			}
			return nil
		})
	})
	regStmt("PCOPY", func(m *VM, a []Value) error {
		src, err := intArg(a[0])
		if err != nil {
			return err
		}
		dst, err := intArg(a[1])
		if err != nil {
			return err
		}
		gc, ok := m.gfxCon()
		if !ok || gc.ScreenMode() == 0 {
			if src == 0 && dst == 0 {
				return nil
			}
			return ErrIllegalCall
		}
		return m.withGfx(func(g *gfx.Screen) error { return g.PCopy(src, dst) })
	})
	regStmt("GFX_GET", func(m *VM, a []Value) error { // s1, x1, y1, s2, x2, y2, arrayRef, idx
		arr, start, err := arrayStart(a[6], a[7])
		if err != nil {
			return err
		}
		var img []byte
		err = m.withGfx(func(g *gfx.Screen) error {
			var e error
			img, e = g.Get(a[0].IsTrue(), num(a[1]), num(a[2]), a[3].IsTrue(), num(a[4]), num(a[5]))
			return e
		})
		if err != nil {
			return err
		}
		return writeArrayBytes(arr, start, img)
	})
	regStmt("GFX_PUT", func(m *VM, a []Value) error { // step, x, y, arrayRef, idx, acción
		arr, start, err := arrayStart(a[3], a[4])
		if err != nil {
			return err
		}
		img := readArrayBytes(arr, start)
		return m.withGfx(func(g *gfx.Screen) error {
			return g.Put(a[0].IsTrue(), num(a[1]), num(a[2]), img, a[5].S)
		})
	})

	reg(Builtin{Name: "POINT", Args: "n|n", Ret: KInt, Fn: func(m *VM, a []Value) (Value, error) {
		var r Value
		err := m.withGfx(func(g *gfx.Screen) error {
			if len(a) == 1 {
				n, err := intArg(a[0])
				if err != nil {
					return err
				}
				v, err := g.PointFn(n)
				if err != nil {
					return err
				}
				r = Single(v)
				return nil
			}
			r = Int(int64(g.Point(num(a[0]), num(a[1]))))
			return nil
		})
		return r, err
	}})
	reg(Builtin{Name: "PMAP", Args: "nn", Ret: KSingle, Fn: func(m *VM, a []Value) (Value, error) {
		fn, err := intArg(a[1])
		if err != nil {
			return none, err
		}
		var r Value
		err = m.withGfx(func(g *gfx.Screen) error {
			v, e := g.PMap(num(a[0]), fn)
			r = Single(v)
			return e
		})
		return r, err
	}})
}
