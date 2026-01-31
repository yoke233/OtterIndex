package otidxcli

import (
	"github.com/spf13/cobra"

	"otterindex/internal/version"
)

func NewRootCommand() *cobra.Command {
	opts := newDefaultOptions()
	cmd := &cobra.Command{
		Use:   "otidx",
		Short: "OtterIndex local index/search tool",
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version.String()

	withOptionsContext(cmd, opts)
	bindFlags(cmd, opts)

	cmd.AddCommand(newQCommand())
	return cmd
}

func newQCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "q <q>",
		Short: "Query (placeholder)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts := optionsFrom(cmd); opts != nil {
				opts.normalize()
			}
			return printTodo(cmd)
		},
	}
}
