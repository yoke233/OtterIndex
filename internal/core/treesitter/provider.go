//go:build treesitter && cgo

package treesitter

import (
	"path/filepath"
	"strings"

	"otterindex/internal/index/sqlite"
)

type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) Extract(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch ext {
	case ".go":
		return extractGo(path, src)
	case ".java":
		return extractJava(path, src)
	case ".py":
		return extractPython(path, src)
	case ".js", ".jsx", ".mjs", ".cjs":
		return extractJavaScript(path, src)
	case ".ts":
		return extractTypeScript(path, src)
	case ".tsx":
		return extractTSX(path, src)
	case ".php":
		return extractPHP(path, src)
	case ".cs", ".csx":
		return extractCSharp(path, src)
	case ".json", ".jsonc":
		return extractJSON(path, src)
	case ".sh", ".bash":
		return extractBash(path, src)
	case ".c":
		return extractC(path, src)
	case ".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx":
		return extractCPP(path, src)
	case ".h":
		// Prefer C++ for headers; it can usually parse C too.
		return extractCPP(path, src)
	default:
		return nil, nil, ErrUnsupported
	}
}
