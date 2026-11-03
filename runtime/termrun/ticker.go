package termrun

import "time"

// Frecuencia de refresco de la pantalla (≈30 cuadros por segundo).
const frame = 33 * time.Millisecond

func newTicker() *time.Ticker { return time.NewTicker(frame) }
