package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CodexConfigOptions struct {
	ServerName    string
	Executable    string
	WorkingDir    string
	LogPath       string
	LogLevel      string
	Model         string
	Retries       int
	RetryInterval time.Duration
	LockPath      string
}

func GenerateCodexConfig(options CodexConfigOptions) string {
	name := options.ServerName
	if name == "" {
		name = "visionmcp"
	}
	executable := options.Executable
	if executable == "" {
		executable = filepath.Join("path", "to", "visionmcp.exe")
	}
	if options.WorkingDir != "" {
		executable = filepath.Join(options.WorkingDir, "visionmcp.exe")
	}
	logPath := options.LogPath
	if logPath == "" {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			logPath = filepath.Join(cacheDir, "visionmcp", "visionmcp.log")
		} else {
			logPath = filepath.Join("path", "to", "visionmcp.log")
		}
	}
	logLevel := options.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}
	model := options.Model
	if model == "" {
		model = "glm-4.6v-flash"
	}
	retries := options.Retries
	if retries == 0 {
		retries = 5
	}
	retryInterval := options.RetryInterval
	if retryInterval == 0 {
		retryInterval = time.Second
	}
	args := []string{
		fmt.Sprintf("%q", "--model"),
		fmt.Sprintf("%q", model),
		fmt.Sprintf("%q", "--retries"),
		fmt.Sprintf("%d", retries),
		fmt.Sprintf("%q", "--retry-interval"),
		fmt.Sprintf("%q", retryInterval.String()),
		fmt.Sprintf("%q", "--log"),
		fmt.Sprintf("%q", logPath),
		fmt.Sprintf("%q", "--log-level"),
		fmt.Sprintf("%q", logLevel),
	}
	if options.LockPath != "" {
		args = append(args, fmt.Sprintf("%q", "--lock-path"), fmt.Sprintf("%q", options.LockPath))
	}
	return fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = [%s]
env_vars = ["GLM_API_KEY"]
`, name, executable, strings.Join(args, ", "))
}

func GenerateCodexCLICommand(options CodexConfigOptions) string {
	parts := []string{"codex mcp add", options.ServerName, "--env GLM_API_KEY", "--"}
	if options.ServerName == "" {
		parts[1] = "visionmcp"
	}
	parts = append(parts, options.Executable)
	if options.LockPath != "" {
		parts = append(parts, "--lock-path", options.LockPath)
	}
	return strings.Join(parts, " ")
}
