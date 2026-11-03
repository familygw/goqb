package vm

import (
	"bufio"
	"io"
	"strings"
	"sync"

	"goqb/gfx"
	"goqb/lang/cp437"
)

// Console es la pantalla de texto y el teclado del programa.
// Las coordenadas son 1-based como en QB. El texto es CP437.
type Console interface {
	Print(s string) // escribe en el cursor (con salto de línea automático y scroll)
	Newline()
	Cls()
	Locate(row, col int) // 0 = sin cambio
	Cursor() (row, col int)
	SetCursor(visible bool)
	Color(fg, bg, border int) // -1 = sin cambio
	Size() (cols, rows int)
	SetWidth(cols, rows int) error
	ViewPrint(top, bottom int) error // 0, 0 restablece
	CharAt(row, col int) (ch, attr byte)
	PutChar(row, col int, ch, attr byte) // para POKE en &HB800
	Beep()
	InKey() string             // "" si no hay tecla
	ReadKey() (string, error)  // bloqueante; error si se detuvo el programa
	ReadLine() (string, error) // edición de línea de INPUT, con eco
	Flush()
}

// PlainConsole es una consola de flujo (stdin/stdout) sin pantalla:
// sirve para tests, tuberías y terminales que no son TTY.
// LOCATE, COLOR y CLS no tienen efecto visible.
type PlainConsole struct {
	mu    sync.Mutex
	out   io.Writer
	in    *bufio.Reader
	col   int
	width int
	keys  chan byte
	once  sync.Once
	UTF8  bool // decodifica CP437 a UTF-8 al escribir
	// EchoInput repite en la salida lo leído por INPUT (cuando la entrada
	// no es una terminal, para que la salida quede legible y testeable).
	EchoInput bool
	// justRead: la terminal ya bajó de línea al presionar Enter en INPUT.
	justRead bool
	gfx      *gfx.Screen
	mode     int
}

// NewPlainConsole crea una consola de flujo.
func NewPlainConsole(in io.Reader, out io.Writer) *PlainConsole {
	return &PlainConsole{out: out, in: bufio.NewReader(in), col: 1, width: 80, UTF8: true}
}

func (c *PlainConsole) write(s string) {
	if c.UTF8 {
		io.WriteString(c.out, cp437.Decode([]byte(s)))
		return
	}
	io.WriteString(c.out, s)
}

func (c *PlainConsole) Print(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.justRead = false
	for len(s) > 0 {
		i := strings.IndexAny(s, "\r\n\a")
		chunk := s
		if i >= 0 {
			chunk = s[:i]
		}
		for len(chunk) > 0 {
			room := c.width - c.col + 1
			if room <= 0 {
				c.write("\n")
				c.col = 1
				room = c.width
			}
			n := min(room, len(chunk))
			c.write(chunk[:n])
			c.col += n
			chunk = chunk[n:]
		}
		if i < 0 {
			return
		}
		switch s[i] {
		case '\r', '\n':
			c.write("\n")
			c.col = 1
		case '\a':
			c.write("\a")
		}
		s = s[i+1:]
	}
}

func (c *PlainConsole) Newline() {
	c.mu.Lock()
	if c.justRead {
		c.justRead = false
		c.mu.Unlock()
		return
	}
	c.write("\n")
	c.col = 1
	c.mu.Unlock()
}

func (c *PlainConsole) Cls()                     {}
func (c *PlainConsole) Locate(row, col int)      {}
func (c *PlainConsole) Cursor() (int, int)       { return 1, c.col }
func (c *PlainConsole) SetCursor(bool)           {}
func (c *PlainConsole) Color(fg, bg, border int) {}
func (c *PlainConsole) Size() (int, int) {
	if c.gfx != nil { // en modo gráfico, la grilla de texto del modo
		return c.gfx.M.Cols, c.gfx.M.Rows
	}
	return c.width, 25
}
func (c *PlainConsole) SetWidth(cols, rows int) error {
	if cols > 0 {
		c.width = cols
	}
	return nil
}
func (c *PlainConsole) ViewPrint(top, bottom int) error     { return nil }
func (c *PlainConsole) CharAt(row, col int) (byte, byte)    { return ' ', 7 }
func (c *PlainConsole) PutChar(row, col int, ch, attr byte) {}
func (c *PlainConsole) Beep()                               { c.write("\a") }
func (c *PlainConsole) Flush()                              {}

func (c *PlainConsole) startKeys() {
	c.once.Do(func() {
		c.keys = make(chan byte, 256)
		go func() {
			for {
				b, err := c.in.ReadByte()
				if err != nil {
					close(c.keys)
					return
				}
				if b == '\n' {
					b = '\r'
				}
				c.keys <- b
			}
		}()
	})
}

func (c *PlainConsole) InKey() string {
	c.startKeys()
	select {
	case b, ok := <-c.keys:
		if ok {
			return string([]byte{b})
		}
	default:
	}
	return ""
}

func (c *PlainConsole) ReadKey() (string, error) {
	c.startKeys()
	b, ok := <-c.keys
	if !ok {
		return "", io.EOF
	}
	return string([]byte{b}), nil
}

func (c *PlainConsole) ReadLine() (string, error) {
	if c.keys != nil { // ya se usó INKEY$: leer del canal
		var sb strings.Builder
		for {
			k, err := c.ReadKey()
			if err != nil {
				return sb.String(), err
			}
			if k == "\r" {
				break
			}
			sb.WriteString(k)
		}
		c.mu.Lock()
		if c.EchoInput {
			c.write(sb.String())
		} else {
			c.col, c.justRead = 1, true
		}
		c.mu.Unlock()
		return sb.String(), nil
	}
	line, err := c.in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	c.mu.Lock()
	if c.EchoInput {
		c.write(line)
		c.col += len(line)
	} else {
		c.col = 1
		c.justRead = true
	}
	c.mu.Unlock()
	return string(cp437.ToCP437([]byte(line))), nil
}

// ------------------------------------------------------------- gráficos sin pantalla

// La consola de flujo también admite modos gráficos: dibuja en memoria
// (útil para tests: se puede inspeccionar el framebuffer con Graphics).
func (c *PlainConsole) SetScreen(mode, apage, vpage int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mode == 0 {
		c.gfx, c.mode = nil, 0
		return nil
	}
	if c.gfx == nil || c.mode != mode {
		g, err := gfx.New(mode)
		if err != nil {
			return err
		}
		c.gfx, c.mode = g, mode
	}
	return c.gfx.SetPages(apage, vpage)
}

func (c *PlainConsole) ScreenMode() int { return c.mode }

func (c *PlainConsole) WithGfx(fn func(g *gfx.Screen) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gfx == nil {
		return ErrIllegalCall
	}
	return fn(c.gfx)
}

// Graphics devuelve la pantalla gráfica en memoria (nil en modo texto).
func (c *PlainConsole) Graphics() *gfx.Screen { return c.gfx }
