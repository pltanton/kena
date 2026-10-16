package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"

	"github.com/pltanton/kena/internal/store"
)

const (
	scope            = "kena"
	userKey          = "user"
	personalTokenTTL = time.Hour
)

func (s *Server) Verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	switch {
	case strings.HasPrefix(token, "kena_at_"):
		g, u, err := s.Store.GrantByAccess(ctx, token)
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("%w: unknown or expired access token", auth.ErrInvalidToken)
		}
		if err != nil {
			return nil, err
		}
		return tokenInfo(u, g.AccessExpiresAt), nil
	case strings.HasPrefix(token, "kena_"):
		u, err := s.Store.UserByToken(ctx, token)
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("%w: unknown or revoked token", auth.ErrInvalidToken)
		}
		if err != nil {
			return nil, err
		}
		return tokenInfo(u, s.now().Add(personalTokenTTL)), nil
	default:
		return nil, fmt.Errorf("%w: unrecognised token format", auth.ErrInvalidToken)
	}
}

func tokenInfo(u store.User, exp time.Time) *auth.TokenInfo {
	return &auth.TokenInfo{
		Scopes:     []string{scope},
		Expiration: exp,
		UserID:     strconv.FormatInt(u.ID, 10),
		Extra:      map[string]any{userKey: u},
	}
}

func UserFrom(ti *auth.TokenInfo) (store.User, bool) {
	if ti == nil {
		return store.User{}, false
	}
	u, ok := ti.Extra[userKey].(store.User)
	return u, ok
}
