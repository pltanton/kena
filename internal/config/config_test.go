package config

import (
	"strings"
	"testing"
)

func TestFromEnvErrorsNameTheVariable(t *testing.T) {
	for _, c := range []struct{ db, public, want string }{
		{"", "https://kena.example.org", "KENA_DATABASE_URL"},
		{"postgres://localhost/kena", "", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "not a url", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "ftp://kena.example.org", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "https://", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "https://kena.example.org/path", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "https://kena.example.org?q=1", "KENA_PUBLIC_URL"},
		{"postgres://localhost/kena", "https://kena.example.org#frag", "KENA_PUBLIC_URL"},
	} {
		t.Setenv("KENA_DATABASE_URL", c.db)
		t.Setenv("KENA_PUBLIC_URL", c.public)
		if _, err := FromEnv(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("db=%q public=%q: err = %v, want one naming %s", c.db, c.public, err, c.want)
		}
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("KENA_DATABASE_URL", "postgres://localhost/kena")
	for _, c := range []struct{ public, listen, wantPublic, wantListen string }{
		{"https://kena.example.org/", "", "https://kena.example.org", "127.0.0.1:8080"},
		{"http://127.0.0.1:8080", "0.0.0.0:9090", "http://127.0.0.1:8080", "0.0.0.0:9090"},
	} {
		t.Setenv("KENA_PUBLIC_URL", c.public)
		t.Setenv("KENA_LISTEN", c.listen)
		cfg, err := FromEnv()
		if err != nil || cfg.PublicURL != c.wantPublic || cfg.Listen != c.wantListen {
			t.Errorf("%+v: got %+v, %v", c, cfg, err)
		}
	}
}
