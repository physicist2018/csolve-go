package main

import (
	"os"

	"csolve/internal/solve"
)

func main() {
	os.Exit(solve.Main(os.Args[1:]))
}
