// Package config loads API configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Environment names supported by the API.
const (
	EnvDevelopment = "development"
	EnvPreview     = "preview"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Config holds runtime configuration for the API process.
type Config struct {
	Env             string
	HTTPAddr        string
	DatabaseURL     string
	AllowedOrigins  []string
	TrustedProxies  []string
	ShutdownTimeout time.Duration
}

// Load reads configuration using getenv (usually os.Getenv).
// It returns an error describing every invalid or missing value.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Env:             valueOr(getenv("CAVI_ENV"), EnvDevelopment),
		HTTPAddr:        valueOr(getenv("CAVI_HTTP_ADDR"), ":8080"),
		DatabaseURL:     getenv("CAVI_DATABASE_URL"),
		AllowedOrigins:  splitList(getenv("CAVI_ALLOWED_ORIGINS")),
		TrustedProxies:  splitList(getenv("CAVI_TRUSTED_PROXIES")),
		ShutdownTimeout: 15 * time.Second,
	}

	var errs []error

	switch cfg.Env {
	case EnvDevelopment, EnvPreview, EnvStaging, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("CAVI_ENV must be one of development, preview, staging, production; got %q", cfg.Env))
	}

	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("CAVI_DATABASE_URL is required"))
	}

	if raw := getenv("CAVI_SHUTDOWN_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			errs = append(errs, fmt.Errorf("CAVI_SHUTDOWN_TIMEOUT must be a positive duration such as 15s; got %q", raw))
		} else {
			cfg.ShutdownTimeout = d
		}
	}

	for _, origin := range cfg.AllowedOrigins {
		if origin == "*" {
			errs = append(errs, errors.New("CAVI_ALLOWED_ORIGINS must list explicit origins; wildcard is not allowed"))
		}
	}

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, strings.TrimRight(p, "/"))
		}
	}
	return out
}
