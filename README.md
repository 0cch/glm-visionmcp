# visionmcp

Vision MCP is a Go stdio Model Context Protocol server that gives non-multimodal Codex models image understanding through GLM-4.6V-Flash.

## Build

```powershell
go build -o visionmcp.exe ./cmd/visionmcp
go test ./...
```

## Run

The server communicates over stdin/stdout and exposes one tool:

- `analyze_image(prompt, image)` — analyzes exactly one image and returns text.

`image` accepts exactly one source:

- `path`: an absolute path, or a path relative to the server process working directory. Relative paths containing `..` are rejected.
- `url`: an HTTP(S) URL. Embedded credentials are rejected.
- `data`: raw base64 image bytes. A data URL prefix is not accepted.

Supported input formats are JPEG, PNG, and GIF. Images are decoded, normalized to PNG, and sent as data URLs.

## Configuration

Print a Codex `config.toml` snippet without starting the server:

```powershell
$env:GLM_API_KEY = "..."
.\visionmcp.exe --dry-run
```

Copy the output into `~/.codex/config.toml` or a project `.codex/config.toml`. The command does not modify Codex configuration automatically. The generated snippet forwards `GLM_API_KEY` from your local environment:

```toml
[mcp_servers.visionmcp]
command = "D:\\Codes\\visionmcp\\visionmcp.exe"
args = ["--model", "glm-4.6v-flash", "--retries", 5, "--retry-interval", "1s", "--log", "...", "--log-level", "info"]
env_vars = ["GLM_API_KEY"]
```

## Logging

Logs are newline-delimited JSON in `%LOCALAPPDATA%\\visionmcp\\visionmcp.log` by default. Each request includes method, request ID, and errors. Use `--log-level debug` for protocol-level diagnostics. Logs never include API keys or base64 image data.

## Security

- API keys are accepted only from `GLM_API_KEY` or `--api-key`.
- Relative image paths cannot escape the server working directory. Absolute paths are supported and depend on the operating system permissions of the MCP process.
- Image URLs must use HTTP(S), have a host, and cannot embed credentials.
- Image size is capped at 20 MiB by default (`--max-image-mb`).
- GLM responses are capped at 10 MiB.
## Retries

GLM requests are retried after the initial attempt. Defaults are 5 retries with a 1-second delay:

```powershell
.\visionmcp.exe --retries 5 --retry-interval 1s
```

Authentication errors are not retried; transient network failures and GLM overload/rate-limit code `1305` are retried.

## DeepSeek Harness integration

This server can also be bridged into [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) as a plugin. The bridge does not modify this codebase — visionmcp stays a standalone stdio MCP server and acts as the underlying capability provider. Harness's built-in `@deepseek-ai/dsh-mcp-client` spawns it, discovers the `analyze_image` tool, and registers it as `mcp__vision__analyze_image`.

See [dsh-vision-mcp/README.md](dsh-vision-mcp/README.md) for installation and usage.
## Single instance

Only one `visionmcp` process is allowed per user. The server uses an OS-level exclusive lock on:

- Windows: `%LOCALAPPDATA%\visionmcp\visionmcp.lock`
- macOS: `~/Library/Caches/visionmcp/visionmcp.lock`
- Linux/XDG: `~/.cache/visionmcp/visionmcp.lock`

If another instance is already running, the new process exits with code `3`. The lock is released automatically when the owning process exits. `VISIONMCP_LOCK_PATH` can override the lock location for testing.

