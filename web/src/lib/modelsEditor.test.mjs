import test from 'node:test'
import assert from 'node:assert/strict'
import {
  API_MODE_OPTIONS,
  THINKING_TYPE_OPTIONS,
  addModelConfigs,
  createModelConfig,
  modelProtocolFields,
  profileModelConfigs,
  isModelConfigEnabled,
  modelAvailabilitySummary,
  reconcileModelAvailability,
  reconcileModelProbeResults,
  orderedModelRows,
  applyModelOrder,
  mergePersistedModelOrder,
  moveOrderedItem,
  reasoningEffortOptions,
  withModelConfigs,
} from './modelsEditor.js'

test('reconcileModelAvailability auto-disables missing models and restores recovered models', () => {
  const profile = {
    model_configs: [
      { model: 'active-model', reasoning_effort: 'high' },
      { model: 'missing-model', read_timeout: 600 },
      { model: 'recovered-model', enabled: false, auto_disabled: true },
    ],
  }
  const checkedAt = '2026-07-15T02:00:00Z'
  const result = reconcileModelAvailability(profile, ['active-model', 'recovered-model'], checkedAt)
  const configs = result.profile.model_configs

  assert.equal(isModelConfigEnabled(configs[0]), true)
  assert.equal(configs[0].availability, 'available')
  assert.equal(configs[1].enabled, false)
  assert.equal(configs[1].auto_disabled, true)
  assert.equal(configs[1].read_timeout, 600)
  assert.equal(configs[2].enabled, true)
  assert.equal(configs[2].auto_disabled, false)
  assert.deepEqual(result.summary, { available: 2, unavailable: 1, disabled: 1, restored: 1, checkedAt })
  assert.deepEqual(modelAvailabilitySummary(result.profile), {
    total: 3,
    enabled: 2,
    disabled: 1,
    unavailable: 1,
    checked: 3,
    checkedAt,
  })
  assert.equal(profile.model_configs[1].enabled, undefined)
})

test('reconcileModelAvailability preserves a manually disabled model when it is available', () => {
  const profile = { model_configs: [{ model: 'manual-off', enabled: false }] }
  const result = reconcileModelAvailability(profile, ['manual-off'], '2026-07-15T03:00:00Z')
  assert.equal(result.profile.model_configs[0].enabled, false)
  assert.equal(result.profile.model_configs[0].auto_disabled, undefined)
  assert.equal(result.profile.model_configs[0].availability, 'available')
})

test('reconcileModelProbeResults rejects listed models that fail a real chat request', () => {
  const profile = { model_configs: [{ model: 'listed-but-broken' }, { model: 'working-model', auto_disabled: true, enabled: false }] }
  const result = reconcileModelProbeResults(profile, [
    { id: 'listed-but-broken', available: false, status: 'request_failed', detail: 'HTTP 404', latency_ms: 15 },
    { id: 'working-model', available: true, status: 'available', detail: '真实对话验证通过', latency_ms: 23 },
  ], '2026-07-15T06:35:00Z')

  assert.equal(result.profile.model_configs[0].enabled, false)
  assert.equal(result.profile.model_configs[0].availability_detail, 'HTTP 404')
  assert.equal(result.profile.model_configs[0].availability_latency_ms, 15)
  assert.equal(result.profile.model_configs[1].enabled, true)
  assert.equal(result.profile.model_configs[1].auto_disabled, false)
  assert.deepEqual(result.summary, {
    available: 1, unavailable: 1, disabled: 1, restored: 1, checkedAt: '2026-07-15T06:35:00Z',
  })
})

test('profileModelConfigs migrates legacy provider settings into independent rows', () => {
  const profile = {
    model: 'alpha',
    models: ['alpha', 'beta'],
    stream: false,
    max_retries: 5,
    read_timeout: 120,
    connect_timeout: 9,
    reasoning_effort: 'high',
  }

  assert.deepEqual(profileModelConfigs(profile), [
    { model: 'alpha', stream: false, max_retries: 5, read_timeout: 120, connect_timeout: 9, reasoning_effort: 'high' },
    { model: 'beta', stream: false, max_retries: 5, read_timeout: 120, connect_timeout: 9, reasoning_effort: 'high' },
  ])
})

