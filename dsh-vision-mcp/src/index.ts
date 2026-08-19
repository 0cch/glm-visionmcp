import type { Context } from '@deepseek-ai/cordis'
import Schema from '@deepseek-ai/schemastery'
import type { ContentBlock, GenerateOptions, LlmAdapter, LlmModelInfo, LlmResolvedModelInfo, StreamChunk, Message } from '@deepseek-ai/dsh-llm'
import { LlmAdapter as LlmAdapterBase } from '@deepseek-ai/dsh-llm'
import type { ToolDefinition } from '@deepseek-ai/dsh-tools'
import type { ImageAttachmentRef } from '@deepseek-ai/dsh-attachment'
import type {} from '@deepseek-ai/dsh-system-prompt'
import type {} from '@deepseek-ai/dsh-attachment'

export const name = 'dsh-vision-mcp'

/** Services required by this plugin. */
export const inject = ['systemPrompt', 'llm', 'attachments', 'tools']

export interface Config {
  /** Whether `-vision` parallel routes are registered for text-only providers (default true). */
  routeEnabled: boolean
  /** Whether the `mcp__vision__analyze_image` tool is registered (default true). */
  toolEnabled: boolean
  /** Whether the automatic llm/stream bridge is registered (kept for backward compat; route wrapping supersedes it). */
  bridgeEnabled: boolean
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
  /** Suffix added to each wrapped provider route (e.g. deepseek-official -> deepseek-official-vision). */
  routeSuffix: string
  /** Lock file path for the visionmcp child (defaults to VISIONMCP_LOCK_PATH, then <DSH_HOME>/visionmcp-bridge.lock). */
  lockPath?: string

  // ── system-prompt guidance ────────────────────────────────────────────────
  /** Whether to inject the vision usage guidance into the system prompt. */
  guidance: boolean
  /** Custom guidance text; defaults to the built-in instruction. */
  guidanceText?: string
  /** Section order in the assembled system prompt (tool guidance band: 100-199). */
  sectionOrder: number
}

