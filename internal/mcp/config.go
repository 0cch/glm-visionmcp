package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	BaseURL       string
	Retries       int
	RetryInterval time.Duration
	LockPath      string
	HTTP          bool
	HTTPURL       string
}

// GenerateCodexConfig renders a [mcp_servers.*] snippet for Codex. In stdio
// mode it emits command + args; in HTTP mode it emits url so that every Codex
// session shares the single running instance.
func GenerateCodexConfig(options CodexConfigOptions) string {
	name := options.ServerName
	if name == "" {
		name = "visionmcp"
	}
	if options.HTTP {
		url := options.HTTPURL
		if url == "" {
			url = "http://127.0.0.1:8765/mcp"
		}
		return fmt.Sprintf("[mcp_servers.%s]\nurl = %q\n", name, url)
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
		strconv.Quote(strconv.Itoa(retries)),
		fmt.Sprintf("%q", "--retry-interval"),
		fmt.Sprintf("%q", retryInterval.String()),
		fmt.Sprintf("%q", "--log"),
		fmt.Sprintf("%q", logPath),
		fmt.Sprintf("%q", "--log-level"),
		fmt.Sprintf("%q", logLevel),
	}
	if options.BaseURL != "" {
		args = append(args, fmt.Sprintf("%q", "--base-url"), fmt.Sprintf("%q", options.BaseURL))
	}
	if options.LockPath != "" {
		args = append(args, fmt.Sprintf("%q", "--lock-path"), fmt.Sprintf("%q", options.LockPath))
	}
	return fmt.Sprintf(`[mcp_servers.%s]
command = %q
args = [%s]
env_vars = ["OPENAI_API_KEY", "GLM_API_KEY"]
`, name, executable, strings.Join(args, ", "))
}

func GenerateCodexCLICommand(options CodexConfigOptions) string {
	parts := []string{"codex mcp add", options.ServerName, "--env OPENAI_API_KEY --env GLM_API_KEY --"}
	if options.ServerName == "" {
		parts[1] = "visionmcp"
	}
	if options.HTTP {
		url := options.HTTPURL
		if url == "" {
			url = "http://127.0.0.1:8765/mcp"
		}
		return "codex mcp add " + options.ServerName + " --url " + url
	}
	parts = append(parts, options.Executable)
	if options.BaseURL != "" {
		parts = append(parts, "--base-url", options.BaseURL)
	}
	if options.LockPath != "" {
		parts = append(parts, "--lock-path", options.LockPath)
	}
	return strings.Join(parts, " ")
}
