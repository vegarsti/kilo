package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

var originalSttyState bytes.Buffer
var in *bufio.Reader
var out *bufio.Writer

type row struct {
	content string
	render  string
}

type editorConfig struct {
	screenRows    int
	screenCols    int
	cX            int
	cY            int
	rX            int
	numRows       int
	rows          []row
	rowOffset     int
	colOffset     int
	filename      string
	statusMsg     string
	statusMsgTime time.Time
}

var e editorConfig

var (
	editorKeys = struct {
		arrowLeft  int
		arrowRight int
		arrowUp    int
		arrowDown  int
		delete     int
		pageUp     int
		pageDown   int
		home       int
		end        int
	}{
		arrowLeft:  1000,
		arrowRight: 1001,
		arrowUp:    1002,
		arrowDown:  1003,
		delete:     1004,
		pageUp:     1005,
		pageDown:   1006,
		home:       1007,
		end:        1008,
	}
)

var (
	kiloVersion = "0.0.1"
)

var (
	kiloTabSize = 4
)

var exitStatus int

func getSttyState(state *bytes.Buffer) error {
	cmd := exec.Command("stty", "-g")
	cmd.Stdin = os.Stdin
	cmd.Stdout = state
	return cmd.Run()
}

func setSttyState(state *bytes.Buffer) error {
	cmd := exec.Command("stty", state.String())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func enableRawMode() error {
	if err := getSttyState(&originalSttyState); err != nil {
		return fmt.Errorf("get stty: %v", err)
	}
	sttyOptions := []string{
		"cbreak",  // Turn off canonical mode
		"-echo",   // Turn off terminal echoing
		"-isig",   // Turn off Ctrl-C and Ctrl-Z signals
		"-ixon",   // Turn off Ctrl-S and Ctrl-Q
		"-iexten", // Turn off Ctrl-V
		"-icrnl",  // Fix Ctrl-M
		"-opost",  // Turn off all output processing (translation of newlines)
		"-brkint", // Turn off miscellaneous things ...
		"-inpck",
		"-istrip",
	}
	for _, option := range sttyOptions {
		if err := setSttyState(bytes.NewBufferString(option)); err != nil {
			return fmt.Errorf("set stty %s: %v", option, err)
		}
	}
	return nil
}

func editorCreateRow(line []byte) row {
	r := row{}
	r.content = string(line)
	var render []byte
	render = make([]byte, 0)
	idx := 0
	for _, b := range line {
		if b == '\t' {
			render = append(render, ' ')
			idx++
			for idx%kiloTabSize != 0 {
				render = append(render, ' ')
				idx++
			}
		} else {
			render = append(render, b)
			idx++
		}
	}
	r.render = string(render)
	return r
}

func disableRawMode() error {
	if err := setSttyState(&originalSttyState); err != nil {
		return fmt.Errorf("set stty state: %v", err)
	}
	return nil
}

func getCursorPosition() (int, int, error) {
	if _, err := out.Write([]byte("\x1b[6n")); err != nil {
		return 0, 0, fmt.Errorf("query cursor position: %v", err)
	}
	if err := out.Flush(); err != nil {
		return 0, 0, fmt.Errorf("flush: %v", err)
	}
	buffer := make([]byte, 32)
	i := 0
	for {
		c, err := in.ReadByte()
		if err != nil {
			return 0, 0, fmt.Errorf("ReadByte: %v", err)
		}
		if c == 'R' {
			break
		}
		buffer[i] = c
		i++
	}
	if buffer[0] != '\x1b' || buffer[1] != '[' {
		return 0, 0, fmt.Errorf("failed to parse cursor position")
	}
	var rows, cols int
	if _, err := fmt.Sscanf(string(buffer[2:]), "%d;%d", &rows, &cols); err != nil {
		return 0, 0, fmt.Errorf("fmt.Sscanf failed to parse cursor position: %v", err)
	}
	return rows, cols, nil
}

func getWindowSize() (int, int, error) {
	if _, err := out.Write([]byte("\x1b[999C\x1b[999B")); err != nil {
		return 0, 0, err
	}
	rows, cols, err := getCursorPosition()
	if err != nil {
		return 0, 0, fmt.Errorf("getCursorPosition: %v", err)
	}
	return rows, cols, nil
}

func initEditor() error {
	rows, cols, err := getWindowSize()
	if err != nil {
		return fmt.Errorf("getWindowSize: %v", err)
	}
	e.screenRows = rows - 2
	e.screenCols = cols
	return nil
}

func editorOpen(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer f.Close()
	e.filename = filename
	r := bufio.NewReader(f)
	i := 0
	for {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ReadLine: %v", err)
		}
		e.rows = append(e.rows, editorCreateRow(line))
		i++
	}
	e.numRows = len(e.rows)
	return nil
}

