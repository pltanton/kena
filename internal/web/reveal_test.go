package web_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestPassphraseFieldsHaveRevealToggle(t *testing.T) {
	e := setup(t)
	_, body := e.get("/login")
	if strings.Count(body, `class="reveal"`) != 1 || !strings.Contains(body, `aria-controls="passphrase"`) || !strings.Contains(body, `src="/static/reveal.js"`) {
		t.Errorf("login page reveal toggle:\n%s", body)
	}
	e.user("anna")
	e.login("anna")
	_, body = e.get("/account")
	for _, id := range []string{"current", "new"} {
		if !strings.Contains(body, `aria-controls="`+id+`"`) {
			t.Errorf("account: no reveal toggle for %s", id)
		}
	}
	if strings.Count(body, `type="password"`) != 2 || strings.Count(body, `aria-pressed="false"`) < 2 {
		t.Errorf("account passphrase fields:\n%s", body)
	}
	resp, js := e.get("/static/reveal.js")
	wantStatus(t, resp, http.StatusOK)
	wantAll(t, "reveal.js", js, "aria-pressed", "submit", `"password"`)
}
