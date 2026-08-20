# dsh-vision-mcp

A generic plugin bundle that gives [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) vision capabilities. It uses [visionmcp](../../README.md) (a GLM-4.6V-Flash based vision MCP server) as the underlying capability provider, so **any non-multimodal model** (DeepSeek, pi-ai, custom providers, etc.) can upload/paste images and have them analyzed with GLM automatically — no more "the current model does not support images" errors.

It **does not modify any visionmcp code** nor the dsh framework. visionmcp stays a standalone executable (`visionmcp.exe`); this plugin spawns it over stdio and calls `analyze_image`.

**中文版：[README.zh-CN.md](README.zh-CN.md)**

## Table of contents

- [How it works](#how-it-works)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Troubleshooting](#troubleshooting)
- [Known limitations](#known-limitations)

## How it works

The core mechanism is borrowed from [dsh-vision/vision-route](https://github.com/314857493/dsh-vision/tree/main/packages/vision-route), but generalized: **for every text-only provider route it automatically registers a parallel `<provider>-vision` wrapper route**.

```
DeepSeek Harness (dsh)
  └─ dsh-vision-mcp
        ├─ listens to llm/adapters-updated and scans all provider routes
        │     └─ auto-registers deepseek-official-vision / pi-ai-vision / ...
        │           wrapper routes:
        │             resolveModel() declares inputModalities: ['text','image']
        │               → api-proxy attachment pre-check allows image upload/paste
        │             stream() sends image blocks to visionmcp for text, then delegates to the real adapter
        ├─ native tool mcp__vision__analyze_image (models can analyze image files on demand)
        ├─ system-prompt guidance (teaches models when to use the tool)
        └─ a single visionmcp.exe child process (shared by tool and transcription)
```

- **Generic**: any provider route whose models declare text-only input automatically gets a parallel `-vision` route; multimodal routes that already support images are not wrapped and stay untouched.
- **Original routes unchanged**: official routes like `deepseek-official` stay text-only with zero overhead; to use vision, pick the model group with the "+ auto vision" label in the model selector.
- **Single process**: all `-vision` routes and the tool share one visionmcp connection; the whole process runs exactly one visionmcp.exe.
- **Graceful degradation**: if an image fails to analyze, it is replaced with a `[visionmcp: ...]` placeholder so the conversation continues; transcription results are cached per `attachmentId` in-process.

## Installation

First build visionmcp (see [README](../../README.md#build)), make sure `visionmcp.exe` is ready, and have a `GLM_API_KEY`.

### Option 1: install as a bundle (recommended)

```sh
dsh plugin --profile web add /path/to/dsh-vision-mcp
```

### Option 2: temporary enable with --patch (quick trial)

```sh
dsh web --patch /path/to/dsh-vision-mcp/cordis.patch.yml
```

`cordis.patch.yml` declares `dsh.bundle.patch`, so `dsh plugin add` applies it as a separate layer.

> When installing from GitHub (`dsh plugin add github:you/dsh-vision-mcp`), pnpm fetches the source and runs the `prepare` build; the first time it will ask you to add `dsh-vision-mcp` to `allowBuilds` in the profile's `pnpm-workspace.yaml`. That is a normal security confirmation.

## Configuration

### Environment variables

| Variable | Effect | Default |
|---|---|---|
| `VISIONMCP_PATH` | Absolute path to visionmcp.exe (the `command` config takes precedence). | `<cwd>/visionmcp.exe` |
| `GLM_API_KEY` / `OPENAI_API_KEY` | Vision API key (passed through to the visionmcp child unchanged). | empty |
| `VISIONMCP_LOCK_PATH` | visionmcp single-instance lock path (the `lockPath` config takes precedence). | `$DSH_HOME/visionmcp-bridge.lock` (falls back to `LOCALAPPDATA`/`HOME` without `DSH_HOME`) |

The visionmcp executable is resolved in this order: `config.command` → `VISIONMCP_PATH` → `<cwd>/visionmcp.exe`.

### Plugin config

Override via `id: vision` in the profile's `cordis.patch.yml`:

```yaml
- id: vision
  config:
    routeEnabled: true       # auto-register -vision parallel routes for text-only providers (default true)
    routeSuffix: '-vision'   # wrapper route suffix: deepseek-official → deepseek-official-vision
    toolEnabled: true        # register the mcp__vision__analyze_image tool (default true)
    bridgeEnabled: false     # legacy llm/stream bridge switch, superseded by route wrapping (default false)
    guidance: true           # inject system-prompt guidance (default true)
    guidanceText: ''         # custom guidance text; empty uses the built-in default
    sectionOrder: 120        # position of the guidance in the system prompt (tool guidance band 100-199)
    command: 'C:\\tools\\visionmcp.exe'   # override the visionmcp executable path
    model: glm-4.6v-flash    # visionmcp --model
    retries: 5               # GLM request retry count
    retryInterval: 1s        # GLM retry interval (string, e.g. 500ms)
    timeoutMs: 120000        # per-image analysis timeout (ms)
    cwd: 'C:\\tools'         # visionmcp child working directory (base for relative image paths)
    labelPrefix: ''          # optional prefix rendered before each image's analysis text
    serverName: vision       # tool namespace: mcp__<serverName>__analyze_image
    lockPath: 'C:\\tools\\visionmcp-bridge.lock'  # optional: override the visionmcp single-instance lock path
#                                (passed to visionmcp as --lock-path and mirrored into the env)
```

Set any boolean to `false` to disable that capability.

## Usage

1. Install and restart dsh (or refresh the page after hot-reload).
2. In the chat model selector, pick a model group labeled "+ auto vision" (e.g. `deepseek-official-vision`), next to the original group.
3. **Paste an image / screenshot** with a question — it is automatically analyzed by GLM into text before being handed to the model. No more "image not supported".
4. The model can also call `mcp__vision__analyze_image` on demand to analyze an image file (`prompt` + one of `path` / `url` / `data`).

## Troubleshooting

- **Child process does not start**: make sure `VISIONMCP_PATH` or `config.command` points to a real, built `visionmcp.exe`. The plugin logs the connection failure reason.
- **Analyses always fail / placeholder text**: make sure `GLM_API_KEY` (or `OPENAI_API_KEY`) is set and valid; `--base-url` defaults to Zhipu AI — for a self-hosted endpoint configure it on the visionmcp side (this plugin does not forward a `baseUrl`; set the `OPENAI_BASE_URL` env var instead).
- **Relative image paths resolve incorrectly**: set `config.cwd` to the visionmcp child working directory; visionmcp only accepts relative paths that do not escape the current directory.
- **Need protocol-level diagnostics**: the visionmcp child does not receive an explicit `--log`, so it writes to `visionmcp.log` next to the executable by default; for more detail, start a test instance manually with `--log <path> --log-level debug`.

## Known limitations

- The user must **select the `-vision` model group** (original text-only routes stay untouched by design — this avoids side effects from hard-declaring image capability on the original routes).
- visionmcp has a single-instance lock (see [README](../../README.md#single-instance)). The plugin sets a separate `VISIONMCP_LOCK_PATH` for the child (default `$DSH_HOME/visionmcp-bridge.lock`) so it does not collide with a manually started instance.
- The lock path is overridden in priority order `lockPath` (config) > `VISIONMCP_LOCK_PATH` (env) > default `$DSH_HOME/visionmcp-bridge.lock`. When `lockPath` is configured it is passed to visionmcp as `--lock-path` (new builds) and mirrored into `VISIONMCP_LOCK_PATH` as a fallback (old builds), enabling multiple instances or a shared lock.
- Image bytes are uploaded to the Zhipu AI servers for vision analysis; if no key is configured or analysis fails, a `[visionmcp: ...]` placeholder is used and the conversation continues without hanging.
