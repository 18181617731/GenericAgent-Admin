import test from 'node:test'
import assert from 'node:assert/strict'
import {
  nextProviderVarName,
  providerDisplayName,
  providerVarNameFromDisplayName,
  providerVarNameOnProtocolChange,
  suggestedProviderVarOnProtocolChange,
} from './modelsProvider.js'

test('nextProviderVarName creates a unique provider variable from the protocol prefix', () => {
  const profiles = [
    { var_name: 'native_oai_config1' },
    { var_name: 'native_oai_config2' },
  ]

  assert.equal(nextProviderVarName('native_oai_config', profiles), 'native_oai_config3')
})

test('providerVarNameOnProtocolChange replaces an official prefix and preserves its suffix', () => {
  const profiles = [
    { var_name: 'native_oai_config_gpt55_medium' },
    { var_name: 'native_claude_config_gpt55_medium' },
  ]

  assert.equal(
    providerVarNameOnProtocolChange(
      'native_oai_config_gpt55_medium',
      'native_claude_config',
      profiles,
      0,
    ),
    'native_claude_config_gpt55_medium_2',
  )
  assert.equal(
    providerVarNameOnProtocolChange('native_oai_config7', 'claude_config', profiles, 0),
    'claude_config7',
  )
})

test('providerVarNameOnProtocolChange preserves a custom configuration variable name', () => {
  assert.equal(
    providerVarNameOnProtocolChange('acme_api', 'native_claude_config', [], 0),
    'acme_api',
  )
})

test('providerDisplayName hides official protocol prefixes', () => {
  assert.equal(providerDisplayName('native_oai_config_gpt55_medium'), 'gpt55_medium')
  assert.equal(providerDisplayName('native_claude_config1'), '1')
  assert.equal(providerDisplayName('acme_api'), 'acme_api')
})

test('providerVarNameFromDisplayName keeps the protocol prefix internal', () => {
  assert.equal(
    providerVarNameFromDisplayName('gpt55_large', 'native_oai_config', 'native_oai_config_gpt55_medium'),
    'native_oai_config_gpt55_large',
  )
  assert.equal(
    providerVarNameFromDisplayName('2', 'native_claude_config', 'native_claude_config1'),
    'native_claude_config2',
  )
  assert.equal(
    providerVarNameFromDisplayName('acme_api', 'native_oai_config', 'acme_api'),
    'acme_api',
  )
  assert.equal(
    providerVarNameFromDisplayName('new_vendor', 'native_oai_config', 'native_oai_config'),
    'native_oai_config_new_vendor',
  )
  assert.equal(
    providerVarNameFromDisplayName('', 'native_oai_config', 'native_oai_config_new_vendor'),
    'native_oai_config',
  )
})

test('protocol changes preserve the visible provider name while replacing its internal prefix', () => {
  const changed = providerVarNameOnProtocolChange(
    'native_oai_config_gpt55_medium',
    'native_claude_config',
    [],
  )
  assert.equal(changed, 'native_claude_config_gpt55_medium')
  assert.equal(providerDisplayName(changed), 'gpt55_medium')
})
