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
	var workers int
	cmd := &cobra.Command{
		Use:   "build [path]",
		Short: "Build (or rebuild) the local SQLite index",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			maybePrintViz(cmd)

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

			var ex *ExplainCollector
			if opts.Explain != "" {
				ex = NewExplainCollector(ExplainOptions{Format: opts.Explain})
			}

			err = indexer.Build(root, opts.DBPath, indexer.Options{
				WorkspaceID:  root,
				Workers:      workers,
				ScanAll:      opts.ScanAll,
				IncludeGlobs: opts.IncludeGlobs,
				ExcludeGlobs: opts.ExcludeGlobs,
				Explain:      ex,
			})
			if err != nil {
				return err
			}

			if ex != nil {
				_ = ex.Emit(cmd.ErrOrStderr())
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&workers, "workers", "j", 0, "number of parallel index workers (default: CPU/2)")
	return cmd
}
