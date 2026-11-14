package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"goqb/runtime/termrun"
)

// config son las opciones del IDE que se guardan entre sesiones, en
// $GOQB_CONFIG o en <directorio de configuración>/goqb/config.json
// (~/.config/goqb en Linux, ~/Library/Application Support/goqb en macOS).
type config struct {
	SyntaxCheck    bool     `json:"syntax_check"`
	AutoFormat     bool     `json:"auto_format"`
	SyntaxColoring bool     `json:"syntax_coloring"`
	LineNumbers    bool     `json:"line_numbers"`
	IncludePath    []string `json:"include_path,omitempty"` // directorios para $INCLUDE (por ejemplo, el de QB.BI)
	CellAspect     float64  `json:"cell_aspect,omitempty"`  // alto/ancho de la celda para los gráficos (0: automático)
	TabSize        int      `json:"tab_size,omitempty"`     // tabulaciones del editor (por defecto 8)
	FullView       bool     `json:"full_view,omitempty"`    // todo el programa en una sola vista
}

func configPath() string {
	if p := os.Getenv("GOQB_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "goqb", "config.json")
}

// loadConfig aplica la configuración guardada (si existe).
func (a *App) loadConfig() {
	cfg := config{SyntaxCheck: true, AutoFormat: true, SyntaxColoring: true, LineNumbers: true}
	if p := configPath(); p != "" {
		if data, err := os.ReadFile(p); err == nil {
			json.Unmarshal(data, &cfg)
		}
	}
	a.syntaxCheck, a.autoFormat, a.lineNumbers = cfg.SyntaxCheck, cfg.AutoFormat, cfg.LineNumbers
	a.ed.Colorize = nil
	if cfg.SyntaxColoring {
		a.ed.Colorize = highlight
	}
	a.includePath = cfg.IncludePath
	a.setTabSize(cfg.TabSize)
	a.fullView = cfg.FullView
	if cfg.CellAspect >= 0.5 && cfg.CellAspect <= 4 {
		termrun.CellAspectOverride = cfg.CellAspect
	}
}

// saveConfig guarda las opciones actuales.
func (a *App) saveConfig() {
	p := configPath()
	if p == "" {
		return
	}
	cfg := config{SyntaxCheck: a.syntaxCheck, AutoFormat: a.autoFormat, SyntaxColoring: a.ed.Colorize != nil,
		LineNumbers: a.lineNumbers, IncludePath: a.includePath, CellAspect: termrun.CellAspectOverride, TabSize: a.ed.TabSize, FullView: a.fullView}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, append(data, '\n'), 0o644)
}

// setTabSize aplica el tamaño de tabulación (1-16; fuera de rango, 8).
func (a *App) setTabSize(n int) {
	if n < 1 || n > 16 {
		n = 8
	}
	a.ed.TabSize, a.imEd.TabSize = n, n
}

// tabDialog es Options → Tab Stops.
func (a *App) tabDialog() {
	in := &TextInput{Label: "Tab Stops (1-16):", Width: 6}
	in.set(strconv.Itoa(a.ed.TabSize))
	a.pushDialog(&Dialog{Title: "Tab Stops", Input: in, Buttons: []string{"OK", "Cancel"},
		OnClose: func(d *Dialog, b int) bool {
			if b != 0 {
				return true
			}
			n, err := strconv.Atoi(strings.TrimSpace(d.InputText()))
			if err != nil || n < 1 || n > 16 {
				a.message("", "Valor inválido: usá un número entre 1 y 16.")
				return true
			}
			a.setTabSize(n)
			a.saveConfig()
			return true
		}})
}
