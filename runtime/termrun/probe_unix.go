//go:build darwin || linux || freebsd || netbsd || openbsd

package termrun

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// probeTerminal envía las consultas de tamaño y un DA1 (que responden todas
// las terminales) y lee las respuestas en modo crudo, con un tope de tiempo.
func probeTerminal() float64 {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0
	}
	defer tty.Close()
	fd := int(tty.Fd())
	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return 0
	}
	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	raw.Cc[unix.VMIN] = 0
	raw.Cc[unix.VTIME] = 1 // lecturas con espera máxima de 0,1 s
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return 0
	}
	defer unix.IoctlSetTermios(fd, ioctlSetTermios, old)

	if _, err := tty.WriteString("\x1b[16t\x1b[14t\x1b[18t\x1b[c"); err != nil {
		return 0
	}
	var b strings.Builder
	buf := make([]byte, 256)
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		n, _ := tty.Read(buf)
		if n > 0 {
			b.Write(buf[:n])
			if s := b.String(); strings.Contains(s, "\x1b[?") && strings.HasSuffix(s, "c") {
				break // llegó la respuesta al DA1: no hay más
			}
		}
	}
	if v := parseReports(b.String()); validAspect(v) {
		return v
	}
	return 0
}
