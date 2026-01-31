package otidxcli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"otterindex/internal/core/indexer"
)

func newIndexCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Index management",
	}

	cmd.AddCommand(newIndexBuildCommand())
	return cmd
}

func newIndexBuildCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "build [path]",
		Short: "Build (or rebuild) the local SQLite index",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}

			root, err := filepath.Abs(root)
			if err != nil {
				return err
			}

			opts := optionsFrom(cmd)
			if opts == nil {
				return fmt.Errorf("options missing")
			}

			return indexer.Build(root, opts.DBPath, indexer.Options{
				WorkspaceID:  root,
				ScanAll:      opts.ScanAll,
				IncludeGlobs: opts.IncludeGlobs,
				ExcludeGlobs: opts.ExcludeGlobs,
			})
		},
	}
}