test('profileModelConfigs treats model_configs as the authoritative source', () => {
  const profile = {
    model: 'legacy',
    models: ['legacy'],
    model_configs: [
      { model: 'alpha', reasoning_effort: 'low' },
      { model: 'beta', read_timeout: 60 },
    ],
  }

  assert.deepEqual(profileModelConfigs(profile), profile.model_configs)
})

test('withModelConfigs synchronizes compatibility model indexes without sharing row settings', () => {
  const profile = { var_name: 'native_oai_config1', model: 'old', models: ['old'] }
  const next = withModelConfigs(profile, [
    { model: ' alpha ', reasoning_effort: 'low' },
    { model: 'beta', reasoning_effort: 'high' },
  ])

  assert.equal(next.model, 'alpha')
  assert.deepEqual(next.models, ['alpha', 'beta'])
  assert.deepEqual(next.model_configs, [
    { model: 'alpha', reasoning_effort: 'low' },
    { model: 'beta', reasoning_effort: 'high' },
  ])
})

test('addModelConfigs quick-adds unique discovered models and keeps existing rows', () => {
  const profile = withModelConfigs({}, [{ model: 'alpha', max_retries: 7 }])
  const next = addModelConfigs(profile, ['alpha', { id: 'beta' }, { name: 'gamma' }, ''])

  assert.deepEqual(next.model_configs, [
    { model: 'alpha', max_retries: 7 },
    createModelConfig('beta'),
    createModelConfig('gamma'),
  ])
  assert.deepEqual(next.models, ['alpha', 'beta', 'gamma'])
})

const optionValues = options => options.map(option => option.value)

test('modelProtocolFields distinguishes native and legacy protocol capabilities', () => {
  assert.deepEqual(modelProtocolFields('native_oai'), { apiMode: true, reasoningFamily: 'oai', userAgent: true })
  assert.deepEqual(modelProtocolFields('oai'), { apiMode: true, reasoningFamily: 'oai', userAgent: true })
  assert.deepEqual(modelProtocolFields('native_claude'), {
    thinkingType: true,
    reasoningFamily: 'claude',
    userAgent: true,
    fakeClaudeCode: true,
  })
  assert.deepEqual(modelProtocolFields('claude'), { thinkingType: true, reasoningFamily: 'claude' })
  assert.deepEqual(modelProtocolFields('unknown'), modelProtocolFields('native_oai'))
})

