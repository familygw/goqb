//go:build window

package window

import "goqb/lang/cp437"

func cp437Byte(r rune) (byte, bool) { return cp437.Byte(r) }
