/**
 * dsh-vision-mcp — DeepSeek Harness vision bundle (single plugin, single MCP).
 *
 * One plugin gives a text-only DeepSeek Harness deployment vision with exactly
 * ONE visionmcp stdio process, shared by both capabilities:
 *
 * 1. Native vision tool: registers `mcp__vision__analyze_image` on
 *    `ctx.tools` (the same public name the in-box dsh-mcp-client bridge would
 *    produce) so the model can analyze an image it references by path, URL, or
 *    raw base64 bytes.
 * 2. Automatic image bridge: listens on the `llm/stream` waterfall (the single
 *    entry every provider's model request passes through) and replaces any
 *    image block in a user message with the text produced by the SAME
 *    visionmcp connection. Uploading or pasting an image into a session no
 *    longer hits "model does not support images" — the image never reaches the
 *    text-only provider, the GLM description does. Models that already declare
 *    `image` input are left untouched.
 *
 * Both capabilities share one lazily-started MCP client, so only one
 * visionmcp.exe child runs per harness process (see `VisionMcpClient`).
 */

import type { Context } from '@deepseek-ai/cordis'
import Schema from '@deepseek-ai/schemastery'
import type { ContentBlock, GenerateOptions, StreamChunk } from '@deepseek-ai/dsh-llm'
import type { ToolDefinition } from '@deepseek-ai/dsh-tools'
import type {} from '@deepseek-ai/dsh-system-prompt'
import type {} from '@deepseek-ai/dsh-attachment'

export const name = 'dsh-vision-mcp'

/** Services required by this plugin. */
export const inject = ['systemPrompt', 'llm', 'attachments', 'tools']

export interface Config {
  // ── single visionmcp connection (shared) ──────────────────────────────────
  /** Whether the automatic image bridge is enabled (default true). */
  bridgeEnabled: boolean
  /** Whether the `mcp__vision__analyze_image` tool is registered (default true). */
  toolEnabled: boolean
  /** visionmcp executable path (defaults to VISIONMCP_PATH then <cwd>/visionmcp.exe). */
  command?: string
  /** Vision model passed to visionmcp (default glm-4.6v-flash). */
  model: string
  /** GLM API retry count passed to visionmcp. */
  retries: number
  /** GLM API retry interval passed to visionmcp. */
  retryInterval: string
  /** Per-image analysis timeout in ms. */
  timeoutMs: number
  /** Working directory for the visionmcp child (relative image paths resolve here). */
  cwd?: string
  /** Prefix rendered before each image's analysis text (empty drops it). */
  labelPrefix: string
  /** Public tool namespace (`mcp__<serverName>__analyze_image`). */
  serverName: string

  // ── system-prompt guidance ────────────────────────────────────────────────
  /** Whether to inject the vision usage guidance into the system prompt. */
  guidance: boolean
  /** Custom guidance text; defaults to the built-in instruction. */
  guidanceText?: string
  /** Section order in the assembled system prompt (tool guidance band: 100-199). */
  sectionOrder: number
}

export const Config: Schema<Config> = Schema.object({
  bridgeEnabled: Schema.boolean().default(true),
  toolEnabled: Schema.boolean().default(true),
  command: Schema.string(),
  model: Schema.string().default('glm-4.6v-flash'),
  retries: Schema.number().default(5),
  retryInterval: Schema.string().default('1s'),
  timeoutMs: Schema.number().default(120_000),
  cwd: Schema.string(),
  labelPrefix: Schema.string().default(''),
  serverName: Schema.string().default('vision'),
  guidance: Schema.boolean().default(true),
  guidanceText: Schema.string(),
  sectionOrder: Schema.number().default(120),
})

const DEFAULT_GUIDANCE = `## Vision
You have access to an image-analysis tool (\`mcp__{{serverName}}__analyze_image\`). Use it whenever a task requires understanding visual content: screenshots, diagrams, photos, UI mockups, charts, scanned documents, or any image the user references.

Call it with:
- \`prompt\`: a concise, self-contained question or extraction task about the image.
- \`image\`: exactly one of \`path\` (absolute path or workspace-relative path), \`url\` (HTTP(S) image URL), or \`data\` (raw base64 bytes). Provide only one source.

The tool returns plain text describing or answering about the image. Prefer it over guessing when the answer depends on something visual.`

export function apply(ctx: Context, config: Config): void {
  const client = new VisionMcpClient(config)
  ctx.effect(() => () => client.dispose())

  const publicName = `mcp__${config.serverName}__analyze_image`

  // 1. Native vision tool (shared connection).
  if (config.toolEnabled) {
    ctx.effect(() => ctx.tools.register(createToolDefinition(client, publicName, config)))
  }

  // 2. Automatic image bridge on llm/stream.
  if (config.bridgeEnabled) {
    ctx.on('llm/stream', (options, next) => {
      if (!hasImageBlock(options.messages)) return next()
      return rewriteStream(ctx, config, client, options, next)
    }, { global: true })
  }

  // 3. System-prompt guidance.
  if (config.guidance) {
    const text = config.guidanceText?.trim()
      || DEFAULT_GUIDANCE.replace('{{serverName}}', config.serverName)
    ctx.effect(() =>
      ctx.systemPrompt.section({
        name: 'vision-mcp-guidance',
        order: config.sectionOrder,
        text,
      }),
    )
  }
}

