package walk

import (
	"path"
	"path/filepath"
)

type Filter struct {
	opts Options
	ig   *ignoreMatcher
}

func NewFilter(root string, opts Options) (*Filter, error) {
	ig, err := loadIgnoreMatcher(root, opts.ScanAll)
	if err != nil {
		return nil, err
	}
	return &Filter{
		opts: opts,
		ig:   ig,
	}, nil
}

func (f *Filter) ShouldInclude(rel string, isDir bool) bool {
	if f == nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	name := path.Base(rel)

	if isDir {
		if !f.opts.ScanAll && (isHidden(name) || isDefaultSkippedDir(name)) {
			return false
		}
		if !f.opts.ScanAll && f.ig.isIgnored(rel, true) {
			return false
		}
		return true
	}

	if !f.opts.ScanAll && isHidden(name) {
		return false
	}
	if !f.opts.ScanAll && f.ig.isIgnored(rel, false) {
		return false
	}
	if len(f.opts.IncludeGlobs) > 0 && !anyGlobMatch(f.opts.IncludeGlobs, rel) {
		return false
	}
	if anyGlobMatch(f.opts.ExcludeGlobs, rel) {
		return false
	}
	return true
}

func ShouldInclude(root string, rel string, isDir bool, opts Options) (bool, error) {
	f, err := NewFilter(root, opts)
	if err != nil {
		return false, err
	}
	return f.ShouldInclude(rel, isDir), nil
}
