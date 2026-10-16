package config

import (
	"strings"
	"testing"
)

func TestFromEnvMissingDatabaseURL(t *testing.T) {
	t.Setenv("KENA_PUBLIC_URL", "https://kena.example.org")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for missing KENA_DATABASE_URL")
	}
	if got := err.Error(); !strings.Contains(got, "KENA_DATABASE_URL") {
		t.Fatalf("error %q does not name KENA_DATABASE_URL", got)
	}
}

func TestFromEnvMissingPublicURL(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected error for missing KENA_PUBLIC_URL")
	}
	if got := err.Error(); !strings.Contains(got, "KENA_PUBLIC_URL") {
		t.Fatalf("error %q does not name KENA_PUBLIC_URL", got)
	}
}

func TestFromEnvDefaultListen(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	t.Setenv("KENA_PUBLIC_URL", "https://kena.example.org")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Fatalf("Listen = %q, want default", cfg.Listen)
	}
}

func TestFromEnvTrailingSlashTrimmed(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	t.Setenv("KENA_PUBLIC_URL", "https://kena.example.org/")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PublicURL != "https://kena.example.org" {
		t.Fatalf("PublicURL = %q, want trailing slash trimmed", cfg.PublicURL)
	}
}

func TestFromEnvPublicURLInvalid(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	cases := []string{
		"not a url",
		"ftp://kena.example.org",
		"https://",
		"https://kena.example.org/path",
		"https://kena.example.org?q=1",
		"https://kena.example.org#frag",
	}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			t.Setenv("KENA_PUBLIC_URL", v)
			_, err := FromEnv()
			if err == nil {
				t.Fatalf("expected error for KENA_PUBLIC_URL=%q", v)
			}
			if got := err.Error(); !strings.Contains(got, "KENA_PUBLIC_URL") {
				t.Fatalf("error %q does not name KENA_PUBLIC_URL", got)
			}
		})
	}
}

func TestFromEnvPublicURLHTTPAllowed(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	t.Setenv("KENA_PUBLIC_URL", "http://127.0.0.1:8080")
	if _, err := FromEnv(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFromEnvCustomListen(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	t.Setenv("KENA_PUBLIC_URL", "https://kena.example.org")
	t.Setenv("KENA_LISTEN", "0.0.0.0:9090")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Listen != "0.0.0.0:9090" {
		t.Fatalf("Listen = %q, want custom value", cfg.Listen)
	}
}