func iscntrl(b byte) bool {
	switch {
	case b < 32:
		return true
	case b == 127:
		return true
	default:
		return false
	}
}

func die(err error) {
	out.Write([]byte("\x1b[2J\x1b[H")) // Refresh the screen. Ignore any errors
	out.Flush()
	fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	exitStatus = 1
}

func ctrlKey(b int) int {
	return b & 0b00011111
}

func editorReadKey() (int, error) {
	c, err := in.ReadByte()
	if err != nil {
		return 0, err
	}
	// Read escape sequence
	if c == '\x1b' {
		c0, err := in.ReadByte() // TODO: May need to time out here (after 0.1s)
		if err != nil {
			return 0, fmt.Errorf("ReadByte: %v", err)
		}
		c1, err := in.ReadByte()
		if err != nil {
			return 0, fmt.Errorf("ReadByte: %v", err)
		}
		if c0 != '[' && c0 != '0' {
			return '\x1b', nil
		}
		if c0 == '0' {
			switch c1 {
			case 'H':
				return editorKeys.home, nil
			case 'F':
				return editorKeys.end, nil
			default:
				return '\x1b', nil
			}
		}
		if c1 >= '0' && c1 <= '9' {
			c2, err := in.ReadByte()
			if err != nil {
				return 0, fmt.Errorf("ReadByte: %v", err)
			}
			if c2 != '~' {
				return '\x1b', nil
			}
			switch c1 {
			case '1':
				return editorKeys.home, nil
			case '3':
				return editorKeys.delete, nil
			case '4':
				return editorKeys.end, nil
			case '5':
				return editorKeys.pageUp, nil
			case '6':
				return editorKeys.pageDown, nil
			case '7':
				return editorKeys.home, nil
			case '8':
				return editorKeys.end, nil
			default:
				return '\x1b', nil
			}
		}
		switch c1 {
		case 'A':
			return editorKeys.arrowUp, nil
		case 'B':
			return editorKeys.arrowDown, nil
		case 'C':
			return editorKeys.arrowRight, nil
		case 'D':
			return editorKeys.arrowLeft, nil
		case 'H':
			return editorKeys.home, nil
		case 'F':
			return editorKeys.end, nil
		default:
			return '\x1b', nil
		}
	}
	return int(c), nil
}

func editorMoveCursor(c int) error {
	var row string
	var rowExists bool
	if e.cY >= e.numRows {
		rowExists = false
		row = ""
	}
	if e.cY < e.numRows {
		rowExists = true
		row = e.rows[e.cY].content
	}
	if c == editorKeys.arrowLeft {
		if e.cX != 0 {
			e.cX--
		} else if e.cY > 0 {
			e.cY--
			e.cX = len(e.rows[e.cY].content)
		}
	} else if c == editorKeys.arrowRight {
		if rowExists && e.cX < len(row) {
			e.cX++
		} else if rowExists && e.cX == len(row) {
			e.cY++
			e.cX = 0
		}
	} else if c == editorKeys.arrowUp {
		if e.cY != 0 {
			e.cY--
		}
	} else if c == editorKeys.arrowDown {
		if e.cY < e.numRows {
			e.cY++
		}
	} else {
		return fmt.Errorf("invalid cursor %c", c)
	}
	if e.cY >= e.numRows {
		row = ""
	} else {
		row = e.rows[e.cY].content
	}
	if e.cX > len(row) {
		e.cX = len(row)
	}
	return nil
}

