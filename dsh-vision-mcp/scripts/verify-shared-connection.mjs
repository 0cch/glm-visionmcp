/**
 * Real single-connection verification: instantiate the bundle's
 * VisionMcpClient (the exact class the plugin uses for BOTH the native tool
 * and the automatic bridge), connect once to visionmcp.exe, and analyze the
 * same image through both call styles:
 *   - analyzeTool:  MCP-shaped { path } args (what the model-facing tool sends)
 *   - analyzeData:  raw bytes → base64 data (what the bridge sends)
 * Both flow through the same client/connection → proves one visionmcp process
 * serves both capabilities.
 *
 * Needs a one-off SDK install (present in node_modules):
 *   npm install --no-save @modelcontextprotocol/sdk
 *
 * Usage:
 *   $env:VISIONMCP_PATH = "F:\Code\glm-visionmcp\visionmcp.exe"
 *   $env:TEST_IMAGE = "C:\path\test.png"
 *   node scripts/verify-shared-connection.mjs
 */
import * as bundle from '../lib/index.js'

const exe = process.env.VISIONMCP_PATH || 'F:\\Code\\glm-visionmcp\\visionmcp.exe'
const img = process.env.TEST_IMAGE || 'C:\\Users\\wps\\AppData\\Local\\Temp\\ScreenShot_2026-08-11_203803_416.png'

const config = {
  command: exe,
  model: 'glm-4.6v-flash',
  retries: 3,
  retryInterval: '1s',
  timeoutMs: 120000,
  cwd: 'F:\\Code\\glm-visionmcp',
  labelPrefix: '',
}

const client = new bundle.VisionMcpClient(config)
console.log('✓ VisionMcpClient created (lazy — no process yet)')

// 1) Tool path: MCP-shaped args with a file path.
console.log('\n=== tool path: analyzeTool({ path }) ===')
const viaPath = await client.analyzeTool(config, 'What text and shapes are visible in this image? Summarize it.', { path: img })
console.log('tool-path analysis:', viaPath.slice(0, 300))

// 2) Bridge path: raw bytes as base64 data (same connection).
console.log('\n=== bridge path: analyzeData(bytes) ===')
const { readFile } = await import('node:fs/promises')
const bytes = await readFile(img)
const viaData = await client.analyzeData(config, 'Describe this image briefly.', new Uint8Array(bytes))
console.log('bridge-path analysis:', viaData.slice(0, 300))

await client.dispose()
console.log('\nSHARED-CONNECTION OK: one client served both paths')
process.exit(0)
