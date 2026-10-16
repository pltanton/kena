package oauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"

	"github.com/pltanton/kena/internal/store"
)

const (
	accessTTL  = time.Hour
	refreshTTL = 90 * 24 * time.Hour
	codeTTL    = 10 * time.Minute

	maxRedirectURIs    = 10
	maxClientNameRunes = 100
)

var (
	challengeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	verifierRe  = regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`)
)

//go:embed templates/authorize.html
var templatesFS embed.FS

var authorizeTmpl = template.Must(template.ParseFS(templatesFS, "templates/authorize.html"))

type Server struct {
	Store     *store.Store
	PublicURL string
	HTTP      *http.Client
	Now       func() time.Time
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Server) ResourceURL() string {
	return s.PublicURL + "/mcp"
}

func (s *Server) ResourceMetadataURL() string {
	return s.PublicURL + "/.well-known/oauth-protected-resource/mcp"
}

func (s *Server) Register(mux *http.ServeMux) {
	prm := auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               s.ResourceURL(),
		AuthorizationServers:   []string{s.PublicURL},
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "kena",
		ScopesSupported:        []string{scope},
	})
	mux.Handle("/.well-known/oauth-protected-resource", prm)
	mux.Handle("/.well-known/oauth-protected-resource/mcp", prm)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", s.handleASMetadata)
	mux.HandleFunc("GET /.well-known/openid-configuration", s.handleASMetadata)
	mux.HandleFunc("POST /oauth/register", s.handleRegister)
	mux.HandleFunc("OPTIONS /oauth/register", preflight)
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorize)
	mux.HandleFunc("POST /oauth/authorize", s.handleAuthorize)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	mux.HandleFunc("OPTIONS /oauth/token", preflight)
}

func (s *Server) handleASMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                         s.PublicURL,
		"authorization_endpoint":                         s.PublicURL + "/oauth/authorize",
		"token_endpoint":                                 s.PublicURL + "/oauth/token",
		"registration_endpoint":                          s.PublicURL + "/oauth/register",
		"response_types_supported":                       []string{"code"},
		"grant_types_supported":                          []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":               []string{"S256"},
		"token_endpoint_auth_methods_supported":          []string{"none"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
		"scopes_supported":                               []string{scope},
	})
}

func preflight(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("oauth: writing response", "err", err)
	}
}

func oauthError(w http.ResponseWriter, status int, code, desc string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func validateRedirectURIs(uris []string) error {
	if len(uris) == 0 {
		return errors.New("redirect_uris is required")
	}
	for _, raw := range uris {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || u.Fragment != "" {
			return errors.New("malformed redirect_uri " + raw)
		}
		switch {
		case u.Scheme == "https":
		case u.Scheme == "http" && isLoopbackHost(u.Hostname()):
		default:
			return errors.New("redirect_uri must be https or loopback http: " + raw)
		}
	}
	return nil
}

func matchRedirect(registered []string, got string) bool {
	g, err := url.Parse(got)
	if err != nil {
		return false
	}
	for _, reg := range registered {
		if reg == got {
			return true
		}
		r, err := url.Parse(reg)
		if err != nil {
			continue
		}
		if r.Scheme == "http" && g.Scheme == "http" &&
			isLoopbackHost(r.Hostname()) && r.Hostname() == g.Hostname() &&
			r.EscapedPath() == g.EscapedPath() && r.RawQuery == g.RawQuery &&
			r.User == nil && g.User == nil && r.Fragment == "" && g.Fragment == "" {
			return true
		}
	}
	return false
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	var req struct {
		RedirectURIs []string `json:"redirect_uris"`
		ClientName   string   `json:"client_name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", "request body must be a JSON object")
		return
	}
	if len(req.RedirectURIs) > maxRedirectURIs {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", fmt.Sprintf("at most %d redirect_uris are allowed", maxRedirectURIs))
		return
	}
	if err := validateRedirectURIs(req.RedirectURIs); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_redirect_uri", err.Error())
		return
	}
	if utf8.RuneCountInString(req.ClientName) > maxClientNameRunes {
		oauthError(w, http.StatusBadRequest, "invalid_client_metadata", fmt.Sprintf("client_name must be at most %d characters", maxClientNameRunes))
		return
	}
	id, err := store.NewSecret("kc_")
	if err != nil {
		slog.Error("oauth: register", "err", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}
	c := store.OAuthClient{ClientID: id, RedirectURIs: req.RedirectURIs, ClientName: req.ClientName}
	if err := s.Store.SaveClient(r.Context(), c); err != nil {
		slog.Error("oauth: register", "err", err)
		oauthError(w, http.StatusInternalServerError, "server_error", "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  id,
		"client_id_issued_at":        s.now().Unix(),
		"redirect_uris":              req.RedirectURIs,
		"client_name":                req.ClientName,
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
	})
}

