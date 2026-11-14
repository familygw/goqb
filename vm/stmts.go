package vm

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"goqb/gfx"
)

// Statement es una sentencia intrínseca invocada con OpStmt.
type Statement struct {
	Name string
	Fn   func(m *VM, a []Value) error
}

// Statements es la tabla de sentencias intrínsecas.
var Statements []Statement

// StmtIndex mapea nombre → índice en Statements.
var StmtIndex = map[string]int{}

func regStmt(name string, fn func(m *VM, a []Value) error) {
	StmtIndex[name] = len(Statements)
	Statements = append(Statements, Statement{name, fn})
}

func optInt(a []Value, i, def int) (int, error) {
	if i >= len(a) || isNone(a[i]) {
		return def, nil
	}
	return intArg(a[i])
}

// Flags de INPUT.
const (
	InputNoNewline = 1
	InputQuestion  = 2
)

func init() {
	// ---- PRINT / WRITE
	regStmt("PRINT_VAL", func(m *VM, a []Value) error { return m.printValue(a[0]) })
	regStmt("PRINT_COMMA", func(m *VM, a []Value) error { m.printComma(); return nil })
	regStmt("PRINT_NL", func(m *VM, a []Value) error { m.printer().newline(); return nil })
	regStmt("PRINT_TAB", func(m *VM, a []Value) error {
		n, err := intArg(a[0])
		if err != nil {
			return err
		}
		m.printTab(n)
		return nil
	})
	regStmt("PRINT_SPC", func(m *VM, a []Value) error {
		n, err := intArg(a[0])
		if err != nil {
			return err
		}
		m.printSpc(n)
		return nil
	})
	regStmt("PRINT_FILE", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		if q.mode != ModeOutput && q.mode != ModeAppend {
			return ErrBadFileMode
		}
		m.out = q
		return nil
	})
	regStmt("PRINT_LPT", func(m *VM, a []Value) error { m.out = lpt; return nil })
	regStmt("PRINT_DONE", func(m *VM, a []Value) error {
		m.out = nil
		m.Con.Flush()
		return nil
	})
	regStmt("USING_BEGIN", func(m *VM, a []Value) error {
		if a[0].K != KString {
			return ErrTypeMismatch
		}
		return m.usingBegin(a[0].S)
	})
	regStmt("USING_VAL", func(m *VM, a []Value) error { return m.usingValue(a[0]) })
	regStmt("USING_END", func(m *VM, a []Value) error { m.usingEnd(a[0].IsTrue()); return nil })
	regStmt("WRITE_VAL", func(m *VM, a []Value) error { return m.writeValue(a[0], a[1].IsTrue()) })
	regStmt("WRITE_NL", func(m *VM, a []Value) error { m.printer().newline(); return nil })

	// ---- INPUT
	regStmt("INPUT", func(m *VM, a []Value) error {
		flags, _ := intArg(a[0])
		return m.consoleInput(flags&InputNoNewline != 0, flags&InputQuestion != 0, a[1].S, a[2:])
	})
	regStmt("INPUT_FILE", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		return m.fileInput(q, a[1:])
	})
	regStmt("LINE_INPUT", func(m *VM, a []Value) error {
		flags, _ := intArg(a[0])
		m.Con.Print(a[1].S)
		line, err := m.Con.ReadLine()
		if err != nil {
			return m.inputErr(err)
		}
		if flags&InputNoNewline == 0 {
			m.Con.Newline()
		}
		return Assign(a[2].P, Str(line))
	})
	regStmt("LINE_INPUT_FILE", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		s, err := q.readLine()
		if err != nil {
			return err
		}
		return Assign(a[1].P, Str(s))
	})

	// ---- pantalla
	regStmt("CLS", func(m *VM, a []Value) error { m.Con.Cls(); return nil })
	regStmt("COLOR", func(m *VM, a []Value) error {
		v := [3]int{-1, -1, -1}
		for i := 0; i < 3; i++ {
			n, err := optInt(a, i, -1)
			if err != nil {
				return err
			}
			if n != -1 && (n < 0 || n > 255) {
				return ErrIllegalCall
			}
			v[i] = n
		}
		if gc, ok := m.gfxCon(); ok && gc.ScreenMode() != 0 {
			return m.withGfx(func(g *gfx.Screen) error { return g.Color(v[0], v[1]) })
		}
		if v[0] > 31 || v[1] > 15 || v[2] > 15 {
			return ErrIllegalCall
		}
		m.Con.Color(v[0], v[1], v[2])
		return nil
	})
	regStmt("LOCATE", func(m *VM, a []Value) error {
		w, h := m.Con.Size()
		row, err := optInt(a, 0, 0)
		if err != nil {
			return err
		}
		col, err := optInt(a, 1, 0)
		if err != nil {
			return err
		}
		if row < 0 || row > h || col < 0 || col > w {
			return ErrIllegalCall
		}
		// omitidos valen 0 (no cambiar); escritos, tienen que ser al menos 1 (LOCATE 1, 0 es error en QB)
		for i, v := range []int{row, col} {
			if v == 0 && len(a) > i && !isNone(a[i]) {
				return ErrIllegalCall
			}
		}
		m.Con.Locate(row, col)
		if len(a) > 2 && !isNone(a[2]) {
			m.Con.SetCursor(a[2].IsTrue())
		}
		return nil
	})
	regStmt("WIDTH", func(m *VM, a []Value) error {
		cols, err := optInt(a, 0, 0)
		if err != nil {
			return err
		}
		rows, err := optInt(a, 1, 0)
		if err != nil {
			return err
		}
		if cols != 0 && cols != 40 && cols != 80 || rows != 0 && rows != 25 && rows != 43 && rows != 50 {
			return ErrIllegalCall
		}
		return m.Con.SetWidth(cols, rows)
	})
	regStmt("WIDTH_FILE", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		w, err := intArg(a[1])
		if err != nil || w < 0 || w > 255 {
			return ErrIllegalCall
		}
		q.wid = w
		return nil
	})
	regStmt("VIEW_PRINT", func(m *VM, a []Value) error {
		top, err := optInt(a, 0, 0)
		if err != nil {
			return err
		}
		bottom, err := optInt(a, 1, 0)
		if err != nil {
			return err
		}
		return m.Con.ViewPrint(top, bottom)
	})
	regStmt("SCREEN", func(m *VM, a []Value) error { // modo, colorswitch, apage, vpage
		gc, ok := m.gfxCon()
		cur := 0
		if ok {
			cur = gc.ScreenMode()
		}
		mode, err := optInt(a, 0, cur)
		if err != nil {
			return err
		}
		apage, err := optInt(a, 2, 0)
		if err != nil {
			return err
		}
		vpage, err := optInt(a, 3, apage)
		if err != nil {
			return err
		}
		if !ok {
			if mode != 0 {
				return ErrAdvancedFeature
			}
			return nil
		}
		if err := gc.SetScreen(mode, apage, vpage); err != nil {
			return ErrIllegalCall
		}
		return nil
	})
	regStmt("NOP", func(m *VM, a []Value) error { return nil }) // KEY ON/OFF, TRON...: sin efecto en la terminal
	regStmt("SLEEP", func(m *VM, a []Value) error {
		secs, err := optInt(a, 0, 0)
		if err != nil {
			return err
		}
		deadline := time.Now().Add(time.Duration(secs) * time.Second)
		m.Con.Flush()
		for secs <= 0 || time.Now().Before(deadline) {
			if m.stop.Load() {
				return errStopped
			}
			if m.Con.InKey() != "" {
				return nil
			}
			time.Sleep(15 * time.Millisecond)
		}
		return nil
	})
	regStmt("RANDOMIZE", func(m *VM, a []Value) error {
		var x float64
		if len(a) == 0 || isNone(a[0]) {
			for {
				m.Con.Print("Random-number seed (-32768 to 32767)? ")
				line, err := m.Con.ReadLine()
				if err != nil {
					return m.inputErr(err)
				}
				m.Con.Newline()
				f, ok := ParseInputNumber(line)
				if ok && f >= -32768 && f <= 32767 {
					x = f
					break
				}
			}
		} else {
			x = a[0].Num()
		}
		bits := math.Float64bits(x)
		w := uint32(bits>>48) ^ uint32(bits>>32&0xFFFF)
		m.seed = (m.seed&0xFF | w<<8) & 0xFFFFFF
		return nil
	})

	// ---- archivos
	regStmt("OPEN", func(m *VM, a []Value) error { // archivo, modo, número, largo
		mode, _ := intArg(a[1])
		reclen, err := optInt(a, 3, 0)
		if err != nil {
			return err
		}
		if reclen < 0 || reclen > 32767 {
			return ErrBadRecordLen
		}
		return m.open(a[0].S, mode, a[2], reclen)
	})
	regStmt("OPEN_LEGACY", func(m *VM, a []Value) error { // modo$, número, archivo, largo
		mode := 0
		if a[0].S != "" {
			switch a[0].S[0] | 0x20 {
			case 'i':
				mode = ModeInput
			case 'o':
				mode = ModeOutput
			case 'a':
				mode = ModeAppend
			case 'r':
				mode = ModeRandom
			case 'b':
				mode = ModeBinary
			}
		}
		if mode == 0 {
			return ErrBadFileMode
		}
		reclen, err := optInt(a, 3, 0)
		if err != nil {
			return err
		}
		return m.open(a[2].S, mode, a[1], reclen)
	})
	regStmt("CLOSE", func(m *VM, a []Value) error {
		if len(a) == 0 {
			m.closeAll()
			return nil
		}
		for _, v := range a {
			if isNone(v) {
				continue
			}
			n, err := fileNum(v)
			if err != nil {
				return err
			}
			m.closeFile(n)
		}
		return nil
	})
	regStmt("RESET", func(m *VM, a []Value) error { m.closeAll(); return nil })
	regStmt("FIELD", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		if q.mode != ModeRandom {
			return ErrBadFileMode
		}
		total := 0
		q.fields = q.fields[:0]
		for i := 1; i+1 < len(a); i += 2 {
			w, err := intArg(a[i])
			if err != nil || w < 0 {
				return ErrIllegalCall
			}
			total += w
			if total > q.reclen {
				return ErrFieldOverflow
			}
			ref := a[i+1].P.Deref()
			if ref.K != KString {
				return ErrTypeMismatch
			}
			ref.S = strings.Repeat(" ", w)
			q.fields = append(q.fields, fieldVar{ref, w})
		}
		return nil
	})
	getput := func(put bool) func(m *VM, a []Value) error {
		return func(m *VM, a []Value) error {
			q, err := m.file(a[0])
			if err != nil {
				return err
			}
			var ref *Value
			if !isNone(a[2]) {
				ref = a[2].P
			}
			return m.getPut(put, q, a[1], !isNone(a[1]), ref)
		}
	}
	regStmt("GET", getput(false))
	regStmt("PUT", getput(true))
	regStmt("SEEK", func(m *VM, a []Value) error {
		q, err := m.file(a[0])
		if err != nil {
			return err
		}
		p, err := ToLong(a[1])
		if err != nil {
			return err
		}
		if p.I < 1 {
			return ErrBadRecordNum
		}
		switch q.mode {
		case ModeRandom:
			q.recno = p.I - 1
			q.pos = (p.I - 1) * int64(q.reclen)
		case ModeBinary:
			q.pos = p.I - 1
		default:
			if q.w != nil {
				q.w.Flush()
			}
			q.f.Seek(p.I-1, 0)
			if q.r != nil {
				q.r.Reset(q.f)
				q.nread = p.I - 1
			}
		}
		return nil
	})
	regStmt("LSET", func(m *VM, a []Value) error { return setStr(a[0].P, a[1], false) })
	regStmt("RSET", func(m *VM, a []Value) error { return setStr(a[0].P, a[1], true) })
	regStmt("MID_ASSIGN", func(m *VM, a []Value) error { // ref, inicio, largo|none, valor
		dst := a[0].P.Deref()
		if dst.K != KString || a[3].K != KString {
			return ErrTypeMismatch
		}
		st, err := intArg(a[1])
		if err != nil || st < 1 || st > len(dst.S) {
			return ErrIllegalCall
		}
		n := len(a[3].S)
		if !isNone(a[2]) {
			k, err := intArg(a[2])
			if err != nil || k < 0 {
				return ErrIllegalCall
			}
			n = min(n, k)
		}
		n = min(n, len(dst.S)-st+1)
		b := []byte(dst.S)
		copy(b[st-1:st-1+n], a[3].S[:n])
		dst.S = string(b)
		return nil
	})
	regStmt("KILL", func(m *VM, a []Value) error {
		matches, _ := filepath.Glob(a[0].S)
		if len(matches) == 0 {
			return ErrFileNotFound
		}
		for _, f := range matches {
			if err := os.Remove(f); err != nil {
				return osErr(err)
			}
		}
		return nil
	})
	regStmt("NAME", func(m *VM, a []Value) error {
		if _, err := os.Stat(a[1].S); err == nil {
			return ErrFileExists
		}
		if err := os.Rename(a[0].S, a[1].S); err != nil {
			return osErr(err)
		}
		return nil
	})
	regStmt("MKDIR", func(m *VM, a []Value) error { return pathErr(os.Mkdir(a[0].S, 0o755)) })
	regStmt("RMDIR", func(m *VM, a []Value) error { return pathErr(os.Remove(a[0].S)) })
	regStmt("CHDIR", func(m *VM, a []Value) error { return pathErr(os.Chdir(a[0].S)) })
	regStmt("FILES", func(m *VM, a []Value) error {
		pat := "*"
		if len(a) > 0 && !isNone(a[0]) {
			pat = a[0].S
		}
		matches, _ := filepath.Glob(pat)
		if len(matches) == 0 {
			return ErrFileNotFound
		}
		sort.Strings(matches)
		wd, _ := os.Getwd()
		m.Con.Print(wd)
		m.Con.Newline()
		w, _ := m.Con.Size()
		col := 0
		for _, f := range matches {
			name := padRight(filepath.Base(f), 18)
			if col+18 > w {
				m.Con.Newline()
				col = 0
			}
			m.Con.Print(name)
			col += 18
		}
		m.Con.Newline()
		return nil
	})
	regStmt("SHELL", func(m *VM, a []Value) error {
		if len(a) == 0 || isNone(a[0]) || a[0].S == "" {
			return ErrAdvancedFeature
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", a[0].S)
		} else {
			cmd = exec.Command("sh", "-c", a[0].S)
		}
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		cmd.Run()
		for _, line := range strings.SplitAfter(out.String(), "\n") {
			if line == "" {
				continue
			}
			m.Con.Print(strings.TrimRight(line, "\r\n"))
			if strings.HasSuffix(line, "\n") {
				m.Con.Newline()
			}
		}
		return nil
	})
	regStmt("ENVIRON", func(m *VM, a []Value) error {
		k, v, ok := strings.Cut(a[0].S, "=")
		if !ok {
			return ErrIllegalCall
		}
		os.Setenv(strings.TrimSpace(strings.ToUpper(k)), v)
		return nil
	})
	regStmt("CLEAR", func(m *VM, a []Value) error {
		m.closeAll()
		for i, g := range m.Prog.Globals {
			m.Globals[i] = g.Clone()
		}
		m.handler, m.dataPtr = -1, 0
		return nil
	})
	regStmt("RUN", func(m *VM, a []Value) error {
		if len(a) > 0 && !isNone(a[0]) {
			if a[0].K == KString {
				return m.chain(a[0].S, false)
			}
			pc, _ := intArg(a[0])
			m.Reset()
			m.started = true
			m.pc = pc
			return nil
		}
		m.Reset()
		m.started = true
		return nil
	})
}

func setStr(ref *Value, v Value, right bool) error {
	dst := ref.Deref()
	if dst.K != KString || v.K != KString {
		return ErrTypeMismatch
	}
	n := len(dst.S)
	if dst.I > 0 {
		n = int(dst.I)
	}
	if right {
		dst.S = padLeft(v.S, n)
	} else {
		dst.S = padRight(v.S, n)
	}
	return nil
}

func pathErr(err error) error {
	if err == nil {
		return nil
	}
	if os.IsNotExist(err) {
		return ErrPathNotFound
	}
	return ErrPathAccess
}
