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

/* TODO Holding left during line clear pause causes key to get stuck.
 * Ghost piece is wrong when alternative rotation states occur. */

const (
	lineClearDelay = 66       /* *5 for real value */
	lockDelay      = 32       /* in frames */
	fps            = 16639267 /* nano seconds per frame, 1,000,000,000/fps */
	softDropRate   = 1        /* frames per drop. */
	dasDelay       = 12       /* in frames, NES = 16 */
	dasRate        = 3        /* DAS rate in frames, NES = 6 */

	/* Controls - see linux/include/uapi/linux/input-event-codes.h */
	softDropKey, hardDropKey      = 32, 57
	leftKey, rightKey             = 31, 33
	rotateLeftKey, rotateRightKey = 36, 37
	holdKey                       = 18

	/* UI positions */
	width, height  = 10, 22
	scoreX, scoreY = width + 2, 2
	nextX, nextY   = width + 2, (height >> 1) - 2
	levelX, levelY = width + 2, (height >> 1) + 2
	holdX, holdY   = width + 2, height - 2
)

var (
	shapes = [8][4][4][2]int{{}, /* Blank */
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

	speeds = [...]int{48, 43, 38, 33, 28, 23, 18, 13, 8, 6, 5, 5, 5, 4, 4, 4, 3, 3, 3,
		2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}

	/* cKicks, acKicks, cLKicks, acLKicks */
	kicks = [4][4][5][2]int{{{{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}}, /*0->1*/
		{{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},     /*1->2*/
		{{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},    /*2->3*/
		{{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}}}, /*3->0*/

		{{{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}}, /*0->3*/
			{{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},     /*1->0*/
			{{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}}, /*2->1*/
			{{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}}}, /*3->2*/

		{{{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
			{{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
			{{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
			{{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}}},

		{{{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
			{{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
			{{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
			{{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}}}}

	keyStates                    [255][2]bool
	r                            = rand.New(rand.NewSource(time.Now().Unix()))
	grid                         [height][width]int
	curPiece, ghost              [4][4][2]int
	curType, rotation, holdPiece int
	exit, held                   bool
	nextType                     = r.Intn(7) + 1
	scores                       = [4]int{40, 100, 300, 1200}
)

func cord(x int, y int) string {
	return fmt.Sprintf("\033[%d;%dH", y+2, (x+2)<<1)
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

	var lines, level, score, das, sddas, lockCount int

	/* Configure terminal. */
	t, _ := terminal.MakeRaw(0)
	defer terminal.Restore(0, t)
	fmt.Print("\033[2J\033[1;1H\033[?25l\033[39m")
	defer fmt.Print("\033[2j\033[?25h\033[49m\033[39m", cord(-2, height+2))

	/* Draw UI */
	frameBuffer := "\033[37m" + cord(-1, 1) + " ┏" + cord(width, 1) + "┓ " +
		cord(-1, height) + " ┗" + cord(width, height) + "┛ " +
		cord(3, 0) + "LINES-000" +
		cord(scoreX, scoreY) + "SCORE" +
		cord(holdX, holdY-2) + "HOLD" +
		cord(nextX, nextY-2) + "NEXT" +
		cord(levelX, levelY) + "LEVEL" +
		fmt.Sprintf("%s  %.2d", cord(levelX, levelY+2), level) +
		fmt.Sprintf("%s%.6d", cord(scoreX, scoreY+2), score)
	for y := 4; y <= height+1; y++ {
		frameBuffer += cord(-1, y-2) + " ┃" + cord(width, y-2) + "┃ "
	}
	for x := 2; x <= width+1; x++ {
		frameBuffer += cord(x-2, 1) + "━━" + cord(x-2, height) + "━━"
	}
	frameBuffer += newPiece(0)
	fmt.Print(frameBuffer)

	/* Start main loop. */
	for frame := 1; !exit; frame++ {
		frameBuffer = ""
		nextFrame := time.Now().UnixNano() + fps

		/* Handle key input. */
		if keyStates[hardDropKey][0] {
			can, _ := canDrop()
			frameBuffer += move(0, can, rotation)
			lockCount = lockDelay
			keyStates[hardDropKey] = [2]bool{false, false}
		} else if keyStates[holdKey][0] && !held {
			buf := holdPiece
			holdPiece = curType
			frameBuffer += uiPieceString(holdX, holdY, holdPiece) +
				pieceString(curPiece[rotation], colors[0], "  ") +
				pieceString(ghost[rotation], colors[0], "  ") +
				newPiece(buf)
			held = true
			keyStates[holdKey] = [2]bool{false, false}
		} else {
			var m string

			if keyStates[leftKey][0] {
				if das > 0 {
					das = 0
				}
				if das == 0 || -das == dasDelay || (-das > dasDelay && (-das-dasDelay)%dasRate == 0) {
					m += move(-1, 0, rotation)
				}
				das--
				keyStates[leftKey][1] = true
			} else if keyStates[rightKey][0] {
				if das < 0 {
					das = 0
				}
				if das == 0 || das == dasDelay || (das > dasDelay && (das-dasDelay)%dasRate == 0) {
					m += move(1, 0, rotation)
				}
				das++
				keyStates[rightKey][1] = true
			} else {
				das = 0
			}

			if keyStates[rotateLeftKey][0] {
				m += rotate(false)
				keyStates[rotateLeftKey] = [2]bool{false, false}
			} else if keyStates[rotateRightKey][0] {
				m += rotate(true)
				keyStates[rotateRightKey] = [2]bool{false, false}
			}

			if keyStates[softDropKey][0] {
				if sddas%softDropRate == 0 {
					frame = 0
				}
				sddas++
				keyStates[softDropKey][1] = true
			} else {
				sddas = 0
			}

			if m != "" {
				lockCount = 0
			}
			frameBuffer += m
		}

		/* Lock logic */
		if lockCount == lockDelay { /* Lock piece in. */
			for _, v := range curPiece[rotation] {
				grid[v[1]][v[0]] = curType
			}

			/* Check lines and add score. */
			l := make([]int, 0, 4)
			lc, last := 0, -1
			for i, v := range grid {
				var zeros bool
				for _, w := range v {
					if w == 0 {
						zeros = true
						break
					}
				}
				if !zeros {
					l = append(l, i)
					if lc == 0 || last+1 == i {
						lc++
					}
					last = i
				}
				if zeros || height-1 == i {
					if lc > 0 {
						score += (level + 1) * scores[lc-1]
					}
					lc = 0
				}
			}

			if len(l) > 0 {
				flashLines(l)
				grid = clearLines(l)
				frameBuffer += refreshString()
				//score += evalScore(level, l)
				frameBuffer += fmt.Sprintf("\033[49m\033[37m%s%.6d", cord(scoreX, scoreY+2), score)
			}

			if len(l) > 0 {
				lines += len(l)
				level = lines / 10
				frameBuffer += fmt.Sprintf("%s\033[37m%s%.3d", colors[0], cord(6, 0), lines)
				frameBuffer += fmt.Sprintf("%s\033[37m%s  %.2d", colors[0], cord(levelX, levelY+2), level)
			}
			held = false
			frameBuffer += newPiece(0)
			if !isValid(curPiece[rotation]) {
				exit = true
			}
			lockCount = 0
		} else if lockCount > 0 {
			lockCount++
		} else if frame >= 0 && frame%speeds[level] == 0 {
			s := move(0, 1, rotation)
			frameBuffer += s
			if s == "" {
				lockCount++
			}
		}

		fmt.Print(frameBuffer)

		/* Wait for the next frame time. */
		dt := nextFrame - time.Now().UnixNano()
		if dt > 0 {
			time.Sleep(time.Microsecond * time.Duration(dt/1000))
		}
	}
}

func isValid(piece [4][2]int) bool {
	for _, v := range piece {
		if v[0] < 0 || v[0] >= width || v[1] < 0 || v[1] >= height || grid[v[1]][v[0]] != 0 {
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
	lastRotation := rotation
	var k int
	if clockwise {
		rotation = (rotation + 1) % 4
	} else {
		rotation = (rotation + 3) % 4
		k++
	}
	if curType == 1 {
		k += 2
	}

	for _, v := range kicks[k][rotation] {
		str := move(v[0], -v[1], lastRotation)
		if str != "" {
			return str
		}
	}
	rotation = lastRotation
	return ""
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
	}
	curPiece = backup
	return ""
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

func flashLines(ar []int) {
	mid := width >> 1
	for h := 1; h <= mid; h++ {
		set := ""
		for _, v := range ar {
			set += colors[0] + cord(mid-h, v) + "  " +
				colors[0] + cord(mid+h-1, v) + "  "
		}
		fmt.Print(set)
		time.Sleep(time.Millisecond * lineClearDelay)
	}
}

func clearLines(ar []int) [height][width]int {
	var cleared [height][width]int
	var lines [height]int
	pos := height - 1
	for _, line := range ar {
		lines[line] = 1
	}
	for i := height - 1; i >= 0; i-- {
		if lines[i] == 1 {
			continue
		}
		cleared[pos] = grid[i]
		pos--
	}
	return cleared
}

func refreshString() string {
	var set string
	for y := 2; y < height; y++ {
		for x, v := range grid[y] {
			set += colors[v] + cord(x, y) + "  "
		}
	}
	return set
}

type keyEvent struct {
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
	var ev keyEvent
	for !exit {
		err = binary.Read(file, binary.LittleEndian, &ev)
		if err == nil && ev.Kind == 1 {
			if ev.Value == 1 {
				keyStates[int(ev.Code)] = [2]bool{true, false}
			} else if ev.Value == 0 && keyStates[int(ev.Code)][1] {
				keyStates[int(ev.Code)] = [2]bool{false, false}
			}
			if ev.Code == 1 {
				exit = true
			}
		}
	}
}
