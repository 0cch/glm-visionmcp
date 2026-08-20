# dsh-vision-mcp

给 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) 提供视觉能力的通用插件 bundle。它把 [visionmcp](../../README.md)（基于 GLM-4.6V-Flash 的视觉 MCP 服务器）作为底层能力提供方，让**任意非多模态模型**（DeepSeek、pi-ai、自定义提供方等）都能上传/粘贴图片并自动用 GLM 分析，不再提示“当前模型不支持图片”。

**不修改 visionmcp 的任何代码**，也不修改 dsh 框架。visionmcp 保持独立可执行程序（`visionmcp.exe`），本插件通过 stdio 启动它并调用 `analyze_image`。

English version: [README.md](README.md)

## 目录

- [原理](#原理)
- [安装](#安装)
- [配置](#配置)
- [使用](#使用)
- [故障排查](#故障排查)
- [已知限制](#已知限制)

## 原理

核心机制借鉴 [dsh-vision/vision-route](https://github.com/314857493/dsh-vision/tree/main/packages/vision-route)，但通用化：**自动为每个纯文本 provider 路由注册一个并行的 `<provider>-vision` 包装路由**。

```
DeepSeek Harness (dsh)
  └─ dsh-vision-mcp
        ├─ 监听 llm/adapters-updated，扫描所有 provider 路由
        │     └─ 对纯文本路由自动注册 deepseek-official-vision / pi-ai-vision / ...
        │           包装路由:
        │             resolveModel() 声明 inputModalities: ['text','image']
        │               → api-proxy 附件预检放行上传/粘贴图片
        │             stream() 把图片块经 visionmcp 转成文本，再委派真实 adapter
        ├─ 原生工具 mcp__vision__analyze_image（模型可主动分析图片文件）
        ├─ 系统提示指引（教模型何时用工具）
        └─ 唯一的 visionmcp.exe 子进程（工具与转译共用）
```

- **通用**：任何 provider 路由只要模型声明的是纯文本，就自动获得 `-vision` 并行路由；本身支持图片的多模态路由不包装、不受影响。
- **无需改原路由**：官方 `deepseek-official` 等原路由保持纯文本、零开销；需要识图时在模型选择器选带「+ 自动识图」的模型组。
- **单一进程**：所有 `-vision` 路由和工具共享一个 visionmcp 连接，全进程只跑一个 visionmcp.exe。
- **优雅降级**：某张图分析失败时替换为 `[visionmcp: ...]` 占位文本，对话不卡死；转译结果按 `attachmentId` 进程内缓存。

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

`cordis.patch.yml` 已声明 `dsh.bundle.patch`，`dsh plugin add` 会把它作为独立 layer 应用。

> 从 GitHub 安装（`dsh plugin add github:you/dsh-vision-mcp`）时，pnpm 会拉取源码并运行 `prepare` 构建；首次会要求你在 profile 的 `pnpm-workspace.yaml` 中把 `dsh-vision-mcp` 加入 `allowBuilds` 授权，属于正常的安全确认。

## 配置

### 环境变量

| 环境变量 | 作用 | 默认值 |
|---|---|---|
| `VISIONMCP_PATH` | visionmcp.exe 的绝对路径（`command` 配置项优先于它） | `<cwd>/visionmcp.exe` |
| `GLM_API_KEY` / `OPENAI_API_KEY` | 视觉 API Key（原样透传给 visionmcp 子进程） | 空 |
| `VISIONMCP_LOCK_PATH` | visionmcp 单实例锁路径（`lockPath` 配置项优先于它） | `$DSH_HOME/visionmcp-bridge.lock`（无 `DSH_HOME` 时用 `LOCALAPPDATA`/`HOME`） |

visionmcp 可执行文件的解析顺序：`config.command` → `VISIONMCP_PATH` → `<cwd>/visionmcp.exe`。

### 插件配置项

在 profile 的 `cordis.patch.yml` 按 `id: vision` 覆盖：

```yaml
- id: vision
  config:
    routeEnabled: true       # 为纯文本 provider 自动注册 -vision 并行路由（默认 true）
    routeSuffix: '-vision'   # 包装路由后缀：deepseek-official → deepseek-official-vision
    toolEnabled: true        # 注册 mcp__vision__analyze_image 工具（默认 true）
    bridgeEnabled: false     # 旧的 llm/stream 桥接开关，已被路由包装取代（默认 false）
    guidance: true           # 注入系统提示指引（默认 true）
    guidanceText: ''         # 自定义指引文本；留空用内置默认
    sectionOrder: 120        # 指引在系统提示中的位置（工具指引区段 100-199）
    command: 'C:\\tools\\visionmcp.exe'   # 覆盖 visionmcp 可执行文件路径
    model: glm-4.6v-flash    # visionmcp 的 --model
    retries: 5               # GLM 请求重试次数
    retryInterval: 1s        # GLM 重试间隔（字符串，如 500ms）
    timeoutMs: 120000        # 单次分析超时（ms）
    cwd: 'C:\\tools'         # visionmcp 子进程工作目录（相对图片路径的解析基准）
    labelPrefix: ''          # 每个图片分析文本前的可选前缀
    serverName: vision       # 工具命名空间：mcp__<serverName>__analyze_image
    lockPath: 'C:\\tools\\visionmcp-bridge.lock'  # 可选：覆盖 visionmcp 单实例锁路径
#                                （作为 --lock-path 传给 visionmcp，也同步设环境变量）
```

想关闭某项能力，把对应布尔设为 `false`。

## 使用

1. 安装并重启 dsh（或热生效后刷新页面）。
2. 在聊天框的**模型选择器**里选带「+ 自动识图」的模型组（如 `deepseek-official-vision`），它会出现在原模型组旁边。
3. **直接粘贴图片 / 截图**并附带问题——图片会被自动用 GLM 分析成文字再交给模型，不再提示“不支持图片”。
4. 模型也可主动调用 `mcp__vision__analyze_image` 分析图片文件（参数 `prompt` + `image` 三选一：`path` / `url` / `data`）。

## 故障排查

- **子进程没有起来**：确认 `VISIONMCP_PATH` 或 `config.command` 指向真实存在的 `visionmcp.exe`，且已用 `go build` 构建。插件日志会记录连接失败原因。
- **分析总是失败/占位文本**：确认 `GLM_API_KEY`（或 `OPENAI_API_KEY`）已配置且有效；`--base-url` 默认指向智谱，若自建端点需在 visionmcp 侧配置（本插件未透传 `baseUrl`，可在 visionmcp 环境变量 `OPENAI_BASE_URL` 中设置）。
- **相对图片路径解析不对**：用 `config.cwd` 指定 visionmcp 子进程的工作目录；visionmcp 只接受相对路径不逃逸当前目录。
- **想排查协议细节**：visionmcp 子进程未显式传 `--log`，会按默认写到 exe 旁的 `visionmcp.log`；如需更细日志可手动以 `--log <path> --log-level debug` 启动一个测试实例排查。

## 已知限制

- 需要用户**选择 `-vision` 模型组**（原纯文本路由保持不动，这是有意的：避免给原路由硬声明图片能力带来副作用）。
- visionmcp 有单实例锁（见 [README](../../README.md#single-instance)）。插件为子进程设置独立的 `VISIONMCP_LOCK_PATH`（默认 `$DSH_HOME/visionmcp-bridge.lock`），与手动启动的实例互不冲突。
- 单实例锁路径可按 `lockPath`（配置）> `VISIONMCP_LOCK_PATH`（环境变量）> 默认 `$DSH_HOME/visionmcp-bridge.lock` 的优先级覆盖。配置 `lockPath` 时以 `--lock-path` 命令行参数传给 visionmcp（新构建），并同步设 `VISIONMCP_LOCK_PATH` 兜底（兼容旧构建），便于多实例并存或改用共享锁。
- 图片字节会上传到智谱服务器进行视觉分析；未配置 key 或分析失败时以 `[visionmcp: ...]` 占位继续，不会卡死。
