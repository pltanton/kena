package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/pltanton/kena/internal/config"
	"github.com/pltanton/kena/internal/session"
	"github.com/pltanton/kena/internal/store"
)

const (
	ssoCookie   = "kena_sso"
	ssoPath     = "/login/sso"
	ssoCallback = ssoPath + "/callback"
	ssoTTL      = 10 * time.Minute
)

type sso struct {
	cfg      config.OIDC
	redirect string
	http     *http.Client

	mu       sync.Mutex
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

type ssoState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Next     string `json:"x"`
}

type Option func(*handler)

func WithSSO(cfg config.OIDC, client *http.Client) Option {
	return func(h *handler) {
		if cfg.Issuer == "" {
			return
		}
		if client == nil {
			client = &http.Client{Timeout: 10 * time.Second}
		}
		h.sso = &sso{cfg: cfg, redirect: h.publicURL + ssoCallback, http: client}
	}
}

func (s *sso) setup(ctx context.Context) (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.oauth != nil {
		return s.oauth, s.verifier, nil
	}
	p, err := oidc.NewProvider(oidc.ClientContext(ctx, s.http), s.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc discovery: %w", err)
	}
	s.oauth = &oauth2.Config{
		ClientID: s.cfg.ClientID, ClientSecret: s.cfg.ClientSecret, RedirectURL: s.redirect,
		Endpoint: p.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile"},
	}
	s.verifier = p.Verifier(&oidc.Config{ClientID: s.cfg.ClientID})
	return s.oauth, s.verifier, nil
}

func randomString() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func ssoURL(next string) string {
	return ssoPath + "?next=" + url.QueryEscape(next)
}

func (h *handler) ssoName() string {
	if h.sso == nil {
		return ""
	}
	return h.sso.cfg.Name
}

func (h *handler) ssoStart(w http.ResponseWriter, r *http.Request) {
	if h.sso == nil {
		h.notFound(w, r)
		return
	}
	conf, _, err := h.sso.setup(r.Context())
	if err != nil {
		h.ssoFailed(w, r, "", err)
		return
	}
	st := ssoState{State: randomString(), Nonce: randomString(), Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
	b, _ := json.Marshal(st)
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: ssoPath,
		MaxAge: int(ssoTTL / time.Second), HttpOnly: true, Secure: session.Secure(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, conf.AuthCodeURL(st.State, oauth2.S256ChallengeOption(st.Verifier), oidc.Nonce(st.Nonce)), http.StatusFound)
}

func readSSOState(r *http.Request) (ssoState, bool) {
	c, err := r.Cookie(ssoCookie)
	if err != nil {
		return ssoState{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	var st ssoState
	if err != nil || json.Unmarshal(b, &st) != nil || st.State == "" {
		return ssoState{}, false
	}
	return st, true
}

func (h *handler) ssoCallback(w http.ResponseWriter, r *http.Request) {
	if h.sso == nil {
		h.notFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ssoCookie, Path: ssoPath, MaxAge: -1, HttpOnly: true, Secure: session.Secure(r), SameSite: http.SameSiteLaxMode})
	q := r.URL.Query()
	st, ok := readSSOState(r)
	switch {
	case !ok:
		h.ssoFailed(w, r, "", errors.New("no sso state cookie"))
		return
	case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1:
		h.ssoFailed(w, r, st.Next, errors.New("state mismatch"))
		return
	case q.Get("error") != "":
		h.ssoFailed(w, r, st.Next, fmt.Errorf("idp error %q: %s", q.Get("error"), q.Get("error_description")))
		return
	}
	ctx := oidc.ClientContext(r.Context(), h.sso.http)
	conf, verifier, err := h.sso.setup(ctx)
	if err != nil {
		h.ssoFailed(w, r, st.Next, err)
		return
	}
	tok, err := conf.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		h.ssoFailed(w, r, st.Next, fmt.Errorf("code exchange: %w", err))
		return
	}
	raw, _ := tok.Extra("id_token").(string)
	idt, err := verifier.Verify(ctx, raw)
	if err != nil {
		h.ssoFailed(w, r, st.Next, fmt.Errorf("id token: %w", err))
		return
	}
	if subtle.ConstantTimeCompare([]byte(idt.Nonce), []byte(st.Nonce)) != 1 {
		h.ssoFailed(w, r, st.Next, errors.New("nonce mismatch"))
		return
	}
	var claims struct {
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idt.Claims(&claims); err != nil {
		h.ssoFailed(w, r, st.Next, fmt.Errorf("claims: %w", err))
		return
	}
	u, err := h.ssoUser(r.Context(), idt.Subject, claims.PreferredUsername)
	if err != nil {
		h.ssoRefused(w, r, st.Next, claims.PreferredUsername, err)
		return
	}
	if err := session.Start(w, r, h.st, u.ID); err != nil {
		h.serverError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "sso login", "user", u.Username)
	h.loggedIn(w, r, u, st.Next)
}

func (h *handler) ssoUser(ctx context.Context, subject, preferred string) (store.User, error) {
	u, err := h.st.UserBySubject(ctx, subject)
	if !errors.Is(err, store.ErrNotFound) {
		return u, err
	}
	name, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(preferred)), "@")
	if name == "" {
		return store.User{}, store.ErrNotFound
	}
	return h.st.LinkSubject(ctx, name, subject)
}

func (h *handler) ssoFailed(w http.ResponseWriter, r *http.Request, next string, err error) {
	slog.WarnContext(r.Context(), "sso login failed", "err", err)
	h.page(w, r, http.StatusBadGateway, "login", h.loginView(next, "", tr(r, "login.sso_failed", h.ssoName())))
}

func (h *handler) ssoRefused(w http.ResponseWriter, r *http.Request, next, name string, err error) {
	msg := ""
	switch {
	case errors.Is(err, store.ErrNotFound):
		msg = tr(r, "login.sso_no_account", name)
	case errors.Is(err, store.ErrLinkedElsewhere):
		msg = tr(r, "login.sso_linked", name)
	default:
		h.serverError(w, r, err)
		return
	}
	slog.WarnContext(r.Context(), "sso login refused", "name", name, "err", err)
	h.page(w, r, http.StatusForbidden, "login", h.loginView(next, "", msg))
}
