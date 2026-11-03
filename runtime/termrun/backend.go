package termrun

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"

	"goqb/gfx"
)

// Rect es un área de celdas de la terminal.
type Rect struct{ X, Y, W, H int }

// Backend muestra el framebuffer de un modo gráfico en la terminal.
type Backend interface {
	Name() string
	// Present dibuja f en el área (las imágenes se reenvían sólo si f cambió).
	Present(scr tcell.Screen, f *gfx.Frame, area Rect)
	// Clear borra las imágenes al volver a modo texto o al IDE.
	Clear(scr tcell.Screen)
}

// DetectBackend elige el mejor backend para la terminal actual.
// GOQB_GFX=blocks|kitty|iterm|sixel fuerza uno.
func DetectBackend(scr tcell.Screen) Backend {
	tty, ok := scr.Tty()
	name := strings.ToLower(os.Getenv("GOQB_GFX"))
	if name == "" {
		switch {
		case !ok:
			name = "blocks"
		case os.Getenv("TMUX") != "":
			name = "blocks" // tmux no deja pasar las imágenes sin configuración extra
		case os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(os.Getenv("TERM"), "kitty") || os.Getenv("TERM_PROGRAM") == "ghostty":
			name = "kitty"
		case os.Getenv("TERM_PROGRAM") == "iTerm.app" || os.Getenv("TERM_PROGRAM") == "WezTerm":
			name = "iterm"
		case strings.Contains(os.Getenv("TERM"), "sixel") || os.Getenv("TERM_PROGRAM") == "mlterm":
			name = "sixel"
		default:
			name = "blocks"
		}
	}
	if !ok {
		return &blocks{}
	}
	switch name {
	case "kitty":
		return &image_{kind: "kitty", tty: tty}
	case "iterm":
		return &image_{kind: "iterm", tty: tty}
	case "sixel":
		return &image_{kind: "sixel", tty: tty}
	}
	return &blocks{}
}

// FitArea calcula el área de celdas con relación 4:3 (como un monitor de
// la época) centrada en la terminal. cellAspect es alto/ancho de la celda.
func FitArea(scr tcell.Screen, cellAspect float64) Rect {
	w, h := scr.Size()
	// ancho en celdas para que w : h*cellAspect = 4 : 3
	cw := w
	ch := int(float64(cw) * 3 / 4 / cellAspect)
	if ch > h {
		ch = h
		cw = int(float64(ch) * cellAspect * 4 / 3)
	}
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	return Rect{(w - cw) / 2, (h - ch) / 2, cw, ch}
}

// ------------------------------------------------------------- medios bloques

// blocks dibuja con '▀': cada celda muestra dos píxeles (arriba y abajo).
type blocks struct{}

func (b *blocks) Name() string { return "blocks" }

func (b *blocks) Clear(scr tcell.Screen) {}

func (b *blocks) Present(scr tcell.Screen, f *gfx.Frame, area Rect) {
	ph := area.H * 2
	for cy := 0; cy < area.H; cy++ {
		for cx := 0; cx < area.W; cx++ {
			sx := cx * f.W / area.W
			sy1 := (2 * cy) * f.H / ph
			sy2 := (2*cy + 1) * f.H / ph
			top, bot := f.RGB(sx, sy1), f.RGB(sx, sy2)
			st := tcell.StyleDefault.Foreground(tcell.NewHexColor(int32(top))).Background(tcell.NewHexColor(int32(bot)))
			scr.SetContent(area.X+cx, area.Y+cy, '▀', nil, st)
		}
	}
}

// ------------------------------------------------------------- imágenes (kitty / iTerm2 / sixel)

type image_ struct {
	kind   string
	tty    tcell.Tty
	last   *gfx.Frame
	area   Rect
	sent   time.Time
	locked bool
}

func (b *image_) Name() string { return b.kind }

func (b *image_) Clear(scr tcell.Screen) {
	if b.locked {
		scr.LockRegion(b.area.X, b.area.Y, b.area.W, b.area.H, false)
		b.locked = false
	}
	if b.kind == "kitty" && b.last != nil {
		b.tty.Write([]byte("\x1b_Ga=d,d=A,q=2\x1b\\"))
	}
	if b.last != nil {
		scr.Sync()
	}
	b.last = nil
}

func (b *image_) Present(scr tcell.Screen, f *gfx.Frame, area Rect) {
	if f == b.last && area == b.area {
		return
	}
	// limitar a ~15 cuadros por segundo: transmitir imágenes es caro
	if time.Since(b.sent) < 66*time.Millisecond && area == b.area && b.last != nil {
		return
	}
	if b.locked && area != b.area {
		scr.LockRegion(b.area.X, b.area.Y, b.area.W, b.area.H, false)
		b.locked = false
	}
	if !b.locked {
		// fondo negro bajo la imagen, luego se bloquea para que tcell no lo pise
		for y := 0; y < area.H; y++ {
			for x := 0; x < area.W; x++ {
				scr.SetContent(area.X+x, area.Y+y, ' ', nil, tcell.StyleDefault.Background(tcell.ColorBlack))
			}
		}
		scr.Show()
		scr.LockRegion(area.X, area.Y, area.W, area.H, true)
		b.locked = true
	}
	b.last, b.area, b.sent = f, area, time.Now()

	var out bytes.Buffer
	fmt.Fprintf(&out, "\x1b7\x1b[%d;%dH", area.Y+1, area.X+1) // guardar cursor y posicionar
	switch b.kind {
	case "kitty":
		writeKitty(&out, f, area)
	case "iterm":
		writeITerm(&out, f, area)
	case "sixel":
		cw, chh := cellPixels(b.tty)
		writeSixel(&out, f, area.W*cw, area.H*chh)
	}
	out.WriteString("\x1b8") // restaurar cursor
	b.tty.Write(out.Bytes())
}