func editorProcessKeypress() error {
	c, err := editorReadKey()
	if err != nil {
		return fmt.Errorf("editorReadKey: %v", err)
	}
	if c == ctrlKey('q') {
		out.Write([]byte("\x1b[2J\x1b[H")) // Ignore potential errors when refreshing screen
		return io.EOF
	}
	if c == editorKeys.arrowUp || c == editorKeys.arrowDown || c == editorKeys.arrowLeft || c == editorKeys.arrowRight {
		if err := editorMoveCursor(c); err != nil {
			return fmt.Errorf("editorMoveCursor: %v", err)
		}
		return nil
	}
	if c == editorKeys.pageUp {
		e.cY = e.rowOffset
		times := e.screenRows
		for i := 0; i < times; i++ {
			if err := editorMoveCursor(editorKeys.arrowUp); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
		}
		return nil
	}
	if c == editorKeys.pageDown {
		e.cY = e.rowOffset + e.screenRows - 1
		times := e.screenRows
		for i := 0; i < times; i++ {
			if err := editorMoveCursor(editorKeys.arrowDown); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
		}
		return nil
	}
	if c == editorKeys.home {
		e.cX = 0
		return nil
	}
	if c == editorKeys.end {
		if e.cY < e.numRows {
			e.cX = len(e.rows[e.cY].content)
		}
		return nil
	}
	return nil
}

func editorDrawRows() error {
	for y := 0; y < e.screenRows; y++ {
		filerow := y + e.rowOffset
		// Stored rows
		if filerow < e.numRows {
			rowLen := len(e.rows[filerow].render) - e.colOffset
			if rowLen < 0 {
				rowLen = 0
			}
			var displayLine string
			if e.colOffset < len(e.rows[filerow].render) {
				displayLine = e.rows[filerow].render[e.colOffset:]
			}
			if rowLen == 0 {
				displayLine = ""
			} else if rowLen > e.screenCols {
				rowLen = e.screenCols
				displayLine = e.rows[filerow].render[e.colOffset : e.colOffset+rowLen]
			}
			if _, err := out.Write([]byte(displayLine)); err != nil {
				return fmt.Errorf("write row: %v", err)
			}
			// Welcome message
		} else if y == e.screenRows/3 && e.numRows == 0 {
			welcome := fmt.Sprintf("Kilo editor -- version %s", kiloVersion)
			if len(welcome) > e.screenCols {
				welcome = welcome[:e.screenCols]
			}
			padding := (e.screenCols - len(welcome)) / 2
			if padding > 0 {
				if _, err := out.Write([]byte("~")); err != nil {
					return fmt.Errorf("write ~: %v", err)
				}
			}
			for p := 0; p < padding-1; p++ {
				if _, err := out.Write([]byte(" ")); err != nil {
					return fmt.Errorf("write ' ': %v", err)
				}
			}
			if _, err := out.Write([]byte(welcome)); err != nil {
				return fmt.Errorf("write newline: %v", err)
			}
			// Left-hand side markers for unused lines
		} else {
			if _, err := out.Write([]byte("~")); err != nil {
				return fmt.Errorf("write ~: %v", err)
			}
		}
		// Write to end of line
		if _, err := out.Write([]byte("\x1b[K")); err != nil {
			return fmt.Errorf("write: %v", err)
		}
		// Newline
		if _, err := out.Write([]byte("\r\n")); err != nil {
			return fmt.Errorf("write newline: %v", err)
		}
	}
	return nil
}

func editorRowCxToRx(r row, cX int) int {
	rx := 0
	for j := 0; j < cX; j++ {
		if r.content[j] == '\t' {
			rx += (kiloTabSize - 1) - (rx % kiloTabSize)
		}
		rx++
	}
	return rx
}

func editorScroll() error {
	e.rX = e.cX
	if e.cY < e.numRows {
		e.rX = editorRowCxToRx(e.rows[e.cY], e.cX)
	}
	if e.cY < e.rowOffset {
		e.rowOffset = e.cY
	}
	if e.cY >= e.rowOffset+e.screenRows {
		e.rowOffset = e.cY - e.screenRows + 1
	}
	if e.rX < e.colOffset {
		e.colOffset = e.rX
	}
	if e.rX >= e.colOffset+e.screenCols {
		e.colOffset = e.rX - e.screenCols + 1
	}
	return nil
}

