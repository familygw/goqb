// Package cp437 convierte entre la página de códigos 437 (DOS) y Unicode.
//
// Los programas QB guardan texto en CP437: CHR$(219) es █ y los strings
// del programa son bytes. goqb compila siempre desde bytes CP437; los
// archivos fuente en UTF-8 se transcodifican al cargarse.
package cp437

import (
	"unicode/utf8"
)

// Glyphs es el glifo Unicode de cada byte CP437, incluidos los caracteres
// de control 0x01-0x1F, que en la pantalla de texto de DOS se ven como símbolos.
var Glyphs = [256]rune{
	' ', '☺', '☻', '♥', '♦', '♣', '♠', '•', '◘', '○', '◙', '♂', '♀', '♪', '♫', '☼',
	'►', '◄', '↕', '‼', '¶', '§', '▬', '↨', '↑', '↓', '→', '←', '∟', '↔', '▲', '▼',
	' ', '!', '"', '#', '$', '%', '&', '\'', '(', ')', '*', '+', ',', '-', '.', '/',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ':', ';', '<', '=', '>', '?',
	'@', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M', 'N', 'O',
	'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', '[', '\\', ']', '^', '_',
	'`', 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'o',
	'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z', '{', '|', '}', '~', '⌂',
	'Ç', 'ü', 'é', 'â', 'ä', 'à', 'å', 'ç', 'ê', 'ë', 'è', 'ï', 'î', 'ì', 'Ä', 'Å',
	'É', 'æ', 'Æ', 'ô', 'ö', 'ò', 'û', 'ù', 'ÿ', 'Ö', 'Ü', '¢', '£', '¥', '₧', 'ƒ',
	'á', 'í', 'ó', 'ú', 'ñ', 'Ñ', 'ª', 'º', '¿', '⌐', '¬', '½', '¼', '¡', '«', '»',
	'░', '▒', '▓', '│', '┤', '╡', '╢', '╖', '╕', '╣', '║', '╗', '╝', '╜', '╛', '┐',
	'└', '┴', '┬', '├', '─', '┼', '╞', '╟', '╚', '╔', '╩', '╦', '╠', '═', '╬', '╧',
	'╨', '╤', '╥', '╙', '╘', '╒', '╓', '╫', '╪', '┘', '┌', '█', '▄', '▌', '▐', '▀',
	'α', 'ß', 'Γ', 'π', 'Σ', 'σ', 'µ', 'τ', 'Φ', 'Θ', 'Ω', 'δ', '∞', 'φ', 'ε', '∩',
	'≡', '±', '≥', '≤', '⌠', '⌡', '÷', '≈', '°', '∙', '·', '√', 'ⁿ', '²', '■', ' ',
}

var reverse = map[rune]byte{}

func init() {
	for i, r := range Glyphs {
		if i >= 0x20 || i == 0 {
			if _, ok := reverse[r]; !ok {
				reverse[r] = byte(i)
			}
		}
	}
	for i := 1; i < 0x20; i++ { // los glifos de control también se pueden escribir
		if _, ok := reverse[Glyphs[i]]; !ok {
			reverse[Glyphs[i]] = byte(i)
		}
	}
	reverse['\t'], reverse['\n'], reverse['\r'] = '\t', '\n', '\r'
	reverse[' '] = 0xFF
}

// Rune devuelve el glifo para mostrar el byte b.
func Rune(b byte) rune { return Glyphs[b] }

// Byte convierte un rune a CP437; ok es false si no tiene representación.
func Byte(r rune) (byte, bool) {
	if r < 0x80 && r >= 0x20 || r == '\t' || r == '\n' || r == '\r' {
		return byte(r), true
	}
	b, ok := reverse[r]
	return b, ok
}

// Decode convierte bytes CP437 a un string Unicode. Los controles
// (TAB, CR, LF) se conservan como tales.
func Decode(b []byte) string {
	out := make([]rune, 0, len(b))
	for _, c := range b {
		switch c {
		case '\t', '\n', '\r':
			out = append(out, rune(c))
		default:
			out = append(out, Glyphs[c])
		}
	}
	return string(out)
}

// Encode convierte texto Unicode a CP437; lo no representable se reemplaza por '?'.
func Encode(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if b, ok := Byte(r); ok {
			out = append(out, b)
		} else {
			out = append(out, '?')
		}
	}
	return out
}

// IsUTF8Text reporta si b parece texto UTF-8 con caracteres no ASCII
// (y por lo tanto no CP437). Un archivo sólo ASCII es válido en ambas.
func IsUTF8Text(b []byte) bool {
	ascii := true
	for _, c := range b {
		if c >= 0x80 {
			ascii = false
			break
		}
	}
	return !ascii && utf8.Valid(b)
}

// ToCP437 normaliza un fuente a CP437: si está en UTF-8 lo transcodifica.
func ToCP437(b []byte) []byte {
	if IsUTF8Text(b) {
		return Encode(string(b))
	}
	return b
}