export const Config: Schema<Config> = Schema.object({
  routeEnabled: Schema.boolean().default(true),
  toolEnabled: Schema.boolean().default(true),
  bridgeEnabled: Schema.boolean().default(false),
  command: Schema.string(),
  model: Schema.string().default('glm-4.6v-flash'),
  retries: Schema.number().default(5),
  retryInterval: Schema.string().default('1s'),
  timeoutMs: Schema.number().default(120_000),
  cwd: Schema.string(),
  labelPrefix: Schema.string().default(''),
  serverName: Schema.string().default('vision'),
  routeSuffix: Schema.string().default('-vision'),
  lockPath: Schema.string(),
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

  // 2. -vision parallel routes for every text-only provider.
  if (config.routeEnabled) {
    const registry = new VisionRouteRegistry(ctx, config, client)
    ctx.effect(() => () => registry.dispose())
    registry.sync()
    ctx.on('llm/adapters-updated', () => registry.sync())
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

/** Whether a resolved model info declares image input. */
function supportsImages(info: LlmResolvedModelInfo | LlmModelInfo | undefined): boolean {
  return info?.inputModalities?.includes('image') === true
}

/**
 * Manages the `<provider>-vision` wrapper registrations. Scans the live
 * provider set on every `llm/adapters-updated` and registers a wrapper route
 * for each text-only provider, keeping the set in sync.
 */
class VisionRouteRegistry {
  private readonly registered = new Map<string, () => void>()
  private readonly seen = new Set<string>()

  constructor(
    private readonly ctx: Context,
    private readonly config: Config,
    private readonly client: VisionMcpClient,
  ) {}

  dispose(): void {
    for (const dispose of this.registered.values()) {
      try { dispose() } catch { /* already gone */ }
    }
    this.registered.clear()
  }

  sync(): void {
    let providers: Array<{ id: string; name: string }>
    try {
      providers = this.ctx.llm.listProviders()
    } catch {
      return
    }
    const live = new Set<string>()
    for (const provider of providers) {
      live.add(provider.id)
      const visionRoute = `${provider.id}${this.config.routeSuffix}`
      if (this.registered.has(visionRoute) || this.seen.has(visionRoute)) continue
      // Try synchronously first (fast path); probe is async so fall back to the
      // adapters-updated re-entry for providers that need probing.
      void this.tryWrap(provider.id, visionRoute)
    }
    // Release registrations whose source provider disappeared.
    for (const [visionRoute, dispose] of [...this.registered]) {
      const source = visionRoute.endsWith(this.config.routeSuffix)
        ? visionRoute.slice(0, -this.config.routeSuffix.length)
        : visionRoute
      if (!live.has(source)) {
        try { dispose() } catch { /* already gone */ }
        this.registered.delete(visionRoute)
        this.seen.delete(visionRoute)
      }
    }
  }

  private async tryWrap(sourceProvider: string, visionRoute: string): Promise<void> {
    let inner: LlmAdapter | undefined
    try {
      // registration() is TS-private but present at runtime in dsh rc.5+.
      const registration = (this.ctx.llm as unknown as { registration(p: string): { adapter: LlmAdapter } }).registration(sourceProvider)
      inner = registration?.adapter
    } catch {
      inner = undefined
    }
    if (!inner) return

    // Only wrap routes that do NOT already declare image input natively. Probe
    // via listModels; if any model natively accepts image, skip wrapping.
    try {
      if (inner.listModels) {
        const models = await Promise.resolve(inner.listModels(sourceProvider))
        if (models.some(model => supportsImages(model))) {
          this.seen.add(visionRoute)
          return
        }
      }
    } catch {
      // Can't probe; attempt to wrap anyway (registerAdapter will still validate).
    }

    try {
      const handle = this.ctx.llm.registerAdapter(
        [visionRoute],
        new VisionRouteAdapter(this.ctx, inner, sourceProvider, visionRoute, this.config, this.client),
      )
      this.registered.set(visionRoute, handle)
    } catch (error) {
      // DUPLICATE_ADAPTER or an invalid provider — another instance owns it or
      // this provider can't be wrapped; mark it seen so we don't retry forever.
      this.seen.add(visionRoute)
    }
  }
}

/**
 * Wraps a text-only provider adapter. Declares image input on every model so
 * the harness admits pasted/uploaded images, and transcribes image content
 * blocks through the shared visionmcp connection before delegating to the real
 * adapter. Only transcribes for models that do not natively accept image.
 */
class VisionRouteAdapter extends LlmAdapterBase {
  private readonly modalityPromises = new Map<string, Promise<boolean>>()

  constructor(
    private readonly ctx: Context,
    private readonly inner: LlmAdapter,
    private readonly sourceProvider: string,
    private readonly visionProvider: string,
    private readonly config: Config,
    private readonly client: VisionMcpClient,
  ) {
    super()
  }

  override providerInfo(provider: string): { id: string; name: string } {
    return { id: provider, name: `${this.sourceProvider} + 自动识图` }
  }

  override providerRetryPolicy(_provider: string): ReturnType<LlmAdapter['providerRetryPolicy']> | undefined {
    return this.inner.providerRetryPolicy?.(this.sourceProvider)
  }

  override listModels(provider: string): Promise<readonly LlmModelInfo[]> {
    return Promise.resolve(this.inner.listModels ? this.inner.listModels(this.sourceProvider) : Promise.resolve([]))
      .then(models => models.map(model => ({
        ...model,
        provider,
        ...supportsImages(model)
          ? {}
          : { inputModalities: [...(model.inputModalities ?? ['text']), 'image'] },
      })))
  }

  override resolveModel(
    provider: string,
    model: string,
    signal?: AbortSignal,
  ): Promise<LlmResolvedModelInfo> {
    return Promise.resolve(this.inner.resolveModel(this.sourceProvider, model, signal))
      .then((info) => ({
        ...info,
        provider,
        ...supportsImages(info)
          ? {}
          : { inputModalities: [...(info.inputModalities ?? ['text']), 'image'] },
      }))
  }

  async *stream(options: GenerateOptions): AsyncIterable<StreamChunk> {
    const modelSupportsImages = await this.modelSupportsImages(options.model, options.signal)
    let messages = options.messages
    if (!modelSupportsImages && hasImageBlock(messages)) {
      messages = await this.cleanMessages(messages, options.signal)
    }
    const next = messages === options.messages
      ? options
      : { ...options, messages }
    // Delegate with the source provider so adapter internals that key off the
    // provider route (e.g. pi-ai profile lookup, deepseek replay) behave.
    yield* this.inner.stream({ ...next, provider: this.sourceProvider })
  }

  private modelSupportsImages(model: string, signal?: AbortSignal): Promise<boolean> {
    let promise = this.modalityPromises.get(model)
    if (!promise) {
      promise = Promise.resolve(this.inner.resolveModel(this.sourceProvider, model, signal))
        .then(supportsImages)
        .catch(() => false)
      this.modalityPromises.set(model, promise)
    }
    return promise
  }

  private async cleanMessages(messages: readonly Message[], signal?: AbortSignal): Promise<Message[]> {
    const out: Message[] = []
    for (const message of messages) {
      if (!Array.isArray(message.content) || message.content.length === 0) {
        out.push(message)
        continue
      }
      const contextText = (message.content as readonly { type?: string; text?: string }[])
        .filter(block => block.type === 'text' && typeof block.text === 'string')
        .map(block => block.text as string)
        .join(' ')
      const content = await this.cleanBlocks(message.content, contextText, signal)
      out.push(content === message.content ? message : { ...message, content })
    }
    return out
  }

  private async cleanBlocks(blocks: readonly ContentBlock[], contextText: string, signal?: AbortSignal): Promise<ContentBlock[]> {
    const out: ContentBlock[] = []
    for (const block of blocks) {
      if (block.type === 'image' && block.attachment) {
        const text = await this.transcribe(block.attachment, contextText, signal)
        out.push({ type: 'text', text })
      } else if ('content' in block && Array.isArray(block.content) && block.content.length > 0) {
        const nested = await this.cleanBlocks(block.content as unknown as ContentBlock[], '', signal)
        out.push({ ...block, content: nested })
      } else {
        out.push(block)
      }
    }
    return out
  }

  private readonly transcribeCache = new Map<string, string>()

  private async transcribe(
    ref: ImageAttachmentRef,
    contextText: string,
    signal?: AbortSignal,
  ): Promise<string> {
    const key = String(ref.attachmentId)
    const cached = this.transcribeCache.get(key)
    if (cached !== undefined) return cached

    const attachments = this.ctx.get('attachments') as { readImage(ref: ImageAttachmentRef): Promise<{ data: Uint8Array }> } | undefined
    let text: string
    if (!attachments) {
      text = '[visionmcp: attachment service unavailable]'
    } else {
      try {
        const stored = await attachments.readImage(ref)
        const prompt = contextText && contextText.trim()
          ? `The user attached this image alongside the following text. Answer the user's request using the image content, then summarize what is shown.\n\nUser text:\n${contextText}`
          : 'Describe this image in detail.'
        text = await this.client.analyzeData(this.config, prompt, stored.data)
        if (this.config.labelPrefix !== '') text = `${this.config.labelPrefix}${text}`
      } catch (error) {
        text = `[visionmcp: failed to analyze this image — ${error instanceof Error ? error.message : String(error)}]`
      }
    }
    this.transcribeCache.set(key, text)
    return text
  }
}

/** Build the model-facing `mcp__<serverName>__analyze_image` tool. */
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
        properties: { analysis: { type: 'string' } },
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

/** Whether any message in a request carries an image block. */
function hasImageBlock(messages: readonly { content: readonly { type?: string }[] }[]): boolean {
  return messages.some(message => message.content.some(block => block.type === 'image'))
}

/**
 * A lazy, shared MCP client to ONE visionmcp stdio server. Connects on first
 * use and stays connected until the plugin is disposed.
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
    const childEnv = resolveChildEnv(config)

    const args = [
      '--model', config.model,
      '--retries', String(config.retries),
      '--retry-interval', config.retryInterval,
      '--log-level', 'info',
    ]
    const lockOverride = config.lockPath?.trim()
    if (lockOverride) args.push('--lock-path', lockOverride)
    this.transport = new StdioClientTransport({
      command,
      args,
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
 * Ambient env for the visionmcp child. When `config.lockPath` is set the
 * lock path is passed as `--lock-path` (see connect()); otherwise it
 * falls back to an existing `VISIONMCP_LOCK_PATH` env or the default
 * `<DSH_HOME>/visionmcp-bridge.lock`.
 */
function resolveChildEnv(config: Config): Record<string, string> {
  const env: Record<string, string> = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (value === undefined) continue
    env[key] = value
  }
  const explicit = config.lockPath?.trim()
  if (explicit) {
    // visionmcp prefers --lock-path over the env var; keep env in sync for
    // older visionmcp builds that only read the env var.
    env.VISIONMCP_LOCK_PATH = explicit
  } else if (!env.VISIONMCP_LOCK_PATH) {
    const dshHome = process.env.DSH_HOME
    const lockRoot = dshHome ?? process.env.LOCALAPPDATA ?? process.env.HOME ?? ''
    env.VISIONMCP_LOCK_PATH = requireNodePathJoin(lockRoot, 'visionmcp-bridge.lock')
  }
  return env
}




