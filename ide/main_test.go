package ide

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain aísla los tests de la configuración real del usuario.
func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "goqb-test")
	os.Setenv("GOQB_CONFIG", filepath.Join(dir, "config.json"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
