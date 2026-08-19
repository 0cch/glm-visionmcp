# dsh-vision-mcp

给 [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness) 提供视觉能力的插件包（bundle）。它把 [visionmcp](../../README.md)（基于 GLM-4.6V-Flash 的视觉 MCP 服务器）作为底层能力桥接进 harness，让模型通过原生工具 `mcp__vision__analyze_image` 分析图片。

**不修改 visionmcp 的任何代码**。visionmcp 保持独立可执行程序（`visionmcp.exe`），本插件只是通过 harness 内置的 `@deepseek-ai/dsh-mcp-client` 插件以 stdio 方式启动它、发现并注册它的工具。

## 原理

```
DeepSeek Harness (dsh)
  └─ @deepseek-ai/dsh-mcp-client   （内置插件，本 bundle 在 cordis.patch.yml 中插入一行）
        └─ stdio 启动 visionmcp.exe
              └─ 工具发现：analyze_image
                    └─ 注册为 ctx.tools 上的 mcp__vision__analyze_image
```

- `dsh-mcp-client` 负责进程生命周期、工具发现/注册、超时与断线重连。
- 模型调 `mcp__vision__analyze_image(prompt, image)`，参数原样转发给 visionmcp。
- 本 bundle 还带一个可选增强插件：向系统提示注入一段视觉使用指引，提升模型主动调用工具的倾向。

## 安装

先构建 visionmcp（见 [README](../../README.md#build)），确保 `visionmcp.exe` 就绪，并准备好 `GLM_API_KEY`。

### 方式一：直接以 bundle 安装（推荐）

```sh
# 在包含本目录的路径下执行（或换成 dsh-vision-mcp 的实际路径）
dsh plugin --profile web add /path/to/dsh-vision-mcp
```

或者从 npm 安装：

```sh
dsh plugin --profile web add dsh-vision-mcp
```

### 方式二：用 --patch 临时启用（快速试玩）

在任意目录写一个 `vision.cordis.yml`：

```yaml
- insert:
    - id: vision-mcp
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: vision
        transport: stdio
        command: 'F:\\Code\\glm-visionmcp\\visionmcp.exe'
        args: ['--model', 'glm-4.6v-flash', '--retries', '5', '--retry-interval', '1s', '--log-level', 'info']
        env:
          GLM_API_KEY: !!js 'process.env.GLM_API_KEY ?? ""'
        cwd: !!js 'process.cwd()'
        toolCallTimeoutMs: 120000
        failOnStartupError: true
```

然后启动：

```sh
dsh web --patch vision.cordis.yml
```


> 从 GitHub 安装（`dsh plugin add github:you/dsh-vision-mcp`）时，pnpm 会拉取源码并运行 `prepare` 构建；首次会要求你在 profile 的 `pnpm-workspace.yaml` 中把 `dsh-vision-mcp` 加入 `allowBuilds` 授权，属于正常的安全确认。
> 提示：用 `--patch` 直接指向 `cordis.patch.yml`（bundle 的主配置）时，其中的系统提示指引插件（`vision-guidance`）需要已安装 `dsh-vision-mcp` 包才能解析。上面这种"快速试玩"写法只启用核心工具桥接；想同时启用指引，用 `dsh plugin add` 完整安装本 bundle（方式一）。
## 配置项

| 环境变量 | 作用 | 默认值 |
|---|---|---|
| `VISIONMCP_PATH` | visionmcp.exe 的绝对路径 | `<cwd>/visionmcp.exe` |
| `VISIONMCP_CWD` | visionmcp 的工作目录（影响相对图片路径解析） | `<cwd>` |
| `GLM_API_KEY` | 智谱 API Key（visionmcp 必填） | 空 |

如需在 profile 中覆盖默认配置，在 profile 的 `cordis.patch.yml` 中按 `id` 覆盖：

```yaml
- id: vision-mcp
  config:
    command: 'C:\\tools\\visionmcp.exe'
    env:
      GLM_API_KEY: !!js 'process.env.GLM_API_KEY ?? ""'
```

## 使用

安装后，模型会看到工具 `mcp__vision__analyze_image`，参数：

- `prompt`: 对图片的提问或提取任务（必填）。
- `image`: 三种来源之一（三选一）：
  - `path`: 绝对路径，或相对于 visionmcp 工作目录的相对路径；
  - `url`: HTTP(S) 图片地址；
  - `data`: 原始 base64 图片字节。

例如让模型“分析屏幕截图并说明其中的报错信息”，它会自动调用该工具并返回文本分析结果。

## 调整

- 想关闭系统提示指引：`- id: vision-guidance` 行设 `disabled: true`。
- 想换模型：在覆盖层改 `args` 里的 `--model`。
- 工具超时默认 120s；GLM 分析较慢时可调大 `toolCallTimeoutMs`。

## 已知限制

- harness 目前只桥接 MCP 的 **tools** 能力，visionmcp 仅暴露工具，因此不受影响。
- visionmcp 有单实例锁（见 [README](../../README.md#single-instance)）；若另一个进程已占用锁，dsh 启动的实例会以退出码 3 失败。此时重启那个进程或设置 `VISIONMCP_LOCK_PATH` 指向独立路径。

