DEFINT A-Z
PRINT "Hola"; 1; -2; 3.5; 1/3; 1#/3; 2^10; 7\2; 7 MOD 3
PRINT 1E+7; 12345678; .001; .0001; 100000!*100
DIM a(1 TO 5) AS LONG
FOR i = 1 TO 5: a(i) = i * i: NEXT
FOR i = 5 TO 1 STEP -1: PRINT a(i);: NEXT: PRINT
s$ = "QuickBASIC"
PRINT LEFT$(s$, 5), MID$(s$, 6), RIGHT$(s$, 3), LEN(s$), UCASE$("abc"), INSTR(s$, "BA")
PRINT STR$(42); "|"; VAL(" 1 2.5x"); HEX$(255); " "; HEX$(-1); " "; OCT$(8); CHR$(65); ASC("Z")
x! = 1.5: y# = 2.25: PRINT x! + y#, CINT(2.5), CINT(3.5), INT(-2.5), FIX(-2.5), ABS(-3), SGN(-9)
SELECT CASE i
CASE 0: PRINT "cero"
CASE IS < 0: PRINT "neg"
CASE ELSE: PRINT "otro"
END SELECT
PRINT Suma(2, 3), Fact&(10)
CALL Doble(i): PRINT "i ="; i
k = 1
DO WHILE k < 100: k = k * 2: LOOP
PRINT "k ="; k
GOSUB sub1
PRINT USING "###.##  $$#,###.##  +##  \  \  !"; 3.14159; 1234.5; -5; "ABCDEF"; "xyz"
ON ERROR GOTO handler
PRINT 1 / 0
PRINT "despues del error"
z% = 32767: z% = z% + 1
PRINT "fin"
END
sub1: PRINT "en gosub": RETURN
handler:
PRINT "Error"; ERR; ":"; " atrapado"
RESUME NEXT
FUNCTION Suma (a, b)
  Suma = a + b
END FUNCTION
FUNCTION Fact& (n)
  IF n <= 1 THEN Fact& = 1 ELSE Fact& = n * Fact&(n - 1)
END FUNCTION
SUB Doble (v)
  v = v * 2
END SUB
