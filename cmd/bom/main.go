package main

import (
	"fmt"
	"os"

	"github.com/codesphere-cloud/bom/internal/cli"
)

func main() {
	application := cli.New(os.Stdin, os.Stdout, os.Stderr)
	if err := application.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
