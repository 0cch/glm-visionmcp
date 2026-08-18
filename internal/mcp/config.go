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
	return fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = ["--model", %q, "--retries", %d, "--retry-interval", %q, "--log", %q, "--log-level", %q]
env_vars = ["GLM_API_KEY"]
`, name, executable, model, retries, retryInterval.String(), logPath, logLevel)
}

func GenerateCodexCLICommand(options CodexConfigOptions) string {
	parts := []string{"codex mcp add", options.ServerName, "--env GLM_API_KEY", "--"}
	if options.ServerName == "" {
		parts[1] = "visionmcp"
	}
	parts = append(parts, options.Executable)
	return strings.Join(parts, " ")
}
