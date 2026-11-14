package vm

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
)

// Formato serializable de un Program. Los tipos de registro se guardan en
// una tabla y los valores los referencian por índice: así, al cargar, todos
// los registros de un mismo TYPE comparten el mismo *RecordType (la VM
// compara tipos por identidad).

type sValue struct {
	K      Kind
	I      int64
	F      float64
	S      string
	Rec    int // índice+1 en la tabla de tipos (0: ninguno)
	Fields []sValue
}

type sField struct {
	Name  string
	Proto sValue
	Size  int
}

type sRecType struct {
	Name   string
	Fields []sField
	Size   int
}

type sProc struct {
	Name                    string
	Entry, NParams, RetSlot int
	Locals                  []sValue
	IsFunc                  bool
	File, Line, EndLine     int
	Start, End              int
}

type sProgram struct {
	Version     int
	Code        []Instr
	Consts      []sValue
	Globals     []sValue
	Procs       []sProc
	Tables      [][]int
	Data        []DataItem
	Stmts       []StmtInfo
	Files       []string
	Entry       int
	GlobalNames map[string]int
	CommonSlots []int
	Types       []sRecType
}

const serialVersion = 1

type encoder struct {
	types map[*RecordType]int
	list  []*RecordType
}

func (e *encoder) typeIndex(t *RecordType) int {
	if t == nil {
		return 0
	}
	if i, ok := e.types[t]; ok {
		return i + 1
	}
	i := len(e.list)
	e.types[t] = i
	e.list = append(e.list, t)
	for _, f := range t.Fields { // registrar tipos anidados
		e.value(f.Proto)
	}
	return i + 1
}

func (e *encoder) value(v Value) sValue {
	s := sValue{K: v.K, I: v.I, F: v.F, S: v.S}
	if v.K == KRecord && v.R != nil {
		s.Rec = e.typeIndex(v.R.T)
		for _, f := range v.R.F {
			s.Fields = append(s.Fields, e.value(f))
		}
	}
	return s
}

func (e *encoder) values(vs []Value) []sValue {
	out := make([]sValue, len(vs))
	for i, v := range vs {
		out[i] = e.value(v)
	}
	return out
}

// Marshal serializa un programa compilado (comprimido).
func Marshal(p *Program) ([]byte, error) {
	e := &encoder{types: map[*RecordType]int{}}
	sp := sProgram{
		Version: serialVersion, Code: p.Code, Consts: e.values(p.Consts), Globals: e.values(p.Globals),
		Tables: p.Tables, Data: p.Data, Stmts: p.Stmts, Files: p.Files, Entry: p.Entry,
		GlobalNames: p.GlobalNames, CommonSlots: p.CommonSlots,
	}
	for _, pr := range p.Procs {
		sp.Procs = append(sp.Procs, sProc{Name: pr.Name, Entry: pr.Entry, NParams: pr.NParams, RetSlot: pr.RetSlot,
			Locals: e.values(pr.Locals), IsFunc: pr.IsFunc, File: pr.File, Line: pr.Line, EndLine: pr.EndLine,
			Start: pr.Start, End: pr.End})
	}
	for i := 0; i < len(e.list); i++ { // la lista puede crecer con tipos anidados
		t := e.list[i]
		st := sRecType{Name: t.Name, Size: t.Size}
		for _, f := range t.Fields {
			st.Fields = append(st.Fields, sField{Name: f.Name, Proto: e.value(f.Proto), Size: f.Size})
		}
		sp.Types = append(sp.Types, st)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := gob.NewEncoder(zw).Encode(&sp); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type decoder struct{ types []*RecordType }

func (d *decoder) value(s sValue) Value {
	v := Value{K: s.K, I: s.I, F: s.F, S: s.S}
	if s.K == KRecord && s.Rec > 0 {
		r := &Record{T: d.types[s.Rec-1], F: make([]Value, len(s.Fields))}
		for i, f := range s.Fields {
			r.F[i] = d.value(f)
		}
		v.R = r
	}
	return v
}

func (d *decoder) values(ss []sValue) []Value {
	out := make([]Value, len(ss))
	for i, s := range ss {
		out[i] = d.value(s)
	}
	return out
}

// Unmarshal reconstruye un programa serializado con Marshal.
func Unmarshal(data []byte) (*Program, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var sp sProgram
	if err := gob.NewDecoder(zr).Decode(&sp); err != nil {
		return nil, err
	}
	if sp.Version != serialVersion {
		return nil, fmt.Errorf("versión de programa %d no soportada", sp.Version)
	}
	d := &decoder{types: make([]*RecordType, len(sp.Types))}
	for i, st := range sp.Types {
		d.types[i] = &RecordType{Name: st.Name, Size: st.Size}
	}
	for i, st := range sp.Types {
		for _, f := range st.Fields {
			d.types[i].Fields = append(d.types[i].Fields, FieldDef{Name: f.Name, Proto: d.value(f.Proto), Size: f.Size})
		}
	}
	p := &Program{Code: sp.Code, Consts: d.values(sp.Consts), Globals: d.values(sp.Globals), Tables: sp.Tables,
		Data: sp.Data, Stmts: sp.Stmts, Files: sp.Files, Entry: sp.Entry, GlobalNames: sp.GlobalNames, CommonSlots: sp.CommonSlots}
	if p.GlobalNames == nil {
		p.GlobalNames = map[string]int{}
	}
	for _, pr := range sp.Procs {
		p.Procs = append(p.Procs, &Proc{Name: pr.Name, Entry: pr.Entry, NParams: pr.NParams, RetSlot: pr.RetSlot,
			Locals: d.values(pr.Locals), IsFunc: pr.IsFunc, File: pr.File, Line: pr.Line, EndLine: pr.EndLine,
			Start: pr.Start, End: pr.End})
	}
	return p, nil
}
