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
var reader *bufio.Reader
var writer *bufio.Writer

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
	writer.Write([]byte("\x1b[2J\x1b[H")) // Refresh the screen. Ignore any errors
	fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	os.Exit(1)
}

func ctrlKey(b byte) byte {
	return b & 0b00011111
}

func editorReadKey() (byte, error) {
	c, err := reader.ReadByte()
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
		writer.Write([]byte("\x1b[2J\x1b[H")) // Refresh the screen. Ignore any errors
		return io.EOF
	}
	return nil
}

func editorDrawRows() error {
	for y := 0; y < 24; y++ {
		if _, err := writer.Write([]byte("~\r\n")); err != nil {
			return fmt.Errorf("write: %v", err)
		}
	}
	return nil
}

func editorRefreshScreen() error {
	if _, err := writer.Write([]byte("\x1b[2J")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if _, err := writer.Write([]byte("\x1b[H")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if err := editorDrawRows(); err != nil {
		return fmt.Errorf("editorDrawRows: %v", err)
	}
	if _, err := writer.Write([]byte("\x1b[H")); err != nil {
		return fmt.Errorf("write: %v", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush: %v", err)
	}
	return nil
}

func main() {
	writer = bufio.NewWriter(os.Stdout)
	if err := enableRawMode(); err != nil {
		die(fmt.Errorf("enableRawMode: %v", err))
	}
	defer disableRawMode()
	reader = bufio.NewReader(os.Stdin)
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
