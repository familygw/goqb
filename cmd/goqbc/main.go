// goqbc es la herramienta de línea de comandos de goqb.
//
//	goqbc check  [-I dir] archivo.bas|proyecto.mak ...   verifica sintaxis y estructura
//	goqbc tokens archivo.bas                             muestra los tokens
//	goqbc ast    archivo.bas                             muestra el árbol sintáctico
//	goqbc run    archivo.bas                             ejecuta
//	goqbc build  [-o salida] archivo.bas                 genera un ejecutable (Make EXE)
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"goqb/lang/ast"
	"goqb/lang/check"
	"goqb/lang/lexer"
	"goqb/lang/parser"
	"goqb/lang/source"
	"goqb/lang/token"
	"goqb/runtime/launch"
)

const version = "0.3.0"

func usage() {
	fmt.Fprintf(os.Stderr, `goqbc %s — compilador QuickBASIC 4.5

Uso:
  goqbc check  [-I dir] archivo.bas|proyecto.mak ...
  goqbc run    [-I dir] [-plain] [-nowait] archivo.bas|proyecto.mak [COMMAND$]
  goqbc build  [-I dir] [-o salida] [-target os/arch] [-stub runtime] archivo.bas|proyecto.mak
  goqbc tokens archivo.bas
  goqbc ast    archivo.bas
  goqbc version
`, version)
}

func main() {
	if code, ok := launch.Embedded(); ok { // ejecutable generado con build
		os.Exit(code)
	}
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "check":
		os.Exit(runCheck(args))
	case "run":
		os.Exit(runCmd(args))
	case "build":
		os.Exit(buildCmd(args))
	case "tokens":
		os.Exit(withFile(args, func(name string, src []byte) int {
			for _, t := range lexer.Tokenize(src) {
				fmt.Printf("%-7s %s\n", t.Pos, t)
				if t.Kind == token.EOF {
					break
				}
			}
			return 0
		}))
	case "ast":
		os.Exit(withFile(args, func(name string, src []byte) int {
			f, errs := parser.ParseFile(name, src)
			fmt.Println(ast.Dump(f))
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "%s:%s\n", name, e)
			}
			if len(errs) > 0 {
				return 1
			}
			return 0
		}))
	case "version", "-v", "--version":
		fmt.Println("goqbc", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "goqbc: comando desconocido %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func runCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	var inc multiFlag
	fs.Var(&inc, "I", "directorio adicional para $INCLUDE (repetible)")
	quiet := fs.Bool("q", false, "no mostrar el resumen")
	fs.Parse(args)
	if fs.NArg() == 0 {
		usage()
		return 2
	}
	total := 0
	for _, path := range fs.Args() {
		ld := &source.Loader{IncludePath: inc}
		prog, diags := ld.Load(path)
		diags = append(diags, check.Program(prog)...)
		order := map[string]int{} // orden de aparición de cada archivo
		for _, d := range diags {
			if _, ok := order[d.File]; !ok {
				order[d.File] = len(order)
			}
		}
		sort.SliceStable(diags, func(i, j int) bool {
			a, b := diags[i], diags[j]
			if a.File != b.File {
				return order[a.File] < order[b.File]
			}
			if a.Pos.Line != b.Pos.Line {
				return a.Pos.Line < b.Pos.Line
			}
			return a.Pos.Col < b.Pos.Col
		})
		for _, d := range diags {
			fmt.Println(d)
		}
		total += len(diags)
	}
	if !*quiet {
		if total == 0 {
			fmt.Fprintln(os.Stderr, "OK: sin errores")
		} else {
			fmt.Fprintf(os.Stderr, "%d error(es)\n", total)
		}
	}
	if total > 0 {
		return 1
	}
	return 0
}

func withFile(args []string, fn func(string, []byte) int) int {
	if len(args) != 1 {
		usage()
		return 2
	}
	src, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "goqbc:", err)
		return 1
	}
	return fn(args[0], src)
}
