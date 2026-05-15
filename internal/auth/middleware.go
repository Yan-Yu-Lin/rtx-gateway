package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

type contextKey string

const principalContextKey contextKey = "principal"

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}

func ContextWithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey, principal)
}

func AuthenticateRequest(ctx context.Context, database *sql.DB, pepper string, request *http.Request) (Principal, error) {
	header := strings.TrimSpace(request.Header.Get("Authorization"))
	if header == "" {
		return Principal{}, ErrMissingBearer
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return Principal{}, ErrMissingBearer
	}

	principal, err := ValidateAPIKey(ctx, database, pepper, strings.TrimSpace(parts[1]))
	if err != nil {
		if errors.Is(err, ErrInvalidKeyFormat) || errors.Is(err, ErrInvalidKey) || errors.Is(err, ErrDisabledKey) {
			return Principal{}, err
		}
		return Principal{}, err
	}
	return principal, nil
}

var ErrMissingBearer = errors.New("missing bearer token")
