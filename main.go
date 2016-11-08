package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"math/rand"
	"os"
	"syscall"
	"time"
)

/* TODO
 * Holding left during line clear pause causes key to get stuck.
 * Ghost piece is wrong when alternative rotation states occur.
 */

/* Size of playing field. Larger H will cause pieces to spawn off center. */
const H = 10
const V = 22

/* Padding from left edge. */
const LE = 2

/* Padding from top edge. */
const TE = 2

const LCD = 66       /* Line Clear Delay divided by 5 */
const LD = 32        /* Lock Delay in frames. */
const FPS = 16639267 /* nano seconds per frame, 1,000,000,000/fps */
const SDR = 1        /* Soft Drop rate, 1 line per 5 frames (0.5G) */
const DASD = 12      /* DAS delay in frames, NES = 16 */
const DASR = 3       /* DAS rate in frames, NES = 6 */
const ARER = 0       /* Multiplier to new piece delay. NES = 1 */

/* Controls - see linux/include/uapi/linux/input-event-codes.h */
const DROP = 57
const LEFT = 31
const DOWN = 32
const RIGHT = 33
const CROTATE = 37
const ACROTATE = 36
const HOLD = 18

/* Positions of UI elements. */
var scoreX = H + 2
var scoreY = 2
var nextX = H + 2
var nextY = (V >> 1) - 2
var levelX = H + 2
var levelY = (V >> 1) + 2
var holdX = H + 2
var holdY = V - 2

var shapes = [8][4][4][2]int{{}, /* Blank */
	{{{3, 2}, {4, 2}, {5, 2}, {6, 2}}, {{5, 1}, {5, 2}, {5, 3}, {5, 4}}, /* I */
		{{3, 3}, {4, 3}, {5, 3}, {6, 3}}, {{4, 1}, {4, 2}, {4, 3}, {4, 4}}},
	{{{3, 2}, {3, 3}, {4, 3}, {5, 3}}, {{5, 2}, {4, 2}, {4, 3}, {4, 4}}, /* J */
		{{5, 4}, {5, 3}, {4, 3}, {3, 3}}, {{3, 4}, {4, 4}, {4, 3}, {4, 2}}},
	{{{3, 3}, {4, 3}, {5, 3}, {5, 2}}, {{4, 2}, {4, 3}, {4, 4}, {5, 4}}, /* L */
		{{5, 3}, {4, 3}, {3, 3}, {3, 4}}, {{4, 4}, {4, 3}, {4, 2}, {3, 2}}},
	{{{4, 2}, {4, 3}, {5, 2}, {5, 3}}, {{4, 2}, {4, 3}, {5, 2}, {5, 3}}, /* O */
		{{4, 2}, {4, 3}, {5, 2}, {5, 3}}, {{4, 2}, {4, 3}, {5, 2}, {5, 3}}},
	{{{3, 3}, {4, 3}, {4, 2}, {5, 2}}, {{4, 2}, {4, 3}, {5, 3}, {5, 4}}, /* S */
		{{5, 3}, {4, 3}, {4, 4}, {3, 4}}, {{4, 4}, {4, 3}, {3, 3}, {3, 2}}},
	{{{3, 3}, {4, 3}, {5, 3}, {4, 2}}, {{4, 2}, {4, 3}, {4, 4}, {5, 3}}, /* T */
		{{3, 3}, {4, 3}, {5, 3}, {4, 4}}, {{4, 2}, {4, 3}, {4, 4}, {3, 3}}},
	{{{3, 2}, {4, 2}, {4, 3}, {5, 3}}, {{5, 2}, {5, 3}, {4, 3}, {4, 4}}, /* Z */
		{{5, 4}, {4, 4}, {4, 3}, {3, 3}}, {{3, 4}, {3, 3}, {4, 3}, {4, 2}}}}

var colors = [8]string{"\033[49m", "\033[46m", "\033[44m", "\033[47m",
	"\033[43m", "\033[42m", "\033[45m", "\033[41m"}

