package config

import (
	"cmp"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	Listen      string
	PublicURL   string
	Version     string
}

func validatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("KENA_PUBLIC_URL: %w", err)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https":
		return fmt.Errorf("KENA_PUBLIC_URL: scheme must be http or https, got %q", u.Scheme)
	case u.Host == "":
		return fmt.Errorf("KENA_PUBLIC_URL: host is required")
	case u.Path != "":
		return fmt.Errorf("KENA_PUBLIC_URL: must not contain a path, got %q", u.Path)
	case u.RawQuery != "":
		return fmt.Errorf("KENA_PUBLIC_URL: must not contain a query")
	case u.Fragment != "":
		return fmt.Errorf("KENA_PUBLIC_URL: must not contain a fragment")
	}
	return nil
}

func FromEnv() (Config, error) {
	databaseURL := os.Getenv("KENA_DATABASE_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("KENA_DATABASE_URL is required")
	}

	publicURL := os.Getenv("KENA_PUBLIC_URL")
	if publicURL == "" {
		return Config{}, fmt.Errorf("KENA_PUBLIC_URL is required")
	}
	publicURL = strings.TrimSuffix(publicURL, "/")
	if err := validatePublicURL(publicURL); err != nil {
		return Config{}, err
	}

	return Config{
		DatabaseURL: databaseURL,
		Listen:      cmp.Or(os.Getenv("KENA_LISTEN"), "127.0.0.1:8080"),
		PublicURL:   publicURL,
	}, nil
}
