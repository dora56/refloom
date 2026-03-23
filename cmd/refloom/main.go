package main

import (
	"os"

	"github.com/dora56/refloom/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
