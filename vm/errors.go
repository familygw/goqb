package vm

import (
	"errors"
	"fmt"
)

// Error es un error de runtime de QB con su código (el valor de ERR).
type Error struct {
	Code int
}

func (e *Error) Error() string { return ErrorMessage(e.Code) }

// Errores frecuentes, como valores reutilizables.
var (
	ErrNextWithoutFor  = &Error{1}
	ErrSyntax          = &Error{2}
	ErrReturnNoGosub   = &Error{3}
	ErrOutOfData       = &Error{4}
	ErrIllegalCall     = &Error{5}
	ErrOverflow        = &Error{6}
	ErrOutOfMemory     = &Error{7}
	ErrSubscript       = &Error{9}
	ErrDuplicate       = &Error{10}
	ErrDivZero         = &Error{11}
	ErrTypeMismatch    = &Error{13}
	ErrStringSpace     = &Error{14}
	ErrCannotContinue  = &Error{17}
	ErrNoResume        = &Error{19}
	ErrResumeNoError   = &Error{20}
	ErrStackSpace      = &Error{28}
	ErrFieldOverflow   = &Error{50}
	ErrBadFileNumber   = &Error{52}
	ErrFileNotFound    = &Error{53}
	ErrBadFileMode     = &Error{54}
	ErrFileOpen        = &Error{55}
	ErrDeviceIO        = &Error{57}
	ErrFileExists      = &Error{58}
	ErrBadRecordLen    = &Error{59}
	ErrInputPastEnd    = &Error{62}
	ErrBadRecordNum    = &Error{63}
	ErrBadFileName     = &Error{64}
	ErrTooManyFiles    = &Error{67}
	ErrPermission      = &Error{70}
	ErrAdvancedFeature = &Error{73}
	ErrPathAccess      = &Error{75}
	ErrPathNotFound    = &Error{76}
	errStopped         = errors.New("stopped")
	errEnd             = errors.New("end")
)

var messages = map[int]string{
	1: "NEXT without FOR", 2: "Syntax error", 3: "RETURN without GOSUB", 4: "Out of DATA",
	5: "Illegal function call", 6: "Overflow", 7: "Out of memory", 8: "Label not defined",
	9: "Subscript out of range", 10: "Duplicate definition", 11: "Division by zero",
	12: "Illegal in direct mode", 13: "Type mismatch", 14: "Out of string space",
	16: "String formula too complex", 17: "Cannot continue", 18: "Function not defined",
	19: "No RESUME", 20: "RESUME without error", 24: "Device timeout", 25: "Device fault",
	26: "FOR without NEXT", 27: "Out of paper", 28: "Out of stack space", 29: "WHILE without WEND",
	30: "WEND without WHILE", 33: "Duplicate label", 35: "Subprogram not defined",
	37: "Argument-count mismatch", 38: "Array not defined", 40: "Variable required",
	50: "FIELD overflow", 51: "Internal error", 52: "Bad file name or number",
	53: "File not found", 54: "Bad file mode", 55: "File already open",
	56: "FIELD statement active", 57: "Device I/O error", 58: "File already exists",
	59: "Bad record length", 61: "Disk full", 62: "Input past end of file",
	63: "Bad record number", 64: "Bad file name", 67: "Too many files",
	68: "Device unavailable", 69: "Communication-buffer overflow", 70: "Permission denied",
	71: "Disk not ready", 72: "Disk-media error", 73: "Advanced feature unavailable",
	74: "Rename across disks", 75: "Path/File access error", 76: "Path not found",
}

// ErrorMessage devuelve el mensaje de QB para un código de error.
func ErrorMessage(code int) string {
	if m, ok := messages[code]; ok {
		return m
	}
	return "Unprintable error"
}

// RuntimeError es un error no atrapado, con la posición de la sentencia.
type RuntimeError struct {
	Err  error
	File string
	Line int
	Col  int
	PC   int
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Col, e.Err)
}

func (e *RuntimeError) Unwrap() error { return e.Err }

// ErrBreak indica que el programa fue interrumpido (Ctrl+Break) o ejecutó STOP.
var ErrBreak = errors.New("Break")
