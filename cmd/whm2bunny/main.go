package main

import (
	"fmt"
	"os"

	"github.com/mordenhost/whm2bunny/cmd/whm2bunny/commands"
)

func main() {
	if err := commands.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
