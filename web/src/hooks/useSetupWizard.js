import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, apiStream } from '../lib/api.js'
import { confirmDanger } from '../lib/danger.js'
import {
  finishBlockReason,
  installTargetPath,
  normalizeRootPath,
  normalizeSetupEnv,
  readSetupEvidence,
  setupCurrentStep,
  setupEnvFailure,
  setupProgress,
  setupSteps,
  statusTone,
  writeSetupEvidence,
} from '../lib/setupWizard.js'

const LOG_LIMIT = 600

// Storage access itself throws when the browser blocks it, so every caller goes
// through here and the wizard falls back to session-only step evidence.
const evidenceStore = () => {
  try {
    return globalThis.localStorage ?? null
  } catch {
    return null
  }
}

async function consumeNdjson(res, onEvent, unsupportedMessage) {
  const reader = res.body?.getReader()
  if (!reader) throw new Error(unsupportedMessage)
  const decoder = new TextDecoder()
  let buffer = ''
  let completion = null
  const handle = (chunk) => {
    if (!chunk.trim()) return
    let event
    try {
      event = JSON.parse(chunk)
    } catch {
      return
    }
    onEvent(event)
    if (event.type === 'done') completion = event
  }
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    lines.forEach(handle)
  }
  handle(buffer)
  return completion
}

/**
 * Drives the first-run wizard: one loader, one action runner, and a progress
 * model derived from the server rather than from whatever this session happens
 * to remember.
 */
