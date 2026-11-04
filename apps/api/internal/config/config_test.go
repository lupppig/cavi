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
}

func TestLoadParsesValues(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"CAVI_ENV":              "staging",
		"CAVI_HTTP_ADDR":        ":9000",
		"CAVI_DATABASE_URL":     "postgres://db/cavi",
		"CAVI_ALLOWED_ORIGINS":  " http://localhost:3000/ , https://app.example.com ,,",
		"CAVI_SHUTDOWN_TIMEOUT": "30s",
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
