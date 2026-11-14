// Package ast define el árbol sintáctico de QuickBASIC 4.5.
//
// Criterios de diseño:
//   - Las sentencias con sintaxis uniforme ("PALABRA arg, , arg") se
//     representan con Simple; los argumentos omitidos son nil. La validación
//     de aridad y tipos es tarea del análisis semántico.
//   - Elemento de array y llamada a función son indistinguibles sintácticamente
//     en QB: ambos son Call y se resuelven en el análisis semántico.
//   - Los identificadores con puntos (a.b.c) se conservan enteros en Ident;
//     si a es una variable de tipo registro, el análisis semántico lo
//     convierte en acceso a campo.
package ast

import "goqb/lang/token"

type Pos = token.Pos

// Node es cualquier nodo del árbol.
type Node interface{ Position() Pos }

// Expr es una expresión.
type Expr interface {
	Node
	exprNode()
}

// Stmt es una sentencia.
type Stmt interface {
	Node
	stmtNode()
}

// File es un módulo (.BAS o .BI).
type File struct {
	Name  string
	Stmts []Stmt
}

// TypeRef es un tipo en una cláusula AS.
type TypeRef struct {
	P        Pos
	Name     string // INTEGER, LONG, SINGLE, DOUBLE, STRING, ANY o nombre de TYPE (mayúsculas)
	FixedLen Expr   // STRING * n
}

// ---------------------------------------------------------------- Expresiones

type (
	NumberLit struct {
		P       Pos
		Text    string // tal como está en la fuente (&HFF, 1.5D3, 10)
		Suffix  byte   // % & ! # o 0
		IsFloat bool
		Int     int64   // valor si !IsFloat (en &H/&O: valor sin signo)
		Float   float64 // valor si IsFloat
		Radix   int     // 10, 16 u 8
	}

	StringLit struct {
		P     Pos
		Value string
	}

	// Ident es una variable o constante simple (o función sin argumentos).
	Ident struct {
		P      Pos
		Name   string // como está escrito, incluye el sufijo
		Suffix byte
	}

	// Call es nombre(args): elemento de array, FUNCTION, DEF FN o intrínseca.
	Call struct {
		P       Pos
		Name    string
		Suffix  byte
		Args    []Expr
		Keyword bool // el nombre es palabra reservada (TIMER, SCREEN, LEN...)
		Empty   bool // a() — array completo como argumento
	}

	// FieldAccess es x.campo cuando x no es un identificador simple: a(1).x
	FieldAccess struct {
		P     Pos
		X     Expr
		Field string
	}

	Unary struct {
		P  Pos
		Op string // "-", "+", "NOT", "BYVAL", "SEG"
		X  Expr
	}

	Binary struct {
		P    Pos
		Op   string // + - * / \ ^ MOD = <> < > <= >= AND OR XOR EQV IMP
		L, R Expr
	}

	Paren struct {
		P Pos
		X Expr
	}

	// FileNum es #n en una lista de argumentos.
	FileNum struct {
		P    Pos
		X    Expr
		Hash bool // estaba escrito con #
	}
)

func (e *NumberLit) Position() Pos   { return e.P }
func (e *StringLit) Position() Pos   { return e.P }
func (e *Ident) Position() Pos       { return e.P }
func (e *Call) Position() Pos        { return e.P }
func (e *FieldAccess) Position() Pos { return e.P }
func (e *Unary) Position() Pos       { return e.P }
func (e *Binary) Position() Pos      { return e.P }
func (e *Paren) Position() Pos       { return e.P }
func (e *FileNum) Position() Pos     { return e.P }

func (*NumberLit) exprNode()   {}
func (*StringLit) exprNode()   {}
func (*Ident) exprNode()       {}
func (*Call) exprNode()        {}
func (*FieldAccess) exprNode() {}
func (*Unary) exprNode()       {}
func (*Binary) exprNode()      {}
func (*Paren) exprNode()       {}
func (*FileNum) exprNode()     {}

// ---------------------------------------------------------------- Sentencias

// Coord es [STEP](x, y) en sentencias gráficas.
type Coord struct {
	Step bool
	X, Y Expr
}