test('protocol-specific selects expose only supported values', () => {
  assert.deepEqual(optionValues(API_MODE_OPTIONS), ['chat_completions', 'responses'])
  assert.deepEqual(optionValues(THINKING_TYPE_OPTIONS), ['adaptive', 'enabled', 'disabled'])
  assert.deepEqual(optionValues(reasoningEffortOptions('native_oai')), ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'])
  assert.deepEqual(optionValues(reasoningEffortOptions('oai')), ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'])
  assert.deepEqual(optionValues(reasoningEffortOptions('native_claude')), ['low', 'medium', 'high', 'xhigh'])
  assert.deepEqual(optionValues(reasoningEffortOptions('claude')), ['low', 'medium', 'high', 'xhigh'])
})

const orderingProfiles = () => ([
  {
    var_name: 'provider_a',
    model_configs: [
      { model: 'a-one', sort_order: 0, stream: true },
      { model: 'a-two', sort_order: 2, stream: false },
    ],
  },
  {
    var_name: 'provider_b',
    model_configs: [
      { model: 'b-one', sort_order: 1, max_retries: 7 },
    ],
  },
])

test('orderedModelRows expands providers into the persisted global model order', () => {
  const rows = orderedModelRows(orderingProfiles())
  assert.deepEqual(rows.map(row => row.model), ['a-one', 'b-one', 'a-two'])
  assert.deepEqual(rows.map(row => row.providerVarName), ['provider_a', 'provider_b', 'provider_a'])
  assert.deepEqual(rows.map(row => row.variableName), ['provider_a', 'provider_b', 'provider_a_2'])
  assert.deepEqual(rows.map(row => row.id), ['0:0', '1:0', '0:1'])
})

test('orderedModelRows keeps legacy provider and model order without metadata', () => {
  const profiles = orderingProfiles().map(profile => ({
    ...profile,
    model_configs: profile.model_configs.map(({ sort_order: _sortOrder, ...config }) => config),
  }))
  assert.deepEqual(orderedModelRows(profiles).map(row => row.model), ['a-one', 'a-two', 'b-one'])
})

test('applyModelOrder writes consecutive metadata without moving provider configs', () => {
  const profiles = orderingProfiles()
  const rows = orderedModelRows(profiles)
  const next = applyModelOrder(profiles, [rows[2], rows[0], rows[1]])

  assert.deepEqual(next.map(profile => profile.model_configs.map(config => config.model)), [
    ['a-one', 'a-two'],
    ['b-one'],
  ])
  assert.deepEqual(next[0].model_configs.map(config => config.sort_order), [1, 0])
  assert.deepEqual(next[1].model_configs.map(config => config.sort_order), [2])
  assert.equal(next[0].model_configs[0].stream, true)
  assert.equal(next[0].model_configs[1].stream, false)
  assert.equal(next[1].model_configs[0].max_retries, 7)
  assert.notEqual(next, profiles)
  assert.notEqual(next[0].model_configs[0], profiles[0].model_configs[0])
})

test('mergePersistedModelOrder preserves draft fields and appends draft-only models', () => {
  const persisted = orderingProfiles()
  const persistedRows = orderedModelRows(persisted)
  const reorderedPersisted = applyModelOrder(persisted, [persistedRows[2], persistedRows[1], persistedRows[0]])
  const drafts = [
    {
      ...persisted[0],
      model_configs: [
        { ...persisted[0].model_configs[0], stream: false },
        { ...persisted[0].model_configs[1], read_timeout: 333 },
        { model: 'a-draft', max_retries: 11 },
      ],
    },
    {
      ...persisted[1],
      model_configs: [
        { ...persisted[1].model_configs[0], max_retries: 99 },
      ],
    },
  ]

  const merged = mergePersistedModelOrder(drafts, reorderedPersisted)

  assert.deepEqual(merged[0].model_configs.map(config => config.sort_order), [2, 0, 3])
  assert.deepEqual(merged[1].model_configs.map(config => config.sort_order), [1])
  assert.equal(merged[0].model_configs[0].stream, false)
  assert.equal(merged[0].model_configs[1].read_timeout, 333)
  assert.equal(merged[0].model_configs[2].max_retries, 11)
  assert.equal(merged[1].model_configs[0].max_retries, 99)
  assert.equal(drafts[0].model_configs[2].sort_order, undefined)
  assert.notEqual(merged[0].model_configs[0], drafts[0].model_configs[0])
})

test('moveOrderedItem reorders immutably and ignores invalid moves', () => {
  const rows = [{ id: 'a' }, { id: 'b' }, { id: 'c' }]
  const moved = moveOrderedItem(rows, 0, 2)

  assert.deepEqual(moved.map(row => row.id), ['b', 'c', 'a'])
  assert.notEqual(moved, rows)
  assert.deepEqual(rows.map(row => row.id), ['a', 'b', 'c'])
  assert.equal(moveOrderedItem(rows, 1, 1), rows)
  assert.equal(moveOrderedItem(rows, -1, 1), rows)
  assert.equal(moveOrderedItem(rows, 1, 3), rows)
})
