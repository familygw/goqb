package ide

import (
	"sort"
	"strings"
	"unicode"
)

// Ayuda por palabra clave (F1): sintaxis y una descripción breve.
// Formato de cada línea: PALABRA|sintaxis|descripción (la sintaxis admite
// varias líneas separadas por "\n").
const helpData = `ABS|ABS(n)|Valor absoluto.
ABSOLUTE|CALL ABSOLUTE([args,] offset%)|Ejecuta código máquina en DEF SEG:offset (8086 emulado). Los argumentos se pasan por referencia.
ACCESS|OPEN ... ACCESS {READ | WRITE | READ WRITE}|Tipo de acceso al abrir un archivo (se acepta y se ignora).
APPEND|OPEN archivo$ FOR APPEND AS #n|Abre un archivo secuencial para agregar al final.
AS|DIM v AS tipo · OPEN ... AS #n|Declara el tipo de una variable o el número de archivo.
ASC|ASC(s$)|Código ASCII del primer carácter.
ATN|ATN(n)|Arcotangente en radianes.
BEEP|BEEP|Emite un pitido.
BLOAD|BLOAD archivo$[, offset%]|Carga en memoria un archivo grabado con BSAVE (en DEF SEG:offset o en su dirección original).
BSAVE|BSAVE archivo$, offset%, largo&|Graba un bloque de memoria (DEF SEG:offset) en un archivo.
CALL|CALL nombre[(argumentos)]|Llama a un SUB. Sin CALL, los argumentos van sin paréntesis.
CASE|SELECT CASE expr\nCASE valor[, ...] | CASE a TO b | CASE IS > x | CASE ELSE|Alternativas de SELECT CASE.
CDBL|CDBL(n)|Convierte a DOUBLE.
CHAIN|CHAIN archivo$|Carga y ejecuta otro programa conservando los archivos abiertos y las variables del COMMON sin nombre.
CHDIR|CHDIR ruta$|Cambia el directorio actual.
CHR$|CHR$(código%)|Carácter con ese código ASCII.
CINT|CINT(n)|Redondea a INTEGER (redondeo bancario).
CIRCLE|CIRCLE [STEP](x, y), radio[, color[, inicio[, fin[, aspecto]]]]|Dibuja un círculo, arco o elipse.
CLEAR|CLEAR|Borra las variables y cierra los archivos.
CLNG|CLNG(n)|Redondea a LONG.
CLOSE|CLOSE [[#]n[, ...]]|Cierra archivos (todos si no hay números).
CLS|CLS [{0 | 1 | 2}]|Borra la pantalla (0: todo, 1: viewport gráfico, 2: VIEW PRINT).
COLOR|COLOR [primer plano][, fondo][, borde]|Colores del texto (16-31: parpadeo) o, en gráficos, de dibujo.
COMMAND$|COMMAND$|La línea de comandos del programa.
COMMON|COMMON [SHARED] [/bloque/] variables|Variables compartidas entre módulos y con CHAIN.
CONST|CONST nombre = expresión|Define una constante.
COS|COS(ángulo)|Coseno (radianes).
CSNG|CSNG(n)|Convierte a SINGLE.
CSRLIN|CSRLIN|Fila actual del cursor.
CVI|CVI(s$) · CVL · CVS · CVD|Convierten strings de 2, 4, 4 y 8 bytes en números (inversas de MKI$, MKL$, MKS$, MKD$).
DATA|DATA constante[, ...]|Datos para READ.
DATE$|DATE$|Fecha actual (mm-dd-aaaa).
DECLARE|DECLARE {SUB | FUNCTION} nombre [(parámetros)]|Declara un procedimiento (el IDE los genera al guardar).
DEF|DEF FNnombre[(parámetros)] = expresión · DEF SEG [= segmento]|Función de una línea, o segmento para PEEK, POKE, BLOAD, BSAVE y CALL ABSOLUTE.
DEFINT|DEFINT letras · DEFLNG · DEFSNG · DEFDBL · DEFSTR|Tipo por defecto de las variables según su inicial (por ejemplo DEFINT A-Z).
DIM|DIM [SHARED] variable[(límites)] [AS tipo][, ...]|Declara variables y arrays.
DO|DO [{WHILE | UNTIL} cond]\n  ...\nLOOP [{WHILE | UNTIL} cond]|Bucle; EXIT DO sale.
DRAW|DRAW comandos$|Dibuja con el macrolenguaje (U D L R E F G H M B N C P S A TA X).
ELSE|IF cond THEN ... ELSE ...|Alternativa de IF.
END|END [{IF | SELECT | SUB | FUNCTION | TYPE | DEF}]|Termina el programa o cierra un bloque.
ENVIRON$|ENVIRON$(nombre$ | n%)|Variable de entorno.
EOF|EOF(n)|Verdadero al llegar al final del archivo.
ERASE|ERASE array[, ...]|Borra un array estático o libera uno dinámico.
ERL|ERL|Número de línea del último error.
ERR|ERR|Código del último error.
ERROR|ERROR código%|Provoca un error.
EXIT|EXIT {DO | FOR | SUB | FUNCTION | DEF}|Sale del bloque.
EXP|EXP(n)|e elevado a n.
FIELD|FIELD #n, ancho AS variable$[, ...]|Define los campos de un registro de un archivo RANDOM.
FILES|FILES [patrón$]|Lista archivos.
FIX|FIX(n)|Parte entera (trunca hacia cero).
FOR|FOR v = inicio TO fin [STEP paso]\n  ...\nNEXT [v]|Bucle con contador; EXIT FOR sale.
FREEFILE|FREEFILE|Próximo número de archivo libre.
FUNCTION|FUNCTION nombre[(parámetros)] [STATIC]\n  nombre = valor\nEND FUNCTION|Procedimiento que devuelve un valor.
GET|GET [#]n[, registro][, variable] · GET [STEP](x1,y1)-[STEP](x2,y2), array|Lee de un archivo RANDOM/BINARY, o copia una imagen de la pantalla a un array.
GOSUB|GOSUB etiqueta ... RETURN|Llama a una subrutina local.
GOTO|GOTO etiqueta|Salta a una línea.
HEX$|HEX$(n)|Número en hexadecimal.
IF|IF cond THEN sentencias [ELSE sentencias]\nIF cond THEN ... [ELSEIF cond THEN ...] [ELSE ...] END IF|Ejecución condicional.
INKEY$|INKEY$|Lee una tecla sin esperar ("" si no hay; CHR$(0)+código para las extendidas).
INP|INP(puerto%)|Lee un puerto de E/S emulado (por ejemplo &H3DA: retrazado vertical).
INPUT|INPUT [;] ["texto"{; | ,}] variables · INPUT #n, variables|Lee datos del teclado o de un archivo.
INPUT$|INPUT$(n[, [#]archivo])|Lee n caracteres.
INSTR|INSTR([inicio,] s$, buscado$)|Posición de un string dentro de otro.
INT|INT(n)|Mayor entero menor o igual.
INTERRUPT|CALL INTERRUPT(n%, entrada AS RegType, salida AS RegType)\nCALL INTERRUPTX(n%, entrada AS RegTypeX, salida AS RegTypeX)|Llama a un servicio del BIOS o del DOS emulado (INT 10h, 16h, 1Ah, 21h, 33h...).
INTERRUPTX|CALL INTERRUPTX(n%, entrada AS RegTypeX, salida AS RegTypeX)|Como INTERRUPT, con DS y ES (-1: el valor actual).
KEY|KEY n, texto$ · KEY(n) {ON | OFF | STOP} · KEY {ON | OFF | LIST}|Teclas de función y eventos de teclado.
KILL|KILL archivo$|Borra archivos.
LBOUND|LBOUND(array[, dimensión])|Límite inferior de un array.
LCASE$|LCASE$(s$)|Minúsculas.
LEFT$|LEFT$(s$, n)|Primeros n caracteres.
LEN|LEN(s$ | variable)|Largo de un string o tamaño en bytes de una variable.
LET|[LET] variable = expresión|Asignación.
LINE|LINE [[STEP](x1,y1)]-[STEP](x2,y2)[, [color][, [B[F]][, estilo]]]\nLINE INPUT [;] ["texto";] v$ · LINE INPUT #n, v$|Dibuja una línea o caja; LINE INPUT lee una línea completa.
LOC|LOC(n)|Posición actual en un archivo.
LOCATE|LOCATE [fila][, [columna][, [cursor][, inicio, fin]]]|Mueve el cursor.
LOF|LOF(n)|Largo del archivo en bytes.
LOG|LOG(n)|Logaritmo natural.
LOOP|DO ... LOOP [{WHILE | UNTIL} cond]|Fin de un bucle DO.
LPRINT|LPRINT [USING f$;] expresiones|Imprime en la impresora (se descarta).
LSET|LSET v$ = s$|Asigna un string alineado a la izquierda (campos de FIELD).
LTRIM$|LTRIM$(s$)|Quita espacios a la izquierda.
MID$|MID$(s$, inicio[, n]) · MID$(v$, inicio[, n]) = s$|Extrae o reemplaza una parte de un string.
MKDIR|MKDIR ruta$|Crea un directorio.
MKI$|MKI$(n%) · MKL$ · MKS$ · MKD$|Convierten números en strings de 2, 4, 4 y 8 bytes.
NAME|NAME viejo$ AS nuevo$|Renombra un archivo.
NEXT|NEXT [variable[, ...]]|Fin de un bucle FOR.
OCT$|OCT$(n)|Número en octal.
ON|ON ERROR GOTO etiqueta · ON n GOTO|GOSUB lista · ON {TIMER(s) | KEY(n) | PLAY(n)} GOSUB etiqueta|Manejo de errores, saltos calculados y eventos.
OPEN|OPEN archivo$ FOR {INPUT | OUTPUT | APPEND | RANDOM | BINARY} AS #n [LEN = largo]|Abre un archivo.
OPTION|OPTION BASE {0 | 1}|Límite inferior por defecto de los arrays.
OUT|OUT puerto%, valor%|Escribe un puerto de E/S emulado (3C8h/3C9h: paleta VGA).
PAINT|PAINT [STEP](x, y)[, relleno[, borde]]|Rellena un área (relleno: color o mosaico$).
PALETTE|PALETTE [atributo, color] · PALETTE USING array|Cambia la paleta.
PCOPY|PCOPY origen, destino|Copia una página de video.
PEEK|PEEK(offset%)|Lee un byte de DEF SEG:offset (variables, BIOS, &HB800 texto, &HA000 SCREEN 13).
PLAY|PLAY comandos$ · PLAY(n)|Toca música con el macrolenguaje (O L T N P MF MB...).
PMAP|PMAP(coordenada, función)|Convierte entre coordenadas físicas y lógicas.
POINT|POINT(x, y) · POINT(n)|Color de un pixel o posición del cursor gráfico.
POKE|POKE offset%, byte%|Escribe un byte en DEF SEG:offset.
POS|POS(0)|Columna actual del cursor.
PRESET|PRESET [STEP](x, y)[, color]|Dibuja un pixel con el color de fondo.
PRINT|PRINT [#n,] [USING f$;] [expresiones][{; | ,}]|Escribe en pantalla o en un archivo.
PSET|PSET [STEP](x, y)[, color]|Dibuja un pixel.
PUT|PUT [#]n[, registro][, variable] · PUT [STEP](x, y), array[, acción]|Escribe en un archivo RANDOM/BINARY o dibuja una imagen (PSET PRESET AND OR XOR).
RANDOMIZE|RANDOMIZE [semilla]|Inicializa el generador de RND (RANDOMIZE TIMER).
READ|READ variables|Lee de los DATA.
REDIM|REDIM [SHARED] array(límites) [AS tipo]|Redimensiona un array dinámico.
REM|REM comentario · ' comentario|Comentario.
RESTORE|RESTORE [etiqueta]|Vuelve a leer los DATA desde el principio o desde una etiqueta.
RESUME|RESUME [0 | NEXT | etiqueta]|Vuelve de un manejador de ON ERROR.
RETURN|RETURN [etiqueta]|Vuelve de un GOSUB.
RIGHT$|RIGHT$(s$, n)|Últimos n caracteres.
RMDIR|RMDIR ruta$|Borra un directorio.
RND|RND[(n)]|Número al azar entre 0 y 1.
RSET|RSET v$ = s$|Asigna un string alineado a la derecha.
RTRIM$|RTRIM$(s$)|Quita espacios a la derecha.
RUN|RUN [etiqueta | archivo$]|Reinicia el programa o carga y ejecuta otro.
SADD|SADD(s$)|Offset de los datos de un string en DGROUP.
SCREEN|SCREEN modo[, [color][, [activa][, visible]]] · SCREEN(fila, col[, color])|Cambia el modo de video (0, 1, 2, 7-13) o lee un carácter de la pantalla.
SEEK|SEEK [#]n, posición · SEEK(n)|Posición en un archivo.
SELECT|SELECT CASE expr ... END SELECT|Selección múltiple.
SGN|SGN(n)|Signo (-1, 0, 1).
SHARED|SHARED variables · DIM SHARED|Comparte variables del módulo con los procedimientos.
SHELL|SHELL [comando$]|Ejecuta un comando del sistema.
SIN|SIN(ángulo)|Seno (radianes).
SLEEP|SLEEP [segundos]|Espera (o hasta una tecla).
SOUND|SOUND frecuencia, duración|Tono (la duración en ticks de 1/18,2 s).
SPACE$|SPACE$(n)|n espacios.
SPC|SPC(n)|Saltea n espacios en PRINT.
SQR|SQR(n)|Raíz cuadrada.
STATIC|STATIC variables · SUB ... STATIC|Variables locales que conservan su valor.
STEP|FOR ... STEP paso · PSET STEP(dx, dy)|Paso de FOR o coordenadas relativas.
STOP|STOP|Detiene el programa (en el IDE se puede continuar con F5).
STR$|STR$(n)|Número como string.
STRING$|STRING$(n, código% | s$)|n veces un carácter.
SUB|SUB nombre[(parámetros)] [STATIC]\n  ...\nEND SUB|Define un procedimiento.
SWAP|SWAP a, b|Intercambia dos variables.
SYSTEM|SYSTEM|Termina el programa.
TAB|TAB(columna)|Va a una columna en PRINT.
TAN|TAN(ángulo)|Tangente (radianes).
TIME$|TIME$|Hora actual (hh:mm:ss).
TIMER|TIMER · ON TIMER(s) GOSUB · TIMER {ON | OFF | STOP}|Segundos desde la medianoche; eventos periódicos.
TYPE|TYPE nombre\n  campo AS tipo\nEND TYPE|Define un registro.
UBOUND|UBOUND(array[, dimensión])|Límite superior de un array.
UCASE$|UCASE$(s$)|Mayúsculas.
VAL|VAL(s$)|Valor numérico de un string (acepta &H y &O).
VARPTR|VARPTR(variable)|Offset de una variable (los arrays tienen su propio segmento; ver VARSEG).
VARSEG|VARSEG(variable)|Segmento de una variable.
VIEW|VIEW [[SCREEN] (x1,y1)-(x2,y2)[, [relleno][, borde]]] · VIEW PRINT [arriba TO abajo]|Viewport gráfico o región de texto con scroll.
WAIT|WAIT puerto%, and%[, xor%]|Espera a que un puerto de E/S cumpla la condición (por ejemplo WAIT &H3DA, 8).
WEND|WHILE cond ... WEND|Fin de WHILE.
WHILE|WHILE cond\n  ...\nWEND|Bucle mientras se cumpla la condición.
WIDTH|WIDTH [columnas][, filas] · WIDTH #n, ancho|Cambia el tamaño de la pantalla de texto (40/80 × 25/43/50).
WINDOW|WINDOW [[SCREEN] (x1,y1)-(x2,y2)]|Coordenadas lógicas para los gráficos.
WRITE|WRITE [#n,] expresiones|Escribe datos separados por comas y strings entre comillas.`

