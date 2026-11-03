// goqb es el entorno de desarrollo QuickBASIC 4.5 para la terminal.
//
//	goqb [archivo.bas]
package main

import (
	"fmt"
	"os"

	"goqb/ide"
)

func main() {
	path := ""
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help", "help":
			fmt.Println("Uso: goqb [archivo.bas]")
			return
		case "-v", "--version", "version":
			fmt.Println("goqb 0.3.0")
			return
		}
		path = os.Args[1]
	}
	app, err := ide.New(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "goqb:", err)
		os.Exit(1)
	}
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "goqb:", err)
		os.Exit(1)
	}
}
