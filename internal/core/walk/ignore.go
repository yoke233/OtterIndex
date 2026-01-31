package walk

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type ignoreMatcher struct {
	patterns []string
}

func loadIgnoreMatcher(root string, scanAll bool) (*ignoreMatcher, error) {
	if scanAll {
		return &ignoreMatcher{patterns: nil}, nil
	}

	var patterns []string
	for _, name := range []string{".gitignore", ".otidxignore"} {
		p, err := readIgnoreFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, p...)
	}
	return &ignoreMatcher{patterns: patterns}, nil
}

func readIgnoreFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *ignoreMatcher) isIgnored(relPath string) bool {
	if m == nil || len(m.patterns) == 0 {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	base := path.Base(relPath)
	for _, raw := range m.patterns {
		pat := strings.TrimSpace(raw)
		if pat == "" {
			continue
		}
		pat = strings.ReplaceAll(pat, "\\", "/")
		if strings.Contains(pat, "/") {
			if ok, _ := path.Match(pat, relPath); ok {
				return true
			}
			continue
		}
		if ok, _ := path.Match(pat, base); ok {
			return true
		}
	}
	return false
}

