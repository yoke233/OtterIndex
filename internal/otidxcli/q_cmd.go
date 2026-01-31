package otidxcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"otterindex/internal/core/query"
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

			opts := optionsFrom(cmd)
			if opts == nil {
				return fmt.Errorf("options missing")
			}

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			workspaceID, err := filepath.Abs(cwd)
			if err != nil {
				return err
			}

			items, err := query.Query(opts.DBPath, workspaceID, args[0], opts.Unit, opts.ContextLines, opts.CaseInsensitive)
			if err != nil {
				return err
			}

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

