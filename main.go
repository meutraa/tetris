package main

import (
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"math/rand"
	"os"
	"strconv"
	"syscall"
	"time"
	"flag"
	"encoding/binary"
)

const H = 10
const V = 22
const LE = 2
const TE = 2
const LCD = 66 /* Line Clear Delay divided by 5 */
const FPS = 16639267 /* nano seconds per frame, 1,000,000,000/fps */
const SDR = 5 /* Soft Drop rate, 1 line per 5 frames (0.5G) */
const DASD = 12 /* DAS delay in frames, NES = 16 */
const DASR = 3 /* DAS rate in frames, NES = 6 */
const HDLD = 0 /* Hard Drop Lock Delay, NES = ??? */
const ARER = 0 /* Multiplier to new piece delay. NES = 1 */

/* Controls - see linux/include/uapi/linux/input-event-codes.h */
const DROP = 57
const LEFT = 31
const DOWN = 32
const RIGHT = 33
const CROTATE = 37
const ACROTATE = 36

func hasZeros(ar [H]int) bool {
	for _, v := range ar {
		if v == 0 {
			return true
		}
	}
	return false
}

func cord(x int, y int) string {
	return "\033[" + strconv.Itoa(y+TE) + ";" + strconv.Itoa((x+LE) << 1) + "H"
}

func nextStr(h1 int, h2 int, str1 string, str2 string) string {
	return cord(H+h1, (V>>1)+1) + str1 + cord(H+h2, (V>>1)+2) + str2
}

