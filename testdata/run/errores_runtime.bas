' Manejo de errores: RESUME, RESUME NEXT, RESUME etiqueta, ERR y ERL
ON ERROR GOTO Manejador
10 x = 1 / 0
20 PRINT "sigue tras division por cero"
30 OPEN "no_existe.txt" FOR INPUT AS #1
40 PRINT "sigue tras archivo inexistente"
intentos = 0
50 y = LOG(intentos)
PRINT "LOG reintentado, intentos ="; intentos
ERROR 200
PRINT "fin"
END
Manejador:
SELECT CASE ERR
CASE 11: PRINT "Division by zero en linea"; ERL: RESUME NEXT
CASE 53: PRINT "File not found en linea"; ERL: RESUME NEXT
CASE 5: intentos = intentos + 1: RESUME
CASE ELSE: PRINT "Error"; ERR; "->"; : RESUME Final
END SELECT
Final:
PRINT " salto a Final"
END
