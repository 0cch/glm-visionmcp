/**
 * Real-call verification: invoke visionmcp's analyze_image on a local image
 * and print GLM's answer. This proves the underlying vision capability works
 * end to end (it is what dsh's mcp__vision__analyze_image tool forwards to).
 *
 * Developer sanity check, not part of the published bundle. Needs:
 *   npm install --no-save @modelcontextprotocol/sdk
 *
 * Usage:
 *   $env:TEST_IMAGE = "C:\\path\\to\\test.png"
 *   $env:VISIONMCP_LOCK_PATH = "$env:TEMP\\visionmcp-real-call.lock"
 *   node scripts/vision-real-call.mjs
 */
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StdioClientTransport } from '@modelcontextprotocol/sdk/client/stdio.js'

const exe = process.env.VISIONMCP_PATH || 'F:\\Code\\glm-visionmcp\\visionmcp.exe'
const imgPath = process.env.TEST_IMAGE || 'C:\\Users\\wps\\AppData\\Local\\Temp\\vision-test.png'

const transport = new StdioClientTransport({
  command: exe,
  args: ['--model', 'glm-4.6v-flash', '--retries', '3', '--retry-interval', '1s', '--log-level', 'info'],
  env: { ...process.env },
})

const client = new Client({ name: 'dsh-vision-mcp-call-test', version: '0.0.1' })
await client.connect(transport)

console.log('=== calling analyze_image ===')
const result = await client.callTool({
  name: 'analyze_image',
  arguments: {
    prompt: 'What text and shapes do you see in this image? Describe it.',
    image: { path: imgPath },
  },
})

console.log('result:', JSON.stringify(result, null, 2).slice(0, 1500))
await client.close()
console.log('\ncall complete')