var(
	nextShape = [8]string{nextStr(2, 2, "        ", "        "), nextStr(2, 2, "        ", ""),
		nextStr(2, 2, "  ", "      "), nextStr(4, 2, "  ", "      "),
		nextStr(2, 2, "    ", "    "), nextStr(3, 2, "    ", "    "),
		nextStr(3, 2, "  ", "      "), nextStr(2, 3, "    ", "    ")}

	shapes = [8][4][4][2]int{[4][4][2]int{}, /* Blank */
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

	colors = [8]string{"\033[49m", "\033[46m", "\033[44m", "\033[47m",
		"\033[43m", "\033[42m", "\033[45m", "\033[41m"}

	speeds = []int {48, 43, 38, 33, 28, 23, 18, 13, 8, 6, 5, 5, 5, 4, 4, 4, 3, 3, 3,
			2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

	ARE = [V]int {166, 166, 200, 200, 200, 200, 233, 233, 233, 233, 266, 266, 266, 266,
		      300, 300, 300, 300, 333, 333, 333, 333}

	keys [255]int
	frameString string
	bufKey int
	nextFrame int64
	frame int 
	r = rand.New(rand.NewSource(time.Now().Unix()))
	grid [V][H]int
	curPiece [4][4][2]int
	ghost [4][4][2]int
	curType, curRotation int
	nextType = r.Intn(7) + 1
	level, lines, tlevel, score int
	scores = [4]int{40, 100, 300, 1200}
	exit bool
)

func dasCheck(das *int, last int, code int) bool {
	if last != code {
		(*das) = 0
	} else if code == DOWN && (*das) % SDR == 0 {
		(*das)++
		return true
	} else if (*das) == DASD || ((*das) > DASD && (*das - DASD) % DASR == 0) {
		(*das)++
		return true
	} else {
		(*das)++
		return false
	}
	return true
}

func main() {
	/* Get input device and read it. */
	var kbd string
	flag.StringVar(&kbd, "i", "", "keyboard input file")
	flag.Parse()
	if kbd == "" {
		return
	}
	go bufferInput(kbd)

	/* Configure terminal. */
	oldState, _ := terminal.MakeRaw(0)
	defer terminal.Restore(0, oldState)
	fmt.Print("\033[2J\033[1;1H\033[?25l\033[39m")
	defer fmt.Print("\033[2j\033[?25h\033[49m\033[39m", cord(-LE, V+2))

	/* Draw UI */
	set := cord(-1, 1) + " ┏" + cord(H, 1) + "┓ " +
	      cord(-1, V) + " ┗" + cord(H, V) + "┛ " +
	      cord(3, 0) + "\033[37mLINES-000" +
	      cord(H+2, (V>>1)-6) + "\033[37mSCORE" +
	      cord(H+2, (V>>1)-1) + "\033[37mNEXT" +
	      cord(H+2, (V>>1)+5) + "\033[37mLEVEL" +
	      fmt.Sprintf("\033[37m%s  %.2d", cord(H+2, (V>>1)+6), level) +
	      fmt.Sprintf("\033[37m%s%.6d", cord(H+2, (V>>1)-5), score)
	for y := TE + 2; y <= TE+V-1; y++ {
		set += cord(-1, y-TE) + " ┃" + cord(H, y-TE) + "┃ "
	}
	for x := LE; x <= LE+H-1; x++ {
		set += cord(x-LE, 1) + "━━" + cord(x-LE, V) + "━━"
	}
	fmt.Print(set)


	newPiece()
	var das, lastKey int

	for frame = 1; !exit; frame++ {
		nextFrame = time.Now().UnixNano() + FPS
		if bufKey == DROP {
			can, _ := canDrop()
			s := move(0, can, 0)
			frameString += s
			if s != "" {
				frame = -HDLD
			}
		}
		if keys[DOWN] != 0 || bufKey == DOWN {
			if(lastKey != DOWN) { das = DASD }
			if dasCheck(&das, DOWN, DOWN) {
				frame = 0
			}
			lastKey = DOWN
		} else if keys[LEFT] != 0 || bufKey == LEFT {
			if dasCheck(&das, lastKey, LEFT) {
				frameString += move(-1, 0, 0)
			}
			lastKey = LEFT
		} else if keys[RIGHT] != 0 || bufKey == RIGHT {
			if dasCheck(&das, lastKey, RIGHT) {
				frameString += move(1, 0, 0)
			}
			lastKey = RIGHT
		} else {
			lastKey = 0
		}
		bufKey = 0

		if frame >= 0 && frame % speeds[level] == 0 {
			s := move(0, 1, 0)
			frameString += s
			if s == "" { /* Lock piece in. */
				for _, v := range curPiece[curRotation] {
					grid[v[1]][v[0]] = curType
				}
				nlines := checkLines()
				if nlines > 0 {
					lines += nlines
					frameString += fmt.Sprintf("%s\033[37m%s%.3d", colors[0], cord(6, 0), lines)
					printLevel()
				}
				pause(ARER * ARE[highestRow(curPiece[curRotation])])
				newPiece()
				if !isValid(curPiece[curRotation]) {
					exit = true
				}
			}
		}
		if frameString != "" {
			fmt.Print(frameString)
			frameString = ""
		}
		dt := nextFrame - time.Now().UnixNano()
		if dt > 0 {
			time.Sleep(time.Microsecond * time.Duration(dt / 1000))
		}
	}
}

func pause(duration int) {
	time.Sleep(time.Millisecond * time.Duration(duration))
	nextFrame += int64(duration) * 1000000
}

func printLevel() {
	level = lines / 10
	if tlevel > level {
		level = tlevel
	}
	frameString += fmt.Sprintf("%s\033[37m%s  %.2d", colors[0], cord(H+2, (V>>1)+6), level)
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
		valid = isValid(piece[curRotation])
		if valid {
			save = piece
			count++
		}
	}
	return count, save
}

func move(dx int, dy int, r int) string {
	backup := curPiece
	lastRotation := curRotation
	var str string
	if r != 0 {
		curRotation = (curRotation + r) % 4
	} else {
		for i, v := range curPiece {
			for j, w := range v {
				curPiece[i][j] = [2]int{w[0] + dx, w[1] + dy}
			}
		}
	}
	valid := isValid(curPiece[curRotation])
	if valid && dy == 0 {
		gclear, nghost := getGhost(lastRotation)
		str += gclear + nghost
	}
	if !valid {
		curPiece = backup
		curRotation = lastRotation
		return ""
	}
	return str + printPiece(lastRotation, backup, colors[0], "  ") + printPiece(curRotation, curPiece, colors[curType], "  ")
}

func getGhost(lastRotation int) (string, string) {
	clear := printPiece(lastRotation, ghost, colors[0], "  ")
	nGhost := ""
	_, newGhost := canDrop()
	nGhost = printPiece(curRotation, newGhost, "\033[37m\033[49m", "░░" )
	ghost = newGhost
	return clear, nGhost
}

func printPiece(curRotationation int, piece [4][4][2]int, c string, char string) string {
	set := ""
	for _, v := range piece[curRotationation] {
		if v[1] >= 2 {
			set += c + cord(v[0], v[1]) + char
		}
	}
	return set
}

func newPiece() {
	curRotation = 0
	curType = nextType
	curPiece = shapes[curType]
	nextType = r.Intn(7) + 1
	_, newGhost := getGhost(curRotation)
	frameString += newGhost + colors[0] + nextShape[0] + colors[nextType] + nextShape[nextType] +
		       printPiece(curRotation, curPiece, colors[curType], "  ")
}

func flashLines(ar []int) {
	mid := H >> 1
	for h := 1; h <= mid; h++ {
		set := ""
		for _, v := range ar {
			set += colors[0] + cord(mid-h, v) + "  " +
				colors[0] + cord(mid+h-1, v) + "  "
		}
		fmt.Print(set)
		pause(LCD)
	}
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
		if n - 1 == last {
			last = n
		} else {
			list = append(list, [2]int{first, last})
			first, last = n, n
		}
	}
	list = append(list, [2]int{first, last})
	for _, v := range list {
		s += scores[v[1] - v[0]] * (level + 1)
	}
	return s
}

func reprint() {
	set := ""
	for y := 2; y < V; y++ {
		for x, v := range grid[y] {
			set += colors[v] + cord(x, y) + "  "
		}
	}
	frameString += set
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

func checkLines() int {
	lines := make([]int, 0, 4)
	for i, v := range grid {
		if !hasZeros(v) {
			lines = append(lines, i)
		}
	}
	if len(lines) > 0 {
		flashLines(lines)
		grid = clearLines(lines)
		reprint()
		score += evalScore(lines)
		frameString += fmt.Sprintf("\033[49m\033[37m%s%.6d", cord(H+2, (V>>1)-5), score)
	}
	return len(lines)
}

type Event struct {
	_ syscall.Timeval
	Kind uint16
	Code uint16
	Value uint32
}

func bufferInput(kbd string) {
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
		keys[int(ev.Code)] = int(ev.Value)
		if ev.Value == 1 {
			bufKey = int(ev.Code)
			switch ev.Code {
			case ACROTATE:
				frameString += move(0, 0, 3)
			case CROTATE:
				frameString += move(0, 0, 1)
			case 23:
				tlevel++
				printLevel()
			}
		}
	}
}
