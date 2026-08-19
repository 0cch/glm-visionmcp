package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultAPIEndpoint = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
const DefaultModel = "glm-4.6v-flash"

type Config struct {
	APIKey        string
	APIEndpoint   string
	Model         string
	LogPath       string
	LogLevel      string
	Timeout       time.Duration
	MaxImageMB    int64
	Retries       int
	DryRun        bool
	RetryInterval time.Duration
	LockPath      string
}

func Parse(args []string, env func(string) string) (Config, error) {
	cfg := Config{
		APIEndpoint:   DefaultAPIEndpoint,
		Model:         DefaultModel,
		LogLevel:      "info",
		Timeout:       2 * time.Minute,
		MaxImageMB:    20,
		Retries:       5,
		RetryInterval: time.Second,
	}
	fs := flag.NewFlagSet("visionmcp", flag.ContinueOnError)
	fs.StringVar(&cfg.APIKey, "api-key", env("GLM_API_KEY"), "GLM API key (or GLM_API_KEY)")
	fs.StringVar(&cfg.APIEndpoint, "api-endpoint", cfg.APIEndpoint, "GLM chat completions endpoint")
	fs.StringVar(&cfg.Model, "model", cfg.Model, "GLM vision model")
	fs.StringVar(&cfg.LogPath, "log", "", "log file path (default: stderr)")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, error")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "API request timeout")
	fs.Int64Var(&cfg.MaxImageMB, "max-image-mb", cfg.MaxImageMB, "maximum image file size in MiB")
	fs.IntVar(&cfg.Retries, "retries", cfg.Retries, "GLM API retry count after the first request")
	fs.BoolVar(&cfg.DryRun, "dry-run", false, "print the generated Codex MCP config and exit")
	fs.DurationVar(&cfg.RetryInterval, "retry-interval", cfg.RetryInterval, "delay between GLM API retries")
	fs.StringVar(&cfg.LockPath, "lock-path", "", "single-instance lock file path (default: VISIONMCP_LOCK_PATH or user cache)")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument: %s", fs.Arg(0))
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if !c.DryRun && strings.TrimSpace(c.APIKey) == "" {
		return errors.New("API key is required (set GLM_API_KEY or use --api-key)")
	}
	if !strings.HasPrefix(c.APIEndpoint, "https://") && !strings.HasPrefix(c.APIEndpoint, "http://") {
		return errors.New("API endpoint must start with http:// or https://")
	}
	if strings.TrimSpace(c.Model) == "" {
		return errors.New("model is required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if c.MaxImageMB <= 0 {
		return errors.New("max image size must be positive")
	}
	if c.Retries < 0 {
		return errors.New("retries must not be negative")
	}
	if c.RetryInterval <= 0 {
		return errors.New("retry interval must be positive")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("log level must be debug, info, warn, or error")
	}
	return nil
}

func DefaultLogPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(executable); err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(executable), "visionmcp.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}
