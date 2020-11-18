package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

var originalSttyState bytes.Buffer
var in *bufio.Reader
var out *bufio.Writer

type editorConfig struct {
	screenRows int
	screenCols int
	cX         int
	cY         int
	numRows    int
	row        []string
	rowOffset  int
	colOffset  int
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
	e.screenRows = rows
	e.screenCols = cols
	return nil
}

func editorOpen(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("ReadLine: %v", err)
		}
		e.row = append(e.row, string(line))
	}
	e.numRows = len(e.row)
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
	if c == editorKeys.arrowLeft {
		if e.cX != 0 {
			e.cX--
		}
	} else if c == editorKeys.arrowRight {
		if e.cY < e.numRows {
			row = e.row[e.cY]
		}
		if row != "" && e.cX < len(row) {
			e.cX++
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
		row = e.row[e.cY]
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
		for e.cY > 0 {
			if err := editorMoveCursor(editorKeys.arrowUp); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
		}
		return nil
	}
	if c == editorKeys.pageDown {
		for e.cY < e.numRows {
			if err := editorMoveCursor(editorKeys.arrowDown); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
		}
		return nil
	}
	if c == editorKeys.home {
		for e.cX > 0 {
			if err := editorMoveCursor(editorKeys.arrowLeft); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
		}
		return nil
	}
	if c == editorKeys.end {
		for e.cX < e.screenCols-1 {
			if err := editorMoveCursor(editorKeys.arrowRight); err != nil {
				return fmt.Errorf("editorMoveCursor: %v", err)
			}
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
			rowLen := len(e.row[filerow]) - e.colOffset
			if rowLen < 0 {
				rowLen = 0
			}
			var displayLine string
			if e.colOffset < len(e.row[filerow]) {
				displayLine = e.row[filerow][e.colOffset:]
			}
			if rowLen == 0 {
				displayLine = ""
			} else if rowLen > e.screenCols {
				rowLen = e.screenCols
				displayLine = e.row[filerow][e.colOffset : e.colOffset+rowLen]
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
		if y < e.screenRows-1 {
			if _, err := out.Write([]byte("\r\n")); err != nil {
				return fmt.Errorf("write newline: %v", err)
			}
		}
	}
	return nil
}

func editorScroll() error {
	if e.cY < e.rowOffset {
		e.rowOffset = e.cY
	}
	if e.cY >= e.rowOffset+e.screenRows {
		e.rowOffset = e.cY - e.screenRows + 1
	}
	if e.cX < e.colOffset {
		e.colOffset = e.cX
	}
	if e.cX >= e.colOffset+e.screenCols {
		e.colOffset = e.cX - e.screenCols + 1
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
	if _, err := out.Write([]byte(fmt.Sprintf("\x1b[%d;%dH", e.cY-e.rowOffset+1, e.cX-e.colOffset+1))); err != nil {
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
