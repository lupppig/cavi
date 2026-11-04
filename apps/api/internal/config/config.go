// Package config loads API configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
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

	// PublicAPIURL is this API's externally reachable base URL, used to
	// build the sign-in callback URL.
	PublicAPIURL string
	// WebURL is the frontend's base URL, where users land after signing
	// in or out.
	WebURL string
	// WorkOS credentials. Both empty disables sign-in (development only).
	WorkOSAPIKey   string
	WorkOSClientID string
	SessionTTL     time.Duration
}

// AuthEnabled reports whether sign-in is configured.
func (c Config) AuthEnabled() bool { return c.WorkOSAPIKey != "" && c.WorkOSClientID != "" }

// SecureCookies reports whether cookies must carry the Secure flag.
func (c Config) SecureCookies() bool { return c.Env != EnvDevelopment }

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
		PublicAPIURL:    strings.TrimRight(getenv("CAVI_PUBLIC_API_URL"), "/"),
		WebURL:          strings.TrimRight(getenv("CAVI_WEB_URL"), "/"),
		WorkOSAPIKey:    getenv("WORKOS_API_KEY"),
		WorkOSClientID:  getenv("WORKOS_CLIENT_ID"),
		SessionTTL:      30 * 24 * time.Hour,
	}
	if cfg.Env == EnvDevelopment {
		cfg.PublicAPIURL = valueOr(cfg.PublicAPIURL, "http://localhost:8080")
		cfg.WebURL = valueOr(cfg.WebURL, "http://localhost:3000")
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

	for name, v := range map[string]string{"CAVI_PUBLIC_API_URL": cfg.PublicAPIURL, "CAVI_WEB_URL": cfg.WebURL} {
		if v == "" {
			errs = append(errs, fmt.Errorf("%s is required outside development", name))
		} else if u, err := url.Parse(v); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
			errs = append(errs, fmt.Errorf("%s must be an http(s) origin such as https://app.example.com; got %q", name, v))
		} else if cfg.Env != EnvDevelopment && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("%s must use https outside development", name))
		}
	}

	if (cfg.WorkOSAPIKey == "") != (cfg.WorkOSClientID == "") {
		errs = append(errs, errors.New("WORKOS_API_KEY and WORKOS_CLIENT_ID must be set together"))
	} else if !cfg.AuthEnabled() && cfg.Env != EnvDevelopment {
		errs = append(errs, errors.New("WORKOS_API_KEY and WORKOS_CLIENT_ID are required outside development"))
	}

	if raw := getenv("CAVI_SESSION_TTL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < time.Hour {
			errs = append(errs, fmt.Errorf("CAVI_SESSION_TTL must be a duration of at least 1h such as 720h; got %q", raw))
		} else {
			cfg.SessionTTL = d
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
