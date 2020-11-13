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

func iscntrl(b byte) bool {
	if b < 32 {
		return true
	}
	if b == 127 {
		return true
	}
	return false
}

func main() {
	var err error
	err = getSttyState(&originalSttyState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	}
	defer setSttyState(&originalSttyState)

	setSttyState(bytes.NewBufferString("cbreak")) // Turn off canonical mode
	setSttyState(bytes.NewBufferString("-echo"))  // Turn off terminal echoing
	setSttyState(bytes.NewBufferString("-isig"))  // Turn off Ctrl-C and Ctrl-Z signals

	r := bufio.NewReader(os.Stdin)
	var c byte
	for {
		c, err = r.ReadByte()
		if err != nil {
			fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
		}
		if c == 'q' {
			break
		}
		if iscntrl(c) {
			fmt.Printf("%d\n", c)
		} else {
			fmt.Printf("%d ('%c')\n", c, c)
		}
	}
}
