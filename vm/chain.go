package vm

import (
	"errors"
)

// errRestart indica que el programa cambió (CHAIN, RUN "archivo") y hay que
// volver a entrar en el bucle de ejecución con el código nuevo.
var errRestart = errors.New("restart")

// Loader carga el programa de CHAIN o RUN "archivo".
type Loader func(name string) (*Program, error)

// DefaultLoader es el Loader que reciben las VMs creadas con New.
var DefaultLoader Loader

// chain reemplaza el programa en ejecución. Con CHAIN (keep) se conservan los
// archivos abiertos y las variables del COMMON sin nombre (por posición).
func (m *VM) chain(name string, keep bool) error {
	if m.Loader == nil {
		return ErrAdvancedFeature
	}
	p, err := m.Loader(name)
	if err != nil {
		var qe *Error
		if errors.As(err, &qe) {
			return qe
		}
		return err
	}
	var common []Value
	files := m.files
	if keep {
		for _, s := range m.Prog.CommonSlots {
			common = append(common, m.Globals[s])
		}
		m.files = map[int]*qfile{} // que Reset no los cierre
	}
	m.Prog = p
	m.Reset()
	if keep {
		m.files = files
		for i, s := range p.CommonSlots {
			if i < len(common) && s < len(m.Globals) && common[i].K == m.Globals[s].K {
				m.Globals[s] = common[i]
			}
		}
	}
	if d := m.Debug; d != nil {
		d.Breaks = map[int]bool{}
		d.Watchpoints = nil
		d.Mode, d.skipPC, d.History = DebugRun, -1, nil
	}
	m.started = true
	if m.OnChain != nil {
		m.OnChain(p)
	}
	return errRestart
}

func init() {
	regStmt("CHAIN", func(m *VM, a []Value) error {
		if a[0].K != KString {
			return ErrTypeMismatch
		}
		return m.chain(a[0].S, true)
	})
}
