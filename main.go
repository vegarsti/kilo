package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

var originalSttyState bytes.Buffer

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
		return err
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
			return fmt.Errorf("stty %s: %v", option, err)
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
	fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	os.Exit(1)
}

func ctrlKey(b byte) byte {
	return b - 96
}

func main() {
	if err := enableRawMode(); err != nil {
		die(err)
	}
	defer disableRawMode()
	r := bufio.NewReader(os.Stdin)
	for {
		c, err := r.ReadByte()
		if err != nil {
			die(err)
		}
		if c == ctrlKey('q') {
			break
		}
		if iscntrl(c) {
			fmt.Printf("%d\r\n", c)
		} else {
			fmt.Printf("%d ('%c')\r\n", c, c)
		}
	}
}
