//go:build darwin || windows || (linux && alsa)

package sound

import (
	"sync"
	"time"

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
			// En macOS oto divide este búfer en cuatro bloques: 20 ms cada
			// uno. Deben caber completos en el búfer de 50 ms del player.
			// El valor por defecto pide 3072 muestras mono (~70 ms), lo
			// que agotaba el player en cada bloque y añadía silencio.
			BufferSize: 80 * time.Millisecond,
		})
		if err != nil {
			device = &vm.SilentSound{}
			return
		}
		<-ready
		s := &Synth{}
		p := ctx.NewPlayer(s)
		p.SetBufferSize(sampleRate / 20 * 2) // 50 ms de PCM int16 mono; margen para reponer cada bloque
		p.Play()
		keep = p
		device = s
	})
	return device
}

var keep *oto.Player // evita que el reproductor sea recolectado
