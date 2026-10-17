package csp

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func Policy(formAction ...string) string {
	return "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; " +
		"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
		"form-action " + strings.Join(append([]string{"'self'"}, formAction...), " ") + "; frame-ancestors 'none'"
}

func Set(h http.Header, formAction ...string) {
	h.Set("Content-Security-Policy", Policy(formAction...))
	h.Set("X-Frame-Options", "DENY")
}

// CSP host-sources can't express IPv6 literals, so those fall back to the scheme.
func Source(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.To4() == nil {
		return scheme + ":", true
	}
	return scheme + "://" + strings.ToLower(u.Host), true
}