/**
 * Build the model-facing `mcp__<serverName>__analyze_image` tool backed by the
 * shared visionmcp connection. `execute` mirrors the wire contract of the MCP
 * server: `prompt` + exactly one of `image.path`, `image.url`, `image.data`.
 */
function createToolDefinition(client: VisionMcpClient, publicName: string, config: Config): ToolDefinition {
  return {
    name: publicName,
    description: 'Analyze one image with GLM-4.6V-Flash and return text for non-multimodal models.',
    parameters: {
      type: 'object',
      properties: {
        prompt: { type: 'string', description: 'Question or extraction task for the image' },
        image: {
          type: 'object',
          description: 'Exactly one image source',
          properties: {
            url: { type: 'string', description: 'HTTP(S) image URL' },
            path: { type: 'string', description: 'Absolute image path, or a path relative to the visionmcp working directory' },
            data: { type: 'string', description: 'Raw base64 image bytes' },
          },
          additionalProperties: false,
        },
      },
      required: ['prompt', 'image'],
      additionalProperties: false,
    },
    output: {
      schema: {
        type: 'object',
        properties: {
          analysis: { type: 'string' },
        },
        required: ['analysis'],
        additionalProperties: false,
      },
      render(_args: unknown, value: { analysis?: string }) {
        return [{ type: 'text', text: value.analysis ?? '' }]
      },
    },
    execute: async (args, exec) => {
      const { prompt, image } = (args ?? {}) as { prompt?: string; image?: { path?: string; url?: string; data?: string } }
      const text = await client.analyzeTool(config, prompt ?? '', image ?? {}, exec.signal)
      return { analysis: text }
    },
    timeoutMs: config.timeoutMs,
  }
}

/** Whether any message in the request carries an image block. */
function hasImageBlock(messages: readonly { content: readonly { type?: string }[] }[]): boolean {
  return messages.some(message => message.content.some(block => block.type === 'image'))
}

/**
 * Return a stream that first rewrites image blocks (asynchronously) and then
 * yields the downstream provider stream. The rewrite happens once, before the
 * first chunk is produced.
 */
function rewriteStream(
  ctx: Context,
  config: Config,
  client: VisionMcpClient,
  options: GenerateOptions,
  next: () => AsyncIterable<StreamChunk>,
): AsyncIterable<StreamChunk> {
  return (async function* () {
    // Never intercept a route that natively supports images; its own path wins.
    const modelSupportsImages = await routeSupportsImages(ctx, options)
    if (!modelSupportsImages) {
      options.messages = await Promise.all(options.messages.map(async (message) => {
        if (!message.content.some(block => block.type === 'image')) return message
        const content = await replaceImages(ctx, config, client, message.content)
        return { ...message, content }
      }))
    }
    yield* next()
  })()
}

async function routeSupportsImages(ctx: Context, options: GenerateOptions): Promise<boolean> {
  try {
    const info = await ctx.llm.resolveModelInfo(options.provider, options.model)
    return info.inputModalities?.includes('image') === true
  } catch {
    // Unknown capability: fall through to the bridge (refuse only when the
    // adapter itself would refuse, which the bridge avoids by never sending
    // an image block downstream).
    return false
  }
}

/** Replace every image block in one message's content with its analysis text. */
async function replaceImages(
  ctx: Context,
  config: Config,
  client: VisionMcpClient,
  blocks: readonly ContentBlock[],
): Promise<ContentBlock[]> {
  const prompt = extractTextPrompt(blocks)
  const result: ContentBlock[] = []
  for (const block of blocks) {
    if (block.type !== 'image') {
      result.push(block)
      continue
    }
    try {
      const stored = await ctx.attachments.readImage(block.attachment)
      const analysis = await client.analyzeData(config, prompt, stored.data)
      const text = config.labelPrefix === '' ? analysis : `${config.labelPrefix}${analysis}`
      result.push({ type: 'text', text })
    } catch (error) {
      // Degrade to a text note instead of failing the whole request.
      result.push({
        type: 'text',
        text: `[visionmcp: failed to analyze this image — ${error instanceof Error ? error.message : String(error)}]`,
      })
    }
  }
  return result
}

