package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

func main() {
	var c byte
	var err error
	r := bufio.NewReader(os.Stdin)
	for {
		c, err = r.ReadByte()
		if err == io.EOF || string(c) == "q" {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "kilo: %v\n", err)
		}
	}
}
