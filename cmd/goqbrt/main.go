// goqbrt es el runtime de los ejecutables generados con Make EXE: un
// binario sin IDE ni compilador al que se le anexa el programa compilado.
//
//	goqbc build -o juego programa.bas   (usa goqbrt como base)
package main

import (
	"fmt"
	"os"

	"goqb/runtime/launch"
)

func main() {
	if code, ok := launch.Embedded(); ok {
		os.Exit(code)
	}
	fmt.Fprintln(os.Stderr, "goqbrt: este runtime no tiene un programa anexado; generá uno con goqbc build")
	os.Exit(2)
}
