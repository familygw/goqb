package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"goqb/compiler"
	"goqb/lang/check"
	"goqb/lang/source"
	"goqb/runtime/bundle"
	"goqb/runtime/launch"
	"goqb/runtime/loader"
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
	vm.DefaultLoader = loader.New(inc, filepath.Dir(fs.Arg(0)))
	command := strings.Join(fs.Args()[1:], " ")
	return launch.Run(prog, command, launch.Options{Plain: *plain, NoWait: *noWait, Window: *win, Title: "goqb - " + filepath.Base(fs.Arg(0))})
}

// buildCmd genera un ejecutable independiente: goqbc build [-o salida] [-target os/arch] programa.bas
func buildCmd(args []string) int {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	var inc multiFlag
	fs.Var(&inc, "I", "directorio adicional para $INCLUDE (repetible)")
	out := fs.String("o", "", "ejecutable de salida (por defecto, el nombre del programa)")
	target := fs.String("target", "", "sistema/arquitectura de destino, por ejemplo windows/amd64")
	stub := fs.String("stub", "", "runtime a usar como base (por defecto, goqbrt-os-arch)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
		return 2
	}
	t, err := bundle.ParseTarget(*target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goqbc:", err)
		return 2
	}
	prog, ok := build(fs.Arg(0), inc)
	if !ok {
		return 1
	}
	if *stub == "" {
		if *stub, err = bundle.FindStub(t); err != nil {
			fmt.Fprintln(os.Stderr, "goqbc:", err)
			return 1
		}
	}
	dst := *out
	if dst == "" {
		base := filepath.Base(fs.Arg(0))
		dst = strings.TrimSuffix(base, filepath.Ext(base))
	}
	dst = t.ExeName(dst)
	if err := bundle.Write(*stub, dst, prog); err != nil {
		fmt.Fprintln(os.Stderr, "goqbc:", err)
		return 1
	}
	fmt.Printf("%s (%s)\n", dst, t)
	return 0
}