type (
	// Label es un número de línea o etiqueta alfanumérica.
	Label struct {
		P       Pos
		Name    string // mayúsculas
		Numeric bool
	}

	Comment struct {
		P    Pos
		Text string
		Rem  bool
	}

	// Meta es un metacomando ($INCLUDE, $DYNAMIC, $STATIC).
	// Tras cargar con el paquete source, un $INCLUDE resuelto contiene
	// las sentencias del archivo incluido.
	Meta struct {
		P        Pos
		Name     string
		Arg      string
		Resolved string // ruta del archivo incluido
		Stmts    []Stmt // sentencias incluidas (inclusión textual)
	}

	// Simple es "PALABRA [args]" con argumentos posiblemente omitidos (nil).
	// Name es la palabra clave en mayúsculas, a veces compuesta:
	// "END", "CLS", "COLOR", "DEF SEG", "WIDTH LPRINT", "KEY ON", "OPTION BASE"...
	Simple struct {
		P    Pos
		Name string
		Args []Expr
	}

	Assign struct {
		P      Pos
		Let    bool
		Target Expr
		Value  Expr
	}

	// MidAssign es MID$(s, inicio[, largo]) = valor.
	MidAssign struct {
		P             Pos
		Target        Expr
		Start, Length Expr
		Value         Expr
	}

	// SetStr es LSET/RSET destino = valor.
	SetStr struct {
		P      Pos
		Right  bool
		Target Expr
		Value  Expr
	}

	CallStmt struct {
		P        Pos
		Name     string
		Args     []Expr
		Explicit bool // con CALL / CALLS
		Calls    bool // CALLS
	}

	PrintItem struct {
		X   Expr // nil si es sólo un separador
		Sep byte // ';', ',' o 0
	}

	Print struct {
		P      Pos
		LPrint bool
		File   Expr // FileNum o nil
		Using  Expr
		Items  []PrintItem
	}

	Write struct {
		P    Pos
		File Expr
		Args []Expr
	}

	Input struct {
		P         Pos
		Line      bool // LINE INPUT
		File      Expr
		NoNewline bool // INPUT ; ...
		Prompt    *StringLit
		PromptSep byte // ';' (muestra "? ") o ','
		Vars      []Expr
	}

	ElseIf struct {
		P    Pos
		Cond Expr
		Body []Stmt
	}

	If struct {
		P          Pos
		Cond       Expr
		Then       []Stmt
		ElseIfs    []ElseIf
		Else       []Stmt
		HasElse    bool
		SingleLine bool
	}

	// CaseTest: valor | desde TO hasta | IS op valor
	CaseTest struct {
		Kind string // "VALUE", "RANGE", "IS"
		Op   string
		X, Y Expr
	}

	CaseClause struct {
		P     Pos
		Else  bool
		Tests []CaseTest
		Body  []Stmt
	}

	SelectCase struct {
		P     Pos
		X     Expr
		Cases []CaseClause
	}

	For struct {
		P              Pos
		Var            Expr
		From, To, Step Expr
		Body           []Stmt
		NextVar        Expr // variable escrita en NEXT, si la hay
		NextPos        Pos  // posición del NEXT (el depurador se detiene ahí)
	}

	// DoLoop: DO [WHILE|UNTIL c] ... LOOP [WHILE|UNTIL c]
	DoLoop struct {
		P         Pos
		PreCond   Expr
		PreUntil  bool
		PostCond  Expr
		PostUntil bool
		Body      []Stmt
		LoopPos   Pos
	}

	While struct {
		P       Pos
		Cond    Expr
		Body    []Stmt
		WendPos Pos
	}

	// Jump es GOTO / GOSUB / RETURN [label] / RESTORE [label].
	Jump struct {
		P      Pos
		Kind   string // GOTO, GOSUB, RETURN, RESTORE
		Target string // mayúsculas; "" si no hay
	}

	OnJump struct {
		P       Pos
		X       Expr
		Gosub   bool
		Targets []string
	}

	OnError struct {
		P      Pos
		Target string // "0" desactiva
	}

	Resume struct {
		P      Pos
		Next   bool
		Target string // "" → RESUME, "0" equivale a RESUME
	}

	// OnEvent: ON TIMER(n) | KEY(n) | COM(n) | PEN | PLAY(n) | STRIG(n) | UEVENT GOSUB label
	OnEvent struct {
		P      Pos
		Event  string
		Arg    Expr
		Target string
	}

	// EventCtl: TIMER ON, KEY(n) OFF, PEN STOP, ...
	EventCtl struct {
		P      Pos
		Event  string
		Arg    Expr
		Action string // ON, OFF, STOP
	}

	Exit struct {
		P    Pos
		What string // FOR, DO, SUB, FUNCTION, DEF
	}

	Bound struct{ Lo, Hi Expr } // Lo nil → OPTION BASE

	DimVar struct {
		P       Pos
		Name    string
		Suffix  byte
		IsArray bool
		Bounds  []Bound // vacío con IsArray → x() sin dimensiones
		Type    *TypeRef
	}

	// Dim cubre DIM, REDIM, STATIC, SHARED y COMMON.
	Dim struct {
		P      Pos
		Kind   string
		Shared bool
		Block  string // COMMON /bloque/
		Vars   []DimVar
	}

	ConstItem struct {
		P     Pos
		Name  string
		Value Expr
	}

	Const struct {
		P     Pos
		Items []ConstItem
	}

	LetterRange struct{ From, To byte }

	DefType struct {
		P      Pos
		Type   string // INTEGER, LONG, SINGLE, DOUBLE, STRING
		Ranges []LetterRange
	}

	TypeField struct {
		P    Pos
		Name string
		Type TypeRef
	}

	TypeDecl struct {
		P      Pos
		Name   string
		Fields []TypeField
	}

	Param struct {
		P       Pos
		Name    string
		Suffix  byte
		IsArray bool
		ByVal   bool
		Seg     bool
		Type    *TypeRef
	}

	Declare struct {
		P         Pos
		Function  bool
		Name      string
		CDecl     bool
		Alias     string
		Params    []Param
		HasParens bool
	}

	// Proc es SUB ... END SUB o FUNCTION ... END FUNCTION.
	Proc struct {
		P        Pos
		Function bool
		Name     string
		Suffix   byte
		Params   []Param
		Static   bool
		Body     []Stmt
		EndPos   Pos
	}

	// DefFn es DEF FNx(...) = expr o el bloque DEF FN ... END DEF.
	DefFn struct {
		P      Pos
		Name   string
		Params []Param
		Expr   Expr // forma de una línea
		Body   []Stmt
	}

	DataItem struct {
		Text   string
		Quoted bool
	}

	Data struct {
		P     Pos
		Raw   string
		Items []DataItem
	}

	Open struct {
		P      Pos
		File   Expr
		Mode   string // INPUT, OUTPUT, APPEND, RANDOM, BINARY ("" = RANDOM)
		Access string // READ, WRITE, READ WRITE
		Lock   string // SHARED, LOCK READ, LOCK WRITE, LOCK READ WRITE
		Num    Expr
		Len    Expr
		// Forma antigua: OPEN modo$, [#]n, archivo$[, largo]
		Legacy     bool
		LegacyMode Expr
	}

	FieldItem struct {
		Width Expr
		Var   Expr
	}

	Field struct {
		P      Pos
		File   Expr
		Fields []FieldItem
	}

	// FileIO es GET/PUT de archivo: GET [#]n[, [registro][, variable]]
	FileIO struct {
		P      Pos
		Put    bool
		File   Expr
		Record Expr
		Var    Expr
	}

	// Lock es LOCK/UNLOCK [#]n[, {registro | [desde] TO hasta}]
	Lock struct {
		P        Pos
		Unlock   bool
		File     Expr
		From, To Expr
		HasTo    bool
	}

	PSet struct {
		P      Pos
		Preset bool
		At     Coord
		Color  Expr
	}

	Line struct {
		P     Pos
		From  *Coord
		To    Coord
		Color Expr
		Box   string // "", "B", "BF"
		Style Expr
	}

	Circle struct {
		P                         Pos
		At                        Coord
		Radius, Color, Start, End Expr
		Aspect                    Expr
	}

	Paint struct {
		P                        Pos
		At                       Coord
		Fill, Border, Background Expr
	}

	GfxGet struct {
		P        Pos
		From, To Coord
		Array    Expr
	}

	GfxPut struct {
		P      Pos
		At     Coord
		Array  Expr
		Action string // PSET, PRESET, AND, OR, XOR
	}

	View struct {
		P             Pos
		Print         bool // VIEW PRINT
		Top, Bottom   Expr
		Screen        bool
		From, To      *Coord
		Color, Border Expr
	}

	Window struct {
		P        Pos
		Screen   bool
		From, To *Coord
	}
)