var authorizeParams = []string{"response_type", "client_id", "redirect_uri", "code_challenge", "code_challenge_method", "state", "resource", "scope"}

type param struct{ Name, Value string }

type authorizePage struct {
	Fatal        string
	ClientName   string
	ClientHost   string
	RedirectHost string
	Error        string
	Params       []param
}

func (s *Server) lookupClient(ctx context.Context, clientID string) (store.OAuthClient, error) {
	if strings.HasPrefix(clientID, "https://") {
		return s.fetchClientMetadata(ctx, clientID)
	}
	c, err := s.Store.GetClient(ctx, clientID)
	if errors.Is(err, store.ErrNotFound) {
		return c, errUnknownClient
	}
	return c, err
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderAuthorize(w, http.StatusBadRequest, authorizePage{Fatal: "Malformed request."})
		return
	}
	get := r.Form.Get
	clientID, redirectURI := get("client_id"), get("redirect_uri")

	client, err := s.lookupClient(r.Context(), clientID)
	if errors.Is(err, errUnknownClient) {
		slog.Warn("oauth: authorize unknown client", "client_id", clientID, "err", err)
		renderAuthorize(w, http.StatusBadRequest, authorizePage{Fatal: "Unknown client."})
		return
	}
	if err != nil {
		s.authorizeServerError(w, "client lookup", err)
		return
	}
	if redirectURI == "" || !matchRedirect(client.RedirectURIs, redirectURI) {
		renderAuthorize(w, http.StatusBadRequest, authorizePage{Fatal: "The redirect URI is not registered for this client."})
		return
	}

	switch {
	case get("response_type") == "":
		s.redirectError(w, r, redirectURI, "invalid_request", "response_type is required")
		return
	case get("response_type") != "code":
		s.redirectError(w, r, redirectURI, "unsupported_response_type", "response_type must be code")
		return
	case !challengeRe.MatchString(get("code_challenge")):
		s.redirectError(w, r, redirectURI, "invalid_request", "code_challenge must be 43 base64url characters")
		return
	case get("code_challenge_method") != "S256":
		s.redirectError(w, r, redirectURI, "invalid_request", "code_challenge_method must be S256")
		return
	case get("resource") != "" && get("resource") != s.ResourceURL():
		s.redirectError(w, r, redirectURI, "invalid_target", "unknown resource")
		return
	}

	page := authorizePage{ClientName: client.ClientName}
	if strings.HasPrefix(client.ClientID, "https://") {
		page.ClientHost = hostOf(client.ClientID)
	}
	page.RedirectHost = hostOf(redirectURI)
	for _, name := range authorizeParams {
		if v := get(name); v != "" {
			page.Params = append(page.Params, param{name, v})
		}
	}

	if r.Method == http.MethodGet {
		renderAuthorize(w, http.StatusOK, page)
		return
	}

	user, err := s.Store.UserByToken(r.Context(), strings.TrimSpace(r.PostForm.Get("token")))
	if errors.Is(err, store.ErrNotFound) {
		page.Error = "Token not recognised."
		renderAuthorize(w, http.StatusOK, page)
		return
	}
	if err != nil {
		s.authorizeServerError(w, "token lookup", err)
		return
	}

	code, err := store.NewSecret("kena_ac_")
	if err != nil {
		s.authorizeServerError(w, "generate code", err)
		return
	}
	resource := get("resource")
	if resource == "" {
		resource = s.ResourceURL()
	}
	err = s.Store.SaveCode(r.Context(), code, store.AuthCode{
		ClientID:      client.ClientID,
		UserID:        user.ID,
		RedirectURI:   redirectURI,
		CodeChallenge: get("code_challenge"),
		Resource:      resource,
		Scope:         scope,
		ExpiresAt:     s.now().Add(codeTTL),
	})
	if err != nil {
		s.authorizeServerError(w, "save code", err)
		return
	}
	s.redirectWith(w, r, redirectURI, url.Values{"code": {code}})
}

