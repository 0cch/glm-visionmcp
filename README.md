# visionmcp

**中文版：[README.zh-CN.md](README.zh-CN.md)**

Vision MCP is a Go stdio or HTTP Model Context Protocol server that gives non-multimodal models (Codex, DeepSeek, Claude CLI, etc.) image understanding through any OpenAI-compatible vision model. It ships configured for GLM-4.6V-Flash but works with every provider that implements the OpenAI Chat Completions protocol: OpenAI, Azure OpenAI, OpenRouter, DeepSeek, Moonshot, Qwen, local vLLM/Ollama, etc.

## Table of contents

- [Features](#features)
- [Build](#build)
- [Run](#run)
- [Configuration](#configuration)
- [MCP clients](#mcp-clients)
- [Single instance](#single-instance)
- [Logging](#logging)
- [Security](#security)
- [Retries](#retries)
- [DeepSeek Harness integration](#deepseek-harness-integration)
- [Development](#development)

## Features

- One tool `analyze_image(prompt, image)` — analyzes exactly one image and returns text.
- **stdio mode** (default) and **HTTP mode** (`--http`) for sharing one instance across every agent.
- OpenAI-compatible: works with any Chat Completions endpoint via `--base-url`.
- Single-instance lock so multiple MCP consumers share one process without colliding.
- Images decoded and normalized to PNG; size-capped before upload.
- Retry with backoff for transient failures.

## Build

```powershell
go build -o visionmcp.exe ./cmd/visionmcp
go test ./...
```

The binary is `visionmcp.exe` (or `visionmcp` on macOS/Linux). No Go toolchain is needed to run the prebuilt binary.

## Run

The server exposes one tool:

- `analyze_image(prompt, image)` — analyzes exactly one image and returns text.

`image` accepts exactly one source:

- `path`: an absolute path, or a path relative to the server process working directory. Relative paths containing `..` are rejected.
- `url`: an HTTP(S) URL. Embedded credentials are rejected.
- `data`: raw base64 image bytes. A data URL prefix is not accepted.

Supported input formats are JPEG, PNG, and GIF. Images are decoded, normalized to PNG, and sent as data URLs.

### stdio mode (default)

```powershell
.\visionmcp.exe
```

Reads JSON-RPC requests from stdin and writes responses to stdout — the standard MCP transport used by Codex and other clients.

### HTTP mode (shared instance)

```powershell
.\visionmcp.exe --http --http-addr 127.0.0.1:8765
```

Runs an MCP **Streamable HTTP** server on `<http-addr>/mcp`. Every agent connects to the same URL and shares one long-running instance, so image analysis is not re-created per client.

```powershell
.\visionmcp.exe --http --http-addr 0.0.0.0:8765
```

Binds all interfaces so other machines on the LAN can reach it. Keep `127.0.0.1` unless you need network access.

## Configuration

The server talks to any OpenAI-compatible Chat Completions API. By default it targets the Zhipu AI (BigModel) endpoint for GLM-4.6V-Flash:

```powershell
$env:OPENAI_API_KEY = "..."
$env:OPENAI_BASE_URL = "https://api.openai.com/v1"
.\visionmcp.exe --model gpt-4o --base-url https://api.openai.com/v1
```

### Flags

| Flag | Description | Default |
|---|---|---|
| `--api-key` | API key. Overrides `OPENAI_API_KEY` / `GLM_API_KEY`. | env |
| `--base-url` | OpenAI-compatible base URL. Endpoint becomes `<base-url>/chat/completions`. Also read from `OPENAI_BASE_URL`. | `https://open.bigmodel.cn/api/paas/v4` |
| `--api-endpoint` | Deprecated: full Chat Completions URL. Prefer `--base-url`. | — |
| `--model` | Vision model name (`glm-4.6v-flash`, `gpt-4o`, `qwen-vl-max`, ...). | `glm-4.6v-flash` |
| `--http` | Run as HTTP (Streamable HTTP) MCP server instead of stdio. | `false` |
| `--http-addr` | Listen address for `--http` mode. | `127.0.0.1:8765` |
| `--timeout` | Per-request API timeout. | `2m` |
| `--max-image-mb` | Maximum image file size in MiB (rejects larger uploads). | `20` |
| `--retries` | Retry count after the first request. | `5` |
| `--retry-interval` | Delay between retries. | `1s` |
| `--log` | Log file path (defaults to `visionmcp.log` next to the executable). | exe dir |
| `--log-level` | `debug`, `info`, `warn`, or `error`. | `info` |
| `--lock-path` | Single-instance lock file path. | see [Single instance](#single-instance) |
| `--dry-run` | Print the generated Codex config and exit. | `false` |

### Environment variables

| Variable | Effect |
|---|---|
| `OPENAI_API_KEY` | API key (takes precedence over `GLM_API_KEY`, overridden by `--api-key`). |
| `GLM_API_KEY` | Fallback API key. |
| `OPENAI_BASE_URL` | Base URL (overridden by `--base-url`). |
| `VISIONMCP_LOCK_PATH` | Lock file path (overridden by `--lock-path`). |

The request is a standard OpenAI Chat Completions call: `Authorization: Bearer <key>` with a `user` message containing an `image_url` (data URL) and `text` part.

## MCP clients

### Print the Codex config

Print a `[mcp_servers.*]` snippet without starting the server:

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

For a non-default provider, set `OPENAI_BASE_URL` (or pass `--base-url`); the generated config includes `--base-url` in `args`.

### Codex + HTTP (shared instance)

Generate a shared-instance config:

```powershell
.\visionmcp.exe --http --dry-run
```

```toml
[mcp_servers.visionmcp]
url = "http://127.0.0.1:8765/mcp"
```

Or register it with the Codex CLI directly:

```sh
codex mcp add visionmcp --url http://127.0.0.1:8765/mcp
```

Start the shared server once (e.g. as a background process), then every Codex session uses that single instance. Run `visionmcp --http --dry-run --http-addr 0.0.0.0:8765` to advertise a LAN-reachable address.

### Other MCP clients

Any MCP client that supports stdio can launch `visionmcp.exe` and call `analyze_image`. Clients that support Streamable HTTP connect to the `--http` URL.

## Single instance

Only one `visionmcp` process is allowed per user. The server uses an OS-level exclusive lock on:

- Windows: `%LOCALAPPDATA%\visionmcp\visionmcp.lock`
- macOS: `~/Library/Caches/visionmcp/visionmcp.lock`
- Linux/XDG: `~/.cache/visionmcp/visionmcp.lock`

If another instance is already running, the new process exits with code `3`. The lock is released automatically when the owning process exits.

The lock location can be overridden (in priority order) by `--lock-path` (CLI), then `VISIONMCP_LOCK_PATH` (environment), then the per-user cache default. This lets several independent MCP consumers each run their own visionmcp instance without colliding.

## Logging

Logs are newline-delimited JSON. By default they are written to `visionmcp.log` next to the executable (falling back to stderr when the executable path cannot be determined); pass `--log <path>` to choose another file. Each entry includes timestamp, level, message, method, and request ID. Use `--log-level debug` for protocol-level diagnostics. Logs never include API keys or base64 image data.

## Security

- API keys are accepted only from `OPENAI_API_KEY`, `GLM_API_KEY`, or `--api-key`.
- Relative image paths cannot escape the server working directory. Absolute paths are supported and depend on the operating system permissions of the MCP process.
- Image URLs must use HTTP(S), have a host, and cannot embed credentials.
- Image size is capped at 20 MiB by default (`--max-image-mb`).
- API responses are capped at 10 MiB.
- In HTTP mode, bind to `127.0.0.1` unless you intentionally want LAN access; the server has no authentication layer.

## Retries

API requests are retried after the initial attempt. Defaults are 5 retries with a 1-second delay:

```powershell
.\visionmcp.exe --retries 5 --retry-interval 1s
```

Authentication errors are not retried; transient network failures and GLM overload/rate-limit code `1305` are retried.

## DeepSeek Harness integration

This server can also be bridged into [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) as a plugin. The bridge does not modify this codebase — visionmcp stays a standalone stdio MCP server and acts as the underlying capability provider. Harness's built-in `@deepseek-ai/dsh-mcp-client` spawns it, discovers the `analyze_image` tool, and registers it as `mcp__vision__analyze_image`.

See [dsh-vision-mcp/README.md](dsh-vision-mcp/README.md) for installation and usage.

## Development

```powershell
go test ./...
go vet ./...
gofmt -l .
```

The server code lives in `cmd/visionmcp` and `internal/`. The HTTP transport is `internal/mcp/http.go`; the stdio transport and JSON-RPC handling are in `internal/mcp/server.go`.
