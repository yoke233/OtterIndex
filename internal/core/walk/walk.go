package walk

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type Options struct {
	IncludeGlobs []string
	ExcludeGlobs []string
	ScanAll      bool
}

func ListFiles(root string, opts Options) ([]string, error) {
	ig, err := loadIgnoreMatcher(root, opts.ScanAll)
	if err != nil {
		return nil, err
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		name := d.Name()
		if d.IsDir() {
			if !opts.ScanAll && (isHidden(name) || isDefaultSkippedDir(name)) {
				return filepath.SkipDir
			}
			if !opts.ScanAll && ig.isIgnored(rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if !opts.ScanAll && isHidden(name) {
			return nil
		}
		if !opts.ScanAll && ig.isIgnored(rel, false) {
			return nil
		}
		if len(opts.IncludeGlobs) > 0 && !anyGlobMatch(opts.IncludeGlobs, rel) {
			return nil
		}
		if anyGlobMatch(opts.ExcludeGlobs, rel) {
			return nil
		}

		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}

func isDefaultSkippedDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "target":
		return true
	default:
		return false
	}
}

func anyGlobMatch(patterns []string, rel string) bool {
	for _, pat := range patterns {
		if matchesGlob(pat, rel) {
			return true
		}
	}
	return false
}

func matchesGlob(pattern string, rel string) bool {
	pat := strings.TrimSpace(pattern)
	if pat == "" {
		return false
	}
	pat = strings.ReplaceAll(pat, "\\", "/")
	rel = filepath.ToSlash(rel)

	// Support csv passed via -x "*.js,*.sql" when not using StringSliceVar.
	if strings.Contains(pat, ",") {
		for _, piece := range strings.Split(pat, ",") {
			if matchesGlob(strings.TrimSpace(piece), rel) {
				return true
			}
		}
		return false
	}

	// Treat patterns without path separators as basename patterns.
	if !strings.Contains(pat, "/") {
		ok, _ := path.Match(pat, path.Base(rel))
		return ok
	}

	ok, _ := path.Match(pat, rel)
	return ok
}
