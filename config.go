package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/crgimenes/filo"
)

const defaultInterval = 5 * time.Minute

type Check struct {
	Name     string        `json:"name"`
	Kind     string        `json:"kind"`
	Target   string        `json:"target"`
	Interval time.Duration `json:"-"`
}

type Config struct {
	Listen   string
	Database string
	Interval time.Duration
	Checks   []Check
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

func configPath() string {
	const local = "healthcheck_init.filo"
	if fileExists(local) {
		return local
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return local
	}
	return filepath.Join(dir, "healthcheck", "init.filo")
}

func checkBuiltin(kind string, checks *[]Check) filo.Builtin {
	return func(_ context.Context, args []filo.Value) (filo.Value, error) {
		if len(args) < 2 || len(args) > 3 {
			return filo.VBool(false), fmt.Errorf("check-%s: want (check-%s name target [interval])", kind, kind)
		}

		name, err := args[0].AsString()
		if err != nil || name == "" {
			return filo.VBool(false), fmt.Errorf("check-%s: name must be a non-empty string", kind)
		}

		target, err := args[1].AsString()
		if err != nil || target == "" {
			return filo.VBool(false), fmt.Errorf("check-%s %q: target must be a non-empty string", kind, name)
		}

		var interval time.Duration
		if len(args) == 3 {
			s, err := args[2].AsString()
			if err != nil {
				return filo.VBool(false), fmt.Errorf("check-%s %q: interval must be a string like \"5m\"", kind, name)
			}
			interval, err = time.ParseDuration(s)
			if err != nil || interval <= 0 {
				return filo.VBool(false), fmt.Errorf("check-%s %q: bad interval %q", kind, name, s)
			}
		}

		*checks = append(*checks, Check{Name: name, Kind: kind, Target: target, Interval: interval})
		return filo.VBool(true), nil
	}
}

func loadConfig(name string) (*Config, error) {
	if !fileExists(name) {
		return nil, fmt.Errorf("config file not found: %s\ncreate healthcheck_init.filo in the current directory, put init.filo in the platform config dir, or pass -config (run healthcheck -h for an example)", name)
	}

	f := filo.New()
	defer f.Close()

	f.SetGlobal("Listen", "127.0.0.1:8317")
	f.SetGlobal("Database", filepath.Join(filepath.Dir(name), "healthcheck.db"))
	f.SetGlobal("Interval", defaultInterval.String())

	var checks []Check
	for _, kind := range []string{"http", "ping"} {
		err := f.RegisterBuiltin("check-"+kind, checkBuiltin(kind, &checks))
		if err != nil {
			return nil, err
		}
	}

	b, err := os.ReadFile(filepath.Clean(name))
	if err != nil {
		return nil, err
	}
	err = f.DoString(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}

	cfg := &Config{Checks: checks}

	cfg.Listen, err = f.GetString("Listen")
	if err != nil {
		return nil, fmt.Errorf("%s: Listen: %w", name, err)
	}
	cfg.Database, err = f.GetString("Database")
	if err != nil {
		return nil, fmt.Errorf("%s: Database: %w", name, err)
	}

	interval, err := f.GetString("Interval")
	if err != nil {
		return nil, fmt.Errorf("%s: Interval: %w", name, err)
	}
	cfg.Interval, err = time.ParseDuration(interval)
	if err != nil || cfg.Interval <= 0 {
		return nil, fmt.Errorf("%s: bad Interval %q", name, interval)
	}

	if len(cfg.Checks) == 0 {
		return nil, fmt.Errorf("%s: no checks declared (use check-http or check-ping)", name)
	}

	seen := make(map[string]bool, len(cfg.Checks))
	for i, c := range cfg.Checks {
		if seen[c.Name] {
			return nil, fmt.Errorf("%s: duplicate check name %q", name, c.Name)
		}
		seen[c.Name] = true
		if c.Interval == 0 {
			cfg.Checks[i].Interval = cfg.Interval
		}
	}

	return cfg, nil
}
