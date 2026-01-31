//go:build !treesitter || !cgo

package treesitter

import (
	"otterindex/internal/index/sqlite"
)

type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) Extract(path string, src []byte) ([]sqlite.SymbolInput, []sqlite.CommentInput, error) {
	return nil, nil, ErrDisabled
}
