// First-run setup logic, kept apart from the wizard component so the step model
// can be unit tested without a DOM.
//
// The guiding rule: a step is only "done" when something outside this session
// says so. `/api/setup/state` is the authority for the GA root, its health and
// the venv, so those three survive a reload. Dependency installation and the
// smoke test leave no server-side trace, so their outcome is recorded per GA
// root in browser storage instead of living in React state, where a refresh used
// to silently reset the wizard to an earlier step.

export const SETUP_STEP_KEYS = ['root', 'venv', 'deps', 'smoke', 'finish']

const EVIDENCE_STORE_KEY = 'ga-admin-setup-evidence'

const toolNamed = (tools, name) => (Array.isArray(tools) ? tools : []).find(tool => tool?.name === name) || { name, ok: false }

export const normalizeRootPath = (value) => String(value ?? '').trim().replace(/\\/g, '/').replace(/(?!^)\/+$/, '')

export const samePath = (a, b) => normalizeRootPath(a).toLowerCase() === normalizeRootPath(b).toLowerCase()

/** Reshape `/api/setup/env` into the flat, camel-cased view the wizard renders. */
export function normalizeSetupEnv(payload = {}) {
  const tools = Array.isArray(payload.tools) ? payload.tools : []
  return {
    ok: payload.ok === true,
    error: String(payload.error || ''),
    checked: String(payload.checked || ''),
    python: toolNamed(tools, 'python'),
    git: toolNamed(tools, 'git'),
    uv: toolNamed(tools, 'uv'),
    npm: toolNamed(tools, 'npm'),
    gitRequired: payload.git_required === true,
    archiveFallback: payload.archive_fallback !== false,
    canInstallPython: payload.python_installer === true,
    configuredPython: String(payload.configured_python || ''),
    effectivePython: String(payload.effective_python || ''),
  }
}

/**
 * The environment probe is advisory: the wizard still works when it fails, so a
 * rejected request becomes a normal env object carrying the reason.
 */
export function setupEnvFailure(error, fallbackMessage) {
  return normalizeSetupEnv({ ok: false, error: error?.message || String(error || '') || fallbackMessage, tools: [] })
}

const readStore = (storage) => {
  try {
    const parsed = JSON.parse(storage?.getItem(EVIDENCE_STORE_KEY) || '{}')
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

/** Evidence is keyed by GA root so pointing the wizard at a new root starts clean. */
export function readSetupEvidence(storage, root) {
  const key = normalizeRootPath(root).toLowerCase()
  if (!key) return {}
  const entry = readStore(storage)[key]
  return entry && typeof entry === 'object' ? entry : {}
}

export function writeSetupEvidence(storage, root, patch = {}) {
  const key = normalizeRootPath(root).toLowerCase()
  if (!key) return {}
  const store = readStore(storage)
  const merged = { ...(store[key] || {}), ...patch }
  try {
    storage?.setItem(EVIDENCE_STORE_KEY, JSON.stringify({ ...store, [key]: merged }))
  } catch {
    // Private-mode storage refuses writes; the wizard degrades to session-only
    // evidence rather than failing the step the user just completed.
  }
  return merged
}

/**
 * Collapse the server snapshot, stored evidence and the live smoke result into
 * the flags every other part of the wizard reads.
 */
export function setupProgress({ state = null, evidence = {}, smoke = null } = {}) {
  const savedRoot = normalizeRootPath(state?.ga_root)
  // The wizard only appears when GA health is failing, so a configured root is
  // not enough on its own -- it has to actually pass the health check.
  const rootReady = Boolean(savedRoot) && state?.health?.ok === true
  const venvReady = rootReady && state?.venv?.ok === true
  const smokeMatchesRoot = Boolean(smoke) && (!smoke.root || samePath(smoke.root, savedRoot))
  const smokePassed = smokeMatchesRoot ? smoke.ok === true : evidence.smoke === true
  return {
    loaded: Boolean(state),
    savedRoot,
    rootReady,
    venvReady,
    depsReady: rootReady && evidence.deps === true,
    smokeReady: rootReady && smokePassed,
    smokeFailed: smokeMatchesRoot && smoke.ok !== true,
    bootstrapDone: state?.bootstrap_done === true,
    health: state?.health || null,
    venv: state?.venv || null,
    python: String(state?.python || ''),
    // Completing needs a healthy root plus a usable interpreter, which is either
    // the venv the server can see or a smoke test that proved the configured
    // Python runs. This mirrors what /api/setup/complete itself accepts.
    canFinish: rootReady && (venvReady || smokePassed),
  }
}

/** Reason the finish action is unavailable, as a key the copy layer resolves. */
export function finishBlockReason(progress) {
  if (!progress?.loaded) return 'loading'
  if (progress.bootstrapDone) return null
  if (!progress.savedRoot) return 'noRoot'
  if (!progress.rootReady) return 'unhealthyRoot'
  if (!progress.canFinish) return 'noInterpreter'
  return null
}

/**
 * Step status for antd's Steps. Exactly one step is `process` unless everything
 * is done, and a failed smoke test surfaces as `error` on its own step rather
 * than dragging the whole wizard backwards.
 */
export function setupSteps(progress) {
  const done = {
    root: Boolean(progress?.rootReady),
    venv: Boolean(progress?.venvReady),
    deps: Boolean(progress?.depsReady),
    smoke: Boolean(progress?.smokeReady),
    finish: Boolean(progress?.bootstrapDone),
  }
  const currentKey = SETUP_STEP_KEYS.find(key => !done[key])
  return SETUP_STEP_KEYS.map(key => ({
    key,
    done: done[key],
    status: done[key]
      ? 'finish'
      : key === 'smoke' && progress?.smokeFailed
        ? 'error'
        : key === currentKey
          ? 'process'
          : 'wait',
  }))
}

export function setupCurrentStep(steps) {
  const index = (steps || []).findIndex(step => step.status === 'process' || step.status === 'error')
  return index === -1 ? Math.max(0, (steps || []).length - 1) : index
}

/** Directory the install action creates, mirroring the server's own join. */
export function installTargetPath(parent) {
  const trimmed = String(parent ?? '').trim().replace(/[\\/]+$/, '')
  if (!trimmed) return ''
  return `${trimmed}${trimmed.includes('\\') ? '\\' : '/'}GenericAgent`
}

export function statusTone(progress) {
  if (!progress?.loaded) return 'loading'
  if (progress.bootstrapDone) return 'done'
  if (progress.canFinish) return 'ready'
  if (progress.rootReady) return 'configuring'
  return 'fresh'
}
