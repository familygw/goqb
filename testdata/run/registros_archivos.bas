DEFINT A-Z
TYPE Punto
  x AS INTEGER
  y AS INTEGER
END TYPE
TYPE Persona
  nombre AS STRING * 10
  edad AS INTEGER
  pos AS Punto
END TYPE
DECLARE SUB Mover (p AS Punto, dx%)
DIM SHARED total AS LONG
DIM gente(1 TO 3) AS Persona
gente(1).nombre = "Ana": gente(1).edad = 30
gente(2).nombre = "Bruno": gente(2).edad = 25
gente(2).pos.x = 10
DIM p AS Persona
p = gente(2)
p.edad = 99
PRINT "["; gente(2).nombre; "]"; gente(2).edad; p.edad; LEN(p); LEN(gente(1).nombre)
CALL Mover(gente(2).pos, 5)
PRINT gente(2).pos.x
Acumular 10: Acumular 20
PRINT "total ="; total; "llamadas ="; Contar; Contar; Contar
' DATA
RESTORE Nums
FOR i = 1 TO 4: READ n: suma = suma + n: NEXT
READ nom$
PRINT "suma"; suma; nom$
' arrays
REDIM v(5)
v(3) = 7
PRINT LBOUND(v); UBOUND(v); v(3)
REDIM v(10)
PRINT UBOUND(v); v(3)
w(2) = 5: PRINT w(2); UBOUND(w)
a = 1: b = 2: SWAP a, b: PRINT a; b
DEF FNcuad (x) = x * x
PRINT FNcuad(7)
FOR i = 1 TO 3
  FOR j = 1 TO 3
    IF j = 2 THEN EXIT FOR
    PRINT i * 10 + j;
  NEXT j, i
PRINT
' archivos
OPEN "prueba.txt" FOR OUTPUT AS #1
PRINT #1, "linea uno"
WRITE #1, "dos", 2, 3.5
PRINT #1, 42; -7
CLOSE #1
OPEN "prueba.txt" FOR INPUT AS #1
LINE INPUT #1, l$
INPUT #1, a$, n1, f!
INPUT #1, x1, x2
PRINT l$; "|"; a$; "|"; n1; f!; x1; x2; EOF(1)
CLOSE
OPEN "reg.dat" FOR RANDOM AS #2 LEN = LEN(p)
PUT #2, 1, gente(1)
PUT #2, 2, gente(2)
DIM q AS Persona
GET #2, 2, q
PRINT RTRIM$(q.nombre); q.edad; q.pos.x; LOF(2)
CLOSE #2
KILL "reg.dat": KILL "prueba.txt"
INPUT "Nombre y edad"; nm$, ed
PRINT "Hola "; nm$; ed + 1
LINE INPUT "Frase: "; fr$
PRINT "<"; fr$; ">"
END
Nums:
DATA 1, 2, 3, 4, "Ana Lia"
SUB Mover (p AS Punto, dx%)
  p.x = p.x + dx%
END SUB
SUB Acumular (n)
  total = total + n
END SUB
FUNCTION Contar
  STATIC c
  c = c + 1
  Contar = c
END FUNCTION
