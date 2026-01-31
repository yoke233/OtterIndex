package otidxcli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"otterindex/internal/core/query"
	"otterindex/internal/index/sqlite"
)

func newQCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "q <query...>",
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

			q := strings.Join(args, " ")

			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			workspaceID, err := filepath.Abs(cwd)
			if err != nil {
				return err
			}

			defaultShow := !opts.Jsonl && !opts.VimLines && !opts.Compact
			wantWorkspaceRoot := opts.Show || defaultShow
			workspaceRoot := ""
			if wantWorkspaceRoot {
				if s, err := sqlite.Open(opts.DBPath); err == nil {
					if ws, err := s.GetWorkspace(workspaceID); err == nil {
						workspaceRoot = ws.Root
					}
					_ = s.Close()
				}
			}

			var ex *ExplainCollector
			if opts.Explain != "" {
				ex = NewExplainCollector(ExplainOptions{Format: opts.Explain})
			}

			qopts := query.Options{
				Unit:            opts.Unit,
				ContextLines:    opts.ContextLines,
				CaseInsensitive: opts.CaseInsensitive,
				IncludeGlobs:    opts.IncludeGlobs,
				ExcludeGlobs:    opts.ExcludeGlobs,
				Limit:           opts.Limit,
				Offset:          opts.Offset,
				Explain:         ex,
			}

			var items []ResultItem
			if opts.Cache {
				cache := query.NewQueryCache(opts.CacheSize)

				s, err := sqlite.Open(opts.DBPath)
				if err != nil {
					return err
				}
				ver, err := s.GetVersion(workspaceID)
				_ = s.Close()
				if err != nil {
					return err
				}

				items, err = query.QueryWithCache(cache, ver, workspaceID, q, qopts, func() ([]ResultItem, error) {
					return query.Query(opts.DBPath, workspaceID, q, qopts)
				})
			} else {
				items, err = query.Query(opts.DBPath, workspaceID, q, qopts)
			}
			if err != nil {
				return err
			}

			if ex != nil {
				_ = ex.Emit(cmd.ErrOrStderr())
			}

			var out string
			if wantWorkspaceRoot && workspaceRoot == "" {
				workspaceRoot = workspaceID
			}

			switch {
			case opts.Jsonl:
				if opts.Show {
					AttachText(workspaceRoot, items)
				}
				out = RenderJSONL(items)
			case opts.VimLines:
				out = RenderVim(items)
			case opts.Compact:
				out = RenderDefault(items)
			default:
				out = RenderShow(workspaceRoot, items)
			}

			_, _ = fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
