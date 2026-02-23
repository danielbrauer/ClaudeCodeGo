// Package mock provides a mock Claude API backend for end-to-end testing
// without network calls or authentication.
package mock

import (
	"context"
)

// StaticTokenSource is a TokenSource that always returns a fixed token.
// It satisfies api.TokenSource and api.RefreshableTokenSource.
type StaticTokenSource struct {
	Token string
}

// GetAccessToken returns the fixed token.
func (s *StaticTokenSource) GetAccessToken(_ context.Context) (string, error) {
	return s.Token, nil
}

// InvalidateToken is a no-op for the static source.
func (s *StaticTokenSource) InvalidateToken() {}
