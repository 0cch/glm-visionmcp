/**
 * End-to-end bridge verification: connect to visionmcp.exe exactly like
 * @deepseek-ai/dsh-mcp-client does (stdio transport via the MCP SDK),
 * discover tools, and print what the model would see.
 *
 * This script is a developer sanity check, not part of the published bundle.
 * It needs a one-off SDK install in the package directory first:
 *
 *   npm install --no-save @modelcontextprotocol/sdk
 *
 * Usage:
 *   $env:VISIONMCP_PATH = "F:\Code\glm-visionmcp\visionmcp.exe"
 *   $env:VISIONMCP_LOCK_PATH = "$env:TEMP\visionmcp-e2e-test.lock"
 *   node scripts/e2e-bridge-test.mjs
 */
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js'

const exe = process.env.VISIONMCP_PATH || 'F:\\Code\\glm-visionmcp\\visionmcp.exe'
const key = process.env.GLM_API_KEY || 'test-does-not-matter-for-tool-discovery'

const transport = new StdioClientTransport({
  command: exe,
  args: ['--model', 'glm-4.6v-flash', '--retries', '5', '--retry-interval', '1s', '--log-level', 'info'],
  env: { ...process.env, GLM_API_KEY: key },
})

const client = new Client({ name: 'dsh-vision-mcp-e2e', version: '0.0.1' })
await client.connect(transport)

const tools = await client.listTools()
console.log('=== discovered tools ===')
for (const t of tools.tools) {
  console.log(`name: ${t.name}`)
  console.log(`description: ${t.description}`)
  console.log(`publicName (dsh-mcp-client): mcp__vision__${t.name}`)
  console.log(`inputSchema: ${JSON.stringify(t.inputSchema)}`)
}

await client.close()
console.log('\nbridge OK: visionmcp tools discovered and closed cleanly')
