# goqb

IDE y compilador de **QuickBASIC 4.5** escritos en Go, con estética DOS y una interfaz de terminal que se adapta al tamaño de la ventana.
La arquitectura completa está en el doc `architecture.md` del proyecto.

## Instalación

```sh
go mod tidy            # la primera vez: completa go.sum
go build -o bin/goqb  ./cmd/goqb     # IDE
go build -o bin/goqbc ./cmd/goqbc    # línea de comandos
go build -tags window -o bin/goqbc ./cmd/goqbc   # opcional: con ventana nativa (ebiten, usa cgo)
go test ./...
```

## Uso

```sh
bin/goqb programa.bas                 # IDE
bin/goqbc run programa.bas [args]     # ejecutar a pantalla completa (args → COMMAND$)
bin/goqbc run -plain programa.bas     # salida de flujo (tuberías, tests)
bin/goqbc run -window programa.bas    # ventana nativa (compilar con -tags window)
bin/goqbc check programa.bas          # verificar sin ejecutar
bin/goqbc check -I ~/qb45/include prog.mak
```

### Teclado del IDE

| Tecla | Acción |
|---|---|
| F1 | Ayuda de teclado |
| F2 / Shift+F2 / Ctrl+F2 | Lista de SUBs / siguiente / anterior |
| F3 | Repetir búsqueda |
| F4 | Pantalla de salida |
| F5 / Shift+F5 | Continuar o ejecutar / ejecutar desde el principio |
| F6 | Alternar entre editor e Immediate |
| F10 o Alt+letra | Menús |
| Ctrl+C (Ctrl+Break) | Detener el programa |
| Shift+flechas, Shift+Del, Ctrl+Ins, Shift+Ins | Seleccionar, cortar, copiar, pegar (también Ctrl+X/C/V) |
| Ctrl+Y / Ctrl+Z / Ins | Borrar línea / deshacer / sobrescribir |

En macOS conviene activar "Option como Meta" en la terminal para que Alt+letra funcione; F10 y el mouse funcionan siempre.

El editor admite selección normal con Shift+flechas y por palabras con
Ctrl+Shift+izquierda/derecha. Alt+Shift+flechas selecciona un bloque rectangular.
También se puede activar **Edit → Column Selection** y usar Shift+flechas para
seleccionar columnas sin depender de Alt. El bloque se puede copiar, cortar,
borrar o reemplazar escribiendo o pegando texto; al copiar y pegar dentro del IDE
conserva su forma rectangular, y Ctrl+Z deshace la operación completa.
Command+Option+Shift+flechas sólo funciona si la terminal transmite los
modificadores; no se puede capturar una combinación que macOS o la terminal
intercepte. Estos atajos usan el portapapeles interno del IDE.

En Terminal de macOS, si Shift+izquierda/derecha selecciona pero
Shift+arriba/abajo no, configurá **Terminal → Configuración → Perfiles →
Teclado** en el perfil que usás. Agregá estas combinaciones con la acción
**Enviar texto**:

| Tecla | Modificador | Texto enviado |
|---|---|---|
| Flecha arriba | Shift | `\033[1;2A` |
| Flecha abajo | Shift | `\033[1;2B` |

`\033` representa el carácter Escape real: en el campo de texto presioná
Escape y luego escribí `[1;2A` o `[1;2B`. Terminal suele mostrar ese carácter
como `\033`. No envíes los caracteres literales de la cadena `\033`.
Sin estos mapeos, Terminal puede enviar las mismas secuencias que para las
flechas normales; el IDE no puede reconstruir el modificador Shift.

## Estado

| Fase | Contenido | Estado |
|---|---|---|
| 1 | Lexer, parser, `$INCLUDE`, `.MAK`, chequeos estructurales | ✅ |
| 2 | Compilador a bytecode, VM, runtime de texto, `goqbc run` | ✅ |
| 3 | IDE: editor con vista F2, menús, F5/F4, layout adaptable, formateo, Immediate | ✅ |
| 4 | Gráficos: SCREEN 1/2/7–13 en bloques, kitty, iTerm2, sixel o ventana | ✅ |
| 5 | Sonido (`SOUND`, `PLAY`, `BEEP`) y eventos (`ON TIMER/KEY/PLAY`) | ✅ **MVP** |
| 6 | Depurador: F8/F10, breakpoints, Watch | pendiente |
| 7 | Make EXE (stubs), CHAIN, RUN "archivo" | pendiente |
| 8 | Extras: `CALL INTERRUPT`, ayuda integrada | pendiente |

### Gráficos

Los modos 1, 2, 7, 8, 9, 10, 11, 12 y 13 con `PSET`, `PRESET`, `LINE` (con `B`/`BF` y estilo), `CIRCLE` (arcos y aspecto), `PAINT` (también con mosaicos), `DRAW`, `GET`/`PUT`, `VIEW`, `WINDOW`, `PMAP`, `POINT`, `PALETTE` (CGA/EGA/VGA), `PCOPY` y páginas. `PRINT` dibuja el texto con una fuente CP437 de 8×8. Cómo se ve depende del backend, que se elige solo:

| Backend | Terminal | Calidad |
|---|---|---|
| `kitty` | kitty, Ghostty | imagen completa |
| `iterm` | iTerm2, WezTerm | imagen completa |
| `sixel` | terminales con sixel (`GOQB_GFX=sixel`) | imagen completa |
| `blocks` | cualquiera (Terminal.app, tmux…) | medios bloques `▀`, menor resolución |
| ventana | `goqbc run -window` (build con `-tags window`) | fidelidad total |

`GOQB_GFX=blocks|kitty|iterm|sixel` fuerza un backend.

### Sonido y eventos

