import test from 'node:test'
import assert from 'node:assert/strict'
import { apiHeaders } from './api.js'
import { READ_ONLY_OBSERVABILITY_ENDPOINTS, buildObservabilitySnapshot, observabilityRequest, observabilityStats } from './observability.js'

test('observability endpoints stay read-only GET without dangerous confirm header', () => {
  for (const endpoint of READ_ONLY_OBSERVABILITY_ENDPOINTS) {
    const req = observabilityRequest(endpoint)
    assert.equal(req.url, endpoint)
    assert.equal(req.options.method, 'GET')
    assert.equal(apiHeaders(req.options)['X-GA-Confirm'], undefined)
  }
})

test('buildObservabilitySnapshot merges health inventory and risk catalog', () => {
  const snapshot = buildObservabilitySnapshot({
    health: { ok: false, root: '/ga', checks: { agentmain: 'ok', reflect: 'empty' }, errors: ['reflect: empty'], warnings: [] },
    inventory: {
      root: '/ga',
      core_files: [{ path: 'agentmain.py', exists: true }, { path: 'llmcore.py', exists: false }],
      tools: [{ path: 'assets/tools_schema.json', exists: true }],
      frontends: [{ name: 'web', path: 'frontends/web' }],
      reflect: [],
      memory: { sops: [{ name: 'tmwebdriver_sop.md' }], utils: [{ name: 'ocr_utils.py' }] },
    },
    risks: [{ path: '/api/files/write', level: 'dangerous', action: 'write_file' }, { path: '/api/health', level: 'read', action: 'inspect' }],
  })
  assert.equal(snapshot.ok, false)
  assert.equal(snapshot.root, '/ga')
  assert.deepEqual(snapshot.missingCore.map(x => x.path), ['llmcore.py'])
  assert.equal(snapshot.memory.sops.length, 1)
  assert.equal(snapshot.riskItems.length, 2)
  assert.equal(snapshot.writeRiskItems.length, 1)
  assert.deepEqual(observabilityStats(snapshot).map(x => x.value), [2, 1, 1, 2])
})

test('observabilityRequest rejects non-observability endpoints', () => {
  assert.throws(() => observabilityRequest('/api/files/write'), /unsupported observability endpoint/)
})

test('buildObservabilitySnapshot keeps stale or null observability payloads empty and explicit', () => {
  const snapshot = buildObservabilitySnapshot({
    health: { ok: true, checks: null, errors: 'stale-error', warnings: null, inventory: { memory: null } },
    inventory: { core_files: null, tools: null, frontends: null, reflect: null, memory: null },
    risks: { items: null },
  })

  assert.equal(snapshot.ok, true)
  assert.equal(snapshot.root, '')
  assert.equal(snapshot.generatedAt, '')
  assert.deepEqual(snapshot.checks, [])
  assert.deepEqual(snapshot.errors, [])
  assert.deepEqual(snapshot.warnings, [])
  assert.deepEqual(snapshot.coreFiles, [])
  assert.deepEqual(snapshot.missingCore, [])
  assert.deepEqual(snapshot.tools, [])
  assert.deepEqual(snapshot.frontends, [])
  assert.deepEqual(snapshot.reflect, [])
  assert.deepEqual(snapshot.memory, { sops: [], utils: [], rawSessions: [], insight: null, facts: null })
  assert.deepEqual(snapshot.riskItems, [])
  assert.deepEqual(snapshot.writeRiskItems, [])
  assert.deepEqual(observabilityStats(snapshot).map(x => x.value), [0, 0, 0, 0])
})

test('buildObservabilitySnapshot accepts array risks and flags write-like stale actions', () => {
  const snapshot = buildObservabilitySnapshot({ risks: [
    { level: 'review', action: 'read_status', reason: 'inspect only' },
    { level: 'review', action: 'delete stale cache', reason: '' },
    { level: 'dangerous', action: '', reason: 'offline fallback' },
  ] })

  assert.equal(snapshot.ok, false)
  assert.equal(snapshot.riskItems.length, 3)
  assert.deepEqual(snapshot.writeRiskItems.map(item => item.action), ['delete stale cache', ''])
})
