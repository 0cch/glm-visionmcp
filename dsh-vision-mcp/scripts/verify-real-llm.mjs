/**
 * Real-LlmRuntime integration verification.
 *
 * Boots a real @deepseek-ai/dsh-llm LlmRuntime service, loads the compiled
 * plugin, and verifies the generic route-wrapping mechanism in BOTH orderings:
 *   A. provider already registered before the plugin loads
 *   B. provider registered AFTER the plugin loads (llm/adapters-updated
 *      re-entry must wrap it)
 * Also checks native-image providers are never wrapped, resolveModelInfo
 * declares image on the wrapper, and the tool + guidance still register.
 *
 * Usage: node scripts/verify-real-llm.mjs
 */
import { Context, Service } from '@deepseek-ai/cordis'
import LlmRuntime, { LlmAdapter } from '@deepseek-ai/dsh-llm'
import SystemPrompt from '@deepseek-ai/dsh-system-prompt'
import * as bundle from '../lib/index.js'

class TextOnlyAdapter extends LlmAdapter {
  async resolveModel(provider, model, signal) { return { provider, id: model, name: model, inputModalities: ['text'] } }
  listModels(provider) { return Promise.resolve([{ provider, id: 'm1', name: 'M1', inputModalities: ['text'] }]) }
  providerInfo(provider) { return { id: provider, name: 'TextOnly' } }
  async *stream(options) { yield { type: 'finish', reason: { kind: 'stop' } } }
}
class VisionNativeAdapter extends LlmAdapter {
  async resolveModel(provider, model, signal) { return { provider, id: model, name: model, inputModalities: ['text', 'image'] } }
  listModels(provider) { return Promise.resolve([{ provider, id: 'v1', name: 'V1', inputModalities: ['text', 'image'] }]) }
  providerInfo(provider) { return { id: provider, name: 'VisionNative' } }
  async *stream(options) { yield { type: 'finish', reason: { kind: 'stop' } } }
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
ctx.plugin(LlmRuntime)
ctx.plugin(AttachmentsStub)
ctx.plugin(ToolsStub)
await new Promise(r => setImmediate(r))

// Case A: text-only provider registered BEFORE plugin load.
ctx.llm.registerAdapter(['test-text'], new TextOnlyAdapter())
// Native-image provider — must never be wrapped.
ctx.llm.registerAdapter(['native-vision'], new VisionNativeAdapter())
await new Promise(r => setTimeout(r, 50))

const plugin = { name: bundle.name, inject: bundle.inject, Config: bundle.Config, apply: bundle.apply }
await ctx.plugin(plugin, {
  routeEnabled: true, toolEnabled: true, bridgeEnabled: false, guidance: true,
  command: 'no-such-visionmcp.exe', model: 'glm-4.6v-flash', serverName: 'vision', routeSuffix: '-vision',
})
console.log('✓ plugin applied without errors')

// Case B: another text-only provider registered AFTER plugin load.
ctx.llm.registerAdapter(['late-text'], new TextOnlyAdapter())
await new Promise(r => setTimeout(r, 300))

const providers = ctx.llm.listProviders().map(p => p.id)
console.log('providers:', JSON.stringify(providers))
if (!providers.includes('test-text-vision')) { console.error('✗ test-text-vision NOT registered'); process.exit(1) }
console.log('✓ test-text-vision registered (case A)')
if (!providers.includes('late-text-vision')) { console.error('✗ late-text-vision NOT registered (adapters-updated re-entry)'); process.exit(1) }
console.log('✓ late-text-vision registered (case B)')
if (providers.includes('native-vision-vision')) { console.error('✗ native-vision-vision should NOT be wrapped'); process.exit(1) }
console.log('✓ native-vision (native image) NOT wrapped')

const info = await ctx.llm.resolveModelInfo('test-text-vision', 'm1')
if (!info.inputModalities?.includes('image')) { console.error('✗ resolveModelInfo missing image:', JSON.stringify(info)); process.exit(1) }
console.log('✓ resolveModelInfo(test-text-vision) declares image:', JSON.stringify(info.inputModalities))

if (!registeredTools.some(t => t.name === 'mcp__vision__analyze_image')) { console.error('✗ tool not registered'); process.exit(1) }
console.log('✓ mcp__vision__analyze_image registered')

const assembly = await ctx.systemPrompt.assemble()
if (!assembly.sections.some(s => s.name === 'vision-mcp-guidance')) { console.error('✗ guidance missing'); process.exit(1) }
console.log('✓ vision-mcp-guidance registered')

console.log('\nALL CHECKS PASSED')
process.exit(0)