type helpTopic struct{ syntax, desc string }

var helpTopics = func() map[string]helpTopic {
	out := map[string]helpTopic{}
	for _, l := range strings.Split(helpData, "\n") {
		parts := strings.SplitN(l, "|", 3)
		if len(parts) == 3 {
			out[parts[0]] = helpTopic{strings.ReplaceAll(parts[1], `\n`, "\n"), parts[2]}
		}
	}
	return out
}()

// helpKeyword devuelve la palabra clave bajo el cursor del editor ("" si no hay ayuda).
func (a *App) helpKeyword() string {
	v := a.doc.Views[a.cur]
	if v.Row >= len(v.Lines) {
		return ""
	}
	l := []rune(v.Lines[v.Row])
	isW := func(r rune) bool { return unicode.IsLetter(r) || r == '$' }
	col := min(v.Col, len(l))
	if col == len(l) || col < len(l) && !isW(l[col]) {
		if col > 0 && isW(l[col-1]) {
			col--
		}
	}
	s, e := col, col
	for s > 0 && isW(l[s-1]) {
		s--
	}
	for e < len(l) && isW(l[e]) {
		e++
	}
	w := strings.ToUpper(string(l[s:e]))
	if _, ok := helpTopics[w]; ok {
		return w
	}
	return ""
}

