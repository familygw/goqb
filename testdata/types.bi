' Declaraciones compartidas
TYPE Punto
    x AS INTEGER
    y AS INTEGER
END TYPE

TYPE Jugador
    nombre AS STRING * 20
    pos AS Punto
    vidas AS INTEGER
    puntaje AS LONG
END TYPE

CONST VERDADERO = -1, FALSO = 0
DECLARE SUB Dibujar (j AS Jugador)
DECLARE FUNCTION Distancia! (a AS Punto, b AS Punto)
