package main

import (
	"fmt"
	"os"

	"csolve/internal/gui"
	"csolve/internal/solve"
	"csolve/internal/tui"
)

func main() {
	args := os.Args[1:]

	if len(args) >= 1 && (args[0] == "--gui" || args[0] == "-gui") {
		os.Exit(gui.Run(args[1:]))
	}

	if len(args) == 1 && (args[0] == "--tui" || args[0] == "-tui") {
		os.Exit(tui.Run())
	}
	for _, arg := range args {
		if arg == "--tui" || arg == "-tui" {
			fmt.Fprintln(os.Stderr, "error: --tui does not accept other options")
			os.Exit(1)
		}
		if arg == "--gui" || arg == "-gui" {
			fmt.Fprintln(os.Stderr, "error: --gui must be the first argument")
			os.Exit(1)
		}
	}

	os.Exit(solve.Main(args))
}
