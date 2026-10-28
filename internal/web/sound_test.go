package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestSoundSwitch(t *testing.T) {
	e := setup(t)
	play := "/s/" + e.set.Slug + "/play"
	_, body := e.get(play)
	wantAll(t, "default", body, `data-sound="chime">`, `<form class="sound-switch" method="post" action="/sound">`,
		`name="sound" value="chime" aria-pressed="true">Chime</button>`, `name="sound" value="off" aria-pressed="false">Off</button>`,
		`<span class="sheet-label">Sound</span>`, `src="/static/sound.js"`)

	resp, _ := e.post("/sound", url.Values{"sound": {"coin"}}, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)
	resp, _ = e.post("/sound", url.Values{"sound": {"kazoo"}}, e.srv.URL)
	wantStatus(t, resp, http.StatusBadRequest)

	resp, _ = e.post("/sound", url.Values{"sound": {"coin"}, "next": {play}}, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != play {
		t.Errorf("redirect to %q", loc)
	}
	_, body = e.get(play)
	wantAll(t, "coin", body, `data-sound="coin">`, `name="sound" value="coin" aria-pressed="true"`, `name="sound" value="chime" aria-pressed="false"`)

	req, _ := http.NewRequest(http.MethodPost, e.srv.URL+"/sound", strings.NewReader("sound=off"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp, _ = e.do(req)
	wantStatus(t, resp, http.StatusNoContent)
	_, body = e.get(play)
	wantAll(t, "off", body, `data-sound="off">`)

	e.user("anna")
	e.login("anna")
	_, body = e.get("/account")
	wantAll(t, "account", body, "<h2>Sound</h2>", `<form class="sound-switch" method="post" action="/sound"><input type="hidden" name="next" value="/account">`)

	_, js := e.get("/static/sound.js")
	wantAll(t, "sound.js", js, "AudioContext", "htmx:afterSettle", "/check")
}
