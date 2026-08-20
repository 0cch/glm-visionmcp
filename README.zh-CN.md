# visionmcp

Vision MCP 是一个 Go 编写的 stdio 或 HTTP Model Context Protocol 服务器，让**非多模态模型**（Codex、DeepSeek、Claude CLI 等）通过任意 OpenAI 兼容视觉模型获得图像理解能力。默认配置为 GLM-4.6V-Flash，但支持任何实现 OpenAI Chat Completions 协议的提供商：OpenAI、Azure OpenAI、OpenRouter、DeepSeek、Moonshot、Qwen、本地 vLLM/Ollama 等。

English version: [README.md](README.md)

## 目录

- [功能特性](#功能特性)
- [构建](#构建)
- [运行](#运行)
- [配置](#配置)
- [MCP 客户端](#mcp-客户端)
- [单实例](#单实例)
- [日志](#日志)
- [安全](#安全)
- [重试](#重试)
- [DeepSeek Harness 集成](#deepseek-harness-集成)
- [开发](#开发)

## 功能特性

- 一个工具 `analyze_image(prompt, image)` —— 分析一张图片并返回文本。
- **stdio 模式**（默认）与 **HTTP 模式**（`--http`），可让所有 agent 共享同一个实例。
- OpenAI 兼容：通过 `--base-url` 连接任意 Chat Completions 端点。
- 单实例锁，多个 MCP 消费者共享一个进程而不冲突。
- 图片解码并归一化为 PNG；上传前限制大小。
- 对瞬时故障自动重试并退避。

## 构建

```powershell
go build -o visionmcp.exe ./cmd/visionmcp
go test ./...
```

产物是 `visionmcp.exe`（macOS/Linux 上是 `visionmcp`）。运行预编译二进制不需要 Go 工具链。

## 运行

服务器暴露一个工具：

- `analyze_image(prompt, image)` —— 分析一张图片并返回文本。

`image` 只能接受一种来源：

- `path`：绝对路径，或相对于服务器进程工作目录的路径。含 `..` 的相对路径会被拒绝。
- `url`：HTTP(S) 图片 URL。拒绝内嵌凭据。
- `data`：原始 base64 图片字节。不接受 data URL 前缀。

支持的输入格式为 JPEG、PNG、GIF。图片会被解码、归一化为 PNG，并以 data URL 发送。

### stdio 模式（默认）

```powershell
.\visionmcp.exe
```

从 stdin 读取 JSON-RPC 请求、向 stdout 写入响应 —— 这是 Codex 及其他客户端使用的标准 MCP 传输方式。

### HTTP 模式（共享实例）

```powershell
.\visionmcp.exe --http --http-addr 127.0.0.1:8765
```

在 `<http-addr>/mcp` 上运行 MCP **Streamable HTTP** 服务器。所有 agent 连接到同一个 URL 并共享一个长期运行的实例，因此图像分析不会为每个客户端重复创建。

```powershell
.\visionmcp.exe --http --http-addr 0.0.0.0:8765
```

绑定所有网卡，使局域网内其他机器可以访问。除非确实需要网络访问，否则保持 `127.0.0.1`。

## 配置

服务器连接任意 OpenAI 兼容的 Chat Completions API。默认目标是智谱 AI（BigModel）的 GLM-4.6V-Flash 端点：

```powershell
$env:OPENAI_API_KEY = "..."
$env:OPENAI_BASE_URL = "https://api.openai.com/v1"
.\visionmcp.exe --model gpt-4o --base-url https://api.openai.com/v1
```

### 参数

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--api-key` | API 密钥。覆盖 `OPENAI_API_KEY` / `GLM_API_KEY`。 | 环境变量 |
| `--base-url` | OpenAI 兼容基础 URL。端点变为 `<base-url>/chat/completions`。也可从 `OPENAI_BASE_URL` 读取。 | `https://open.bigmodel.cn/api/paas/v4` |
| `--api-endpoint` | 已废弃：完整 Chat Completions URL。优先使用 `--base-url`。 | — |
| `--model` | 视觉模型名（`glm-4.6v-flash`、`gpt-4o`、`qwen-vl-max` 等）。 | `glm-4.6v-flash` |
| `--http` | 以 HTTP（Streamable HTTP）MCP 服务器运行，替代 stdio。 | `false` |
| `--http-addr` | `--http` 模式的监听地址。 | `127.0.0.1:8765` |
| `--timeout` | 单次 API 请求超时。 | `2m` |
| `--max-image-mb` | 图片文件大小上限（MiB，超限拒绝上传）。 | `20` |
| `--retries` | 首次请求后的重试次数。 | `5` |
| `--retry-interval` | 重试间隔。 | `1s` |
| `--log` | 日志文件路径（默认为可执行文件旁的 `visionmcp.log`）。 | 可执行文件目录 |
| `--log-level` | `debug`、`info`、`warn` 或 `error`。 | `info` |
| `--lock-path` | 单实例锁文件路径。 | 见[单实例](#单实例) |
| `--dry-run` | 打印生成的 Codex 配置并退出。 | `false` |

### 环境变量

| 变量 | 作用 |
|---|---|
| `OPENAI_API_KEY` | API 密钥（优先于 `GLM_API_KEY`，被 `--api-key` 覆盖）。 |
| `GLM_API_KEY` | 备用 API 密钥。 |
| `OPENAI_BASE_URL` | 基础 URL（被 `--base-url` 覆盖）。 |
| `VISIONMCP_LOCK_PATH` | 锁文件路径（被 `--lock-path` 覆盖）。 |

请求是标准的 OpenAI Chat Completions 调用：`Authorization: Bearer <key>`，`user` 消息包含一个 `image_url`（data URL）和一个 `text` 部分。

## MCP 客户端

### 打印 Codex 配置

不启动服务器即可打印 `[mcp_servers.*]` 片段：

```powershell
$env:OPENAI_API_KEY = "..."
.\visionmcp.exe --dry-run
```

把输出复制到 `~/.codex/config.toml` 或项目 `.codex/config.toml`。该命令不会自动修改 Codex 配置。生成的片段会从你的本地环境转发 `OPENAI_API_KEY` 和 `GLM_API_KEY`：

```toml
[mcp_servers.visionmcp]
command = "D:\\Codes\\visionmcp\\visionmcp.exe"
args = ["--model", "glm-4.6v-flash", "--retries", 5, "--retry-interval", "1s", "--log", "...", "--log-level", "info"]
env_vars = ["OPENAI_API_KEY", "GLM_API_KEY"]
```

对于非默认提供商，设置 `OPENAI_BASE_URL`（或传 `--base-url`）；生成的配置会在 `args` 中包含 `--base-url`。

### Codex + HTTP（共享实例）

生成共享实例配置：

```powershell
.\visionmcp.exe --http --dry-run
```

```toml
[mcp_servers.visionmcp]
url = "http://127.0.0.1:8765/mcp"
```

或者直接用 Codex CLI 注册：

```sh
codex mcp add visionmcp --url http://127.0.0.1:8765/mcp
```

先启动一次共享服务器（例如作为后台进程），然后每个 Codex 会话都使用这一个实例。运行 `visionmcp --http --dry-run --http-addr 0.0.0.0:8765` 可以生成一个局域网可达的地址。

### 其他 MCP 客户端

任何支持 stdio 的 MCP 客户端都可以启动 `visionmcp.exe` 并调用 `analyze_image`。支持 Streamable HTTP 的客户端则连接 `--http` 的 URL。

## 单实例

每个用户只允许一个 `visionmcp` 进程。服务器使用操作系统级别的独占锁：

- Windows：`%LOCALAPPDATA%\visionmcp\visionmcp.lock`
- macOS：`~/Library/Caches/visionmcp/visionmcp.lock`
- Linux/XDG：`~/.cache/visionmcp/visionmcp.lock`

如果已有另一个实例在运行，新进程以退出码 `3` 结束。锁在所属进程退出时自动释放。

锁位置可以按优先级覆盖：`--lock-path`（命令行）> `VISIONMCP_LOCK_PATH`（环境变量）> 每用户缓存默认值。这让多个独立的 MCP 消费者可以各自运行自己的 visionmcp 实例而不冲突。

## 日志

日志为换行分隔的 JSON。默认写入可执行文件旁的 `visionmcp.log`（无法确定可执行文件路径时回退到 stderr）；通过 `--log <path>` 指定其他文件。每条日志包含时间戳、级别、消息、方法名和请求 ID。使用 `--log-level debug` 进行协议级诊断。日志绝不包含 API 密钥或 base64 图片数据。

## 安全

- API 密钥只从 `OPENAI_API_KEY`、`GLM_API_KEY` 或 `--api-key` 接受。
- 相对图片路径不能逃逸服务器工作目录。支持绝对路径，取决于 MCP 进程的操作系统权限。
- 图片 URL 必须使用 HTTP(S)、有主机名，且不能内嵌凭据。
- 图片大小默认上限 20 MiB（`--max-image-mb`）。
- API 响应上限 10 MiB。
- HTTP 模式下，除非你确实需要局域网访问，否则绑定 `127.0.0.1`；服务器没有认证层。

## 重试

首次尝试后 API 请求会重试。默认 5 次重试、间隔 1 秒：

```powershell
.\visionmcp.exe --retries 5 --retry-interval 1s
```

认证错误不重试；瞬时网络故障和 GLM 过载/限流码 `1305` 会重试。

## DeepSeek Harness 集成

本服务器也可以作为插件桥接到 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness)。该桥接不修改本代码库 —— visionmcp 仍是独立的 stdio MCP 服务器，作为底层能力提供方。Harness 内置的 `@deepseek-ai/dsh-mcp-client` 会启动它、发现 `analyze_image` 工具，并注册为 `mcp__vision__analyze_image`。

安装与使用方法见 [dsh-vision-mcp/README.zh-CN.md](dsh-vision-mcp/README.zh-CN.md)。

## 开发

```powershell
go test ./...
go vet ./...
gofmt -l .
```

服务器代码位于 `cmd/visionmcp` 和 `internal/`。HTTP 传输在 `internal/mcp/http.go`；stdio 传输和 JSON-RPC 处理在 `internal/mcp/server.go`。