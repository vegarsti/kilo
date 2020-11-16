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
}

var e editorConfig
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
		return 0, 0, fmt.Errorf("ask for cursor position: %v", err)
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

func initEditor() error {
	if _, err := out.Write([]byte("\x1b[999C\x1b[999B")); err != nil {
		return err
	}
	rows, cols, err := getCursorPosition()
	if err != nil {
		return fmt.Errorf("getCursorPosition: %v", err)
	}
	e.screenRows = rows
	e.screenCols = cols
	return nil
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

func ctrlKey(b byte) byte {
	return b & 0b00011111
}

func editorReadKey() (byte, error) {
	c, err := in.ReadByte()
	if err != nil {
		return 0, err
	}
	return c, nil
}

func editorProcessKeypress() error {
	c, err := editorReadKey()
	if err != nil {
		return fmt.Errorf("editorReadKey: %v", err)
	}
	if c == ctrlKey('q') {
		out.Write([]byte("\x1b[2J\x1b[H")) // Refresh the screen. Ignore any errors
		return io.EOF
	}
	return nil
}

func editorDrawRows() error {
	for y := 0; y < e.screenRows; y++ {
		if y == e.screenRows/3 {
			welcome := fmt.Sprintf("Kilo editor -- version %s", kiloVersion)
			if len(welcome) > e.screenCols {
				welcome = welcome[:e.screenCols]
			}
			if _, err := out.Write([]byte(welcome)); err != nil {
				return fmt.Errorf("write newline: %v", err)
			}
		} else {
			if _, err := out.Write([]byte("~")); err != nil {
				return fmt.Errorf("write ~: %v", err)
			}
		}
		if _, err := out.Write([]byte("\x1b[K")); err != nil {
			return fmt.Errorf("write: %v", err)
		}
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
	if _, err := out.Write([]byte("\x1b[H")); err != nil {
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
