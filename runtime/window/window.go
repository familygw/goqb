//go:build window

// Package window ejecuta un programa en una ventana nativa con ebiten
// (fidelidad total para los modos gráficos). Requiere compilar con
// -tags window (en macOS y Linux usa cgo).
package window

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"goqb/gfx"
	"goqb/runtime/screen"
	"goqb/runtime/sound"
	"goqb/vm"
)

// Available reporta si el backend de ventana fue compilado.
const Available = true

type game struct {
	txt  *screen.Text
	m    *vm.VM
	done chan error
	err  error
	end  bool
	img  *ebiten.Image
	w, h int
}

var specialKeys = map[ebiten.Key]string{
	ebiten.KeyEnter: "\r", ebiten.KeyBackspace: "\b", ebiten.KeyTab: "\t", ebiten.KeyEscape: "\x1b",
	ebiten.KeyArrowUp: "\x00H", ebiten.KeyArrowDown: "\x00P", ebiten.KeyArrowLeft: "\x00K", ebiten.KeyArrowRight: "\x00M",
	ebiten.KeyHome: "\x00G", ebiten.KeyEnd: "\x00O", ebiten.KeyPageUp: "\x00I", ebiten.KeyPageDown: "\x00Q",
	ebiten.KeyInsert: "\x00R", ebiten.KeyDelete: "\x00S",
	ebiten.KeyF1: "\x00;", ebiten.KeyF2: "\x00<", ebiten.KeyF3: "\x00=", ebiten.KeyF4: "\x00>",
	ebiten.KeyF5: "\x00?", ebiten.KeyF6: "\x00@", ebiten.KeyF7: "\x00A", ebiten.KeyF8: "\x00B",
	ebiten.KeyF9: "\x00C", ebiten.KeyF10: "\x00D", ebiten.KeyF11: "\x00\x85", ebiten.KeyF12: "\x00\x86",
}

func (g *game) Update() error {
	select {
	case err := <-g.done:
		g.err, g.end = err, true
	default:
	}
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if g.end {
			return ebiten.Termination
		}
		if ctrl && k == ebiten.KeyC {
			g.m.Stop()
			g.txt.Interrupt()
			continue
		}
		if ctrl && k >= ebiten.KeyA && k <= ebiten.KeyZ {
			g.txt.PushKey(string([]byte{byte(k-ebiten.KeyA) + 1}))
			continue
		}
		if s, ok := specialKeys[k]; ok {
			g.txt.PushKey(s)
		}
	}
	if !ctrl {
		for _, r := range ebiten.AppendInputChars(nil) {
			if b, ok := cp437Byte(r); ok {
				g.txt.PushKey(string([]byte{b}))
			}
		}
	}
	return nil
}

func (g *game) Draw(dst *ebiten.Image) {
	s := g.txt.Snapshot()
	var rgba *image.RGBA
	if s.Gfx != nil {
		rgba = frameImage(s.Gfx)
	} else {
		rgba = textImage(s)
	}
	b := rgba.Bounds()
	if g.img == nil || g.w != b.Dx() || g.h != b.Dy() {
		g.img = ebiten.NewImage(b.Dx(), b.Dy())
		g.w, g.h = b.Dx(), b.Dy()
	}
	g.img.WritePixels(rgba.Pix)
	op := &ebiten.DrawImageOptions{}
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()
	op.GeoM.Scale(float64(dw)/float64(g.w), float64(dh)/float64(g.h))
	dst.DrawImage(g.img, op)
}

// Layout fija una superficie 4:3; ebiten escala a la ventana.
func (g *game) Layout(w, h int) (int, int) { return 640, 480 }

func frameImage(f *gfx.Frame) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, f.W, f.H))
	for i, p := range f.Pix {
		c := f.Pal[p]
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = uint8(c>>16), uint8(c>>8), uint8(c), 255
	}
	return img
}

// textImage dibuja la pantalla de texto con la fuente 8×16 (escalada de 8×8).
func textImage(s screen.Snapshot) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, s.Cols*8, s.Rows*16))
	for r := 0; r < s.Rows; r++ {
		for c := 0; c < s.Cols; c++ {
			cell := s.Cells[r*s.Cols+c]
			fg, bg := gfx.CGA16[cell.Attr&15], gfx.CGA16[cell.Attr>>4&7]
			glyph := gfx.Font8x8[cell.Ch]
			cursor := s.Cursor && r == s.Row-1 && c == s.Col-1
			for y := 0; y < 16; y++ {
				bits := glyph[y/2]
				if cursor && y >= 14 {
					bits = 0xFF
				}
				for x := 0; x < 8; x++ {
					col := bg
					if bits&(0x80>>uint(x)) != 0 {
						col = fg
					}
					img.Set(c*8+x, r*16+y, color.RGBA{uint8(col >> 16), uint8(col >> 8), uint8(col), 255})
				}
			}
		}
	}
	return img
}

// Run ejecuta prog en una ventana hasta que termine y se presione una tecla.
func Run(prog *vm.Program, command string, title string) error {
	txt := screen.New(80, 25)
	m := vm.New(prog, txt)
	m.Command = command
	m.Sound = sound.Device()
	g := &game{txt: txt, m: m, done: make(chan error, 1)}
	go func() { g.done <- m.Run() }()
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowSize(960, 720)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(g); err != nil {
		return err
	}
	return g.err
}
