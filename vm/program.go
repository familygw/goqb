package vm

// Op es un código de operación de la VM (máquina de pila).
type Op uint8

const (
	OpNop      Op = iota
	OpConst       // push Consts[A]
	OpPop         // descarta el tope
	OpLoadG       // push copia de Globals[A]
	OpStoreG      // pop → Globals[A] (con conversión al tipo de la celda)
	OpLoadL       // push copia de Locals[A]
	OpStoreL      // pop → Locals[A]
	OpAddrG       // push referencia a Globals[A]
	OpAddrL       // push referencia a Locals[A]
	OpIndex       // pop A índices y una referencia a array → push referencia al elemento
	OpField       // pop referencia a registro → push referencia al campo A
	OpLoadInd     // pop referencia → push copia
	OpStoreInd    // pop valor, pop referencia → asigna
	OpConv        // convierte el tope al Kind A
	OpTemp        // pop valor → push referencia a una celda nueva (temporal por referencia)
	OpTempK       // pop valor → push referencia a una celda nueva del Kind A (conversión)

	OpAdd
	OpSub
	OpMul
	OpDiv
	OpIDiv
	OpMod
	OpPow
	OpNeg
	OpEq
	OpNe
	OpLt
	OpGt
	OpLe
	OpGe
	OpNot
	OpAnd
	OpOr
	OpXor
	OpEqv
	OpImp

	OpJmp         // pc = A
	OpJmpF        // pop; si es cero, pc = A
	OpJmpT        // pop; si no es cero, pc = A
	OpGosub       // push retorno en la pila GOSUB; pc = A
	OpReturn      // RETURN (A >= 0: RETURN etiqueta)
	OpOnJump      // pop n; salta a Tables[A][n-1]; B=1 → GOSUB
	OpCall        // llama a Procs[A] con B argumentos
	OpRet         // retorno de SUB/FUNCTION/DEF FN
	OpBuiltin     // función intrínseca A con B argumentos → push resultado
	OpStmt        // sentencia intrínseca A con B argumentos
	OpForCmp      // pop paso, límite, valor → push (paso>=0 ? valor<=límite : valor>=límite)
	OpOnError     // A = pc del manejador, -1 desactiva
	OpResume      // A: 0 = RESUME, 1 = NEXT, 2 = etiqueta (pc en B)
	OpRead        // pop referencia; lee el siguiente DATA
	OpRestore     // puntero de DATA = A
	OpDim         // B = cantidad de dimensiones | flags<<8; Consts[A] = prototipo; pila: ref, lo1, hi1, ...
	OpErase       // pop referencia a array
	OpSwap        // pop dos referencias e intercambia
	OpEnd         // END / SYSTEM
	OpStop        // STOP
	OpUnsupported // error 73 (A = índice del nombre en Consts)
	OpError       // ERROR n: pop n
	OpEvt         // inicio de sentencia: revisa eventos atrapados (ON TIMER/KEY/PLAY)
)

// Flags de OpDim.
const (
	DimRedim  = 1 << 8
	DimStatic = 2 << 8
	DimShared = 4 << 8
)

// Instr es una instrucción.
type Instr struct {
	Op Op
	A  int32
	B  int32
}

// StmtInfo asocia el inicio de una sentencia con su posición en la fuente.
type StmtInfo struct {
	PC   int
	File int
	Line int
	Col  int
	ERL  int // último número de línea visto (para ERL)
}

// Proc es un SUB, FUNCTION o DEF FN compilado.
type Proc struct {
	Name    string
	Entry   int
	NParams int
	Locals  []Value // prototipos de las variables locales (los parámetros primero)
	IsFunc  bool
	RetSlot int // slot local del valor de retorno
	File    int
	Line    int
	EndLine int
}

// DataItem es un elemento de DATA.
type DataItem struct {
	Text   string
	Quoted bool
}

// Program es un programa compilado listo para ejecutar.
type Program struct {
	Code    []Instr
	Consts  []Value
	Globals []Value // prototipos (cero tipado) de cada variable global
	Procs   []*Proc
	Tables  [][]int // tablas de ON GOTO/GOSUB
	Data    []DataItem
	Stmts   []StmtInfo // ordenadas por PC
	Files   []string   // nombres de archivos fuente (índices de StmtInfo.File)
	Entry   int
	// GlobalNames ayuda a la ventana Immediate y al depurador.
	GlobalNames map[string]int
}

// StmtAt devuelve el índice de la sentencia que contiene pc (o -1).
func (p *Program) StmtAt(pc int) int {
	lo, hi := 0, len(p.Stmts)-1
	ans := -1
	for lo <= hi {
		m := (lo + hi) / 2
		if p.Stmts[m].PC <= pc {
			ans = m
			lo = m + 1
		} else {
			hi = m - 1
		}
	}
	return ans
}
