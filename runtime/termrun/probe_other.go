//go:build !(darwin || linux || freebsd || netbsd || openbsd)

package termrun

func probeTerminal() float64 { return 0 }
