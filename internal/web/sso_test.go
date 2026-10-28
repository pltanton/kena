package web_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/pltanton/kena/internal/config"
	"github.com/pltanton/kena/internal/web"
)

type fakeIdP struct {
	t   *testing.T
	srv *httptest.Server
	key *rsa.PrivateKey

	mu        sync.Mutex
	sub, name string
	nonce     string
	badNonce  bool
	challenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeIdP{t: t, key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": p.srv.URL, "authorization_endpoint": p.srv.URL + "/authorize", "token_endpoint": p.srv.URL + "/token",
			"jwks_uri": p.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		p.mu.Lock()
		defer p.mu.Unlock()
		if r.PostForm.Get("code") != "good-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != p.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		nonce := p.nonce
		if p.badNonce {
			nonce = "other"
		}
		claims, _ := json.Marshal(map[string]any{"iss": p.srv.URL, "aud": "kena", "sub": p.sub, "preferred_username": p.name,
			"nonce": nonce, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix()})
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, nil)
		if err != nil {
			t.Error(err)
			return
		}
		jws, _ := signer.Sign(claims)
		raw, _ := jws.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": raw})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (e *env) withSSO(p *fakeIdP) {
	e.srv.Config.Handler = web.Handler(e.st, e.srv.URL, web.WithSSO(config.OIDC{Issuer: p.srv.URL, ClientID: "kena", ClientSecret: "s", Name: "Home ID"}, p.srv.Client()))
}

func (e *env) ssoLogin(p *fakeIdP, next, sub, name, stateOverride string) (*http.Response, string) {
	e.t.Helper()
	resp, _ := e.get("/login/sso?next=" + url.QueryEscape(next))
	wantStatus(e.t, resp, http.StatusFound)
	auth, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(auth.String(), p.srv.URL+"/authorize") {
		e.t.Fatalf("redirect to %q", resp.Header.Get("Location"))
	}
	q := auth.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != e.srv.URL+"/login/sso/callback" || q.Get("client_id") != "kena" {
		e.t.Fatalf("authorize query %v", q)
	}
	p.mu.Lock()
	p.sub, p.name, p.nonce, p.challenge = sub, name, q.Get("nonce"), q.Get("code_challenge")
	p.mu.Unlock()
	state := q.Get("state")
	if stateOverride != "" {
		state = stateOverride
	}
	return e.get("/login/sso/callback?code=good-code&state=" + url.QueryEscape(state))
}

func TestSSOOffByDefault(t *testing.T) {
	e := setup(t)
	resp, _ := e.get("/login/sso")
	wantStatus(t, resp, http.StatusNotFound)
	_, body := e.get("/login")
	wantNone(t, "login without sso", body, "/login/sso", "login-or")
}

func TestSSOLogin(t *testing.T) {
	e := setup(t)
	p := newFakeIdP(t)
	e.withSSO(p)
	e.maria()

	_, body := e.get("/login?next=/browse")
	wantAll(t, "login page", body, `href="/login/sso?next=%2Fbrowse"`, "Log in with Home ID", `<p class="login-or">Log in with a passphrase</p>`)

	resp, _ := e.ssoLogin(p, "/browse", "sub-maria", "maria@id.example", "")
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/browse" {
		t.Fatalf("Location %q", loc)
	}
	_, body = e.get("/account")
	wantAll(t, "logged in", body, "maria")

	other := e.clone()
	resp, _ = other.ssoLogin(p, "/", "sub-maria", "renamed", "")
	wantStatus(t, resp, http.StatusSeeOther)

	stranger := e.clone()
	resp, body = stranger.ssoLogin(p, "/", "sub-x", "nobody", "")
	wantStatus(t, resp, http.StatusForbidden)
	wantAll(t, "no account", body, "There is no kena account “nobody”")

	thief := e.clone()
	resp, body = thief.ssoLogin(p, "/", "sub-other", "maria", "")
	wantStatus(t, resp, http.StatusForbidden)
	wantAll(t, "linked elsewhere", body, "linked to another sign-in")
}

func TestSSOFailures(t *testing.T) {
	e := setup(t)
	p := newFakeIdP(t)
	e.withSSO(p)
	e.maria()

	resp, body := e.ssoLogin(p, "/", "sub-maria", "maria", "forged")
	wantStatus(t, resp, http.StatusBadGateway)
	wantAll(t, "state", body, "Logging in with Home ID failed")

	p.badNonce = true
	resp, _ = e.clone().ssoLogin(p, "/", "sub-maria", "maria", "")
	wantStatus(t, resp, http.StatusBadGateway)
	p.badNonce = false

	resp, _ = e.clone().get("/login/sso/callback?code=good-code&state=x")
	wantStatus(t, resp, http.StatusBadGateway)

	c := e.clone()
	resp, _ = c.get("/login/sso?next=/")
	state, _ := url.Parse(resp.Header.Get("Location"))
	resp, _ = c.get("/login/sso/callback?error=access_denied&state=" + url.QueryEscape(state.Query().Get("state")))
	wantStatus(t, resp, http.StatusBadGateway)
	resp, _ = c.get("/account")
	wantStatus(t, resp, http.StatusSeeOther)
}
