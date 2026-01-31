package otidxcli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"otterindex/internal/version"
)

func NewRootCommand() *cobra.Command {
	opts := newDefaultOptions()
	cmd := &cobra.Command{
		Use:   "otidx",
		Short: "OtterIndex local index/search tool",
		RunE: func(cmd *cobra.Command, args []string) error {
			if isTestMode(cmd) {
				return nil
			}

			opts := optionsFrom(cmd)
			if opts == nil {
				return fmt.Errorf("options missing")
			}

			if opts.ListDatabases {
				return listDatabases(cmd)
			}
			if opts.Viz != "" {
				maybePrintViz(cmd)
				return nil
			}

			return cmd.Help()
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.Version = version.String()
	cmd.InitDefaultVersionFlag()
	if f := cmd.Flags().Lookup("version"); f != nil {
		f.Shorthand = "v"
	}

	withOptionsContext(cmd, opts)
	bindFlags(cmd, opts)

	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if opts := optionsFrom(cmd); opts != nil {
			return opts.Prepare()
		}
		return nil
	}

	cmd.AddCommand(newIndexCommand())
	cmd.AddCommand(newQCommand())
	return cmd
}

func listDatabases(cmd *cobra.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	pattern := filepath.Join(cwd, ".otidx", "*.db")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	sort.Strings(matches)
	for _, p := range matches {
		rel := p
		if r, err := filepath.Rel(cwd, p); err == nil {
			rel = filepath.ToSlash(r)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), rel)
	}
	return nil
}
