package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestBlockedIP(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1":        true,
		"::1":              true,
		"10.1.2.3":         true,
		"172.16.0.1":       true,
		"192.168.1.10":     true,
		"fd00::1":          true,
		"169.254.169.254":  true,
		"fe80::1":          true,
		"0.0.0.0":          true,
		"::":               true,
		"224.0.0.1":        true,
		"ff02::1":          true,
		"100.64.0.1":       true,
		"100.127.255.254":  true,
		"::ffff:127.0.0.1": true,
		"::ffff:10.0.0.1":  true,
		"8.8.8.8":          false,
		"100.128.0.1":      false,
		"172.32.0.1":       false,
		"2606:4700::1111":  false,
		"::ffff:1.1.1.1":   false,
	}
	for s, want := range cases {
		if got := blockedIP(netip.MustParseAddr(s)); got != want {
			t.Errorf("blockedIP(%s) = %v, want %v", s, got, want)
		}
	}
}

func TestDefaultClientRefusesLoopback(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	s := &Server{}
	_, err := s.fetchClientDoc(context.Background(), ts.URL+"/client.json")
	if !errors.Is(err, errBlockedAddress) {
		t.Fatalf("err = %v, want errBlockedAddress", err)
	}
}

func TestFetchClientDocSetsUserAgent(t *testing.T) {
	var gotUA string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Write([]byte(`{"client_id":"https://` + r.Host + `/client.json","redirect_uris":["https://a.example/cb"]}`))
	}))
	defer ts.Close()
	clientID := ts.URL + "/client.json"

	s := &Server{HTTP: ts.Client(), UserAgent: "kena/1.2.3"}
	if _, err := s.fetchClientDoc(context.Background(), clientID); err != nil {
		t.Fatalf("fetchClientDoc: %v", err)
	}
	if gotUA != "kena/1.2.3" {
		t.Errorf("User-Agent = %q, want kena/1.2.3", gotUA)
	}

	s2 := &Server{HTTP: ts.Client()}
	if _, err := s2.fetchClientDoc(context.Background(), clientID); err != nil {
		t.Fatalf("fetchClientDoc (default UA): %v", err)
	}
	if gotUA != "kena" {
		t.Errorf("default User-Agent = %q, want kena", gotUA)
	}
}

func TestCIMDCheckRedirect(t *testing.T) {
	via := []*http.Request{httptest.NewRequest("GET", "https://a.example/client.json", nil)}
	if err := cimdCheckRedirect(httptest.NewRequest("GET", "http://a.example/x", nil), via); err == nil {
		t.Error("redirect to http allowed")
	}
	if err := cimdCheckRedirect(httptest.NewRequest("GET", "https://b.example/x", nil), via); err != nil {
		t.Errorf("redirect to https refused: %v", err)
	}
	if err := cimdCheckRedirect(httptest.NewRequest("GET", "https://b.example/x", nil), append(via, via[0], via[0])); err == nil {
		t.Error("fourth redirect allowed")
	}
}
