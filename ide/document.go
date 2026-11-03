package ide

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"goqb/lang/cp437"
	"goqb/lang/format"
	"goqb/lang/lexer"
	"goqb/lang/token"
)

// View es una de las vistas del editor: el código de módulo o un procedimiento.
type View struct {
	Name  string // "" = nivel de módulo
	Kind  string // SUB, FUNCTION
	Lines []string

	// estado del editor en esta vista
	Row, Col  int
	Top, Left int
	undo      []snapshot
}

type snapshot struct {
	lines    []string
	row, col int
}

// Document es un módulo .BAS abierto en el IDE.
type Document struct {
	Path  string
	Views []*View
	Dirty bool
	UTF8  bool // el archivo original estaba en UTF-8
	CRLF  bool
	Names format.Names
	rev   int // cambia con cada edición (para saber si hay que recompilar)
}

// NewDocument crea un documento vacío ("Untitled").
func NewDocument() *Document {
	return &Document{Views: []*View{{Lines: []string{""}}}, CRLF: true, Names: format.Names{}}
}

// LoadDocument lee y separa un archivo en vistas.
func LoadDocument(path string) (*Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := ParseDocument(raw)
	d.Path = path
	return d, nil
}

// ParseDocument separa el texto en módulo y procedimientos.
func ParseDocument(raw []byte) *Document {
	d := &Document{Names: format.Names{}}
	d.CRLF = bytes.Contains(raw, []byte("\r\n")) || len(raw) == 0
	if i := bytes.IndexByte(raw, 0x1A); i >= 0 {
		raw = raw[:i]
	}
	var text string
	if cp437.IsUTF8Text(raw) {
		d.UTF8 = true
		text = string(raw)
	} else {
		text = cp437.Decode(raw)
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range lines {
		lines[i] = expandTabs(l)
	}

	module := &View{}
	d.Views = []*View{module}
	var cur *View
	for _, l := range lines {
		d.Names.Learn(l)
		if cur == nil {
			if kind, name := procHeader(l); kind != "" {
				cur = &View{Name: name, Kind: kind}
				d.Views = append(d.Views, cur)
				cur.Lines = append(cur.Lines, l)
				continue
			}
			module.Lines = append(module.Lines, l)
			continue
		}
		cur.Lines = append(cur.Lines, l)
		if isProcEnd(l, cur.Kind) {
			cur = nil
		}
	}
	// quitar líneas vacías finales del módulo que separaban procedimientos
	for len(module.Lines) > 1 && strings.TrimSpace(module.Lines[len(module.Lines)-1]) == "" && len(d.Views) > 1 {
		module.Lines = module.Lines[:len(module.Lines)-1]
	}
	if len(module.Lines) == 0 {
		module.Lines = []string{""}
	}
	return d
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 8 - col%8
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// procHeader reconoce "SUB nombre" / "FUNCTION nombre" (no DECLARE ni END).
func procHeader(line string) (kind, name string) {
	toks := lexer.Tokenize(cp437.Encode(line))
	i := 0
	if i < len(toks) && toks[i].Kind == token.Int && toks[i].LineStart { // número de línea
		i++
	}
	if i+1 < len(toks) && (toks[i].Is("SUB") || toks[i].Is("FUNCTION")) && toks[i+1].Kind == token.Ident {
		return toks[i].Text, cp437.Decode([]byte(toks[i+1].Text))
	}
	return "", ""
}

func isProcEnd(line, kind string) bool {
	toks := lexer.Tokenize(cp437.Encode(line))
	for i := 0; i+1 < len(toks); i++ {
		if toks[i].Is("END") && toks[i+1].Is(kind) && (i == 0 || toks[i-1].Is(":") || toks[i-1].Kind == token.Int) {
			return true
		}
	}
	return false
}

// Title es el nombre que muestra el IDE.
func (d *Document) Title() string {
	if d.Path == "" {
		return "Untitled"
	}
	return strings.ToUpper(filepath.Base(d.Path))
}

// FindView busca un procedimiento por nombre (sin distinguir mayúsculas ni sufijo).
func (d *Document) FindView(name string) int {
	k := strings.ToUpper(strings.TrimRight(name, "%&!#$"))
	for i, v := range d.Views {
		if i > 0 && strings.ToUpper(strings.TrimRight(v.Name, "%&!#$")) == k {
			return i
		}
	}
	return -1
}

// AddProc crea un procedimiento nuevo y devuelve su índice.
func (d *Document) AddProc(kind, name string, header string) int {
	if header == "" {
		header = kind + " " + name
	}
	v := &View{Name: name, Kind: kind, Lines: []string{header, "", "END " + kind}, Row: 1}
	d.Views = append(d.Views, v)
	d.Touch()
	return len(d.Views) - 1
}

// Touch marca el documento como modificado.
func (d *Document) Touch() {
	d.Dirty = true
	d.rev++
}

// Loc es una línea del texto armado: vista y línea (0-based).
type Loc struct {
	View, Line int
}

var declareRe = regexp.MustCompile(`(?i)^\s*DECLARE\s+(SUB|FUNCTION)\s+([A-Za-z][A-Za-z0-9.]*[%&!#$]?)`)

// Assemble arma el texto del módulo: código de módulo, DECLARE generados y
// procedimientos. Devuelve las líneas y su origen.
func (d *Document) Assemble() ([]string, []Loc) {
	var out []string
	var locs []Loc
	module := d.Views[0].Lines
	declared := map[string]bool{}
	for _, l := range module {
		if m := declareRe.FindStringSubmatch(l); m != nil {
			declared[strings.ToUpper(strings.TrimRight(m[2], "%&!#$"))] = true
		}
	}
	var decls []string
	for _, v := range d.Views[1:] {
		k := strings.ToUpper(strings.TrimRight(v.Name, "%&!#$"))
		if declared[k] || len(v.Lines) == 0 {
			continue
		}
		decls = append(decls, declareFor(v.Lines[0]))
	}
	// Los DECLARE van después de los comentarios iniciales del módulo.
	insertAt := 0
	for insertAt < len(module) {
		t := strings.TrimSpace(module[insertAt])
		if t == "" || strings.HasPrefix(t, "'") || strings.HasPrefix(strings.ToUpper(t), "REM") {
			insertAt++
			continue
		}
		break
	}
	if insertAt == len(module) && len(module) > 0 && strings.TrimSpace(module[len(module)-1]) == "" {
		insertAt = 0
	}
	for i, l := range module {
		if i == insertAt {
			for _, dl := range decls {
				out = append(out, dl)
				locs = append(locs, Loc{0, i})
			}
		}
		out = append(out, l)
		locs = append(locs, Loc{0, i})
	}
	if insertAt >= len(module) {
		for _, dl := range decls {
			out = append(out, dl)
			locs = append(locs, Loc{0, len(module) - 1})
		}
	}
	for vi, v := range d.Views[1:] {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
			locs = append(locs, Loc{vi + 1, 0})
		}
		for li, l := range v.Lines {
			out = append(out, l)
			locs = append(locs, Loc{vi + 1, li})
		}
	}
	return out, locs
}

// declareFor genera "DECLARE SUB Nombre (params)" a partir del encabezado.
func declareFor(header string) string {
	toks := lexer.Tokenize(cp437.Encode(header))
	var b strings.Builder
	b.WriteString("DECLARE ")
	start := 0
	if len(toks) > 0 && toks[0].Kind == token.Int {
		start = 1
	}
	depth := 0
	for i := start; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == token.EOF || t.Kind == token.Newline || t.Kind == token.Comment {
			break
		}
		if t.Is("STATIC") && depth == 0 {
			break
		}
		if t.Is("(") {
			depth++
		}
		if t.Is(")") {
			depth--
		}
		if i > start {
			b.WriteByte(' ')
		}
		switch t.Kind {
		case token.String:
			b.WriteByte('"')
			b.WriteString(cp437.Decode([]byte(t.Text)))
			b.WriteByte('"')
		case token.Int, token.Float:
			b.WriteString(t.Text)
			if t.Suffix != 0 {
				b.WriteByte(t.Suffix)
			}
		default:
			b.WriteString(cp437.Decode([]byte(t.Text)))
		}
	}
	return format.Line(b.String(), nil)
}

// Bytes devuelve el archivo listo para guardar.
func (d *Document) Bytes() []byte {
	lines, _ := d.Assemble()
	nl := "\n"
	if d.CRLF {
		nl = "\r\n"
	}
	text := strings.Join(lines, nl) + nl
	if d.UTF8 {
		return []byte(text)
	}
	return cp437.Encode(text)
}

// Source devuelve el programa en CP437 para compilar, con el mapa de líneas.
func (d *Document) Source() ([]byte, []Loc) {
	lines, locs := d.Assemble()
	return cp437.Encode(strings.Join(lines, "\n") + "\n"), locs
}

// Save guarda el documento en path.
func (d *Document) Save(path string) error {
	if filepath.Ext(path) == "" {
		path += ".BAS"
	}
	if err := os.WriteFile(path, d.Bytes(), 0o644); err != nil {
		return err
	}
	d.Path = path
	d.Dirty = false
	return nil
}
