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