func (s *Server) authorizeServerError(w http.ResponseWriter, op string, err error) {
	slog.Error("oauth: authorize", "op", op, "err", err)
	renderAuthorize(w, http.StatusInternalServerError, authorizePage{Fatal: "Internal error, try again later."})
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.Host
}

func (s *Server) redirectError(w http.ResponseWriter, r *http.Request, redirectURI, code, desc string) {
	s.redirectWith(w, r, redirectURI, url.Values{"error": {code}, "error_description": {desc}})
}

func (s *Server) redirectWith(w http.ResponseWriter, r *http.Request, redirectURI string, extra url.Values) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		renderAuthorize(w, http.StatusBadRequest, authorizePage{Fatal: "Malformed redirect URI."})
		return
	}
	q := u.Query()
	for k, vs := range extra {
		q[k] = vs
	}
	if st := r.Form.Get("state"); st != "" {
		q.Set("state", st)
	}
	q.Set("iss", s.PublicURL)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func renderAuthorize(w http.ResponseWriter, status int, page authorizePage) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
	w.WriteHeader(status)
	if err := authorizeTmpl.Execute(w, page); err != nil {
		slog.Error("oauth: rendering authorize page", "err", err)
	}
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if err := r.ParseForm(); err != nil {
		oauthError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.exchangeCode(w, r)
	case "refresh_token":
		s.refresh(w, r)
	case "":
		oauthError(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
	default:
		oauthError(w, http.StatusBadRequest, "unsupported_grant_type", "supported: authorization_code, refresh_token")
	}
}

func (s *Server) exchangeCode(w http.ResponseWriter, r *http.Request) {
	f := r.PostForm
	code, clientID, verifier := f.Get("code"), f.Get("client_id"), f.Get("code_verifier")
	if code == "" || clientID == "" || verifier == "" || f.Get("redirect_uri") == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code, client_id, redirect_uri and code_verifier are required")
		return
	}
	if !verifierRe.MatchString(verifier) {
		oauthError(w, http.StatusBadRequest, "invalid_request", "code_verifier must be 43-128 characters of [A-Za-z0-9-._~]")
		return
	}
	ac, err := s.Store.ConsumeCode(r.Context(), code)
	if errors.Is(err, store.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired or already used")
		return
	}
	if err != nil {
		serverError(w, "consume code", err)
		return
	}
	switch {
	case ac.ClientID != clientID:
		oauthError(w, http.StatusBadRequest, "invalid_grant", "authorization code was issued to another client")
		return
	case ac.RedirectURI != f.Get("redirect_uri"):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		return
	case !pkceMatches(verifier, ac.CodeChallenge):
		oauthError(w, http.StatusBadRequest, "invalid_grant", "code_verifier does not match code_challenge")
		return
	case f.Get("resource") != "" && f.Get("resource") != ac.Resource:
		oauthError(w, http.StatusBadRequest, "invalid_target", "resource does not match the authorization request")
		return
	}
	access, refresh, _, err := s.Store.CreateGrant(r.Context(), ac.UserID, ac.ClientID, ac.Resource, accessTTL, refreshTTL)
	if err != nil {
		serverError(w, "create grant", err)
		return
	}
	writeTokens(w, access, refresh)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	refresh, clientID := r.PostForm.Get("refresh_token"), r.PostForm.Get("client_id")
	if refresh == "" || clientID == "" {
		oauthError(w, http.StatusBadRequest, "invalid_request", "refresh_token and client_id are required")
		return
	}
	access, newRefresh, _, err := s.Store.RotateRefresh(r.Context(), refresh, clientID, accessTTL, refreshTTL)
	if errors.Is(err, store.ErrNotFound) {
		oauthError(w, http.StatusBadRequest, "invalid_grant", "refresh token is invalid, expired or revoked")
		return
	}
	if err != nil {
		serverError(w, "rotate refresh", err)
		return
	}
	writeTokens(w, access, newRefresh)
}

func pkceMatches(verifier, challenge string) bool {
	sum := sha256.Sum256([]byte(verifier))
	got := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(challenge)) == 1
}

func writeTokens(w http.ResponseWriter, access, refresh string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"token_type":    "Bearer",
		"expires_in":    int(accessTTL / time.Second),
		"refresh_token": refresh,
		"scope":         scope,
	})
}

func serverError(w http.ResponseWriter, op string, err error) {
	slog.Error("oauth: token endpoint", "op", op, "err", err)
	oauthError(w, http.StatusInternalServerError, "server_error", "internal error")
}
