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
	OIDC        OIDC
}

type OIDC struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	Name         string
}

func oidcFromEnv() (OIDC, error) {
	o := OIDC{
		Issuer:       strings.TrimSuffix(os.Getenv("KENA_OIDC_ISSUER"), "/"),
		ClientID:     os.Getenv("KENA_OIDC_CLIENT_ID"),
		ClientSecret: os.Getenv("KENA_OIDC_CLIENT_SECRET"),
		Name:         cmp.Or(os.Getenv("KENA_OIDC_NAME"), "SSO"),
	}
	if o.Issuer == "" {
		return OIDC{}, nil
	}
	if u, err := url.Parse(o.Issuer); err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
		return OIDC{}, fmt.Errorf("KENA_OIDC_ISSUER: must be an http(s) URL, got %q", o.Issuer)
	}
	if o.ClientID == "" {
		return OIDC{}, fmt.Errorf("KENA_OIDC_CLIENT_ID is required with KENA_OIDC_ISSUER")
	}
	return o, nil
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

	oidc, err := oidcFromEnv()
	if err != nil {
		return Config{}, err
	}
	return Config{
		DatabaseURL: databaseURL,
		Listen:      cmp.Or(os.Getenv("KENA_LISTEN"), "127.0.0.1:8080"),
		PublicURL:   publicURL,
		OIDC:        oidc,
	}, nil
}
