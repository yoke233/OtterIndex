package main

import (
	"os"

	"otterindex/internal/otidxcli"
)

func main() {
	if err := otidxcli.NewRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}
