//go:build !window

// Package window ejecuta un programa en una ventana nativa con ebiten.
// Esta compilación no lo incluye: usar go build -tags window.
package window

import (
	"errors"

	"goqb/vm"
)

// Available reporta si el backend de ventana fue compilado.
const Available = false

// Run no está disponible sin el build tag "window".
func Run(prog *vm.Program, command string, title string) error {
	return errors.New("goqb se compiló sin soporte de ventana: go build -tags window ./cmd/goqbc")
}
