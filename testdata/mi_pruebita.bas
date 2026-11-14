DECLARE SUB PrintBig (num AS INTEGER)
DEFINT A-Z

' Declarations
DECLARE SUB DrawBox (w AS INTEGER, h AS INTEGER, doble AS INTEGER)

CLS
'SCREEN 13

COLOR 0, 7
DrawBox 80, 24, 1

' SUB DrawBox
SUB DrawBox (w AS INTEGER, h AS INTEGER, doble AS INTEGER) STATIC

  ' vamos a dibujar un cuadrado
  DIM l AS STRING, c AS STRING, tl AS STRING, tr AS STRING, bl AS STRING, br AS STRING

  DIM currCol AS INTEGER, currRow AS INTEGER

  currCol = POS(0)
  currRow = CSRLIN

  IF doble = 1 THEN tl = "É" ELSE tl = "Ú"
  IF doble = 1 THEN tr = "»" ELSE tr = "¿"
  IF doble = 1 THEN bl = "È" ELSE bl = "À"
  IF doble = 1 THEN br = "¼" ELSE br = "Ù"
  IF doble = 1 THEN l = "Í" ELSE l = "Ä"
  IF doble = 1 THEN c = "º" ELSE c = "³"

  PRINT tl; STRING$(w - 2, l); tr

  FOR i = currRow + 1 TO h - 2
    LOCATE i, currCol
    PRINT c; SPACE$(w - 2); c
  NEXT i

  LOCATE currRow + (h - 2), currCol
  PRINT bl; STRING$(w - 2, l); br

END SUB

SUB PrintBig (num AS INTEGER)

  PRINT "ÜÜÜ"
  PRINT "Û Û "
  PRINT "ßßß"

END SUB
