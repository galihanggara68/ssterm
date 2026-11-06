// Command ssterm turns terminal output into a Screenshot.
package main

import (
	"os"

	"ssterm/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
