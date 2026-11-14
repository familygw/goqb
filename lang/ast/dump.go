package ast

import (
	"fmt"
	"reflect"
	"strings"
)

// Dump devuelve una representación legible e indentada del nodo.
// Omite posiciones y campos vacíos; pensado para depuración y tests.
func Dump(n any) string {
	var b strings.Builder
	dump(&b, reflect.ValueOf(n), 0)
	return b.String()
}

var posType = reflect.TypeOf(Pos{})

func dump(b *strings.Builder, v reflect.Value, depth int) {
	ind := strings.Repeat("  ", depth)
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			b.WriteString("nil")
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		b.WriteString(v.Type().Name())
		b.WriteString(" {")
		any := false
		for i := 0; i < v.NumField(); i++ {
			f, ft := v.Field(i), v.Type().Field(i)
			if ft.Type == posType || f.IsZero() || !ft.IsExported() {
				continue
			}
			if ft.Name == "Text" && v.Type().Name() == "NumberLit" {
				continue
			}
			any = true
			b.WriteString("\n")
			b.WriteString(ind)
			b.WriteString("  ")
			b.WriteString(ft.Name)
			b.WriteString(": ")
			dump(b, f, depth+1)
		}
		if any {
			b.WriteString("\n")
			b.WriteString(ind)
		}
		b.WriteString("}")
	case reflect.Slice:
		if v.Len() == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[")
		for i := 0; i < v.Len(); i++ {
			b.WriteString("\n")
			b.WriteString(ind)
			b.WriteString("  ")
			dump(b, v.Index(i), depth+1)
		}
		b.WriteString("\n")
		b.WriteString(ind)
		b.WriteString("]")
	case reflect.String:
		fmt.Fprintf(b, "%q", v.String())
	case reflect.Uint8:
		fmt.Fprintf(b, "'%c'", byte(v.Uint()))
	default:
		fmt.Fprintf(b, "%v", v.Interface())
	}
}