- **Sonido**: `SOUND`, `BEEP` y `PLAY` con todo el macrolenguaje (notas, `O`, `<`/`>`, `L`, `T`, `P`, `N`, `MN`/`ML`/`MS`, `MF`/`MB`) suenan con una onda cuadrada como el parlante de la PC. En macOS y Windows usan oto, sin cgo. En Linux el modo es silencioso pero respeta los tiempos, así que los juegos que usan `PLAY` como pausa siguen andando.
- **Eventos**: `ON TIMER(n)`, `ON KEY(n)` (F1–F12, flechas, teclas definidas con `KEY 15..25`) y `ON PLAY(n)`, con `ON`, `OFF` y `STOP`. `ON PEN`, `ON STRIG`, `ON COM` y `ON UEVENT` se aceptan pero nunca se disparan.

### Probado con

- `testdata/demo_graficos.bas`: gráficos, música de fondo y `ON TIMER`.
- `NIBBLES.BAS` de QBasic 1.1: anda completo, con un detalle. Calcula la velocidad con un `FOR` de 1000 vueltas, que en una máquina actual tarda menos que la resolución de `TIMER` y termina en `Division by zero`; al QBasic real le pasa lo mismo en hardware moderno. Para jugarlo, cambiá la línea `stopTime# = TIMER` por `stopTime# = TIMER: IF stopTime# - startTime# < .05 THEN stopTime# = startTime# + .05`.

### Paquetes

| Paquete | Qué hace |
|---|---|
| `lang/token`, `lang/lexer`, `lang/parser`, `lang/ast` | Frontend del lenguaje |
| `lang/source`, `lang/check` | Carga de módulos/includes y chequeos estructurales |
| `lang/cp437` | CP437 ↔ Unicode (los fuentes UTF-8 se transcodifican al compilar) |
| `lang/format` | Formateo de línea del editor |
| `compiler` | Análisis semántico + bytecode |
| `vm` | Máquina virtual, semántica numérica de QB, intrínsecas, archivos, `PRINT USING` |
| `runtime/screen` | Pantalla de texto virtual (80×25, 40/80 × 25/43/50) y teclado |
| `gfx` | Rasterizador de los modos gráficos, independiente de la presentación |
| `runtime/termrun` | Dibujo con tcell (paleta DOS), backends gráficos, teclas → `INKEY$`, ejecución a pantalla completa |
| `runtime/sound` | Sintetizador de onda cuadrada (oto) |
| `runtime/window` | Ventana nativa con ebiten (build tag `window`) |
| `ide` | El IDE |

### Qué ejecuta hoy

- **Tipos y semántica numérica**: INTEGER de 16 bits con `Overflow`, LONG, SINGLE (float32), DOUBLE, STRING y `STRING * n`, TYPE (también anidados y en arrays). Redondeo bancario en `CINT`/`CLNG` y formato de números como QB (`.5`, `1E+07`, `1D+16`).
- **Variables**: sufijos, DEFtype, `AS`, arrays dinámicos y estáticos, arrays implícitos (0 a 10), `REDIM`, `ERASE`, `LBOUND`/`UBOUND`, `OPTION BASE`.
- **Procedimientos**: SUB y FUNCTION con pasaje por referencia (el default de QB), recursión, `STATIC`, `SHARED`, `DIM SHARED`, `COMMON SHARED`, DEF FN, CONST.
- **Control**: todo el control de flujo, `GOSUB`/`RETURN`, `ON GOTO/GOSUB`, `ON ERROR` + `RESUME` (también `NEXT` y a etiqueta) con `ERR`/`ERL`.
- **Datos y salida**: DATA/READ/RESTORE, PRINT con zonas, `TAB`/`SPC` y `USING`, WRITE, INPUT con "Redo from start", LINE INPUT.
- **Pantalla**: CLS, COLOR, LOCATE, WIDTH, VIEW PRINT, `SCREEN(f, c)`, `POKE`/`PEEK` sobre `&HB800`.
- **Archivos**: modos INPUT, OUTPUT, APPEND, RANDOM (con TYPE o FIELD) y BINARY; EOF, LOF, LOC, SEEK, FREEFILE; KILL, NAME, MKDIR, FILES, SHELL.
- **Intrínsecas**: las funciones de string y matemáticas de QB, RND con el generador de QB 4.5, `MKI$`/`CVI` y el resto de esa familia.

Sin implementar todavía: `CHAIN` y `RUN "archivo"` (fase 7), y `CALL ABSOLUTE`/`INTERRUPT`, `INP`/`OUT` (error 73, que se puede atrapar con `ON ERROR`).

### IDE

- **Vista por procedimiento como QB**: al escribir `SUB Nombre` se crea su vista. F2 lista los procedimientos. Al guardar se genera un `.BAS` plano con los `DECLARE` automáticos, en la misma codificación (CP437 o UTF-8) y con el mismo fin de línea que el original.
- **Al salir de cada línea**: se aplica el formateo y se verifica la sintaxis, con el diálogo de error de QB y el cursor en la columna del error.
- **Ejecución**: F5 ejecuta y al terminar muestra *Press any key to continue*. Ctrl+Break detiene y F5 continúa. Los errores de runtime ubican el cursor en la sentencia que falló.
- **Immediate**: ejecuta sentencias con acceso a las variables del programa.
- **Coloreo de sintaxis** (Options → Syntax Coloring): palabras clave, funciones intrínsecas, strings, números, comentarios, metacomandos y etiquetas.
- **Números de línea** (Options → Line Numbers): numeración por vista, igual que la posición de la barra de estado; un clic en el número lleva al inicio de esa línea.
- **Layout adaptable**: panel lateral de SUBs desde 100×30; el clásico de 80×25; modo compacto (sin Immediate) por debajo de 80×20; aviso por debajo de 40×12.
