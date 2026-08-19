# dsh-vision-mcp

给 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) 提供视觉能力的通用插件 bundle。它把 [visionmcp](../../README.md)（基于 GLM-4.6V-Flash 的视觉 MCP 服务器）作为底层能力提供方，让**任意非多模态模型**（DeepSeek、pi-ai、自定义提供方等）都能上传/粘贴图片并自动用 GLM 分析，不再提示“当前模型不支持图片”。

**不修改 visionmcp 的任何代码**，也不修改 dsh 框架。visionmcp 保持独立可执行程序（`visionmcp.exe`），本插件通过 stdio 启动它并调用 `analyze_image`。

## 原理（通用路由包装）

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
| `VISIONMCP_PATH` | visionmcp.exe 的绝对路径 | `<cwd>/visionmcp.exe` |
| `VISIONMCP_CWD` | visionmcp 的工作目录（影响相对图片路径解析） | `<cwd>` |
| `GLM_API_KEY` | 智谱 API Key（visionmcp 必填，会透传给子进程） | 空 |

### 插件配置项（在 profile 的 `cordis.patch.yml` 按 `id: vision` 覆盖）

```yaml
- id: vision
  config:
    routeEnabled: true       # 为纯文本 provider 自动注册 -vision 并行路由
    routeSuffix: '-vision'   # 包装路由后缀：deepseek-official → deepseek-official-vision
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

1. 安装并重启 dsh（或热生效后刷新页面）。
2. 在聊天框的**模型选择器**里选带「+ 自动识图」的模型组（如 `deepseek-official-vision`），它会出现在原模型组旁边。
3. **直接粘贴图片 / 截图**并附带问题——图片会被自动用 GLM 分析成文字再交给模型，不再提示“不支持图片”。
4. 模型也可主动调用 `mcp__vision__analyze_image` 分析图片文件（参数 `prompt` + `image` 三选一：`path` / `url` / `data`）。

## 已知限制

- 需要用户**选择 `-vision` 模型组**（原纯文本路由保持不动，这是有意的：避免给原路由硬声明图片能力带来副作用）。
- visionmcp 有单实例锁（见 [README](../../README.md#single-instance)）。插件为子进程设置独立的 `VISIONMCP_LOCK_PATH`（默认 `$DSH_HOME/visionmcp-bridge.lock`），与手动启动的实例互不冲突。
- 图片字节会上传到智谱服务器进行视觉分析；未配置 key 或分析失败时以 `[visionmcp: ...]` 占位继续，不会卡死。