var speeds = []int{48, 43, 38, 33, 28, 23, 18, 13, 8, 6, 5, 5, 5, 4, 4, 4, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

var ARE = [V]int{166, 166, 200, 200, 200, 200, 233, 233, 233, 233, 266, 266, 266, 266,
	300, 300, 300, 300, 333, 333, 333, 333}

var cKicks = [4][5][2]int{{{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}}, /*0->1*/
	{{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},    /*1->2*/
	{{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},   /*2->3*/
	{{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}}} /*3->0*/

var acKicks = [4][5][2]int{{{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}}, /*0->3*/
	{{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},     /*1->0*/
	{{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}}, /*2->1*/
	{{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}}}  /*3->2*/

var cLKicks = [4][5][2]int{{{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
	{{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
	{{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
	{{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}}}

var acLKicks = [4][5][2]int{{{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
	{{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
	{{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
	{{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}}}

var keys [255][2]bool

//var out string
var r = rand.New(rand.NewSource(time.Now().Unix()))
var grid [V][H]int
var curPiece [4][4][2]int
var ghost [4][4][2]int
var curType int
var rotation int
var holdPiece int
var held bool
var nextType = r.Intn(7) + 1
var level int
var lines int
var score int
var scores = [4]int{40, 100, 300, 1200}
var exit bool

func hasZeros(ar [H]int) bool {
	for _, v := range ar {
		if v == 0 {
			return true
		}
	}
	return false
}

func cord(x int, y int) string {
	return fmt.Sprintf("\033[%d;%dH", y+TE, (x+LE)<<1)
}

func uiPieceString(x int, y int, shape int) string {
	a, b := cord(x, y), cord(x, y+1)
	c, d := colors[0], colors[shape]
	switch shape {
	case 1:
		return a + d + "        " + b + c + "        "
	case 2:
		return a + c + "  " + d + "      " + b + c + "      " + d + "  "
	case 3:
		return a + c + "    " + d + "  " + c + "  " + b + d + "      " + c + "  "
	case 4:
		return a + d + "    " + c + "    " + b + d + "    " + c + "    "
	case 5:
		return a + c + "  " + d + "    " + c + "  " + b + d + "    " + c + "    "
	case 6:
		return a + c + "  " + d + "  " + c + "    " + b + d + "      " + c + "  "
	case 7:
		return a + d + "    " + c + "    " + b + "  " + d + "    " + c + "  "
	}
	return ""
}

func main() {
	/* Get input device and read it. */
	var kbd string
	flag.StringVar(&kbd, "i", "", "keyboard input file")
	flag.Parse()
	if kbd == "" {
		return
	}
	go in(kbd)

	/* Configure terminal. */
	oldState, _ := terminal.MakeRaw(0)
	defer terminal.Restore(0, oldState)
	fmt.Print("\033[2J\033[1;1H\033[?25l\033[39m")
	defer fmt.Print("\033[2j\033[?25h\033[49m\033[39m", cord(-LE, V+2))

	/* Draw UI */
	set := "\033[37m" + cord(-1, 1) + " ┏" + cord(H, 1) + "┓ " +
		cord(-1, V) + " ┗" + cord(H, V) + "┛ " +
		cord(3, 0) + "LINES-000" +
		cord(scoreX, scoreY) + "SCORE" +
		cord(holdX, holdY-2) + "HOLD" +
		cord(nextX, nextY-2) + "NEXT" +
		cord(levelX, levelY) + "LEVEL" +
		fmt.Sprintf("%s  %.2d", cord(levelX, levelY+2), level) +
		fmt.Sprintf("%s%.6d", cord(scoreX, scoreY+2), score)
	for y := TE + 2; y <= TE+V-1; y++ {
		set += cord(-1, y-TE) + " ┃" + cord(H, y-TE) + "┃ "
	}
	for x := LE; x <= LE+H-1; x++ {
		set += cord(x-LE, 1) + "━━" + cord(x-LE, V) + "━━"
	}
	set += newPiece(0)
	fmt.Print(set)

	var das, sddas, lockCount int

	/* Start main loop. */
	for frame := 1; !exit; frame++ {
		var out string
		nextFrame := time.Now().UnixNano() + FPS

		/* Handle key input. */
		if keys[DROP][0] {
			can, _ := canDrop()
			out += move(0, can, rotation)
			lockCount = LD
			keys[DROP] = [2]bool{false, false}
		} else if keys[HOLD][0] && !held {
			buf := holdPiece
			holdPiece = curType
			out += uiPieceString(holdX, holdY, holdPiece) +
				pieceString(curPiece[rotation], colors[0], "  ") +
				pieceString(ghost[rotation], colors[0], "  ") +
				newPiece(buf)
			held = true
			keys[HOLD] = [2]bool{false, false}
		} else {
			var m string

			if keys[LEFT][0] {
				if das > 0 {
					das = 0
				}
				if das == 0 || -das == DASD || (-das > DASD && (-das-DASD)%DASR == 0) {
					m += move(-1, 0, rotation)
				}
				das--
				keys[LEFT][1] = true
			} else if keys[RIGHT][0] {
				if das < 0 {
					das = 0
				}
				if das == 0 || das == DASD || (das > DASD && (das-DASD)%DASR == 0) {
					m += move(1, 0, rotation)
				}
				das++
				keys[RIGHT][1] = true
			} else {
				das = 0
			}

			if keys[ACROTATE][0] {
				m += rotate(false)
				keys[ACROTATE] = [2]bool{false, false}
			} else if keys[CROTATE][0] {
				m += rotate(true)
				keys[CROTATE] = [2]bool{false, false}
			}

			if keys[DOWN][0] {
				if sddas%SDR == 0 {
					frame = 0
				}
				sddas++
				keys[DOWN][1] = true
			} else {
				sddas = 0
			}

			if m != "" {
				lockCount = 0
			}
			out += m
		}

		/* Lock logic */
		if lockCount == LD { /* Lock piece in. */
			for _, v := range curPiece[rotation] {
				grid[v[1]][v[0]] = curType
			}

			/* Check lines. */
			l := make([]int, 0, 4)
			for i, v := range grid {
				if !hasZeros(v) {
					l = append(l, i)
				}
			}
			if len(l) > 0 {
				nextFrame += flashLines(l)
				grid = clearLines(l)
				out += refreshString()
				score += evalScore(l)
				out += fmt.Sprintf("\033[49m\033[37m%s%.6d", cord(scoreX, scoreY+2), score)
			}
			nlines := len(l)

			if nlines > 0 {
				lines += nlines
				out += fmt.Sprintf("%s\033[37m%s%.3d", colors[0], cord(6, 0), lines)
				out += levelString()
			}
			held = false
			out += newPiece(0)
			if !isValid(curPiece[rotation]) {
				exit = true
			}
			lockCount = 0
			nextFrame += pause(ARER * ARE[highestRow(curPiece[rotation])])
		} else if lockCount > 0 {
			lockCount++
		} else if frame >= 0 && frame % speeds[level] == 0 {
			s := move(0, 1, rotation)
			out += s
			if s == "" {
				lockCount++
			}
		}

		/* Print and clear the frame string. */
		fmt.Print(out)

		/* Wait for the next frame time. */
		dt := nextFrame - time.Now().UnixNano()
		if dt > 0 {
			time.Sleep(time.Microsecond * time.Duration(dt/1000))
		}
	}
}

func pause(duration int) int64 {
	time.Sleep(time.Millisecond * time.Duration(duration))
	return int64(duration) * 1000000
}

func levelString() string {
	level = lines / 10
	return fmt.Sprintf("%s\033[37m%s  %.2d", colors[0], cord(levelX, levelY+2), level)
}

func isValid(piece [4][2]int) bool {
	for _, v := range piece {
		if v[0] < 0 || v[0] >= H || v[1] < 0 || v[1] >= V || grid[v[1]][v[0]] != 0 {
			return false
		}
	}
	return true
}

func canDrop() (int, [4][4][2]int) {
	piece := curPiece
	save := piece
	valid := true
	var count int
	for valid {
		for i, v := range piece {
			for j, w := range v {
				piece[i][j] = [2]int{w[0], w[1] + 1}
			}
		}
		valid = isValid(piece[rotation])
		if valid {
			save = piece
			count++
		}
	}
	return count, save
}

func rotate(clockwise bool) string {
	var str string
	var kicks *[5][2]int
	lastRotation := rotation
	switch {
	case clockwise:
		rotation = (rotation + 1) % 4
		switch curType {
		case 1:
			kicks = &cLKicks[rotation]
		default:
			kicks = &cKicks[rotation]
		}
	default:
		rotation = (rotation + 3) % 4
		switch curType {
		case 1:
			kicks = &acLKicks[rotation]
		default:
			kicks = &acKicks[rotation]
		}
	}

	for i := 0; i < len(kicks) && str == ""; i++ {
		str = move((*kicks)[i][0], -(*kicks)[i][1], lastRotation)
	}
	if str == "" {
		rotation = lastRotation
	}
	return str
}

func move(dx int, dy int, lastRotation int) string {
	backup := curPiece
	if dx != 0 || dy != 0 {
		for i, v := range curPiece {
			for j, w := range v {
				curPiece[i][j] = [2]int{w[0] + dx, w[1] + dy}
			}
		}
	}
	valid := isValid(curPiece[rotation])
	if valid {
		return getGhost(lastRotation, true) +
			pieceString(backup[lastRotation], colors[0], "  ") +
			pieceString(curPiece[rotation], colors[curType], "  ")
	} else {
		curPiece = backup
		return ""
	}
}

func getGhost(lastRotation int, clear bool) string {
	s := ""
	if clear {
		s = pieceString(ghost[lastRotation], colors[0], "  ")
	}
	_, ghost = canDrop()
	return s + pieceString(ghost[rotation], "\033[37m\033[49m", "░░")
}

func pieceString(piece [4][2]int, color string, char string) string {
	set := ""
	for _, v := range piece {
		if v[1] >= 2 {
			set += color + cord(v[0], v[1]) + char
		}
	}
	return set
}

func newPiece(shape int) string {
	rotation = 0
	if shape != 0 {
		curType = shape
	} else {
		curType = nextType
		nextType = r.Intn(7) + 1
	}
	curPiece = shapes[curType]
	return getGhost(rotation, false) + uiPieceString(nextX, nextY, nextType) +
		pieceString(curPiece[rotation], colors[curType], "  ")
}

func flashLines(ar []int) int64 {
	mid := H >> 1
	var delay int64
	for h := 1; h <= mid; h++ {
		set := ""
		for _, v := range ar {
			set += colors[0] + cord(mid-h, v) + "  " +
				colors[0] + cord(mid+h-1, v) + "  "
		}
		fmt.Print(set)
		delay += pause(LCD)
	}
	return delay
}

func clearLines(ar []int) [V][H]int {
	var cleared [V][H]int
	var lines [V]int
	pos := V - 1
	for _, line := range ar {
		lines[line] = 1
	}
	for i := V - 1; i >= 0; i-- {
		if lines[i] == 1 {
			continue
		}
		cleared[pos] = grid[i]
		pos--
	}
	return cleared
}

func evalScore(ar []int) int {
	var s int
	list := make([][2]int, 0, 4)
	first, last := ar[0], ar[0]
	for _, n := range ar[1:] {
		if n-1 == last {
			last = n
		} else {
			list = append(list, [2]int{first, last})
			first, last = n, n
		}
	}
	list = append(list, [2]int{first, last})
	for _, v := range list {
		s += scores[v[1]-v[0]] * (level + 1)
	}
	return s
}

func refreshString() string {
	var set string
	for y := 2; y < V; y++ {
		for x, v := range grid[y] {
			set += colors[v] + cord(x, y) + "  "
		}
	}
	return set
}

func highestRow(piece [4][2]int) int {
	min := V
	for _, point := range piece {
		if point[1] < min {
			min = point[1]
		}
	}
	return min
}

type Event struct {
	_     syscall.Timeval
	Kind  uint16
	Code  uint16
	Value uint32
}

func in(kbd string) {
	file, err := os.Open(kbd)
	defer file.Close()
	if err != nil {
		fmt.Print("FAILED TO READ FILE", err)
		exit = true
		return
	}
	var ev Event
	for {
		binary.Read(file, binary.LittleEndian, &ev)
		if ev.Kind != 1 /* EV_KEY */ {
			continue
		}
		if ev.Code == 1 && ev.Value != 0 {
			exit = true
			return
		}
		if ev.Value == 1 {
			keys[int(ev.Code)] = [2]bool{true, false}
		} else if ev.Value == 0 && keys[int(ev.Code)][1] {
			keys[int(ev.Code)] = [2]bool{false, false}
		}
	}
}
