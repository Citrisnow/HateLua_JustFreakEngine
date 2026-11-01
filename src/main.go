package main

import (
	"hate/cli"
	"os"
)

func main() {
	cli.Run(os.Args[1:])
}