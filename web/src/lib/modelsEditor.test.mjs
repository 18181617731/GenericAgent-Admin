import test from 'node:test'
import assert from 'node:assert/strict'
import {
  API_MODE_OPTIONS,
  SERVICE_TIER_OPTIONS,
  THINKING_TYPE_OPTIONS,
  addModelConfigs,
  createModelConfig,
  modelProtocolFields,
  profileModelConfigs,
  isModelConfigEnabled,
  modelAvailabilitySummary,
  modelConfigDisplayName,
  reconcileModelAvailability,
  reconcileModelProbeResults,
  orderedModelRows,
  applyModelOrder,
  applyProviderOrder,
  orderedProviderProfiles,
  mergePersistedModelOrder,
  normalizeFailoverGroups,
  FAILOVER_VAR_PREFIX,
  failoverGroupSuffix,
  failoverGroupVarName,
  migrateFailoverGroupNames,
  nextFailoverGroupName,
  remapFailoverGroupReferences,
  mergePersistedFailoverConfig,
  moveOrderedItem,
  applyFailoverConfig,
  orderedFailoverRows,
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
  assert.deepEqual(optionValues(SERVICE_TIER_OPTIONS), ['auto', 'default', 'priority', 'flex'])
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

test('orderedModelRows carries the configured provider display name', () => {
  const profiles = orderingProfiles()
  profiles[0].name = 'Acme 显示名称'
  profiles[0].model_configs[0].name = 'Friendly alpha'
  const rows = orderedModelRows(profiles)

  assert.deepEqual(rows.map(row => row.providerName), ['Acme 显示名称', '', 'Acme 显示名称'])
  assert.deepEqual(rows.map(row => row.displayName), ['Friendly alpha', 'b-one', 'a-two'])
})

test('orderedModelRows exposes model display names for failover candidates', () => {
  const profiles = [{
    var_name: 'provider_a',
    name: 'Paid provider',
    model_configs: [{ model: '12', name: 'Paid primary' }],
  }]
  const [row] = orderedModelRows(profiles)
  assert.equal(row.displayName, 'Paid primary')
  assert.equal(row.model, '12')
  assert.equal(row.providerName, 'Paid provider')
})

test('modelConfigDisplayName accepts API display_name aliases before legacy name', () => {
  assert.equal(modelConfigDisplayName({ display_name: 'API display', displayName: 'camel display', name: 'legacy name' }), 'API display')
  assert.equal(modelConfigDisplayName({ displayName: 'camel display', name: 'legacy name' }), 'camel display')
  assert.equal(modelConfigDisplayName({ name: 'legacy name' }), 'legacy name')
  assert.equal(modelConfigDisplayName({ model: 'gpt-model', name: 'native_oai_config27_2' }), '')
  assert.equal(modelConfigDisplayName({ model: 'gpt-model', name: '12' }), '')
  assert.equal(modelConfigDisplayName({ model: 'gpt-model', name: 'gpt-model' }), '')
  assert.equal(modelConfigDisplayName({ model: 'model-id' }), '')
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


test('failover helpers persist membership and settings on model rows', () => {
  const profiles = [
    { var_name: 'native_oai_config_main', model_configs: [{ model: 'alpha' }, { model: 'beta' }] },
    { var_name: 'native_claude_config_backup', model_configs: [{ model: 'gamma' }] },
  ]
  const rows = orderedModelRows(profiles)
  const next = applyFailoverConfig(profiles, [rows[2], rows[0]], {
    maxRetries: 10,
    baseDelay: 0.5,
    springBack: 120,
  })

  assert.deepEqual(orderedFailoverRows(next).map(row => row.model), ['gamma', 'alpha'])
  assert.equal(next[1].model_configs[0].failover_order, 0)
  assert.equal(next[0].model_configs[0].failover_order, 1)
  assert.equal(next[0].model_configs[1].failover_order, undefined)
  for (const config of [next[1].model_configs[0], next[0].model_configs[0]]) {
    assert.equal(config.failover_max_retries, 10)
    assert.equal(config.failover_base_delay, 0.5)
    assert.equal(config.failover_spring_back, 120)
  }
  assert.equal(profiles[0].model_configs[0].failover_order, undefined)
})

test('normalizes explicit failover groups without losing intentional zero values', () => {
  assert.deepEqual(normalizeFailoverGroups([{
    var_name: '  routing_mixin  ',
    members: [{ provider_var_name: ' provider_a ', model: ' alpha ' }],
    max_retries: 0,
    base_delay: 0,
    spring_back: '',
  }, {
    var_name: 'backup_mixin',
    members: [],
  }]), [{
    var_name: 'routing_mixin',
    members: [{ provider_var_name: 'provider_a', model: 'alpha' }],
    max_retries: 0,
    base_delay: 0,
  }, {
    var_name: 'backup_mixin',
    members: [],
    max_retries: 10,
    base_delay: 0.5,
  }])
})

test('converts failover group names between the fixed prefix and editable suffix', () => {
  assert.equal(FAILOVER_VAR_PREFIX, 'mixin_config_')
  assert.equal(failoverGroupSuffix('mixin_config_primary'), 'primary')
  assert.equal(failoverGroupSuffix('legacy_route'), 'legacy_route')
  assert.equal(failoverGroupVarName('primary_2'), 'mixin_config_primary_2')
})

test('migrates legacy failover group names into the fixed namespace without collisions', () => {
  const groups = [
    { var_name: 'mixin_config_main', marker: 1 },
    { var_name: 'main', marker: 2 },
    { var_name: 'route-prod', marker: 3 },
    { var_name: 'route prod', marker: 4 },
    { var_name: '!!!', marker: 5 },
  ]
  assert.deepEqual(migrateFailoverGroupNames(groups), [
    { var_name: 'mixin_config_main', marker: 1 },
    { var_name: 'mixin_config_main_2', marker: 2 },
    { var_name: 'mixin_config_route_prod', marker: 3 },
    { var_name: 'mixin_config_route_prod_2', marker: 4 },
    { var_name: 'mixin_config_5', marker: 5 },
  ])
})

test('allocates stable unique failover group variable names', () => {
  assert.equal(nextFailoverGroupName([]), 'mixin_config_1')
  assert.equal(nextFailoverGroupName([
    { var_name: 'mixin_config_1' },
    { var_name: 'mixin_config_2' },
    { var_name: 'mixin_config_4' },
  ]), 'mixin_config_3')
})

test('remaps explicit failover member references after provider and model rename', () => {
  const groups = [{
    var_name: 'routing_mixin',
    members: [
      { provider_var_name: 'provider_a', model: 'alpha' },
      { provider_var_name: 'provider_b', model: 'beta' },
    ],
    max_retries: 7,
    base_delay: 1.25,
    spring_back: 12,
  }]
  assert.deepEqual(remapFailoverGroupReferences(groups, [{
    from_provider_var_name: 'provider_a',
    from_model: 'alpha',
    to_provider_var_name: 'provider_next',
    to_model: 'alpha-v2',
  }]), [{
    ...groups[0],
    members: [
      { provider_var_name: 'provider_next', model: 'alpha-v2' },
      { provider_var_name: 'provider_b', model: 'beta' },
    ],
  }])
})

test('failover metadata survives model rename and ordinary model ordering', () => {
  const profiles = [{
    var_name: 'native_oai_config_main',
    model_configs: [
      { model: 'alpha', failover_order: 1, failover_max_retries: 7, failover_base_delay: 1.25 },
      { model: 'beta', failover_order: 0, failover_max_retries: 7, failover_base_delay: 1.25 },
    ],
  }]
  const renamed = withModelConfigs(profiles[0], [
    { ...profiles[0].model_configs[0], model: 'alpha-renamed' },
    profiles[0].model_configs[1],
  ])
  const reordered = applyModelOrder([renamed], orderedModelRows([renamed]).reverse())

  assert.deepEqual(orderedFailoverRows(reordered).map(row => row.model), ['beta', 'alpha-renamed'])
  assert.deepEqual(reordered[0].model_configs.map(config => config.failover_max_retries), [7, 7])
})

test('persisted failover metadata merges without overwriting provider drafts', () => {
  const drafts = [{
    var_name: 'native_oai_config_main',
    apibase: 'https://draft.example',
    apikey: 'draft-secret',
    model_configs: [
      { model: 'alpha-renamed', stream: false, failover_order: 9, failover_spring_back: 9 },
      { model: 'beta', stream: true, failover_order: 8 },
    ],
  }]
  const persisted = [{
    var_name: 'native_oai_config_main',
    apibase: 'https://persisted.example',
    apikey: 'persisted-secret',
    model_configs: [
      { model: 'alpha', failover_order: 1, failover_max_retries: 6, failover_base_delay: 0.75 },
      { model: 'beta' },
    ],
  }]

  const merged = mergePersistedFailoverConfig(drafts, persisted)
  assert.equal(merged[0].apibase, 'https://draft.example')
  assert.equal(merged[0].apikey, 'draft-secret')
  assert.equal(merged[0].model_configs[0].model, 'alpha-renamed')
  assert.equal(merged[0].model_configs[0].stream, false)
  assert.equal(merged[0].model_configs[0].failover_order, 1)
  assert.equal(merged[0].model_configs[0].failover_max_retries, 6)
  assert.equal(merged[0].model_configs[0].failover_base_delay, 0.75)
  assert.equal(merged[0].model_configs[0].failover_spring_back, undefined)
  assert.equal(merged[0].model_configs[1].failover_order, undefined)
})
