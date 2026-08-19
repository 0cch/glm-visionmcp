/**
 * dsh-vision-mcp — DeepSeek Harness vision bridge.
 *
 * Adds a system-prompt guidance section that teaches the model when and how
 * to use the `mcp__vision__analyze_image` tool bridged from the visionmcp MCP
 * server. The heavy lifting (spawning visionmcp.exe and registering its tools)
 * is done by the in-box @deepseek-ai/dsh-mcp-client plugin configured in
 * cordis.patch.yml; this plugin only contributes the prompt guidance.
 *
 * The plugin is intentionally optional: the bundle works without it, but the
 * model is far more likely to reach for the vision tool with the guidance in
 * place.
 */

import type { Context } from '@deepseek-ai/cordis'
import Schema from '@deepseek-ai/schemastery'
import type {} from '@deepseek-ai/dsh-system-prompt'

export const name = 'dsh-vision-mcp'

/** The prompt registry this plugin contributes to. */
export const inject = ['systemPrompt']

export interface Config {
  /** Whether to inject the vision usage guidance into the system prompt. */
  guidance: boolean
  /** Custom guidance text; defaults to the built-in instruction. */
  guidanceText?: string
  /** The public tool name as registered by dsh-mcp-client. */
  toolName: string
  /** Section order in the assembled system prompt (tool guidance band: 100-199). */
  sectionOrder: number
}

export const Config: Schema<Config> = Schema.object({
  guidance: Schema.boolean().default(true),
  guidanceText: Schema.string(),
  toolName: Schema.string().default('mcp__vision__analyze_image'),
  sectionOrder: Schema.number().default(120),
})

const DEFAULT_GUIDANCE = `## Vision
You have access to an image-analysis tool (\`{{toolName}}\`). Use it whenever a task requires understanding visual content: screenshots, diagrams, photos, UI mockups, charts, scanned documents, or any image the user references.

Call it with:
- \`prompt\`: a concise, self-contained question or extraction task about the image.
- \`image\`: exactly one of \`path\` (absolute path or workspace-relative path), \`url\` (HTTP(S) image URL), or \`data\` (raw base64 bytes). Provide only one source.

The tool returns plain text describing or answering about the image. Prefer it over guessing when the answer depends on something visual.`

export function apply(ctx: Context, config: Config) {
  if (!config.guidance) return

  const text = config.guidanceText?.trim() || DEFAULT_GUIDANCE.replace('{{toolName}}', config.toolName)

  ctx.effect(() =>
    ctx.systemPrompt.section({
      name: 'vision-mcp-guidance',
      order: config.sectionOrder,
      text,
    }),
  )
}
