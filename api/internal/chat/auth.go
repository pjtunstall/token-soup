package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// AuthMode selects how Authorization headers are validated.
type AuthMode string

const (
	AuthModeLocal    AuthMode = "local"
	AuthModeSupabase AuthMode = "supabase"
)

// Authenticator validates Bearer tokens according to Mode.
type Authenticator struct {
	Mode    AuthMode
	Cache   *jwk.Cache
	JWKSURL string
}

// UserID returns the authenticated user id from the Authorization header.
func (a *Authenticator) UserID(ctx context.Context, authHeader string) (string, error) {
	if a == nil {
		return "", fmt.Errorf("authenticator not configured")
	}
	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if tokenStr == "" {
		return "", fmt.Errorf("missing bearer token")
	}
	if a.Mode == AuthModeLocal {
		if tokenStr != LocalDevToken {
			return "", fmt.Errorf("invalid local token")
		}
		return LocalDevUserID, nil
	}
	return authUserIDFromJWKS(ctx, a.Cache, a.JWKSURL, tokenStr)
}

func authUserIDFromJWKS(ctx context.Context, cache *jwk.Cache, jwksURL, tokenStr string) (string, error) {
	if cache == nil || jwksURL == "" {
		return "", fmt.Errorf("jwks not configured")
	}
	set, err := cache.Get(ctx, jwksURL)
	if err != nil {
		return "", err
	}
	tok, err := jwt.Parse([]byte(tokenStr), jwt.WithKeySet(set))
	if err != nil {
		return "", err
	}
	sub := tok.Subject()
	if strings.TrimSpace(sub) == "" {
		return "", fmt.Errorf("token has no subject")
	}
	return sub, nil
}
