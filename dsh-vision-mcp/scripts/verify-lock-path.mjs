// Strengthened lock-path verification: two scenarios.
//  A) lockPath configured -> lock file at that exact path
//  B) no lockPath -> default <DSH_HOME>/visionmcp-bridge.lock
import { VisionMcpClient } from '../lib/index.js'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { existsSync, rmSync } from 'node:fs'

const pngB64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='
const base = {
  command: 'F:\\Code\\glm-visionmcp\\visionmcp.exe',
  model: 'glm-4.6v-flash',
  retries: 1,
  retryInterval: '100ms',
  timeoutMs: 45000,
  serverName: 'vision',
  routeSuffix: '-vision',
}

// Scenario A: explicit lockPath
const lockA = join(tmpdir(), `visionmcp-plugin-lock-A-${Date.now()}.lock`)
rmSync(lockA, { force: true })
const cfgA = { ...base, lockPath: lockA }
const clientA = new VisionMcpClient(cfgA)
await clientA.analyzeTool(cfgA, 'x', { data: pngB64 })
const okA = existsSync(lockA)
console.log((okA ? '✓' : '✗') + ' A: lock file at configured lockPath:', lockA, okA ? 'exists' : 'MISSING')
await clientA.dispose()

// Scenario B: no lockPath -> default under DSH_HOME
const dshHome = join(tmpdir(), `dsh-home-${Date.now()}`)
rmSync(dshHome, { recursive: true, force: true })
const defaultLock = join(dshHome, 'visionmcp-bridge.lock')
process.env.DSH_HOME = dshHome
const cfgB = { ...base }
const clientB = new VisionMcpClient(cfgB)
await clientB.analyzeTool(cfgB, 'x', { data: pngB64 })
const okB = existsSync(defaultLock)
console.log((okB ? '✓' : '✗') + ' B: default lock file at DSH_HOME/visionmcp-bridge.lock:', defaultLock, okB ? 'exists' : 'MISSING')
await clientB.dispose()

rmSync(lockA, { force: true })
rmSync(dshHome, { recursive: true, force: true })
if (!okA || !okB) { console.error('\nLOCK-PATH CHECK FAILED'); process.exit(1) }
console.log('\nALL LOCK-PATH CHECKS PASSED')
process.exit(0)
