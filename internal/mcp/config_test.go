package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCodexConfig(t *testing.T) {
	got := GenerateCodexConfig(CodexConfigOptions{
		ServerName: "vision",
		WorkingDir: `D:\Tools\visionmcp`,
		LogPath:    `D:\Logs\visionmcp.log`,
		LogLevel:   "debug",
		Model:      "glm-4.6v-flash",
	})
	want := `[mcp_servers.vision]` + "\n" +
		`command = "` + strings.ReplaceAll(filepath.Join(`D:\Tools\visionmcp`, "visionmcp.exe"), `\`, `\\`) + `"` + "\n" +
		`args = ["--model", "glm-4.6v-flash", "--retries", "5", "--retry-interval", "1s", "--log", "D:\\Logs\\visionmcp.log", "--log-level", "debug"]` + "\n" +
		`env_vars = ["GLM_API_KEY"]` + "\n"
	if got != want {
		t.Fatalf("GenerateCodexConfig() =\n%s\nwant:\n%s", got, want)
	}
	if !strings.Contains(got, "env_vars = [\"GLM_API_KEY\"]") {
		t.Fatalf("config does not forward API key env: %s", got)
	}
}

func TestGenerateCodexConfigDefaults(t *testing.T) {
	got := GenerateCodexConfig(CodexConfigOptions{})
	if !strings.Contains(got, "[mcp_servers.visionmcp]") || !strings.Contains(got, "glm-4.6v-flash") || !strings.Contains(got, "info") {
		t.Fatalf("config = %s", got)
	}
}

func TestGenerateCodexConfigLockPath(t *testing.T) {
	got := GenerateCodexConfig(CodexConfigOptions{
		ServerName: "vision",
		Executable: `D:\Tools\visionmcp\visionmcp.exe`,
		LogPath:    `D:\Logs\visionmcp.log`,
		Model:      "glm-4.6v-flash",
		LockPath:   `C:\locks\visionmcp.lock`,
	})
	if !strings.Contains(got, `"--lock-path", "C:\\locks\\visionmcp.lock"`) {
		t.Fatalf("config missing lock-path arg: %s", got)
	}
}

func TestGenerateCodexCLICommand(t *testing.T) {
	got := GenerateCodexCLICommand(CodexConfigOptions{ServerName: "vision", Executable: `D:\Tools\visionmcp\visionmcp.exe`})
	want := `codex mcp add vision --env GLM_API_KEY -- D:\Tools\visionmcp\visionmcp.exe`
	if got != want {
		t.Fatalf("GenerateCodexCLICommand() = %s, want %s", got, want)
	}
	gotLock := GenerateCodexCLICommand(CodexConfigOptions{ServerName: "vision", Executable: `D:\Tools\visionmcp\visionmcp.exe`, LockPath: `C:\locks\visionmcp.lock`})
	wantLock := `codex mcp add vision --env GLM_API_KEY -- D:\Tools\visionmcp\visionmcp.exe --lock-path C:\locks\visionmcp.lock`
	if gotLock != wantLock {
		t.Fatalf("GenerateCodexCLICommand() lock = %s, want %s", gotLock, wantLock)
	}
}
