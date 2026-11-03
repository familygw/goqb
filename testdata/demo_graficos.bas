' Demo de gráficos y sonido de goqb (fases 4 y 5)
DEFINT A-Z
SCREEN 12
CLS
' cielo, sol y montañas
LINE (0, 0)-(639, 300), 1, BF
CIRCLE (520, 80), 40, 14
PAINT (520, 80), 14, 14
FOR i = 0 TO 2
    x = 80 + i * 200
    LINE (x - 120, 300)-(x, 140), 2
    LINE -(x + 120, 300), 2
    LINE -(x - 120, 300), 2
    PAINT (x, 250), 2, 2
NEXT
LINE (0, 301)-(639, 479), 6, BF
' casa con DRAW: primero las paredes, después el techo
DRAW "BM300,420 C15 R120 U90 L120 D90"
PAINT (360, 380), 7, 15
DRAW "BM300,329 C12 E60 F60 L120"
PAINT (360, 310), 4, 12
LOCATE 2, 3: COLOR 15: PRINT "goqb - SCREEN 12 (640x480, 16 colores)"
' arcoíris de círculos
FOR r = 10 TO 70 STEP 6
    CIRCLE (120, 420), r, (r \ 6) MOD 16, 0, 3.14159
NEXT
' sprite con GET/PUT
DIM sprite(1000) AS INTEGER
GET (500, 40)-(540, 120), sprite
PUT (580, 330), sprite, PSET
' música de fondo y evento de reloj
ON TIMER(1) GOSUB Reloj
TIMER ON
PLAY "MB T150 L8 O3 CEGCEG>C<GEC"
LOCATE 29, 3: PRINT "Presione una tecla para terminar...";
DO: LOOP WHILE INKEY$ = ""
TIMER OFF
SCREEN 0
END

Reloj:
    LOCATE 2, 60: COLOR 14: PRINT TIME$;
    RETURN
