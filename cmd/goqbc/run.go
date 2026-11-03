package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"goqb/compiler"
	"goqb/lang/check"
	"goqb/lang/source"
	"goqb/runtime/sound"
	"goqb/runtime/termrun"
	"goqb/runtime/window"
	"goqb/vm"
)

// build carga, verifica y compila un programa.
func build(path string, inc []string) (*vm.Program, bool) {
	ld := &source.Loader{IncludePath: inc}
	prog, diags := ld.Load(path)
	if len(diags) == 0 {
		diags = check.Program(prog)
	}
	if len(diags) == 0 {
		var p *vm.Program
		p, _, diags = compiler.Compile(prog)
		if len(diags) == 0 {
			return p, true
		}
	}
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d)
	}
	return nil, false
}

func runCmd(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	var inc multiFlag
	fs.Var(&inc, "I", "directorio adicional para $INCLUDE (repetible)")
	plain := fs.Bool("plain", false, "salida de flujo (sin pantalla completa)")
	noWait := fs.Bool("nowait", false, "no esperar una tecla al terminar (modo pantalla)")
	win := fs.Bool("window", false, "ejecutar en una ventana nativa (requiere -tags window)")
	fs.Parse(args)
	if fs.NArg() == 0 {
		usage()
		return 2
	}
	prog, ok := build(fs.Arg(0), inc)
	if !ok {
		return 1
	}
	command := strings.Join(fs.Args()[1:], " ")

	tty := term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stdin.Fd()))
	var err error
	if *win {
		err = window.Run(prog, command, "goqb - "+filepath.Base(fs.Arg(0)))
	} else if *plain || !tty {
		con := vm.NewPlainConsole(os.Stdin, os.Stdout)
		con.EchoInput = !term.IsTerminal(int(os.Stdin.Fd()))
		m := vm.New(prog, con)
		m.Command = command
		m.Sound = sound.Device()
		err = m.Run()
	} else {
		err = termrun.Run(prog, command, !*noWait)
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
