package backend

import (
	"fmt"
	"path/filepath"
	"strings"

	"otterindex/internal/index/bleve"
	"otterindex/internal/index/sqlite"
	"otterindex/internal/index/store"
)

func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "sqlite"
	}
	switch name {
	case "sqlite", "sqlite3", "fts5":
		return "sqlite"
	case "bleve":
		return "bleve"
	default:
		return name
	}
}

func DefaultPath(root string, backend string) string {
	backend = NormalizeName(backend)
	switch backend {
	case "bleve":
		return filepath.Join(root, ".otidx", "index.bleve")
	default:
		return filepath.Join(root, ".otidx", "index.db")
	}
}

func NormalizePath(backend string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	backend = NormalizeName(backend)
	if backend != "bleve" {
		return filepath.Clean(path)
	}

	clean := filepath.Clean(path)
	ext := strings.ToLower(filepath.Ext(clean))
	if ext == "" {
		return clean + ".bleve"
	}
	if ext == ".db" {
		return strings.TrimSuffix(clean, ext) + ".bleve"
	}
	return clean
}

func Open(backend string, path string) (store.Store, error) {
	backend = NormalizeName(backend)
	switch backend {
	case "sqlite":
		return sqlite.Open(path)
	case "bleve":
		return bleve.Open(path)
	default:
		return nil, fmt.Errorf("unknown store backend: %s", backend)
	}
}
