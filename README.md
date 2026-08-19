# visionmcp

Vision MCP is a Go stdio or HTTP Model Context Protocol server that gives non-multimodal Codex models image understanding through an OpenAI-compatible vision model. It ships configured for GLM-4.6V-Flash but works with any provider that implements the OpenAI Chat Completions protocol (OpenAI, Azure OpenAI, OpenRouter, DeepSeek, Moonshot, local vLLM/Ollama servers, etc.).

## Build

```powershell
go build -o visionmcp.exe ./cmd/visionmcp
go test ./...
```

## Run

The server runs in stdio mode by default. Use --http to run as an HTTP MCP server (Streamable HTTP) on a local port instead, so every agent shares one long-running instance:

```powershell
.\visionmcp.exe --http --http-addr 127.0.0.1:8765
```

In both modes it exposes one tool:

- `analyze_image(prompt, image)` — analyzes exactly one image and returns text.

`image` accepts exactly one source:

- `path`: an absolute path, or a path relative to the server process working directory. Relative paths containing `..` are rejected.
- `url`: an HTTP(S) URL. Embedded credentials are rejected.
- `data`: raw base64 image bytes. A data URL prefix is not accepted.

Supported input formats are JPEG, PNG, and GIF. Images are decoded, normalized to PNG, and sent as data URLs.

## Configuration

The server talks to any OpenAI-compatible Chat Completions API. By default it targets the Zhipu AI (BigModel) endpoint for GLM-4.6V-Flash. Point it at another provider with `--base-url` (or `OPENAI_BASE_URL`):

```powershell
$env:OPENAI_API_KEY = "..."
$env:OPENAI_BASE_URL = "https://api.openai.com/v1"
.\visionmcp.exe --model gpt-4o --base-url https://api.openai.com/v1
```

- `--base-url` / `OPENAI_BASE_URL` — base URL of an OpenAI-compatible API. The full endpoint becomes `<base-url>/chat/completions`. Defaults to `https://open.bigmodel.cn/api/paas/v4`.
- `--api-endpoint` — deprecated. Sets the full Chat Completions URL directly; kept for backwards compatibility. Prefer `--base-url`.
- `--api-key` / `OPENAI_API_KEY` / `GLM_API_KEY` — API key. `OPENAI_API_KEY` takes precedence, then `GLM_API_KEY`, then `--api-key` (the flag wins over env).
- `--model` — model name, e.g. `glm-4.6v-flash`, `gpt-4o`, `qwen-vl-max`. Defaults to `glm-4.6v-flash`.
- `--http` — run as an HTTP (Streamable HTTP) MCP server instead of stdio. Codex connects to `http://<http-addr>/mcp`. Single-instance locking still applies, so only one shared instance runs per user.
- `--http-addr` — listen address for `--http` mode. Defaults to `127.0.0.1:8765`. Use `0.0.0.0:8765` to accept LAN connections.

The request is a standard OpenAI Chat Completions call: `Authorization: Bearer <key>` with a `user` message containing an `image_url` (data URL) and `text` part.

Print a Codex `config.toml` snippet without starting the server:

```powershell
$env:OPENAI_API_KEY = "..."
.\visionmcp.exe --dry-run
```

Copy the output into `~/.codex/config.toml` or a project `.codex/config.toml`. The command does not modify Codex configuration automatically. The generated snippet forwards `OPENAI_API_KEY` and `GLM_API_KEY` from your local environment:

```toml
[mcp_servers.visionmcp]
command = "D:\\Codes\\visionmcp\\visionmcp.exe"
args = ["--model", "glm-4.6v-flash", "--retries", 5, "--retry-interval", "1s", "--log", "...", "--log-level", "info"]
env_vars = ["OPENAI_API_KEY", "GLM_API_KEY"]
```

For a non-default provider, set `OPENAI_BASE_URL` (or pass `--base-url`); the generated config will include `--base-url` in `args`.

For HTTP mode, generate a shared-instance config with `--http`:

```powershell
.\visionmcp.exe --http --dry-run
```

```toml
[mcp_servers.visionmcp]
url = "http://127.0.0.1:8765/mcp"
```

Start the shared server once (e.g. as a background process), then every Codex session uses that single instance. Run `visionmcp --http --dry-run --http-addr 0.0.0.0:8765` to advertise a LAN-reachable address.

## Logging

Logs are newline-delimited JSON in `%LOCALAPPDATA%\\visionmcp\\visionmcp.log` by default. Each request includes method, request ID, and errors. Use `--log-level debug` for protocol-level diagnostics. Logs never include API keys or base64 image data.

## Security

- API keys are accepted only from `OPENAI_API_KEY`, `GLM_API_KEY`, or `--api-key`.
- Relative image paths cannot escape the server working directory. Absolute paths are supported and depend on the operating system permissions of the MCP process.
- Image URLs must use HTTP(S), have a host, and cannot embed credentials.
- Image size is capped at 20 MiB by default (`--max-image-mb`).
- API responses are capped at 10 MiB.
## Retries

API requests are retried after the initial attempt. Defaults are 5 retries with a 1-second delay:

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

The lock location can be overridden (in priority order) by `--lock-path` (CLI), then `VISIONMCP_LOCK_PATH` (environment), then the per-user cache default. This lets several independent MCP consumers each run their own visionmcp instance without colliding.
