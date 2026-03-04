package oauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// NewTokenVerifier returns an auth.TokenVerifier that validates opaque tokens
// by looking them up in the MCPOAuthStore.
func NewTokenVerifier(store MCPOAuthStore) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		tokenHash := HashToken(token)
		stored, err := store.GetToken(ctx, tokenHash)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", auth.ErrInvalidToken, err.Error())
		}

		if stored.Revoked {
			return nil, fmt.Errorf("%w: token revoked", auth.ErrInvalidToken)
		}

		if stored.TokenType != "access" {
			return nil, fmt.Errorf("%w: not an access token", auth.ErrInvalidToken)
		}

		if time.Now().After(stored.ExpiresAt) {
			return nil, fmt.Errorf("%w: token expired", auth.ErrInvalidToken)
		}

		return &auth.TokenInfo{
			Expiration: stored.ExpiresAt,
			Extra: map[string]any{
				"client_id": stored.ClientID,
				"family_id": stored.FamilyID,
			},
		}, nil
	}
}
