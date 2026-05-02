// prism/cmd/prism/main.go
package main

import (
	"fmt"
	"os"

	"github.com/prism-data/prism/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
