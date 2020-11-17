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
	row        string
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

func editorOpen() {
	line := "Hello, world!"
	e.row = line
	e.numRows = 1
}

func iscntrl(b byte) bool {
	if b < 32 {
		return true
	}
	if b == 127 {
		return true
	}
	return false
}

func die(err error) {
	out.Write([]byte("\x1b[2J\x1b[H")) // Refresh the screen. Ignore any errors
	fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	os.Exit(1)
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
			if c1 == 'H' {
				return editorKeys.home, nil
			}
			if c1 == 'F' {
				return editorKeys.end, nil
			}
			return '\x1b', nil
		}
		if c1 >= '0' && c1 <= '9' {
			c2, err := in.ReadByte()
			if err != nil {
				return 0, fmt.Errorf("ReadByte: %v", err)
			}
			if c2 != '~' {
				return '\x1b', nil
			}
			if c1 == '1' {
				return editorKeys.home, nil
			}
			if c1 == '3' {
				return editorKeys.delete, nil
			}
			if c1 == '4' {
				return editorKeys.end, nil
			}
			if c1 == '5' {
				return editorKeys.pageUp, nil
			}
			if c1 == '6' {
				return editorKeys.pageDown, nil
			}
			if c1 == '7' {
				return editorKeys.home, nil
			}
			if c1 == '8' {
				return editorKeys.end, nil
			}
			return '\x1b', nil
		}
		if c1 == 'A' {
			return editorKeys.arrowUp, nil
		}
		if c1 == 'B' {
			return editorKeys.arrowDown, nil
		}
		if c1 == 'C' {
			return editorKeys.arrowRight, nil
		}
		if c1 == 'D' {
			return editorKeys.arrowLeft, nil
		}
		if c1 == 'H' {
			return editorKeys.home, nil
		}
		if c1 == 'F' {
			return editorKeys.end, nil
		}
		return '\x1b', nil
	}
	return int(c), nil
}

func editorMoveCursor(c int) error {
	if c == editorKeys.arrowLeft {
		if e.cX != 0 {
			e.cX--
		}
		return nil
	}
	if c == editorKeys.arrowRight {
		if e.cX != e.screenCols-1 {
			e.cX++
		}
		return nil
	}
	if c == editorKeys.arrowUp {
		if e.cY != 0 {
			e.cY--
		}
		return nil
	}
	if c == editorKeys.arrowDown {
		if e.cY != e.screenRows-1 {
			e.cY++
		}
		return nil
	}
	return fmt.Errorf("invalid cursor %c", c)
}

func editorProcessKeypress() error {
	c, err := editorReadKey()
	if err != nil {
		return fmt.Errorf("editorReadKey: %v", err)
	}
	if c == ctrlKey('q') {
		out.Write([]byte("\x1b[2J\x1b[H")) // Ignore errors when refreshing screen
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
			editorMoveCursor(editorKeys.arrowUp)
		}
	}
	if c == editorKeys.pageDown {
		for e.cY < e.screenRows-1 {
			editorMoveCursor(editorKeys.arrowDown)
		}
	}
	if c == editorKeys.home {
		for e.cX > 0 {
			editorMoveCursor(editorKeys.arrowLeft)
		}
		return nil
	}
	if c == editorKeys.end {
		for e.cX < e.screenCols-1 {
			editorMoveCursor(editorKeys.arrowRight)
		}
		return nil
	}
	return nil
}

func editorDrawRows() error {
	for y := 0; y < e.screenRows; y++ {
		// Stored rows
		if y < e.numRows {
			if len(e.row) > e.screenCols {
				e.row = e.row[:e.screenCols]
			}
			if _, err := out.Write([]byte(e.row)); err != nil {
				return fmt.Errorf("write row: %v", err)
			}
			// Welcome message
		} else if y == e.screenRows/3 {
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

func editorRefreshScreen() error {
	if _, err := out.Write([]byte("\x1b[?25l")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if _, err := out.Write([]byte("\x1b[H")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if err := editorDrawRows(); err != nil {
		return fmt.Errorf("editorDrawRows: %v", err)
	}
	if _, err := out.Write([]byte(fmt.Sprintf("\x1b[%d;%dH", e.cY+1, e.cX+1))); err != nil {
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
	in = bufio.NewReader(os.Stdin)
	out = bufio.NewWriter(os.Stdout)
	if err := enableRawMode(); err != nil {
		die(fmt.Errorf("enableRawMode: %v", err))
	}
	defer disableRawMode()
	if err := initEditor(); err != nil {
		die(fmt.Errorf("initEditor: %v", err))
	}
	editorOpen()
	for {
		if err := editorRefreshScreen(); err != nil {
			die(fmt.Errorf("editorRefreshScreen: %v", err))
		}
		if err := editorProcessKeypress(); err != nil {
			if err == io.EOF {
				break
			}
			die(fmt.Errorf("editProcessKeypress: %v", err))
		}
	}
}