/** Build one concise analysis prompt from the surrounding user text (if any). */
function extractTextPrompt(blocks: readonly { type?: string; text?: string }[]): string {
  const text = blocks
    .filter(block => block.type === 'text' && typeof block.text === 'string')
    .map(block => block.text as string)
    .join('\n')
    .trim()
  if (text.length === 0) return 'Describe this image in detail.'
  return `The user attached this image alongside the following text. Answer the user's request using the image content, then summarize what is shown.\n\nUser text:\n${text}`
}

/**
 * A lazy, shared MCP client to ONE visionmcp stdio server. Connects on first
 * use and stays connected until the plugin is disposed. Both the native tool
 * and the automatic bridge call through this single instance, so only one
 * visionmcp.exe child runs per harness process.
 */
export class VisionMcpClient {
  private transport: import('@modelcontextprotocol/sdk/client/stdio.js').StdioClientTransport | undefined
  private mcpClient: import('@modelcontextprotocol/sdk/client/index.js').Client | undefined
  private connecting: Promise<void> | undefined
  private disposed = false

  constructor(config: Config) {
    this.command = config.command ?? ''
    this.cwd = config.cwd
  }

  private readonly command: string
  private readonly cwd?: string

  /** Tool-path analysis: forward the caller's abort signal and MCP-shaped args. */
  async analyzeTool(config: Config, prompt: string, image: { path?: string; url?: string; data?: string }, signal?: AbortSignal): Promise<string> {
    await this.ensureConnected(config)
    const result = await this.mcpClient!.callTool(
      { name: 'analyze_image', arguments: { prompt, image } },
      undefined,
      { signal, timeout: config.timeoutMs },
    )
    return this.extractText(result)
  }

  /** Bridge-path analysis: always sends the raw bytes as base64 `data`. */
  async analyzeData(config: Config, prompt: string, data: Uint8Array): Promise<string> {
    await this.ensureConnected(config)
    const result = await this.mcpClient!.callTool({
      name: 'analyze_image',
      arguments: {
        prompt,
        image: { data: Buffer.from(data).toString('base64') },
      },
    })
    return this.extractText(result)
  }

  private extractText(result: unknown): string {
    const content = (result as { content?: Array<{ type?: string; text?: string }> }).content ?? []
    const text = content
      .filter(part => part.type === 'text' && typeof part.text === 'string')
      .map(part => part.text as string)
      .join('\n')
      .trim()
    if (text.length === 0) throw new Error('visionmcp returned an empty analysis')
    return text
  }

  private async ensureConnected(config: Config): Promise<void> {
    if (this.mcpClient) return
    if (this.connecting) return this.connecting
    this.connecting = this.connect(config)
    try {
      await this.connecting
    } finally {
      this.connecting = undefined
    }
  }

  private async connect(config: Config): Promise<void> {
    const { Client } = await import('@modelcontextprotocol/sdk/client/index.js')
    const { StdioClientTransport } = await import('@modelcontextprotocol/sdk/client/stdio.js')
    if (this.disposed) return

    const command = resolveCommand(config)
    const childEnv = resolveChildEnv()

    this.transport = new StdioClientTransport({
      command,
      args: [
        '--model', config.model,
        '--retries', String(config.retries),
        '--retry-interval', config.retryInterval,
        '--log-level', 'info',
      ],
      env: childEnv,
      ...this.cwd ? { cwd: this.cwd } : {},
    })
    this.mcpClient = new Client({ name: 'dsh-vision-mcp', version: '0.1.0' })
    await this.mcpClient.connect(this.transport)
  }

  async dispose(): Promise<void> {
    this.disposed = true
    if (this.mcpClient) {
      try { await this.mcpClient.close() } catch { /* transport already gone */ }
      this.mcpClient = undefined
      this.transport = undefined
    }
  }
}

/** Resolve the visionmcp executable path from config then environment then cwd. */
function resolveCommand(config: Config): string {
  if (config.command) return config.command
  const fromEnv = process.env.VISIONMCP_PATH?.trim()
  if (fromEnv) return fromEnv
  return requireNodePathJoin(process.cwd(), 'visionmcp.exe')
}

function requireNodePathJoin(...parts: string[]): string {
  const path = process.getBuiltinModule('node:path') as typeof import('node:path')
  return path.join(...parts)
}

/**
 * Ambient env for the visionmcp child, with a dedicated lock path under the
 * DSH data directory so this instance never collides with a user-run or
 * tool-bridged visionmcp process.
 */
function resolveChildEnv(): Record<string, string> {
  const env: Record<string, string> = {}
  const dshHome = process.env.DSH_HOME
  const lockRoot = dshHome ?? process.env.LOCALAPPDATA ?? process.env.HOME ?? ''
  const lockPath = requireNodePathJoin(lockRoot, 'visionmcp-bridge.lock')
  for (const [key, value] of Object.entries(process.env)) {
    if (value === undefined) continue
    env[key] = value
  }
  env.VISIONMCP_LOCK_PATH = lockPath
  return env
}

