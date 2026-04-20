package main

import (
	"os"

	"github.com/cloudwego/codeflow/internal/codeflow/cli"
)

func main() {
	if err := cli.Execute(os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
