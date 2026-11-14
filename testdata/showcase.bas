' Recorrido por la sintaxis de QuickBASIC 4.5
DEFINT A-Z
'$DYNAMIC
'$INCLUDE: 'TYPES.BI'

DIM SHARED tablero(1 TO 10, 1 TO 10) AS INTEGER
DIM j AS Jugador
REDIM lista$(0)
COMMON SHARED /global/ nivel%, nombre$
OPTION BASE 1

ON ERROR GOTO Manejador
ON TIMER(1) GOSUB Tick
TIMER ON
KEY(1) ON
ON KEY(1) GOSUB Ayuda

j.nombre = "Carlos"
j.pos.x = 5: j.pos.y = 7
j.vidas = 3

SCREEN 0: WIDTH 80, 25: COLOR 15, 1: CLS
LOCATE 1, 1, 0
PRINT "Jugador: "; RTRIM$(j.nombre); TAB(40); "Vidas:"; j.vidas
PRINT USING "Puntaje: ######"; j.puntaje
? "atajo de PRINT"

FOR i = 1 TO 10
    FOR k = 1 TO 10 STEP 2
        tablero(i, k) = i * k MOD 7
    NEXT k, i

i = 0
DO WHILE i < 5
    i = i + 1
    IF i = 3 THEN EXIT DO
LOOP

DO
    a$ = INKEY$
LOOP UNTIL a$ <> "" OR TIMER > 100000

WHILE i > 0: i = i - 1: WEND

SELECT CASE i
    CASE 0
        PRINT "cero"
    CASE 1 TO 5, 7
        PRINT "chico"
    CASE IS > 100
        PRINT "grande"
    CASE ELSE
        PRINT "otro"
END SELECT

IF i = 0 AND NOT (k > 3) THEN
    PRINT "si"
ELSEIF i < 0 THEN
    PRINT "negativo"
ELSE
    PRINT "no"
END IF

IF i THEN PRINT "uno" ELSE PRINT "dos": PRINT "tres"
IF x% > 2 GOTO 100
IF x% THEN 100 ELSE 200

x# = -2 ^ 2 + &HFF + &O17 + 1.5D3 + 2.5E-2 + 10& + 3!
z& = &HFFFF&
s$ = MID$("abcdef", 2, 3) + CHR$(65) + STRING$(3, "*") + LEFT$(s$, 1)
MID$(s$, 1, 1) = "Z"
n = LEN(s$) + INSTR(s$, "b") + VAL("12") + ASC("A")
d$ = DATE$: t$ = TIME$
r! = Distancia!(j.pos, j.pos)
CALL Dibujar(j)
Dibujar j
SWAP x%, y%
RANDOMIZE TIMER
y% = INT(RND * 6) + 1

' Archivos
OPEN "datos.txt" FOR OUTPUT AS #1
PRINT #1, "linea"; 1; 2
WRITE #1, "a", 1, 2.5
CLOSE #1
OPEN "datos.txt" FOR INPUT ACCESS READ SHARED AS #2
DO UNTIL EOF(2)
    LINE INPUT #2, l$
    INPUT #2, a$, b
LOOP
CLOSE
OPEN "reg.dat" FOR RANDOM AS #3 LEN = LEN(j)
PUT #3, 1, j
GET #3, , j
FIELD #3, 20 AS f$, 4 AS g$
LSET f$ = "x": RSET g$ = "y"
SEEK #3, 1
LOCK #3, 1 TO 2
UNLOCK #3
CLOSE #3
OPEN "O", #4, "viejo.txt"
CLOSE 4
INPUT "Nombre"; nombre$
INPUT ; "Edad: ", edad
LINE INPUT "Texto: "; texto$

' Gráficos
SCREEN 12
PSET (10, 10), 14
PRESET STEP(1, 1)
LINE (0, 0)-(100, 100), 4, BF
LINE -(200, 50), , B, &HAAAA
CIRCLE (320, 240), 50, 2, , , .5
PAINT (320, 240), 3, 2
DRAW "U10 R10 D10 L10"
DIM sprite(100) AS INTEGER
GET (0, 0)-(9, 9), sprite
PUT (50, 50), sprite, XOR
VIEW (10, 10)-(300, 200), 0, 15
VIEW PRINT 1 TO 20
WINDOW SCREEN (0, 0)-(639, 479)
PALETTE 1, 63
PLAY "MBT120O3L8CDEFGAB"
SOUND 440, 18
BEEP

' DATA / READ
RESTORE Datos
FOR i = 1 TO 3: READ v$: NEXT
Datos:
DATA "Juan, Perez", 42, &H10, sin comillas

DEF FNdoble (v) = v * 2
PRINT FNdoble(21)
DEF SEG = &HB800
POKE 0, 65
DEF SEG

100 PRINT "linea 100"
200 END

Manejador:
    PRINT "Error"; ERR; "en"; ERL
    RESUME NEXT

Tick:
    RETURN

Ayuda:
    RETURN

SUB Dibujar (j AS Jugador) STATIC
    SHARED nivel%
    STATIC veces
    veces = veces + 1
    IF j.vidas <= 0 THEN EXIT SUB
    LOCATE j.pos.y, j.pos.x: PRINT CHR$(2);
END SUB

FUNCTION Distancia! (a AS Punto, b AS Punto)
    dx! = a.x - b.x: dy! = a.y - b.y
    Distancia! = SQR(dx! * dx! + dy! * dy!)
END FUNCTION
