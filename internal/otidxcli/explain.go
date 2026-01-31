package otidxcli

import (
	"fmt"

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
