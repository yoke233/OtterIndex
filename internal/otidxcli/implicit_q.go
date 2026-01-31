package otidxcli

import (
	"strings"

	"github.com/spf13/cobra"
)

func RewriteArgsForImplicitQ(root *cobra.Command, args []string) []string {
	if root == nil || len(args) == 0 {
		return args
	}

	first, ok := firstPositionalArgAfterFlags(args)
	if !ok {
		return args
	}

	known := knownTopLevelCommands(root)
	if known[strings.TrimSpace(first)] {
		return args
	}

	return append([]string{"q"}, args...)
}

func knownTopLevelCommands(root *cobra.Command) map[string]bool {
	known := map[string]bool{
		"help":       true,
		"completion": true,
	}

	if root == nil {
		return known
	}

	for _, c := range root.Commands() {
		if c == nil {
			continue
		}
		known[c.Name()] = true
		for _, a := range c.Aliases {
			known[a] = true
		}
	}

	return known
}

func firstPositionalArgAfterFlags(args []string) (string, bool) {
	skipNext := false
	positionalOnly := false

	for i := 0; i < len(args); i++ {
		a := strings.TrimSpace(args[i])
		if a == "" {
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}

		if a == "--" {
			positionalOnly = true
			continue
		}

		if positionalOnly {
			return a, true
		}

		if strings.HasPrefix(a, "--") {
			if strings.Contains(a, "=") {
				continue
			}

			name := strings.TrimPrefix(a, "--")
			switch name {
			case "database", "exclude", "glob", "context", "limit", "offset", "cache-size", "unit", "viz":
				skipNext = true
			case "explain":
				// Optional value; only consume known formats.
				if i+1 < len(args) {
					next := strings.TrimSpace(args[i+1])
					if next == "text" || next == "json" {
						skipNext = true
					}
				}
			}
			continue
		}

		if strings.HasPrefix(a, "-") && a != "-" {
			// Handle value-taking short flags: -d/-x/-g/-c
			if len(a) == 2 {
				switch a[1] {
				case 'd', 'x', 'g', 'c':
					skipNext = true
				}
				continue
			}

			// Inline values, e.g. -dfoo / -d=foo / -c2
			continue
		}

		return a, true
	}

	return "", false
}

