package main

import (
	"os"

	"otterindex/internal/otidxcli"
)

func main() {
	cmd := otidxcli.NewRootCommand()
	cmd.SetArgs(otidxcli.RewriteArgsForImplicitQ(cmd, os.Args[1:]))
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