// contextHelp muestra la ayuda de la palabra clave bajo el cursor o la del teclado.
func (a *App) contextHelp() {
	if a.imm {
		a.help()
		return
	}
	if w := a.helpKeyword(); w != "" {
		a.showTopic(w)
		return
	}
	a.help()
}

func (a *App) showTopic(w string) {
	t := helpTopics[w]
	lines := strings.Split(t.syntax, "\n")
	lines = append(lines, "")
	lines = append(lines, wrap(t.desc, 60)...)
	a.message(w, lines...)
}

// helpIndex lista todas las palabras clave.
func (a *App) helpIndex() {
	var items []string
	for k := range helpTopics {
		items = append(items, k)
	}
	sort.Strings(items)
	d := &Dialog{Title: "Index", List: &ListBox{Label: "Palabras clave", Items: items, Height: 14, Width: 30},
		Buttons: []string{"Ver", "Cancel"}}
	d.OnActivate = func(d *Dialog) bool { return true }
	d.OnClose = func(d *Dialog, b int) bool {
		if b == 0 {
			a.showTopic(items[d.List.Sel])
		}
		return true
	}
	a.pushDialog(d)
}

// wrap corta un texto en líneas de hasta n caracteres.
func wrap(s string, n int) []string {
	var out []string
	var b strings.Builder
	for _, w := range strings.Fields(s) {
		if b.Len() > 0 && len([]rune(b.String()))+1+len([]rune(w)) > n {
			out = append(out, b.String())
			b.Reset()
		}
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(w)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
