import test from 'node:test'
import assert from 'node:assert/strict'
import {
  SETUP_STEP_KEYS,
  finishBlockReason,
  installTargetPath,
  normalizeSetupEnv,
  readSetupEvidence,
  setupCurrentStep,
  setupEnvFailure,
  setupProgress,
  setupSteps,
  statusTone,
  writeSetupEvidence,
} from './setupWizard.js'
import { SETUP_TEXT } from './i18n.js'

const memoryStorage = (seed = {}) => {
  const data = { ...seed }
  return {
    getItem: key => (key in data ? data[key] : null),
    setItem: (key, value) => { data[key] = String(value) },
  }
}

const healthyState = (extra = {}) => ({
  ok: true,
  bootstrap_done: false,
  ga_root: 'C:/ga',
  python: 'C:/ga/.venv/Scripts/python.exe',
  health: { ok: true },
  venv: { ok: true, path: 'C:/ga/.venv', python: 'C:/ga/.venv/Scripts/python.exe' },
  ...extra,
})

const statusOf = (progress) => Object.fromEntries(setupSteps(progress).map(step => [step.key, step.status]))

test('env normalization lifts every probed tool out of the tools array', () => {
  const env = normalizeSetupEnv({
    ok: true,
    tools: [
      { name: 'git', ok: true, version: 'git version 2.44.0' },
      { name: 'python', ok: true, version: 'Python 3.12.1' },
      { name: 'uv', ok: false, error: 'not found' },
      { name: 'npm', ok: true, version: '10.5.0' },
    ],
    python_installer: true,
    effective_python: 'C:/py/python.exe',
  })
  assert.equal(env.git.version, 'git version 2.44.0')
  assert.equal(env.python.ok, true)
  assert.equal(env.uv.ok, false)
  assert.equal(env.npm.version, '10.5.0')
  assert.equal(env.canInstallPython, true)
  assert.equal(env.effectivePython, 'C:/py/python.exe')
})

test('env normalization reports missing tools instead of undefined', () => {
  const env = normalizeSetupEnv({})
  assert.deepEqual(env.python, { name: 'python', ok: false })
  assert.equal(env.archiveFallback, true)
  assert.equal(env.gitRequired, false)
})

test('a failed env probe still yields a renderable env carrying the reason', () => {
  const env = setupEnvFailure(new Error('offline'), 'fallback')
  assert.equal(env.ok, false)
  assert.equal(env.error, 'offline')
  assert.equal(env.python.ok, false)
  assert.equal(setupEnvFailure(null, 'fallback').error, 'fallback')
})

test('an env is only "probed" once the server has answered, so the placeholder claims nothing', () => {
  assert.equal(normalizeSetupEnv({}).probed, false)
  assert.equal(normalizeSetupEnv({ checked: '2026-08-14T09:01:09+08:00', tools: [] }).probed, true)
  // A probe that failed has still looked at the machine, so the wizard reports
  // the reason rather than sitting on "checking" forever.
  assert.equal(setupEnvFailure(new Error('offline'), 'fallback').probed, true)
})

test('a configured root that fails its health check is not ready', () => {
  const progress = setupProgress({ state: { ga_root: 'C:/ga', health: { ok: false }, venv: { ok: true } } })
  assert.equal(progress.rootReady, false)
  assert.equal(progress.venvReady, false)
  assert.equal(progress.canFinish, false)
  assert.equal(finishBlockReason(progress), 'unhealthyRoot')
})

test('an unconfigured root blocks finishing and starts on the first step', () => {
  const progress = setupProgress({ state: { ok: true, ga_root: '' } })
  assert.equal(finishBlockReason(progress), 'noRoot')
  assert.equal(statusOf(progress).root, 'process')
  assert.equal(statusTone(progress), 'fresh')
})

test('a healthy root with a server-visible venv can finish', () => {
  const progress = setupProgress({ state: healthyState() })
  assert.equal(progress.venvReady, true)
  assert.equal(progress.canFinish, true)
  assert.equal(finishBlockReason(progress), null)
  assert.equal(statusTone(progress), 'ready')
})

test('a passing smoke test substitutes for a missing venv', () => {
  const state = healthyState({ venv: { ok: false, path: 'C:/ga/.venv' } })
  const blocked = setupProgress({ state })
  assert.equal(blocked.canFinish, false)
  assert.equal(finishBlockReason(blocked), 'noInterpreter')

  const proven = setupProgress({ state, smoke: { ok: true, root: 'C:/ga', python: 'C:/py/python.exe' } })
  assert.equal(proven.canFinish, true)
  assert.equal(proven.smokeReady, true)
})

// The old wizard kept the smoke result in React state only, so reloading the
// page disabled the finish button until the user ran it again.
test('stored evidence keeps completed steps done across a reload', () => {
  const state = healthyState({ venv: { ok: false, path: 'C:/ga/.venv' } })
  const progress = setupProgress({ state, evidence: { deps: true, smoke: true } })
  assert.equal(progress.smokeReady, true)
  assert.equal(progress.depsReady, true)
  assert.equal(progress.canFinish, true)
})

test('a smoke result recorded against another root is ignored', () => {
  const progress = setupProgress({ state: healthyState(), smoke: { ok: true, root: 'D:/other' } })
  assert.equal(progress.smokeReady, false)
  assert.equal(progress.smokeFailed, false)
})

