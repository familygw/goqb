package termrun

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Relación alto/ancho de las celdas de la terminal, para que los modos
// gráficos se vean en 4:3 sin deformarse. Orden de prioridad:
//  1. CellAspectOverride (Options → Graphics Aspect del IDE) o GOQB_CELL_ASPECT
//  2. el tamaño en píxeles que informa la terminal (TIOCGWINSZ)
//  3. la respuesta a las consultas CSI 16t / 14t (ProbeCellAspect)
//  4. 2.0, lo habitual en las fuentes monoespaciadas

// CellAspectOverride fuerza la relación alto/ancho de la celda (0 = automática).
var CellAspectOverride float64

var (
	probeMu     sync.Mutex
	probed      bool
	probeAspect float64
)

func envAspect() float64 {
	if s := os.Getenv("GOQB_CELL_ASPECT"); s != "" {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && v >= 0.5 && v <= 4 {
			return v
		}
	}
	return 0
}

// ProbeCellAspect pregunta a la terminal el tamaño de la celda en píxeles.
// Hay que llamarla antes de iniciar tcell (después, tcell lee la entrada).
// Es segura de llamar varias veces: consulta una sola vez.
func ProbeCellAspect() float64 {
	probeMu.Lock()
	defer probeMu.Unlock()
	if !probed {
		probed = true
		if envAspect() == 0 && CellAspectOverride == 0 {
			probeAspect = probeTerminal()
		}
	}
	return probeAspect
}

func probedAspect() float64 {
	probeMu.Lock()
	defer probeMu.Unlock()
	return probeAspect
}

// parseReports interpreta las respuestas CSI 6;h;w t (celda), CSI 4;h;w t
// (área de texto) y CSI 8;filas;cols t, y devuelve alto/ancho de la celda.
func parseReports(s string) float64 {
	var cellH, cellW, areaH, areaW, rows, cols int
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			break
		}
		s = s[i+2:]
		j := strings.IndexAny(s, "tc\x1b")
		if j < 0 {
			break
		}
		if s[j] != 't' {
			s = s[j:]
			continue
		}
		f := strings.Split(s[:j], ";")
		s = s[j+1:]
		if len(f) != 3 {
			continue
		}
		a, _ := strconv.Atoi(f[1])
		b, _ := strconv.Atoi(f[2])
		switch f[0] {
		case "6":
			cellH, cellW = a, b
		case "4":
			areaH, areaW = a, b
		case "8":
			rows, cols = a, b
		}
	}
	if cellH > 0 && cellW > 0 {
		return float64(cellH) / float64(cellW)
	}
	if areaH > 0 && areaW > 0 && rows > 0 && cols > 0 {
		return (float64(areaH) / float64(rows)) / (float64(areaW) / float64(cols))
	}
	return 0
}

func validAspect(v float64) bool { return v >= 0.5 && v <= 4 }
