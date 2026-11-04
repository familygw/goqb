package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
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
}

// saveConfig guarda las opciones actuales.
func (a *App) saveConfig() {
	p := configPath()
	if p == "" {
		return
	}
	cfg := config{SyntaxCheck: a.syntaxCheck, AutoFormat: a.autoFormat, SyntaxColoring: a.ed.Colorize != nil,
		LineNumbers: a.lineNumbers, IncludePath: a.includePath}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, append(data, '\n'), 0o644)
}