func (s *Label) Position() Pos      { return s.P }
func (s *Comment) Position() Pos    { return s.P }
func (s *Meta) Position() Pos       { return s.P }
func (s *Simple) Position() Pos     { return s.P }
func (s *Assign) Position() Pos     { return s.P }
func (s *MidAssign) Position() Pos  { return s.P }
func (s *SetStr) Position() Pos     { return s.P }
func (s *CallStmt) Position() Pos   { return s.P }
func (s *Print) Position() Pos      { return s.P }
func (s *Write) Position() Pos      { return s.P }
func (s *Input) Position() Pos      { return s.P }
func (s *If) Position() Pos         { return s.P }
func (s *SelectCase) Position() Pos { return s.P }
func (s *For) Position() Pos        { return s.P }
func (s *DoLoop) Position() Pos     { return s.P }
func (s *While) Position() Pos      { return s.P }
func (s *Jump) Position() Pos       { return s.P }
func (s *OnJump) Position() Pos     { return s.P }
func (s *OnError) Position() Pos    { return s.P }
func (s *Resume) Position() Pos     { return s.P }
func (s *OnEvent) Position() Pos    { return s.P }
func (s *EventCtl) Position() Pos   { return s.P }
func (s *Exit) Position() Pos       { return s.P }
func (s *Dim) Position() Pos        { return s.P }
func (s *Const) Position() Pos      { return s.P }
func (s *DefType) Position() Pos    { return s.P }
func (s *TypeDecl) Position() Pos   { return s.P }
func (s *Declare) Position() Pos    { return s.P }
func (s *Proc) Position() Pos       { return s.P }
func (s *DefFn) Position() Pos      { return s.P }
func (s *Data) Position() Pos       { return s.P }
func (s *Open) Position() Pos       { return s.P }
func (s *Field) Position() Pos      { return s.P }
func (s *FileIO) Position() Pos     { return s.P }
func (s *Lock) Position() Pos       { return s.P }
func (s *PSet) Position() Pos       { return s.P }
func (s *Line) Position() Pos       { return s.P }
func (s *Circle) Position() Pos     { return s.P }
func (s *Paint) Position() Pos      { return s.P }
func (s *GfxGet) Position() Pos     { return s.P }
func (s *GfxPut) Position() Pos     { return s.P }
func (s *View) Position() Pos       { return s.P }
func (s *Window) Position() Pos     { return s.P }