test('root matching ignores separator and case differences', () => {
  const progress = setupProgress({ state: healthyState(), smoke: { ok: true, root: 'c:\\GA\\' } })
  assert.equal(progress.smokeReady, true)
})

test('exactly one step is current', () => {
  const progress = setupProgress({ state: healthyState() })
  const steps = setupSteps(progress)
  assert.deepEqual(steps.map(step => step.key), SETUP_STEP_KEYS)
  assert.equal(steps.filter(step => step.status === 'process').length, 1)
  assert.equal(statusOf(progress).deps, 'process')
})

test('recording dependencies advances the current step to the smoke test', () => {
  const progress = setupProgress({ state: healthyState(), evidence: { deps: true } })
  const status = statusOf(progress)
  assert.equal(status.venv, 'finish')
  assert.equal(status.deps, 'finish')
  assert.equal(status.smoke, 'process')
  assert.equal(setupCurrentStep(setupSteps(progress)), 3)
})

test('a failed smoke test marks its own step without rewinding the wizard', () => {
  const progress = setupProgress({
    state: healthyState(),
    evidence: { deps: true },
    smoke: { ok: false, root: 'C:/ga', error: 'boom' },
  })
  const status = statusOf(progress)
  assert.equal(status.smoke, 'error')
  assert.equal(status.venv, 'finish')
  assert.equal(status.deps, 'finish')
  assert.equal(setupCurrentStep(setupSteps(progress)), 3)
})

test('a completed bootstrap finishes every step and rests on the last one', () => {
  const progress = setupProgress({ state: healthyState({ bootstrap_done: true }), evidence: { deps: true, smoke: true } })
  const steps = setupSteps(progress)
  assert.ok(steps.every(step => step.status === 'finish'))
  assert.equal(setupCurrentStep(steps), SETUP_STEP_KEYS.length - 1)
  assert.equal(statusTone(progress), 'done')
})

test('an unloaded state waits on the first step', () => {
  const progress = setupProgress({})
  assert.equal(statusOf(progress).root, 'process')
  assert.equal(finishBlockReason(progress), 'loading')
  assert.equal(statusTone(progress), 'loading')
})

test('evidence round-trips per GA root', () => {
  const storage = memoryStorage()
  writeSetupEvidence(storage, 'C:/ga', { deps: true })
  writeSetupEvidence(storage, 'C:/ga', { smoke: true })
  writeSetupEvidence(storage, 'D:/other', { deps: true })
  assert.deepEqual(readSetupEvidence(storage, 'C:\\ga'), { deps: true, smoke: true })
  assert.deepEqual(readSetupEvidence(storage, 'D:/other'), { deps: true })
  assert.deepEqual(readSetupEvidence(storage, 'E:/unknown'), {})
})

test('evidence survives unreadable and unwritable storage', () => {
  assert.deepEqual(readSetupEvidence(memoryStorage({ 'ga-admin-setup-evidence': '{not json' }), 'C:/ga'), {})
  const readOnly = { getItem: () => '{}', setItem: () => { throw new Error('denied') } }
  assert.deepEqual(writeSetupEvidence(readOnly, 'C:/ga', { deps: true }), { deps: true })
  assert.deepEqual(readSetupEvidence(null, 'C:/ga'), {})
})

test('evidence ignores an empty root', () => {
  const storage = memoryStorage()
  assert.deepEqual(writeSetupEvidence(storage, '', { deps: true }), {})
  assert.deepEqual(readSetupEvidence(storage, ''), {})
})

test('the install target appends GenericAgent using the separator already in the path', () => {
  assert.equal(installTargetPath('C:\\Users\\me\\code'), 'C:\\Users\\me\\code\\GenericAgent')
  assert.equal(installTargetPath('/home/me/code/'), '/home/me/code/GenericAgent')
  assert.equal(installTargetPath('  '), '')
})

test('both languages define copy for every step, status and block reason', () => {
  for (const lang of ['zh', 'en']) {
    const copy = SETUP_TEXT[lang]
    assert.ok(copy, `missing SETUP_TEXT for ${lang}`)
    for (const key of SETUP_STEP_KEYS) {
      assert.ok(copy.steps[key]?.title, `${lang} is missing a title for step ${key}`)
      assert.ok(copy.steps[key]?.desc, `${lang} is missing a description for step ${key}`)
    }
    for (const tone of ['loading', 'fresh', 'configuring', 'ready', 'done']) {
      assert.ok(copy.statusLabels[tone], `${lang} is missing a status label for ${tone}`)
    }
    for (const reason of ['loading', 'noRoot', 'unhealthyRoot', 'noInterpreter']) {
      assert.ok(copy.runtime.blocked[reason], `${lang} is missing a block reason for ${reason}`)
    }
  }
})

// The wizard promised `pip install -r requirements.txt` while the backend ran
// `pip install -e .`, so the copy is pinned to what actually executes.
test('dependency copy names the command the backend actually runs', () => {
  for (const lang of ['zh', 'en']) {
    const copy = SETUP_TEXT[lang]
    assert.match(copy.steps.deps.desc, /pip install -e \./)
    assert.match(copy.confirm.deps('C:/ga', 'python'), /python -m pip install -e \./)
    assert.doesNotMatch(copy.steps.deps.desc, /requirements\.txt/)
  }
})
