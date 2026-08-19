import { Context, Service } from '@deepseek-ai/cordis'
import { LlmAdapter } from '@deepseek-ai/dsh-llm'
import SystemPrompt from '@deepseek-ai/dsh-system-prompt'
import * as bundle from '../lib/index.js'

class TextOnlyAdapter extends LlmAdapter {
  async resolveModel(provider, model, signal) { return { provider, id: model, name: model, inputModalities: ['text'] } }
  listModels(provider) { return Promise.resolve([{ provider, id: 'm1', name: 'M1', inputModalities: ['text'] }]) }
  providerInfo(provider) { return { id: provider, name: 'TextOnly' } }
  async *stream(options) { yield { type: 'finish', reason: { kind: 'stop' } } }
}
class VisionAdapter extends LlmAdapter {
  async resolveModel(provider, model, signal) { return { provider, id: model, name: model, inputModalities: ['text', 'image'] } }
  listModels(provider) { return Promise.resolve([{ provider, id: 'v1', name: 'V1', inputModalities: ['text', 'image'] }]) }
  providerInfo(provider) { return { id: provider, name: 'VisionNative' } }
  async *stream(options) { yield { type: 'finish', reason: { kind: 'stop' } } }
}

class LlmStub extends Service {
  constructor(ctx) {
    super(ctx, 'llm')
    this.adapters = new Map()
    this.textAdapter = new TextOnlyAdapter()
    this.visionAdapter = new VisionAdapter()
    this.adapters.set('deepseek-official', { adapter: this.textAdapter, provider: { id: 'deepseek-official', name: 'TextOnly' } })
    this.adapters.set('glm-native', { adapter: this.visionAdapter, provider: { id: 'glm-native', name: 'VisionNative' } })
  }
  listProviders() { return [...this.adapters.values()].map(a => ({ ...a.provider })) }
  registration(provider) { return this.adapters.get(provider) }
  registerAdapter(providers, adapter) {
    for (const p of providers) this.adapters.set(p, { adapter, provider: { id: p, name: adapter.providerInfo(p).name } })
    return () => { for (const p of providers) this.adapters.delete(p) }
  }
}

class AttachmentsStub extends Service {
  constructor(ctx) { super(ctx, 'attachments') }
  async readImage(ref) { return { ref, data: new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]) } }
}

const registeredTools = []
class ToolsStub extends Service {
  constructor(ctx) { super(ctx, 'tools') }
  register(def) { registeredTools.push(def); return () => { const i = registeredTools.indexOf(def); if (i >= 0) registeredTools.splice(i, 1) } }
}

const ctx = new Context()
await ctx.plugin(SystemPrompt, {})
ctx.plugin(LlmStub)
ctx.plugin(AttachmentsStub)
ctx.plugin(ToolsStub)
await new Promise(r => setImmediate(r))

const plugin = { name: bundle.name, inject: bundle.inject, Config: bundle.Config, apply: bundle.apply }
await ctx.plugin(plugin, {
  routeEnabled: true, toolEnabled: true, bridgeEnabled: false, guidance: true,
  command: 'no-such-visionmcp.exe', model: 'glm-4.6v-flash', serverName: 'vision', routeSuffix: '-vision',
})
console.log('✓ plugin applied without errors')

const llm = ctx.llm

// wrapper route registered for text-only provider
const wrapped = llm.adapters.get('deepseek-official-vision')
if (!wrapped) { console.error('✗ deepseek-official-vision wrapper NOT registered'); process.exit(1) }
console.log('✓ deepseek-official-vision wrapper registered')

if (llm.adapters.has('glm-native-vision')) { console.error('✗ glm-native-vision should NOT be registered'); process.exit(1) }
console.log('✓ glm-native (native image) NOT wrapped')

const info = await wrapped.adapter.resolveModel('deepseek-official-vision', 'm1')
if (!info.inputModalities?.includes('image')) { console.error('✗ resolveModel missing image:', JSON.stringify(info)); process.exit(1) }
console.log('✓ wrapper resolveModel declares image:', JSON.stringify(info.inputModalities))

const imageRef = { attachmentId: 'att_0000-test', mediaType: 'image/png', bytes: 10, width: 4, height: 4 }
const options = { provider: 'deepseek-official-vision', model: 'm1', messages: [{ role: 'user', content: [{ type: 'text', text: 'what is in this screenshot?' }, { type: 'image', attachment: imageRef }] }] }
let innerReceived
const originalStream = llm.textAdapter.stream.bind(llm.textAdapter)
llm.textAdapter.stream = async function* (opts) { innerReceived = opts; yield { type: 'finish', reason: { kind: 'stop' } } }
const chunks = []
for await (const chunk of wrapped.adapter.stream(options)) chunks.push(chunk)
llm.textAdapter.stream = originalStream

const userMsg = innerReceived.messages[0]
const hasImageAfter = userMsg.content.some(b => b.type === 'image')
const hasDegradeText = userMsg.content.some(b => b.type === 'text' && b.text.startsWith('[visionmcp:'))
console.log('✓ inner received provider:', innerReceived.provider)
console.log('✓ inner received no image block:', !hasImageAfter)
console.log('✓ inner received degraded analysis text:', hasDegradeText)
if (hasImageAfter || !hasDegradeText || innerReceived.provider !== 'deepseek-official') {
  console.error('✗ wrapper stream did not transcribe+delegate correctly'); process.exit(1)
}

if (!registeredTools.some(t => t.name === 'mcp__vision__analyze_image')) { console.error('✗ tool not registered'); process.exit(1) }
console.log('✓ mcp__vision__analyze_image registered')

const assembly = await ctx.systemPrompt.assemble()
if (!assembly.sections.some(s => s.name === 'vision-mcp-guidance')) { console.error('✗ guidance missing'); process.exit(1) }
console.log('✓ vision-mcp-guidance registered')

console.log('\nALL CHECKS PASSED')
process.exit(0)
