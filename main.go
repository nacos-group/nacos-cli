package main

import (
	"os"

	"github.com/nacos-group/nacos-cli/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
