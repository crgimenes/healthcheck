package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "healthcheck_init.filo")
	err := os.WriteFile(path, []byte(content), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig(t *testing.T) {
	path := writeConfig(t, `(set Listen "127.0.0.1:9999")
(set Interval "1m")
(check-http "site" "https://example.com")
(check-ping "gw" "192.168.0.1" "30s")
`)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9999" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if cfg.Interval != time.Minute {
		t.Errorf("Interval = %v", cfg.Interval)
	}
	if cfg.Database != filepath.Join(filepath.Dir(path), "healthcheck.db") {
		t.Errorf("Database = %q", cfg.Database)
	}
	if len(cfg.Checks) != 2 {
		t.Fatalf("Checks = %v", cfg.Checks)
	}

	site := cfg.Checks[0]
	if site.Name != "site" || site.Kind != "http" || site.Target != "https://example.com" || site.Interval != time.Minute {
		t.Errorf("site = %+v", site)
	}
	gw := cfg.Checks[1]
	if gw.Name != "gw" || gw.Kind != "ping" || gw.Target != "192.168.0.1" || gw.Interval != 30*time.Second {
		t.Errorf("gw = %+v", gw)
	}
}

func TestLoadConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no checks", `(set Interval "1m")`, "no checks declared"},
		{"duplicate name", "(check-ping \"a\" \"h1\")\n(check-ping \"a\" \"h2\")", "duplicate check name"},
		{"bad check interval", `(check-ping "a" "h" "5x")`, "bad interval"},
		{"bad global interval", "(set Interval \"nope\")\n(check-ping \"a\" \"h\")", "bad Interval"},
		{"missing target", `(check-http "a")`, "want (check-http"},
		{"empty name", `(check-http "" "https://x")`, "name must be a non-empty string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.content)
			_, err := loadConfig(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := loadConfig(filepath.Join(t.TempDir(), "absent.filo"))
	if err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("err = %v", err)
	}
}
