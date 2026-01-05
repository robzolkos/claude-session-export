package main

import (
	"fmt"
	"os"

	"github.com/robzolkos/claude-session-export/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
