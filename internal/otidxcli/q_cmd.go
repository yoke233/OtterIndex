package otidxcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"otterindex/internal/core/query"
	"otterindex/internal/index/sqlite"
)

func newQCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "q <q>",
		Short: "Query local index",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isTestMode(cmd) {
				return nil
			}

			maybePrintViz(cmd)

			opts := optionsFrom(cmd)
			if opts == nil {
				return fmt.Errorf("options missing")
			}

			hasFTS := false
			if opts.Explain {
				if s, err := sqlite.Open(opts.DBPath); err == nil {
					hasFTS = s.HasFTS()
					_ = s.Close()
				}
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			workspaceID, err := filepath.Abs(cwd)
			if err != nil {
				return err
			}

			items, err := query.Query(opts.DBPath, workspaceID, args[0], query.Options{
				Unit:            opts.Unit,
				ContextLines:    opts.ContextLines,
				CaseInsensitive: opts.CaseInsensitive,
				IncludeGlobs:    opts.IncludeGlobs,
				ExcludeGlobs:    opts.ExcludeGlobs,
			})
			if err != nil {
				return err
			}

			maybePrintExplainQuery(cmd, args[0], workspaceID, hasFTS, len(items))

			var out string
			switch {
			case opts.Jsonl:
				out = RenderJSONL(items)
			case opts.VimLines:
				out = RenderVim(items)
			default:
				out = RenderDefault(items)
			}

			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
