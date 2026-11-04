// Package launch ejecuta un programa compilado eligiendo la salida
// (pantalla completa, flujo o ventana). Lo usan goqbc run y los
// ejecutables generados con Make EXE.
package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"goqb/runtime/bundle"
	"goqb/runtime/loader"
	"goqb/runtime/sound"
	"goqb/runtime/termrun"
	"goqb/runtime/window"
	"goqb/vm"
)

// Options configura la ejecución.
type Options struct {
	Plain  bool   // salida de flujo aunque haya terminal
	NoWait bool   // no esperar una tecla al terminar (pantalla completa)
	Window bool   // ventana nativa (build con -tags window)
	Title  string // título de la ventana
}

// Run ejecuta prog y devuelve el código de salida del proceso.
func Run(prog *vm.Program, command string, o Options) int {
	tty := term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
	var err error
	switch {
	case o.Window:
		err = window.Run(prog, command, o.Title)
	case o.Plain || !tty:
		con := vm.NewPlainConsole(os.Stdin, os.Stdout)
		con.EchoInput = !term.IsTerminal(int(os.Stdin.Fd()))
		m := vm.New(prog, con)
		m.Command = command
		m.Sound = sound.Device()
		err = m.Run()
	default:
		err = termrun.Run(prog, command, !o.NoWait)
	}
	if err != nil {
		var re *vm.RuntimeError
		if errors.As(err, &re) && errors.Is(re.Err, vm.ErrBreak) {
			fmt.Fprintf(os.Stderr, "%s:%d: Break\n", re.File, re.Line)
			return 130
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Embedded ejecuta el programa anexado al ejecutable actual (Make EXE), si
// lo hay. Los argumentos de la línea de comandos forman COMMAND$.
// Variables: GOQB_PLAIN=1 fuerza la salida de flujo, GOQB_NOWAIT=1 no espera
// una tecla al terminar y GOQB_WINDOW=1 abre una ventana.
func Embedded() (int, bool) {
	prog, ok := bundle.Self()
	if !ok {
		return 0, false
	}
	dir := ""
	if exe, err := os.Executable(); err == nil {
		dir = filepath.Dir(exe)
	}
	vm.DefaultLoader = loader.New(nil, dir)
	name := "goqb"
	if len(prog.Files) > 0 {
		name = strings.ToUpper(filepath.Base(prog.Files[0]))
	}
	o := Options{
		Plain:  os.Getenv("GOQB_PLAIN") == "1",
		NoWait: os.Getenv("GOQB_NOWAIT") == "1",
		Window: os.Getenv("GOQB_WINDOW") == "1",
		Title:  name,
	}
	return Run(prog, strings.Join(os.Args[1:], " "), o), true
}
