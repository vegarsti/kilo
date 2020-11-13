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

func main() {
	var err error
	err = getSttyState(&originalSttyState)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
	}
	defer setSttyState(&originalSttyState)

	setSttyState(bytes.NewBufferString("cbreak")) // Turn off canonical mode
	setSttyState(bytes.NewBufferString("-echo"))  // Turn off terminal echoing

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
		fmt.Printf("%d", c)
	}
}
