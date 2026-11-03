//go:build darwin || windows

package sound

import (
	"sync"

	"github.com/ebitengine/oto/v3"

	"goqb/vm"
)

var (
	once   sync.Once
	device vm.SoundDevice
)

// Device devuelve el dispositivo de sonido del proceso (oto admite un solo
// contexto). Si no hay audio disponible, devuelve uno silencioso.
func Device() vm.SoundDevice {
	once.Do(func() {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
			SampleRate: sampleRate, ChannelCount: 1, Format: oto.FormatSignedInt16LE,
		})
		if err != nil {
			device = &vm.SilentSound{}
			return
		}
		<-ready
		s := &Synth{}
		p := ctx.NewPlayer(s)
		p.SetBufferSize(sampleRate / 20 * 2) // ~50 ms de latencia
		p.Play()
		keep = p
		device = s
	})
	return device
}

var keep *oto.Player // evita que el reproductor sea recolectado
