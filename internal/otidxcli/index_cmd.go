package otidxcli

import (
	"fmt"
	"strings"
	"path/filepath"

	"github.com/spf13/cobra"

	"otterindex/internal/core/indexer"
	"otterindex/internal/index/sqlite"
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

			err = indexer.Build(root, opts.DBPath, indexer.Options{
				WorkspaceID:  root,
				ScanAll:      opts.ScanAll,
				IncludeGlobs: opts.IncludeGlobs,
				ExcludeGlobs: opts.ExcludeGlobs,
			})
			if err != nil {
				return err
			}

			if opts.Explain {
				hasFTS := false
				chunkCount := 0
				if s, err := sqlite.Open(opts.DBPath); err == nil {
					hasFTS = s.HasFTS()
					if n, err := s.CountChunks(root); err == nil {
						chunkCount = n
					}
					_ = s.Close()
				}

				w := cmd.ErrOrStderr()
				fmt.Fprintln(w, "explain:")
				fmt.Fprintln(w, "  action: index build")
				fmt.Fprintf(w, "  root: %s\n", root)
				fmt.Fprintf(w, "  db: %s\n", opts.DBPath)
				fmt.Fprintf(w, "  fts: %v\n", hasFTS)
				if len(opts.IncludeGlobs) > 0 {
					fmt.Fprintf(w, "  include: %s\n", strings.Join(opts.IncludeGlobs, ","))
				}
				if len(opts.ExcludeGlobs) > 0 {
					fmt.Fprintf(w, "  exclude: %s\n", strings.Join(opts.ExcludeGlobs, ","))
				}
				if opts.ScanAll {
					fmt.Fprintln(w, "  scan-all: true")
				}
				fmt.Fprintf(w, "  chunks: %d\n", chunkCount)
			}

			return nil
		},
	}
}
