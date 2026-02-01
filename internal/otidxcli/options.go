package otidxcli

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"otterindex/internal/index/backend"
)

type Options struct {
	DBPath          string
	Store           string
	ScanAll         bool
	IncludeGlobs    []string
	ExcludeGlobs    []string
	CaseInsensitive bool
	ContextLines    int
	Limit           int
	Offset          int
	Cache           bool
	CacheSize       int
	Compact         bool
	Unit            string
	Show            bool
	NoBanner        bool
	VimLines        bool
	Theme           string
	Jsonl           bool
	Explain         string
	Viz             string
	ListDatabases   bool

	colorblind   bool
	noColor      bool
	highContrast bool
}

func (o *Options) Prepare() error {
	o.normalize()
	o.DBPath = normalizeDBPath(o.Store, o.DBPath)

	if strings.TrimSpace(o.DBPath) == "" {
		return fmt.Errorf("database path is required")
	}
	if o.ContextLines < 0 {
		return fmt.Errorf("context lines must be >= 0")
	}
	if o.Limit <= 0 {
		return fmt.Errorf("limit must be >= 1")
	}
	if o.Offset < 0 {
		return fmt.Errorf("offset must be >= 0")
	}
	if o.CacheSize <= 0 {
		return fmt.Errorf("cache size must be >= 1")
	}

	switch backend.NormalizeName(o.Store) {
	case "sqlite", "bleve":
		o.Store = backend.NormalizeName(o.Store)
	default:
		return fmt.Errorf("invalid --store %q (expected: sqlite|bleve)", o.Store)
	}

	switch o.Unit {
	case "line", "block", "symbol", "file":
	default:
		return fmt.Errorf("invalid --unit %q (expected: line|block|symbol|file)", o.Unit)
	}

	o.Explain = strings.TrimSpace(o.Explain)
	switch o.Explain {
	case "", "text", "json":
	default:
		return fmt.Errorf("invalid --explain %q (expected: text|json)", o.Explain)
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
	o.Store = strings.TrimSpace(o.Store)
	if o.Store == "" {
		o.Store = "sqlite"
	}

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
		o.Unit = defaultUnit()
	}
}

type optionsKey struct{}
type testModeKey struct{}

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

func isTestMode(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	root := cmd.Root()
	if root == nil {
		root = cmd
	}
	v := root.Context().Value(testModeKey{})
	ok, _ := v.(bool)
	return ok
}

func bindFlags(cmd *cobra.Command, opts *Options) {
	cmd.PersistentFlags().StringVarP(&opts.DBPath, "database", "d", opts.DBPath, "database to use or /path/to/file.db")
	cmd.PersistentFlags().StringVar(&opts.Store, "store", opts.Store, "store backend (sqlite|bleve)")
	cmd.PersistentFlags().BoolVarP(&opts.ScanAll, "all", "A", opts.ScanAll, "scan unwanted and difficult (ALL) files")
	cmd.PersistentFlags().StringSliceVarP(&opts.ExcludeGlobs, "exclude", "x", nil, "exclude these files (comma separated list: -x *.js,*.sql)")
	cmd.PersistentFlags().StringSliceVarP(&opts.IncludeGlobs, "glob", "g", nil, "only search these files (can repeat)")
	cmd.PersistentFlags().BoolVarP(&opts.CaseInsensitive, "ignore-case", "i", opts.CaseInsensitive, "case in-sensitive scan")
	cmd.PersistentFlags().IntVarP(&opts.ContextLines, "context", "c", opts.ContextLines, "number of lines of context to display before and after a match, default is 1")
	cmd.PersistentFlags().IntVar(&opts.Limit, "limit", opts.Limit, "max results to return")
	cmd.PersistentFlags().IntVar(&opts.Offset, "offset", opts.Offset, "skip first N results")
	cmd.PersistentFlags().BoolVar(&opts.Cache, "cache", opts.Cache, "enable query result cache (mostly useful in daemon/interactive mode)")
	cmd.PersistentFlags().IntVar(&opts.CacheSize, "cache-size", opts.CacheSize, "query cache size (LRU entries)")
	cmd.PersistentFlags().BoolVar(&opts.Compact, "compact", opts.Compact, "compact one-line output (path:line: snippet)")
	cmd.PersistentFlags().BoolVar(&opts.Show, "show", opts.Show, "show unit source (multi-line)")

	cmd.PersistentFlags().BoolVarP(&opts.NoBanner, "no-banner", "B", opts.NoBanner, "suppress banner")
	cmd.PersistentFlags().BoolVarP(&opts.VimLines, "vim-lines", "L", opts.VimLines, "vim friendly lines")
	cmd.PersistentFlags().BoolVarP(&opts.colorblind, "colorblind", "b", false, "colour blind friendly template")
	cmd.PersistentFlags().BoolVarP(&opts.noColor, "no-color", "z", false, "suppress colors")
	cmd.PersistentFlags().BoolVarP(&opts.highContrast, "high-contrast", "Z", false, "high contrast colors")

	cmd.PersistentFlags().BoolVarP(&opts.ListDatabases, "list-databases", "l", opts.ListDatabases, "lists databases available")

	cmd.PersistentFlags().StringVar(&opts.Unit, "unit", opts.Unit, "unit granularity: line|block|symbol|file")
	cmd.PersistentFlags().BoolVar(&opts.Jsonl, "jsonl", opts.Jsonl, "output as JSONL")
	cmd.PersistentFlags().StringVar(&opts.Explain, "explain", opts.Explain, "print explain info to stderr (optional: json)")
	if f := cmd.PersistentFlags().Lookup("explain"); f != nil {
		f.NoOptDefVal = "text"
	}
	cmd.PersistentFlags().StringVar(&opts.Viz, "viz", opts.Viz, "viz output mode (ascii)")
}

func ExecuteForTest(cmd *cobra.Command) (string, Options, error) {
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	cmd.SetContext(context.WithValue(cmd.Context(), testModeKey{}, true))

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
		Store:        "sqlite",
		ContextLines: 1,
		Limit:        20,
		Offset:       0,
		Cache:        false,
		CacheSize:    128,
		Unit:         defaultUnit(),
		Theme:        "default",
	}
}

func withOptionsContext(cmd *cobra.Command, opts *Options) {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(context.WithValue(ctx, optionsKey{}, opts))
}

func printTodo(cmd *cobra.Command) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "TODO")
	return nil
}

func normalizeDBPath(storeName string, db string) string {
	db = strings.TrimSpace(db)
	if db == "" {
		return ""
	}

	// Treat values without separators as a "dbname" under .otidx.
	// e.g. `-d foo` => `.otidx/foo.db`
	if !strings.ContainsAny(db, "/\\") && !strings.Contains(db, ":") {
		if !strings.Contains(db, ".") {
			if backend.NormalizeName(storeName) == "bleve" {
				db += ".bleve"
			} else {
				db += ".db"
			}
		}
		return backend.NormalizePath(storeName, filepath.Join(".otidx", db))
	}

	return backend.NormalizePath(storeName, filepath.Clean(db))
}
