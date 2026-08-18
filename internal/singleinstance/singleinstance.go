package singleinstance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var ErrLocked = errors.New("another visionmcp instance is already running")

func LockPath() (string, error) {
	base := os.Getenv("VISIONMCP_LOCK_PATH")
	if base != "" {
		return base, nil
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("determine user cache directory: %w", err)
	}
	return filepath.Join(cacheDir, "visionmcp", "visionmcp.lock"), nil
}

func Acquire(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := lockProcess(file); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("lock process: %w", err)
	}
	return file, nil
}

func Release(file *os.File) {
	if file == nil {
		return
	}
	_ = file.Close()
}

var _ = runtime.GOOS
