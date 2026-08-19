# dsh-vision-mcp

给 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) 提供视觉能力的单一插件 bundle。它把 [visionmcp](../../README.md)（基于 GLM-4.6V-Flash 的视觉 MCP 服务器）作为底层能力提供方，通过**一个** visionmcp stdio 进程同时提供两种能力：

1. **原生视觉工具**：在 `ctx.tools` 上注册 `mcp__vision__analyze_image`，模型可主动调用它分析图片（按路径 / URL / base64）。
2. **自动图片桥接**：监听 `llm/stream`，把用户消息里的图片块在到达纯文本模型之前，先用同一个 visionmcp 连接分析成文本——上传或粘贴图片不再出现“当前模型不支持图片”。

**不修改 visionmcp 的任何代码**。visionmcp 保持独立可执行程序（`visionmcp.exe`），本插件只是以 stdio 方式启动它并通过 MCP 协议调用 `analyze_image`。

## 原理

```
DeepSeek Harness (dsh)
  └─ dsh-vision-mcp（本 bundle 的插件，自己管理唯一的 visionmcp 连接）
        ├─ ctx.tools.register(mcp__vision__analyze_image)   ← 模型可调用
        ├─ ctx.on('llm/stream') 自动桥接                   ← 上传/截图自动分析
        └─ stdio 启动 visionmcp.exe（唯一子进程，共享）
              └─ analyze_image(prompt, image)
```

- 插件懒启动并复用**同一个** visionmcp 连接：第一次工具调用或第一次图片桥接时才 spawn，之后两个能力共用，全进程只跑一个 visionmcp.exe。
- 自动桥接只在模型声明 `inputModalities` 不含 `image` 时生效；原生支持图片的模型（如 GLM-4V 直连）不拦截，走自己的多模态路径。
- 同时向系统提示注入视觉使用指引，提升模型主动调用工具的倾向。

## 安装

先构建 visionmcp（见 [README](../../README.md#build)），确保 `visionmcp.exe` 就绪，并准备好 `GLM_API_KEY`。

### 方式一：直接以 bundle 安装（推荐）

```sh
dsh plugin --profile web add /path/to/dsh-vision-mcp
```

### 方式二：用 --patch 临时启用（快速试玩）

```sh
dsh web --patch /path/to/dsh-vision-mcp/cordis.patch.yml
```

`cordis.patch.yml` 已声明 `dsh.bundle.patch`，`dsh plugin add` 会把它作为独立 layer 应用；`--patch` 直接指向同一文件也可。

> 从 GitHub 安装（`dsh plugin add github:you/dsh-vision-mcp`）时，pnpm 会拉取源码并运行 `prepare` 构建；首次会要求你在 profile 的 `pnpm-workspace.yaml` 中把 `dsh-vision-mcp` 加入 `allowBuilds` 授权，属于正常的安全确认。

## 配置

### 环境变量

| 环境变量 | 作用 | 默认值 |
|---|---|---|
| `VISIONMCP_PATH` | visionmcp.exe 的绝对路径 | `<cwd>/visionmcp.exe` |
| `VISIONMCP_CWD` | visionmcp 的工作目录（影响相对图片路径解析） | `<cwd>` |
| `GLM_API_KEY` | 智谱 API Key（visionmcp 必填，会透传给子进程） | 空 |

### 插件配置项（在 profile 的 `cordis.patch.yml` 按 `id: vision` 覆盖）

```yaml
- id: vision
  config:
    bridgeEnabled: true      # 自动图片桥接（上传/截图不再“不支持图片”）
    toolEnabled: true        # 注册 mcp__vision__analyze_image 工具
    guidance: true           # 注入系统提示指引
    command: 'C:\\tools\\visionmcp.exe'
    model: glm-4.6v-flash    # visionmcp 的 --model
    retries: 5               # GLM 请求重试次数
    retryInterval: 1s
    timeoutMs: 120000        # 单次分析超时（ms）
    cwd: 'C:\\tools'         # 相对图片路径的解析基准
    serverName: vision       # 工具命名空间：mcp__<serverName>__analyze_image
    labelPrefix: ''          # 每个图片分析文本前的可选前缀
```

想关闭某项能力，把对应布尔设为 `false`。

## 使用

- **上传 / 粘贴图片**：图片块被自动用 GLM 分析成文本后再发给模型，模型直接基于描述回答，不再提示“不支持图片”。
- **让模型主动分析**：模型会看到工具 `mcp__vision__analyze_image`，参数：
  - `prompt`: 对图片的提问或提取任务（必填）；
  - `image`: 三种来源之一（三选一）：`path`（绝对或相对 visionmcp 工作目录的路径）、`url`（HTTP(S) 地址）、`data`（原始 base64 字节）。

## 已知限制

- visionmcp 有单实例锁（见 [README](../../README.md#single-instance)）。插件为子进程设置独立的 `VISIONMCP_LOCK_PATH`（默认 `$DSH_HOME/visionmcp-bridge.lock`），与手动启动的实例互不冲突；同进程内所有能力共享这一个子进程，不会锁冲突。
- 自动桥接需要模型请求经过 `llm/stream`（所有提供方、自定义模型都必经），因此对 deepseek-official、pi-ai、自定义提供方均生效。
