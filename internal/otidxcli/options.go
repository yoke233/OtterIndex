package otidxcli

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type Options struct {
	DBPath          string
	ScanAll         bool
	IncludeGlobs    []string
	ExcludeGlobs    []string
	CaseInsensitive bool
	ContextLines    int
	Unit            string
	NoBanner        bool
	VimLines        bool
	Theme           string
	Jsonl           bool
	Explain         bool
	Viz             string
	ListDatabases   bool

	colorblind   bool
	noColor      bool
	highContrast bool
}

func (o *Options) Prepare() error {
	o.normalize()

	if strings.TrimSpace(o.DBPath) == "" {
		return fmt.Errorf("database path is required")
	}
	if o.ContextLines < 0 {
		return fmt.Errorf("context lines must be >= 0")
	}

	switch o.Unit {
	case "line", "block", "file":
	default:
		return fmt.Errorf("invalid --unit %q (expected: line|block|file)", o.Unit)
	}

	if o.Viz != "" {
		switch o.Viz {
		case "ascii":
		default:
			return fmt.Errorf("invalid --viz %q (expected: ascii)", o.Viz)
		}
	}

	return nil
}

func (o *Options) normalize() {
	o.Theme = "default"
	if o.colorblind {
		o.Theme = "colorblind"
	}
	if o.highContrast {
		o.Theme = "high-contrast"
	}
	if o.noColor {
		o.Theme = "none"
	}

	o.Unit = strings.TrimSpace(o.Unit)
	if o.Unit == "" {
		o.Unit = "block"
	}
}

type optionsKey struct{}

func optionsFrom(cmd *cobra.Command) *Options {
	if cmd == nil {
		return nil
	}
	root := cmd.Root()
	if root == nil {
		root = cmd
	}
	v := root.Context().Value(optionsKey{})
	opts, _ := v.(*Options)
	return opts
}

func bindFlags(cmd *cobra.Command, opts *Options) {
	cmd.PersistentFlags().StringVarP(&opts.DBPath, "database", "d", opts.DBPath, "database to use or /path/to/file.db")
	cmd.PersistentFlags().BoolVarP(&opts.ScanAll, "all", "A", opts.ScanAll, "scan unwanted and difficult (ALL) files")
	cmd.PersistentFlags().StringSliceVarP(&opts.ExcludeGlobs, "exclude", "x", nil, "exclude these files (comma separated list: -x *.js,*.sql)")
	cmd.PersistentFlags().StringSliceVarP(&opts.IncludeGlobs, "glob", "g", nil, "only search these files (can repeat)")
	cmd.PersistentFlags().BoolVarP(&opts.CaseInsensitive, "ignore-case", "i", opts.CaseInsensitive, "case in-sensitive scan")
	cmd.PersistentFlags().IntVarP(&opts.ContextLines, "context", "c", opts.ContextLines, "number of lines of context to display before and after a match, default is 1")

	cmd.PersistentFlags().BoolVarP(&opts.NoBanner, "no-banner", "B", opts.NoBanner, "suppress banner")
	cmd.PersistentFlags().BoolVarP(&opts.VimLines, "vim-lines", "L", opts.VimLines, "vim friendly lines")
	cmd.PersistentFlags().BoolVarP(&opts.colorblind, "colorblind", "b", false, "colour blind friendly template")
	cmd.PersistentFlags().BoolVarP(&opts.noColor, "no-color", "z", false, "suppress colors")
	cmd.PersistentFlags().BoolVarP(&opts.highContrast, "high-contrast", "Z", false, "high contrast colors")

	cmd.PersistentFlags().BoolVarP(&opts.ListDatabases, "list-databases", "l", opts.ListDatabases, "lists databases available")

	cmd.PersistentFlags().StringVar(&opts.Unit, "unit", opts.Unit, "unit granularity: line|block|file")
	cmd.PersistentFlags().BoolVar(&opts.Jsonl, "jsonl", opts.Jsonl, "output as JSONL")
	cmd.PersistentFlags().BoolVar(&opts.Explain, "explain", opts.Explain, "print explain info to stderr")
	cmd.PersistentFlags().StringVar(&opts.Viz, "viz", opts.Viz, "viz output mode (ascii)")
}

func ExecuteForTest(cmd *cobra.Command) (string, Options, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()

	opts := optionsFrom(cmd)
	if opts == nil {
		return out.String(), Options{}, err
	}
	opts.normalize()

	return out.String(), *opts, err
}

func newDefaultOptions() *Options {
	return &Options{
		DBPath:       ".otidx/index.db",
		ContextLines: 1,
		Unit:         "block",
		Theme:        "default",
	}
}

func withOptionsContext(cmd *cobra.Command, opts *Options) {
	cmd.SetContext(context.WithValue(context.Background(), optionsKey{}, opts))
}

func printTodo(cmd *cobra.Command) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "TODO")
	return nil
}
