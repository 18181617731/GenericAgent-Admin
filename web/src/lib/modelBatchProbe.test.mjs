import assert from 'node:assert/strict'
import test from 'node:test'
import {
  normalizeModelProbeProviderKeys,
  resolveModelProbeTargets,
  runModelBatchProbe,
} from './modelBatchProbe.js'

const profiles = [
  {
    var_name: 'native_oai_config_primary',
    name: '主服务商',
    type: 'native_oai',
    apibase: 'https://primary.example/v1',
    apikey: '******',
    model_configs: [{ model: 'primary-model' }],
  },
  {
    var_name: 'native_oai_config_backup',
    name: '备用服务商',
    type: 'native_oai',
    apibase: 'https://backup.example/v1',
    apikey: '******',
    model_configs: [{ model: 'backup-model' }],
  },
]

test('empty provider configuration selects every provider by default', () => {
  assert.deepEqual(resolveModelProbeTargets(profiles, []).map(item => item.key), [
    'native_oai_config_primary',
    'native_oai_config_backup',
  ])
})

test('configured provider keys are normalized and limit the next batch', () => {
  assert.deepEqual(normalizeModelProbeProviderKeys([' native_oai_config_backup ', '', 'native_oai_config_backup']), ['native_oai_config_backup'])
  assert.deepEqual(resolveModelProbeTargets(profiles, ['native_oai_config_backup']).map(item => item.name), ['备用服务商'])
})

test('batch probe keeps failed providers unchanged and reconciles successful providers', async () => {
  const progress = []
  const result = await runModelBatchProbe({
    profiles,
    configuredKeys: [],
    onProgress: value => progress.push(value),
    probeModels: async request => {
      if (request.varName === 'native_oai_config_backup') throw new Error('HTTP 503')
      return {
        checked_at: '2026-07-16T15:00:00+08:00',
        results: [{ id: 'primary-model', available: false, detail: 'HTTP 404', latency_ms: 12 }],
      }
    },
  })

  assert.equal(result.summary.successfulProviders, 1)
  assert.equal(result.summary.failedProviders, 1)
  assert.equal(result.summary.disabled, 1)
  assert.equal(result.profiles[0].model_configs[0].enabled, false)
  assert.equal(result.profiles[0].model_configs[0].auto_disabled, true)
  assert.deepEqual(result.profiles[1], profiles[1])
  assert.deepEqual(progress.at(-1), { completed: 2, total: 2, current: '备用服务商' })
})
