package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	Listen      string
	PublicURL   string
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

	listen := os.Getenv("KENA_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:8080"
	}

	return Config{
		DatabaseURL: databaseURL,
		Listen:      listen,
		PublicURL:   publicURL,
	}, nil
}
