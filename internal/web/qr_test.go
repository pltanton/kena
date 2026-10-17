package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAccountShowsLoginQR(t *testing.T) {
	e := setup(t)
	e.user("anna")
	e.login("anna")

	_, body := e.get("/account")
	wantAll(t, "account", body, "Log in on your phone", `action="/account/qr#phone"`, "Show code")
	wantNone(t, "account", body, `class="qr"`)

	resp, _ := e.post("/account/qr", nil, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)

	resp, body = e.post("/account/qr", nil, e.srv.URL)
	wantStatus(t, resp, http.StatusOK)
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("QR page cacheable: %q", resp.Header.Get("Cache-Control"))
	}
	wantAll(t, "account with code", body, `<svg class="qr"`, "<path", "New code", "Anyone who scans this logs in as you", "5 minutes")
	wantNone(t, "account with code", body, "kena_q_")
}

func TestQRLoginOnPhone(t *testing.T) {
	e := setup(t)
	anna := e.user("anna")
	ctx := context.Background()
	code, err := e.st.CreateLoginCode(ctx, anna.ID, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	path := "/login/qr/" + code

	phone := e.clone()
	phone.check(e.tasks[0].ID, url.Values{"a": {"γράφεις"}, "mode": {"cards"}}, true)
	for i := 0; i < 2; i++ {
		resp, body := phone.get(path)
		wantStatus(t, resp, http.StatusOK)
		wantAll(t, "phone confirm", body, "anna", `action="`+path+`"`)
	}

	resp, _ := phone.post(path, nil, "https://evil.example")
	wantStatus(t, resp, http.StatusForbidden)

	resp, _ = phone.post(path, nil, e.srv.URL)
	wantStatus(t, resp, http.StatusSeeOther)
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Fatalf("redirect to %q", loc)
	}
	_, body := phone.get("/")
	wantAll(t, "phone home", body, `aria-label="Account (anna)"`)

	desktop := e.clone()
	desktop.login("anna")
	_, body = desktop.get("/s/" + e.set.Slug)
	wantAll(t, "merged progress", body, "1/3 solved")

	again := e.clone()
	resp, body = again.post(path, nil, e.srv.URL)
	wantStatus(t, resp, http.StatusGone)
	wantAll(t, "used code", body, "This code expired")
	wantNone(t, "used code", body, "Account (anna)")

	expired, _ := e.st.CreateLoginCode(ctx, anna.ID, -time.Second)
	for _, p := range []string{"/login/qr/" + expired, "/login/qr/kena_q_nope"} {
		resp, body := again.get(p)
		wantStatus(t, resp, http.StatusGone)
		if !strings.Contains(body, "make a new one on your computer") {
			t.Errorf("%s: %s", p, body)
		}
	}
}
