package otidxcli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func maybePrintViz(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	opts := optionsFrom(cmd)
	if opts == nil {
		return
	}
	if opts.Viz == "" {
		return
	}

	switch opts.Viz {
	case "ascii":
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), VizASCII())
	}
}

func maybePrintExplainQuery(cmd *cobra.Command, q string, workspaceID string, hasFTS bool, hitCount int) {
	if cmd == nil {
		return
	}
	opts := optionsFrom(cmd)
	if opts == nil || !opts.Explain {
		return
	}

	w := cmd.ErrOrStderr()

	fmt.Fprintln(w, "explain:")
	fmt.Fprintf(w, "  db: %s\n", opts.DBPath)
	fmt.Fprintf(w, "  workspace: %s\n", workspaceID)
	fmt.Fprintf(w, "  fts: %v\n", hasFTS)
	fmt.Fprintf(w, "  q: %q\n", q)
	fmt.Fprintf(w, "  unit: %s\n", opts.Unit)
	if opts.Unit == "line" {
		fmt.Fprintf(w, "  context: %d\n", opts.ContextLines)
	}
	fmt.Fprintf(w, "  ignore-case: %v\n", opts.CaseInsensitive)
	if len(opts.IncludeGlobs) > 0 {
		fmt.Fprintf(w, "  include: %s\n", strings.Join(opts.IncludeGlobs, ","))
	}
	if len(opts.ExcludeGlobs) > 0 {
		fmt.Fprintf(w, "  exclude: %s\n", strings.Join(opts.ExcludeGlobs, ","))
	}
	if opts.ScanAll {
		fmt.Fprintln(w, "  scan-all: true")
	}
	fmt.Fprintf(w, "  hits: %d\n", hitCount)
}

