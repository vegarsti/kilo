package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
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
	var sttyState bytes.Buffer
	err := getSttyState(&sttyState)
	if err != nil {
		return err
	}
	setSttyState(bytes.NewBufferString("cbreak"))  // Turn off canonical mode
	setSttyState(bytes.NewBufferString("-echo"))   // Turn off terminal echoing
	setSttyState(bytes.NewBufferString("-isig"))   // Turn off Ctrl-C and Ctrl-Z signals
	setSttyState(bytes.NewBufferString("-ixon"))   // Turn off Ctrl-S and Ctrl-Q
	setSttyState(bytes.NewBufferString("-iexten")) // Turn off Ctrl-V
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

func main() {
	if err := enableRawMode(); err != nil {
		fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	}
	r := bufio.NewReader(os.Stdin)
	for {
		c, err := r.ReadByte()
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
