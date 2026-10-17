package session

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
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
	setCookie(w, r, secret, int(TTL/time.Second))
}

func Clear(w http.ResponseWriter, r *http.Request) {
	setCookie(w, r, "", -1)
}

func setCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
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
		Sleep(ctx, FailDelay)
		return store.User{}, err
	}
	if err != nil {
		return store.User{}, err
	}
	return u, Start(w, r, st, u.ID)
}

func Start(w http.ResponseWriter, r *http.Request, st *store.Store, userID int64) error {
	ctx := r.Context()
	if old := Secret(r); old != "" {
		if err := st.DeleteSession(ctx, old); err != nil {
			return err
		}
	}
	secret, err := st.CreateSession(ctx, userID, TTL)
	if err != nil {
		return err
	}
	Set(w, r, secret)
	return nil
}

func Sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

func CrossOrigin(publicURL string) *http.CrossOriginProtection {
	cop := http.NewCrossOriginProtection()
	if u, err := url.Parse(publicURL); err == nil && u.Scheme != "" && u.Host != "" {
		if err := cop.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
			panic(err)
		}
	}
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Cross-site request rejected.", http.StatusForbidden)
	}))
	return cop
}

const qrLoginPrefix = "/login/qr/"

func RedactPath(p string) string {
	if strings.HasPrefix(p, qrLoginPrefix) {
		return qrLoginPrefix + "…"
	}
	return p
}