export function useSetupWizard({ text, initialRoot = '', onComplete } = {}) {
  const [state, setState] = useState(null)
  const [env, setEnv] = useState(() => normalizeSetupEnv({}))
  const [evidence, setEvidence] = useState({})
  const [smoke, setSmoke] = useState(null)
  const [rootDraft, setRootDraft] = useState(() => initialRoot || '')
  const [installDraft, setInstallDraft] = useState('')
  const [pythonDraft, setPythonDraft] = useState('')
  const [busy, setBusy] = useState('')
  const [notice, setNotice] = useState(null)
  const [logLines, setLogLines] = useState([])

  const reload = useCallback(async () => {
    const [next, nextEnv] = await Promise.all([
      api('/api/setup/state'),
      // The probe runs external binaries and is allowed to fail; the wizard is
      // still usable when it does, so its error becomes part of the env view.
      api('/api/setup/env').then(normalizeSetupEnv).catch(error => setupEnvFailure(error, text.env.probeFailed)),
    ])
    setState(next)
    setEnv(nextEnv)
    setEvidence(readSetupEvidence(evidenceStore(), next?.ga_root))
    setRootDraft(current => current || String(next?.ga_root || ''))
    setPythonDraft(current => current || String(nextEnv?.python?.path || ''))
    return next
  }, [text])

  useEffect(() => {
    reload().catch(error => setNotice({ tone: 'error', text: error.message }))
  }, [reload])

  const progress = useMemo(() => setupProgress({ state, evidence, smoke }), [state, evidence, smoke])
  const steps = useMemo(() => setupSteps(progress), [progress])

  // Only busy/notice bookkeeping lives here. Each action asks for its own
  // dangerous-action confirmation next to the request it guards, so the guard
  // stays visible at the call site instead of hiding behind this helper.
  const run = useCallback(async (key, action) => {
    setBusy(key)
    setNotice(null)
    try {
      return await action()
    } catch (error) {
      setNotice({ tone: 'error', text: error.message })
      return null
    } finally {
      setBusy('')
    }
  }, [])

  const reject = (message) => {
    setNotice({ tone: 'error', text: message })
    return null
  }

  const refresh = () => run('setup-refresh', async () => {
    await reload()
    setNotice({ tone: 'success', text: text.messages.envRefreshed })
  })

  // The picker is a native dialog on the machine running GA Admin, and the route
  // persists nothing, so it needs no dangerous-action confirmation.
  const browse = (target) => run(`setup-browse-${target}`, async () => {
    const seed = target === 'root' ? rootDraft : installDraft
    const result = await api('/api/setup/browse', { method: 'POST', body: JSON.stringify({ path: normalizeRootPath(seed) }) })
    if (result?.cancelled || !result?.path) {
      setNotice({ tone: 'info', text: text.messages.browseCancelled })
      return null
    }
    if (target === 'root') setRootDraft(result.path)
    else setInstallDraft(result.path)
    return result.path
  })

  const validateRoot = () => {
    const target = normalizeRootPath(rootDraft)
    if (!target) return reject(text.runtime.blocked.noRoot)
    if (!confirmDanger('setup-validate', text.confirm.validate(target))) return null
    return run('setup-validate', async () => {
      const result = await api('/api/setup/validate', { dangerous: true, method: 'POST', body: JSON.stringify({ path: target }) })
      await reload()
      setNotice(result?.ok
        ? { tone: 'success', text: text.messages.rootValidated }
        : { tone: 'error', text: text.messages.rootRejected })
      return result
    })
  }

  const installGA = () => {
    const parent = normalizeRootPath(installDraft)
    if (!parent) return reject(text.messages.needInstallParent)
    const source = env.git.ok ? text.root.sourceGit : text.root.sourceArchive
    if (!confirmDanger('setup-install', text.confirm.install(source, parent, installTargetPath(parent)))) return null
    return run('setup-install', async () => {
      const result = await api('/api/setup/install', { dangerous: true, method: 'POST', body: JSON.stringify({ path: parent }) })
      if (result?.root) setRootDraft(result.root)
      await reload()
      setNotice(result?.ok
        ? { tone: 'success', text: text.messages.installed(result.method) }
        : { tone: 'error', text: text.messages.installedUnhealthy })
      return result
    })
  }

  const validatePython = () => {
    const candidate = pythonDraft.trim()
    if (!candidate) return reject(text.env.pythonPathRequired)
    if (!confirmDanger('setup-python-validate', text.confirm.pythonPath(candidate))) return null
    return run('setup-python-validate', async () => {
      const result = await api('/api/setup/python/validate', {
        dangerous: true,
        method: 'POST',
        body: JSON.stringify({ path: candidate }),
      })
      setPythonDraft(String(result?.python || candidate))
      await reload()
      setNotice({ tone: 'success', text: text.messages.pythonPathSaved(result?.python || candidate, result?.version || '') })
      return result
    })
  }

  const installPython = () => {
    if (!env.canInstallPython) return reject(text.env.pythonInstallerUnavailable)
    if (!confirmDanger('setup-python-install', text.confirm.python)) return null
    return run('setup-python-install', async () => {
      const result = await api('/api/setup/python/install', { dangerous: true, method: 'POST', body: JSON.stringify({}) })
      await reload()
      setNotice({ tone: 'success', text: text.messages.pythonInstalled(result?.python || '') })
      return result
    })
  }

  const createVenv = () => {
    const target = progress.savedRoot
    if (!target) return reject(text.runtime.blocked.noRoot)
    if (!confirmDanger('setup-venv-create', text.confirm.venv(target))) return null
    return run('setup-venv-create', async () => {
      const result = await api('/api/setup/venv/create', { dangerous: true, method: 'POST', body: JSON.stringify({ root: target }) })
      await reload()
      setNotice({ tone: 'success', text: text.messages.venvCreated })
      return result
    })
  }

  const installDeps = () => {
    const target = progress.savedRoot
    if (!target) return reject(text.runtime.blocked.noRoot)
    if (!confirmDanger('setup-deps-install', text.confirm.deps(target, progress.python || 'python'))) return null
    return run('setup-deps-install', async () => {
      setLogLines([])
      setNotice({ tone: 'info', text: text.messages.depsRunning })
      const res = await apiStream('/api/setup/deps/install', { dangerous: true, method: 'POST', body: JSON.stringify({ root: target }) })
      const append = (line) => setLogLines(lines => [...lines, line].slice(-LOG_LIMIT))
      const completion = await consumeNdjson(res, event => {
        if (event.line) append(event.line)
        if (event.error) append(`ERROR: ${event.error}`)
      }, text.messages.streamUnsupported)
      if (!completion?.ok) throw new Error(completion?.error || text.messages.depsFailed)
      setEvidence(writeSetupEvidence(evidenceStore(), target, { deps: true }))
      await reload()
      setNotice({ tone: 'success', text: text.messages.depsDone })
      return completion
    })
  }

  const runSmoke = () => {
    const target = progress.savedRoot
    if (!target) return reject(text.runtime.blocked.noRoot)
    if (!confirmDanger('setup-smoke', text.confirm.smoke(target))) return null
    return run('setup-smoke', async () => {
      let result
      try {
        result = await api('/api/setup/smoke', { dangerous: true, method: 'POST', body: JSON.stringify({ root: target }) })
      } catch (error) {
        // Record the failure against this root so the smoke step reports it
        // instead of silently looking untouched.
        setSmoke({ ok: false, root: target, error: error.message })
        setEvidence(writeSetupEvidence(evidenceStore(), target, { smoke: false }))
        throw error
      }
      setSmoke({ ...result, root: result?.root || target })
      setEvidence(writeSetupEvidence(evidenceStore(), target, { smoke: result?.ok === true }))
      await reload()
      setNotice({ tone: 'success', text: text.messages.smokePassed(result?.python || '') })
      return result
    })
  }

  const finish = () => {
    const target = progress.savedRoot
    if (!target) return reject(text.runtime.blocked.noRoot)
    const question = progress.depsReady ? text.confirm.finish : text.confirm.finishUnverified
    if (!confirmDanger('setup-complete', question)) return null
    return run('setup-complete', async () => {
      const result = await api('/api/setup/complete', { dangerous: true, method: 'POST', body: JSON.stringify({ root: target }) })
      setNotice({ tone: 'success', text: text.messages.completing })
      onComplete?.(result)
      return result
    })
  }

  return {
    env,
    progress,
    steps,
    currentStep: setupCurrentStep(steps),
    statusTone: statusTone(progress),
    blockReason: finishBlockReason(progress),
    smoke,
    rootDraft,
    setRootDraft,
    installDraft,
    setInstallDraft,
    installTarget: installTargetPath(installDraft),
    pythonDraft,
    setPythonDraft,
    busy,
    notice,
    logLines,
    refresh,
    browse,
    validateRoot,
    installGA,
    validatePython,
    installPython,
    createVenv,
    installDeps,
    runSmoke,
    finish,
  }
}