func (*Label) stmtNode()      {}
func (*Comment) stmtNode()    {}
func (*Meta) stmtNode()       {}
func (*Simple) stmtNode()     {}
func (*Assign) stmtNode()     {}
func (*MidAssign) stmtNode()  {}
func (*SetStr) stmtNode()     {}
func (*CallStmt) stmtNode()   {}
func (*Print) stmtNode()      {}
func (*Write) stmtNode()      {}
func (*Input) stmtNode()      {}
func (*If) stmtNode()         {}
func (*SelectCase) stmtNode() {}
func (*For) stmtNode()        {}
func (*DoLoop) stmtNode()     {}
func (*While) stmtNode()      {}
func (*Jump) stmtNode()       {}
func (*OnJump) stmtNode()     {}
func (*OnError) stmtNode()    {}
func (*Resume) stmtNode()     {}
func (*OnEvent) stmtNode()    {}
func (*EventCtl) stmtNode()   {}
func (*Exit) stmtNode()       {}
func (*Dim) stmtNode()        {}
func (*Const) stmtNode()      {}
func (*DefType) stmtNode()    {}
func (*TypeDecl) stmtNode()   {}
func (*Declare) stmtNode()    {}
func (*Proc) stmtNode()       {}
func (*DefFn) stmtNode()      {}
func (*Data) stmtNode()       {}
func (*Open) stmtNode()       {}
func (*Field) stmtNode()      {}
func (*FileIO) stmtNode()     {}
func (*Lock) stmtNode()       {}
func (*PSet) stmtNode()       {}
func (*Line) stmtNode()       {}
func (*Circle) stmtNode()     {}
func (*Paint) stmtNode()      {}
func (*GfxGet) stmtNode()     {}
func (*GfxPut) stmtNode()     {}
func (*View) stmtNode()       {}
func (*Window) stmtNode()     {}
