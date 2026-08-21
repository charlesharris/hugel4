// Command hugel is the local interface to the garden.
package main

import (
	"fmt"
	"os"

	"github.com/charris/hugel/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "hugel: %v\n", err)
		os.Exit(1)
	}
}