// cellPixels devuelve el tamaño de una celda en píxeles (8×16 si no se conoce).
func cellPixels(tty tcell.Tty) (int, int) {
	ws, err := tty.WindowSize()
	if err != nil || ws.Width == 0 || ws.Height == 0 || ws.PixelWidth == 0 || ws.PixelHeight == 0 {
		return 8, 16
	}
	return ws.PixelWidth / ws.Width, ws.PixelHeight / ws.Height
}

// encodePNG codifica el frame como PNG con paleta (rápido y compacto).
func encodePNG(f *gfx.Frame) []byte {
	pal := make(color.Palette, 256)
	for i, c := range f.Pal {
		pal[i] = color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255}
	}
	img := &image.Paletted{Pix: f.Pix, Stride: f.W, Rect: image.Rect(0, 0, f.W, f.H), Palette: pal}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&buf, img)
	return buf.Bytes()
}

func writeKitty(out *bytes.Buffer, f *gfx.Frame, area Rect) {
	data := base64.StdEncoding.EncodeToString(encodePNG(f))
	out.WriteString("\x1b_Ga=d,d=i,i=1,q=2\x1b\\") // borra la imagen anterior
	const chunk = 4096
	for i := 0; i < len(data); i += chunk {
		end := min(i+chunk, len(data))
		more := 0
		if end < len(data) {
			more = 1
		}
		out.WriteString("\x1b_G")
		if i == 0 {
			out.WriteString("a=T,f=100,i=1,q=2,C=1,c=")
			out.WriteString(strconv.Itoa(area.W))
			out.WriteString(",r=")
			out.WriteString(strconv.Itoa(area.H))
			out.WriteString(",")
		}
		out.WriteString("m=")
		out.WriteString(strconv.Itoa(more))
		out.WriteByte(';')
		out.WriteString(data[i:end])
		out.WriteString("\x1b\\")
	}
}

func writeITerm(out *bytes.Buffer, f *gfx.Frame, area Rect) {
	p := encodePNG(f)
	out.WriteString("\x1b]1337;File=inline=1;preserveAspectRatio=0;width=")
	out.WriteString(strconv.Itoa(area.W))
	out.WriteString(";height=")
	out.WriteString(strconv.Itoa(area.H))
	out.WriteString(";size=")
	out.WriteString(strconv.Itoa(len(p)))
	out.WriteByte(':')
	out.WriteString(base64.StdEncoding.EncodeToString(p))
	out.WriteByte('\a')
}

// writeSixel codifica el frame escalado a w×h píxeles en formato sixel.
func writeSixel(out *bytes.Buffer, f *gfx.Frame, w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	// colores usados y su índice sixel
	idx := map[uint8]int{}
	var used []uint8
	for _, p := range f.Pix {
		if _, ok := idx[p]; !ok {
			idx[p] = len(used)
			used = append(used, p)
		}
	}
	out.WriteString("\x1bPq\"1;1;")
	out.WriteString(strconv.Itoa(w))
	out.WriteByte(';')
	out.WriteString(strconv.Itoa(h))
	for i, p := range used {
		c := f.Pal[p]
		out.WriteByte('#')
		out.WriteString(strconv.Itoa(i))
		out.WriteString(";2;")
		out.WriteString(strconv.Itoa(int(c>>16&0xFF) * 100 / 255))
		out.WriteByte(';')
		out.WriteString(strconv.Itoa(int(c>>8&0xFF) * 100 / 255))
		out.WriteByte(';')
		out.WriteString(strconv.Itoa(int(c&0xFF) * 100 / 255))
	}
	srcX := make([]int, w)
	for x := range srcX {
		srcX[x] = x * f.W / w
	}
	row := make([]byte, w)
	for band := 0; band < h; band += 6 {
		first := true
		for ci, p := range used {
			any := false
			for x := 0; x < w; x++ {
				var bits byte
				for k := 0; k < 6 && band+k < h; k++ {
					sy := (band + k) * f.H / h
					if f.Pix[sy*f.W+srcX[x]] == p {
						bits |= 1 << uint(k)
					}
				}
				row[x] = bits
				if bits != 0 {
					any = true
				}
			}
			if !any {
				continue
			}
			if !first {
				out.WriteByte('$')
			}
			first = false
			out.WriteByte('#')
			out.WriteString(strconv.Itoa(ci))
			// RLE
			for x := 0; x < w; {
				n := 1
				for x+n < w && row[x+n] == row[x] {
					n++
				}
				ch := row[x] + 63
				if n > 3 {
					out.WriteByte('!')
					out.WriteString(strconv.Itoa(n))
					out.WriteByte(ch)
				} else {
					for j := 0; j < n; j++ {
						out.WriteByte(ch)
					}
				}
				x += n
			}
		}
		out.WriteByte('-')
	}
	out.WriteString("\x1b\\")
}
