package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseDefaultsAndEnv(t *testing.T) {
	cfg, err := Parse(nil, func(key string) string {
		if key == "GLM_API_KEY" {
			return "key"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.APIEndpoint != DefaultAPIEndpoint || cfg.Model != DefaultModel || cfg.Timeout != 2*time.Minute {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestParseRejectsMissingKey(t *testing.T) {
	_, err := Parse(nil, func(string) string { return "" })
	if err == nil || err.Error() != "API key is required (set GLM_API_KEY or use --api-key)" {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsUnexpectedArg(t *testing.T) {
	_, err := Parse([]string{"extra"}, func(string) string { return "key" })
	if err == nil || err.Error() != "unexpected argument: extra" {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestValidateLogLevel(t *testing.T) {
	cfg := Config{APIKey: "key", APIEndpoint: DefaultAPIEndpoint, Model: DefaultModel, Timeout: time.Second, MaxImageMB: 1, Retries: 1, RetryInterval: time.Second, LogLevel: "debug"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	cfg.LogLevel = "trace"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected log-level error")
	}
}

func TestDefaultLogPathIsBesideExecutable(t *testing.T) {
	got, err := DefaultLogPath()
	if err != nil {
		t.Fatalf("DefaultLogPath() error = %v", err)
	}
	if filepath.Base(got) != "visionmcp.log" {
		t.Fatalf("DefaultLogPath() = %q", got)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(got) != filepath.Dir(executable) {
		t.Fatalf("log dir = %q, executable dir = %q", filepath.Dir(got), filepath.Dir(executable))
	}
}
func TestParseRetryDefaultsAndOverrides(t *testing.T) {
	cfg, err := Parse(nil, func(string) string { return "key" })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Retries != 5 || cfg.RetryInterval != time.Second {
		t.Fatalf("unexpected retry defaults: %+v", cfg)
	}
	cfg, err = Parse([]string{"--retries", "2", "--retry-interval", "250ms"}, func(string) string { return "key" })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Retries != 2 || cfg.RetryInterval != 250*time.Millisecond {
		t.Fatalf("unexpected retry config: %+v", cfg)
	}
}

func TestValidateRejectsInvalidRetries(t *testing.T) {
	base := Config{APIKey: "key", APIEndpoint: DefaultAPIEndpoint, Model: DefaultModel, Timeout: time.Second, MaxImageMB: 1, LogLevel: "info", Retries: 1, RetryInterval: time.Second}
	base.Retries = -1
	if err := base.Validate(); err == nil || err.Error() != "retries must not be negative" {
		t.Fatalf("Validate() error = %v", err)
	}
	base.Retries = 1
	base.RetryInterval = 0
	if err := base.Validate(); err == nil || err.Error() != "retry interval must be positive" {
		t.Fatalf("Validate() error = %v", err)
	}
}
func TestParseDryRunWithoutAPIKey(t *testing.T) {
	cfg, err := Parse([]string{"--dry-run"}, func(string) string { return "" })
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !cfg.DryRun {
		t.Fatal("expected dry-run to be enabled")
	}
}
