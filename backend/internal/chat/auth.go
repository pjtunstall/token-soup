package chat

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// AuthUserID parses a Supabase JWT from the Authorization header and returns the subject (user id).
func AuthUserID(ctx context.Context, cache *jwk.Cache, jwksURL, authHeader string) (string, error) {
	tokenStr := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if tokenStr == "" {
		return "", fmt.Errorf("missing bearer token")
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
