package config

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"CAVI_DATABASE_URL": "postgres://localhost/cavi",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 15s", cfg.ShutdownTimeout)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, want empty", cfg.AllowedOrigins)
	}
	if cfg.PublicAPIURL != "http://localhost:8080" || cfg.WebURL != "http://localhost:3000" {
		t.Errorf("dev URLs = %q %q", cfg.PublicAPIURL, cfg.WebURL)
	}
	if cfg.AuthEnabled() || cfg.SecureCookies() {
		t.Error("auth should be disabled and cookies insecure in bare development")
	}
}

func TestLoadAuthSettings(t *testing.T) {
	base := map[string]string{
		"CAVI_ENV":             "production",
		"CAVI_DATABASE_URL":    "postgres://db/cavi",
		"CAVI_PUBLIC_API_URL":  "https://api.example.com/",
		"CAVI_WEB_URL":         "https://app.example.com",
		"WORKOS_API_KEY":       "sk_live_x",
		"WORKOS_CLIENT_ID":     "client_x",
		"CAVI_ALLOWED_ORIGINS": "https://app.example.com",
	}
	cfg, err := Load(env(base))
	if err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}
	if cfg.PublicAPIURL != "https://api.example.com" || !cfg.AuthEnabled() || !cfg.SecureCookies() {
		t.Errorf("cfg = %+v", cfg)
	}

	bad := []struct {
		name, key, value, fragment string
	}{
		{"missing workos in production", "WORKOS_API_KEY", "", "WORKOS_API_KEY"},
		{"http web url in production", "CAVI_WEB_URL", "http://app.example.com", "https"},
		{"web url with path", "CAVI_WEB_URL", "https://app.example.com/app", "CAVI_WEB_URL"},
		{"missing api url in production", "CAVI_PUBLIC_API_URL", "", "CAVI_PUBLIC_API_URL"},
		{"short session ttl", "CAVI_SESSION_TTL", "5m", "CAVI_SESSION_TTL"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			vals := map[string]string{}
			for k, v := range base {
				vals[k] = v
			}
			vals[tc.key] = tc.value
			_, err := Load(env(vals))
			if err == nil || !strings.Contains(err.Error(), tc.fragment) {
				t.Errorf("err = %v, want mention of %s", err, tc.fragment)
			}
		})
	}
}

func TestLoadParsesValues(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"CAVI_ENV":              "staging",
		"CAVI_HTTP_ADDR":        ":9000",
		"CAVI_DATABASE_URL":     "postgres://db/cavi",
		"CAVI_ALLOWED_ORIGINS":  " http://localhost:3000/ , https://app.example.com ,,",
		"CAVI_SHUTDOWN_TIMEOUT": "30s",
		"CAVI_PUBLIC_API_URL":   "https://api.example.com",
		"CAVI_WEB_URL":          "https://app.example.com",
		"WORKOS_API_KEY":        "sk_test_x",
		"WORKOS_CLIENT_ID":      "client_x",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"http://localhost:3000", "https://app.example.com"}
	if strings.Join(cfg.AllowedOrigins, "|") != strings.Join(want, "|") {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 30s", cfg.ShutdownTimeout)
	}
	if cfg.Env != EnvStaging || cfg.HTTPAddr != ":9000" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	_, err := Load(env(map[string]string{
		"CAVI_ENV":              "prod",
		"CAVI_ALLOWED_ORIGINS":  "*",
		"CAVI_SHUTDOWN_TIMEOUT": "soon",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, fragment := range []string{"CAVI_ENV", "CAVI_DATABASE_URL", "CAVI_ALLOWED_ORIGINS", "CAVI_SHUTDOWN_TIMEOUT"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not mention %s", err, fragment)
		}
	}
}
