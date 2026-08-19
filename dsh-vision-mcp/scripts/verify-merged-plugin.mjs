/**
 * Merged-plugin (single MCP) load verification.
 *
 * Boots a real cordis context, provides the four services the plugin
 * injects (systemPrompt, llm, attachments, tools) the way the harness does,
 * then loads the compiled bundle and checks:
 *   - apply() runs without inject errors
 *   - the vision-mcp-guidance system-prompt section registers
 *   - the mcp__vision__analyze_image tool is registered with a valid shape
 *   - the llm/stream listener rewrites an image block into a text block
 *     before the downstream stream is produced when the route is text-only
 *     (vision client targets a non-existent command → real degrade path,
 *     no GLM call)
 *   - the llm/stream listener leaves image blocks untouched when the model
 *     natively supports images
 *
 * Usage: node scripts/verify-merged-plugin.mjs
 */
import { Context, Service } from '@deepseek-ai/cordis'
import SystemPrompt from '@deepseek-ai/dsh-system-prompt'
import * as bundle from '../lib/index.js'

const registeredTools = []
const toolDisposers = []

class LlmStub extends Service {
  constructor(ctx) { super(ctx, 'llm') }
  async resolveModelInfo(provider, model, signal) {
    const modalities = model.startsWith('vision-') ? ['text', 'image'] : ['text']
    return { provider, id: model, name: model, inputModalities: modalities }
  }
}

class AttachmentsStub extends Service {
  constructor(ctx) { super(ctx, 'attachments') }
  async readImage(ref) {
    return { ref, data: new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]) }
  }
}

class ToolsStub extends Service {
  constructor(ctx) { super(ctx, 'tools') }
  register(definition) {
    registeredTools.push(definition)
    const disposer = () => {
      const i = registeredTools.indexOf(definition)
      if (i >= 0) registeredTools.splice(i, 1)
    }
    toolDisposers.push(disposer)
    return disposer
  }
}

const ctx = new Context()
await ctx.plugin(SystemPrompt, {})
ctx.plugin(LlmStub)
ctx.plugin(AttachmentsStub)
ctx.plugin(ToolsStub)

const plugin = { name: bundle.name, inject: bundle.inject, Config: bundle.Config, apply: bundle.apply }
await ctx.plugin(plugin, {
  bridgeEnabled: true,
  toolEnabled: true,
  guidance: true,
  command: 'no-such-visionmcp.exe', // forces the degrade path, no GLM call
  model: 'glm-4.6v-flash',
  serverName: 'vision',
})
console.log('✓ plugin applied without errors')

// ── system-prompt section registered ───────────────────────────────────────
const assembly = await ctx.systemPrompt.assemble()
const sections = assembly.sections.map(s => s.name)
if (!sections.includes('vision-mcp-guidance')) {
  console.error('✗ vision-mcp-guidance section missing. sections =', sections)
  process.exit(1)
}
const guidance = assembly.sections.find(s => s.name === 'vision-mcp-guidance')
console.log('✓ vision-mcp-guidance registered, toolName present:',
  guidance.text.includes('mcp__vision__analyze_image'))

// ── tool registered with valid shape ───────────────────────────────────────
const tool = registeredTools.find(t => t.name === 'mcp__vision__analyze_image')
if (!tool) {
  console.error('✗ mcp__vision__analyze_image not registered. got:', registeredTools.map(t => t.name))
  process.exit(1)
}
console.log('✓ mcp__vision__analyze_image registered')
console.log('  parameters.image.data:', !!tool.parameters.properties.image.properties.data)
console.log('  output.render exists:', typeof tool.output.render === 'function')
console.log('  execute exists:', typeof tool.execute === 'function')

// Render projection sanity: value → text content block.
const rendered = tool.output.render({}, { analysis: 'hello' })
if (rendered.length !== 1 || rendered[0].type !== 'text' || rendered[0].text !== 'hello') {
  console.error('✗ tool output.render projection wrong:', JSON.stringify(rendered))
  process.exit(1)
}
console.log('✓ tool output.render projects analysis to text')

// ── helper: run one llm/stream waterfall with the given options ────────────
async function runWaterfall(options) {
  let downstreamCalled = false
  const stream = ctx.waterfall('llm/stream', options, () => {
    downstreamCalled = true
    return (async function* () {
      yield { type: 'finish', reason: { kind: 'stop' } }
    })()
  })
  const chunks = []
  for await (const chunk of stream) chunks.push(chunk)
  return { downstreamCalled, chunks }
}

const imageRef = {
  attachmentId: 'att_0000-test',
  mediaType: 'image/png',
  bytes: 10,
  width: 4,
  height: 4,
}
const userContent = () => [
  { type: 'text', text: 'what is in this screenshot?' },
  { type: 'image', attachment: imageRef },
]

// ── case A: text-only route → image rewritten to degraded analysis text ─────
const optionsA = {
  provider: 'deepseek-official',
  model: 'deepseek-chat',
  messages: [{ role: 'user', content: userContent() }],
}
const a = await runWaterfall(optionsA)
const contentA = optionsA.messages[0].content
const rewrittenA = !contentA.some(b => b.type === 'image')
  && contentA.some(b => b.type === 'text' && b.text.startsWith('[visionmcp:'))
  && contentA.some(b => b.type === 'text' && b.text === 'what is in this screenshot?')
console.log('✓ [text-only] image removed:', !contentA.some(b => b.type === 'image'))
console.log('✓ [text-only] degraded text inserted:', contentA.some(b => b.type === 'text' && b.text.startsWith('[visionmcp:')))
console.log('✓ [text-only] downstream called:', a.downstreamCalled)
if (!rewrittenA || !a.downstreamCalled) {
  console.error('✗ text-only rewrite failed:', JSON.stringify(contentA, null, 2))
  process.exit(1)
}

// ── case B: native image route → image block left untouched ────────────────
const optionsB = {
  provider: 'glm',
  model: 'vision-model',
  messages: [{ role: 'user', content: userContent() }],
}
const b = await runWaterfall(optionsB)
const contentB = optionsB.messages[0].content
const untouchedB = contentB.some(block => block.type === 'image')
console.log('✓ [native-image] image block preserved:', untouchedB)
console.log('✓ [native-image] downstream called:', b.downstreamCalled)
if (!untouchedB || !b.downstreamCalled) {
  console.error('✗ native-image route must not rewrite:', JSON.stringify(contentB, null, 2))
  process.exit(1)
}

console.log('\nALL CHECKS PASSED')
process.exit(0)
