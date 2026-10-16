package session

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/pltanton/kena/internal/store"
)

const (
	CookieName = "kena_session"
	TTL        = 365 * 24 * time.Hour
)

var FailDelay = time.Second

func Secure(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

func Secret(r *http.Request) string {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return ""
	}
	return c.Value
}

func Set(w http.ResponseWriter, r *http.Request, secret string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    secret,
		Path:     "/",
		MaxAge:   int(TTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   Secure(r),
	})
}

func Clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   Secure(r),
	})
}

func Current(r *http.Request, st *store.Store) (store.User, bool, error) {
	secret := Secret(r)
	if secret == "" {
		return store.User{}, false, nil
	}
	u, err := st.UserBySession(r.Context(), secret)
	if errors.Is(err, store.ErrNotFound) {
		return store.User{}, false, nil
	}
	if err != nil {
		return store.User{}, false, err
	}
	return u, true, nil
}

func Login(w http.ResponseWriter, r *http.Request, st *store.Store, username, pass string) (store.User, error) {
	ctx := r.Context()
	u, err := st.CheckPassword(ctx, username, pass)
	if errors.Is(err, store.ErrNotFound) {
		sleep(ctx, FailDelay)
		return store.User{}, err
	}
	if err != nil {
		return store.User{}, err
	}
	secret, err := st.CreateSession(ctx, u.ID, TTL)
	if err != nil {
		return store.User{}, err
	}
	Set(w, r, secret)
	return u, nil
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

func SameOrigin(r *http.Request, publicURL string) bool {
	if publicURL == "" {
		return true
	}
	pub, err := url.Parse(publicURL)
	if err != nil {
		return false
	}
	src := r.Header.Get("Origin")
	if src == "" {
		src = r.Header.Get("Referer")
	}
	if src == "" {
		return true
	}
	u, err := url.Parse(src)
	if err != nil {
		return false
	}
	return u.Host == pub.Host
}
