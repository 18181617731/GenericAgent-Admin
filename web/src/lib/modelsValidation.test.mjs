import test from 'node:test'
import assert from 'node:assert/strict'
import { validateModelProfiles, modelValidationSummary, modelRiskCatalog } from './modelsValidation.js'

test('validateModelProfiles blocks invalid saves before API calls', () => {
  const results = validateModelProfiles([
    { var_name: 'MODEL_1', name: 'model one', model: 'gpt-4o', apibase: 'https://api.example.test', max_retries: 2, read_timeout: 300 },
    { var_name: 'api_config_dup', name: 'dup one', model: 'gpt-4o', apibase: 'https://api.example.test', apikey: 'set' },
    { var_name: 'api_config_dup', name: 'dup two', model: '', apibase: 'localhost:8000', max_retries: -1, read_timeout: 0 }
  ])

  assert.equal(results[0].ok, false)
  assert.match(results[0].errors.join(','), /varNameDiscoveryToken/)
  assert.deepEqual(results[0].warnings, ['apiKeyEmpty'])
  assert.equal(results[1].ok, false)
  assert.match(results[1].errors.join(','), /varNameDuplicate/)
  assert.equal(results[2].ok, false)
  assert.match(results[2].errors.join(','), /varNameDuplicate/)
  assert.match(results[2].errors.join(','), /modelRequired/)
  assert.match(results[2].errors.join(','), /maxRetriesInvalid/)
  assert.match(results[2].errors.join(','), /readTimeoutInvalid/)
  assert.match(results[2].warnings.join(','), /apiBaseProtocol/)
})

test('validateModelProfiles mirrors backend required API base contract', () => {
  const results = validateModelProfiles([
    { var_name: 'api_config_valid', name: 'valid', model: 'gpt-4o', apibase: '', apikey: 'set' }
  ])

  assert.equal(results[0].ok, false)
  assert.deepEqual(results[0].errors, ['apiBaseRequired'])
})

test('modelValidationSummary marks valid profile sets ready', () => {
  const summary = modelValidationSummary(validateModelProfiles([
    { var_name: 'api_config_a', name: 'main', model: 'gpt-4o-mini', apibase: 'http://127.0.0.1:8000', apikey: 'set', max_retries: 0, read_timeout: 1 }
  ]))

  assert.deepEqual(summary, { errors: 0, warnings: 0, ready: true })
})


test('modelRiskCatalog reports null and empty catalogs without claiming safety', () => {
  assert.deepEqual(modelRiskCatalog(null), {
    status: 'empty',
    error: '',
    items: [],
    writeRoutes: ['/api/models/export', '/api/models/import-mykey'],
    confirmedWriteRoutes: [],
    missingConfirmedWriteRoutes: ['/api/models/export', '/api/models/import-mykey'],
  })
  assert.equal(modelRiskCatalog({ items: [] }).status, 'empty')
})

test('modelRiskCatalog isolates model entries and confirmed write guards', () => {
  const catalog = modelRiskCatalog({ items: [
    { route: '/api/health', method: 'GET', level: 'safe' },
    { route: '/api/models/preview', method: 'POST', level: 'review', action: 'preview only' },
    { route: '/api/models/export', method: 'POST', level: 'dangerous', reason: 'confirmDanger required before write' },
  ] })
  assert.equal(catalog.status, 'ready')
  assert.deepEqual(catalog.items.map(item => item.route), ['/api/models/preview', '/api/models/export'])
  assert.deepEqual(catalog.confirmedWriteRoutes, ['/api/models/export'])
  assert.deepEqual(catalog.missingConfirmedWriteRoutes, ['/api/models/import-mykey'])
})

test('modelRiskCatalog preserves catalog load errors for the UI', () => {
  const catalog = modelRiskCatalog([], 'risk endpoint failed')
  assert.equal(catalog.status, 'error')
  assert.equal(catalog.error, 'risk endpoint failed')
  assert.deepEqual(catalog.items, [])
})

test('validateModelProfiles treats null or empty profile arrays as not ready instead of stale success', () => {
  assert.deepEqual(validateModelProfiles(null), [])
  assert.deepEqual(modelValidationSummary(validateModelProfiles([])), { errors: 0, warnings: 0, ready: false })
})

test('modelRiskCatalog ignores malformed stale entries and normalizes route aliases', () => {
  const catalog = modelRiskCatalog({ items: [
    null,
    { endpoint: '/api/models', method: '', risk: '', name: 'list models' },
    { url: '/api/models/import-mykey', method: 'post', risk: 'review', note: 'danger confirmation required' },
    { path: '/api/models-old/export', level: 'dangerous' },
  ] })

  assert.equal(catalog.status, 'ready')
  assert.deepEqual(catalog.items.map(item => [item.route, item.method, item.level]), [
    ['/api/models', 'GET', 'review'],
    ['/api/models/import-mykey', 'POST', 'review'],
  ])
  assert.deepEqual(catalog.confirmedWriteRoutes, ['/api/models/import-mykey'])
  assert.deepEqual(catalog.missingConfirmedWriteRoutes, ['/api/models/export'])
})
