//go:build !treesitter || !cgo

package treesitter

import (
	"otterindex/internal/index/store"
)

type Provider struct{}

func NewProvider() *Provider { return &Provider{} }

func (p *Provider) Extract(path string, src []byte) ([]store.SymbolInput, []store.CommentInput, error) {
	return nil, nil, ErrDisabled
}
