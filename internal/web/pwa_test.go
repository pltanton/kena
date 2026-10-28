package web_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestManifestFollowsTheme(t *testing.T) {
	e := setup(t)
	type manifest struct {
		ID            string `json:"id"`
		ThemeColor    string `json:"theme_color"`
		Background    string `json:"background_color"`
		LaunchHandler struct {
			ClientMode string `json:"client_mode"`
		} `json:"launch_handler"`
	}
	for _, c := range []struct{ query, color string }{{"", "#f4efe4"}, {"?scheme=light", "#f4efe4"}, {"?scheme=sepia", "#f4efe4"}, {"?scheme=dark", "#181b20"}} {
		resp, body := e.get("/static/manifest.webmanifest" + c.query)
		wantStatus(t, resp, http.StatusOK)
		if ct := resp.Header.Get("Content-Type"); ct != "application/manifest+json" {
			t.Errorf("%q: content-type %q", c.query, ct)
		}
		var m manifest
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatalf("%q: %v", c.query, err)
		}
		if m.ID != "/" || m.ThemeColor != c.color || m.Background != c.color || m.LaunchHandler.ClientMode != "navigate-existing" {
			t.Errorf("%q: %+v", c.query, m)
		}
	}

	_, body := e.get("/")
	wantAll(t, "auto", body, `<link rel="manifest" href="/static/manifest.webmanifest">`, `viewport-fit=cover`)
	e.post("/theme", url.Values{"theme": {"dark"}}, e.srv.URL)
	_, body = e.get("/")
	wantAll(t, "dark", body, `<link rel="manifest" href="/static/manifest.webmanifest?scheme=dark">`)
	_, body = e.get("/s/" + e.set.Slug + "/play")
	wantAll(t, "play", body, `interactive-widget=resizes-content, viewport-fit=cover`, `href="/static/manifest.webmanifest?scheme=dark"`)
}