func editorDrawStatusBar() error {
	// Invert colors
	if _, err := out.Write([]byte("\x1b[7m")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	displayFilename := e.filename
	if displayFilename == "" {
		displayFilename = "[No Name]"
	}
	status := fmt.Sprintf("%.20s - %d lines", displayFilename, e.numRows)
	lineStatus := fmt.Sprintf("%d/%d", e.cY+1, e.numRows)
	if len(status) > e.screenCols {
		status = status[:e.screenCols]
	}
	if _, err := out.Write([]byte(status)); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	i := len(status)
	for i < e.screenCols {
		if e.screenCols-i == len(lineStatus) {
			if _, err := out.Write([]byte(lineStatus)); err != nil {
				return fmt.Errorf("write: %v", err)
			}
			break
		}
		if _, err := out.Write([]byte(" ")); err != nil {
			return fmt.Errorf("write: %v", err)
		}
		i++
	}
	// Switch back to normal formatting
	if _, err := out.Write([]byte("\x1b[m")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	// Newline
	if _, err := out.Write([]byte("\r\n")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	return nil
}

func editorDrawMessageBar() error {
	// Clear
	if _, err := out.Write([]byte("\x1b[K")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	statusMsg := e.statusMsg
	if len(statusMsg) > e.screenCols {
		statusMsg = statusMsg[:e.screenCols]
	}
	fiveSecondsAgo := time.Now().Add(-time.Second * 5)
	if statusMsg != "" && e.statusMsgTime.After(fiveSecondsAgo) {
		if _, err := out.Write([]byte(statusMsg)); err != nil {
			return fmt.Errorf("write: %v", err)
		}
	}
	return nil
}

func editorRefreshScreen() error {
	if err := editorScroll(); err != nil {
		return fmt.Errorf("editorScroll: %v", err)
	}
	if _, err := out.Write([]byte("\x1b[?25l")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if _, err := out.Write([]byte("\x1b[H")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if err := editorDrawRows(); err != nil {
		return fmt.Errorf("editorDrawRows: %v", err)
	}
	if err := editorDrawStatusBar(); err != nil {
		return fmt.Errorf("editorDrawStatusBar: %v", err)
	}
	if err := editorDrawMessageBar(); err != nil {
		return fmt.Errorf("editorDrawMessageBar: %v", err)
	}
	if _, err := out.Write([]byte(fmt.Sprintf("\x1b[%d;%dH", e.cY-e.rowOffset+1, e.rX-e.colOffset+1))); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if _, err := out.Write([]byte("\x1b[?25h")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if err := out.Flush(); err != nil {
		return fmt.Errorf("flush: %v", err)
	}
	return nil
}

func editorSetStatusMessage(msg string) {
	e.statusMsg = msg
	e.statusMsgTime = time.Now()
}

func main() {
	defer func() {
		os.Exit(exitStatus)
	}()
	out = bufio.NewWriter(os.Stdout)
	if err := enableRawMode(); err != nil {
		die(fmt.Errorf("enableRawMode: %v", err))
		return
	}
	defer disableRawMode()
	in = bufio.NewReader(os.Stdin)
	if err := initEditor(); err != nil {
		die(fmt.Errorf("initEditor: %v", err))
		return
	}
	if len(os.Args) > 2 {
		die(fmt.Errorf("usage: kilo [filename]"))
		return
	}
	if len(os.Args) == 2 {
		if err := editorOpen(os.Args[1]); err != nil {
			die(fmt.Errorf("editorOpen: %v", err))
			return
		}
	}
	editorSetStatusMessage("HELP: Ctrl-Q = quit")
	for {
		if err := editorRefreshScreen(); err != nil {
			die(fmt.Errorf("editorRefreshScreen: %v", err))
			return
		}
		if err := editorProcessKeypress(); err != nil {
			if err == io.EOF {
				break
			}
			die(fmt.Errorf("editProcessKeypress: %v", err))
			return
		}
	}
}
