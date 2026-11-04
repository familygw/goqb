//go:build !darwin && !windows && !(linux && alsa)

package sound

import "goqb/vm"

var device vm.SoundDevice = &vm.SilentSound{}

// Device devuelve un dispositivo silencioso que respeta los tiempos
// (en Linux el audio con oto requiere cgo y ALSA; queda para más adelante).
func Device() vm.SoundDevice { return device }
