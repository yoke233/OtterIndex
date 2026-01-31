package walk

import (
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	gitignore "github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

type ignoreMatcher struct {
	matcher gitignore.Matcher
}

func loadIgnoreMatcher(root string, scanAll bool) (*ignoreMatcher, error) {
	if scanAll {
		return &ignoreMatcher{matcher: nil}, nil
	}

	fs := osfs.New(root)
	patterns, err := gitignore.ReadPatterns(fs, nil)
	if err != nil {
		return nil, err
	}
	return &ignoreMatcher{matcher: gitignore.NewMatcher(patterns)}, nil
}

func (m *ignoreMatcher) isIgnored(relPath string, isDir bool) bool {
	if m == nil || m.matcher == nil {
		return false
	}

	relPath = strings.Trim(relPath, "/")
	if relPath == "" {
		return false
	}

	segments := strings.Split(relPath, "/")
	return m.matcher.Match(segments, isDir)
}
