package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"visionmcp/internal/config"
	"visionmcp/internal/glm"
	"visionmcp/internal/logging"
	"visionmcp/internal/mcp"
	"visionmcp/internal/singleinstance"
	"visionmcp/internal/vision"
)

const version = "0.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "config" {
		cfg, _ := config.Parse([]string{"--dry-run"}, os.Getenv)
		fmt.Print(generateConfig(cfg))
		return
	}

	cfg, err := config.Parse(os.Args[1:], os.Getenv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "visionmcp: %v\n", err)
		os.Exit(2)
	}
	if cfg.DryRun {
		fmt.Print(generateConfig(cfg))
		return
	}
	logPath := cfg.LogPath
	if logPath == "" {
		if path, err := config.DefaultLogPath(); err == nil {
			logPath = path
		}
	}
	logger, closeLogger, err := logging.New(logPath, cfg.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "visionmcp: init logger: %v\n", err)
		os.Exit(2)
	}
	defer closeLogger()
	lockPath, err := singleinstance.LockPath()
	if err != nil {
		logger.ErrorFields("failed to determine lock path", map[string]any{"error": err.Error()})
		fmt.Fprintf(os.Stderr, "visionmcp: %v\n", err)
		os.Exit(2)
	}
	lock, err := singleinstance.Acquire(lockPath)
	if err != nil {
		logger.ErrorFields("failed to acquire instance lock", map[string]any{"error": err.Error(), "lock_path": lockPath})
		fmt.Fprintf(os.Stderr, "visionmcp: %v\n", err)
		exitCode := 2
		if errors.Is(err, singleinstance.ErrLocked) {
			exitCode = 3
		}
		os.Exit(exitCode)
	}
	defer singleinstance.Release(lock)
	logger.Infof("starting visionmcp %s", version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	client := http.Client{Timeout: cfg.Timeout}
	server := mcp.Server{
		GLM: glm.Client{
			Endpoint:      cfg.APIEndpoint,
			APIKey:        cfg.APIKey,
			Model:         cfg.Model,
			Retries:       cfg.Retries,
			RetryInterval: cfg.RetryInterval,
		},
		Vision: vision.Service{
			Client:     &client,
			MaxImageMB: cfg.MaxImageMB,
		},
		Logger:  logger,
		Timeout: cfg.Timeout,
	}
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		logger.ErrorFields("server stopped", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	logger.Infof("server stopped")
}

func generateConfig(cfg config.Config) string {
	executable, err := os.Executable()
	if err != nil {
		executable = filepath.Join("path", "to", "visionmcp.exe")
	}
	logPath := cfg.LogPath
	if logPath == "" {
		if path, err := config.DefaultLogPath(); err == nil {
			logPath = path
		}
	}
	return mcp.GenerateCodexConfig(mcp.CodexConfigOptions{
		Executable:    executable,
		LogPath:       logPath,
		LogLevel:      cfg.LogLevel,
		Model:         cfg.Model,
		Retries:       cfg.Retries,
		RetryInterval: cfg.RetryInterval,
	})
}
