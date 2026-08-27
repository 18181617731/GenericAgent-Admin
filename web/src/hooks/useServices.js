import { useMemo, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { group } from '../lib/format'

// Owns the GA process list plus every mutating service action. Each action is
// confirm-gated and refreshes only the service list, so a start/stop never has
// to re-run the whole workspace boot sequence.
export function useServices({ t, setMsg, setBusy }) {
  const [services, setServices] = useState([])
  const [actionStates, setActionStates] = useState({})
  const [llms, setLLMs] = useState([])
  const [pickerOpen, setPickerOpen] = useState(false)
  const [reflectLLMNo, setReflectLLMNo] = useState('')
  const [pendingServiceName, setPendingServiceName] = useState('')

  const refresh = async () => {
    const payload = await api('/api/services')
    const list = Array.isArray(payload) ? payload : (payload.services || [])
    setServices(list)
    return list
  }

  const loadLLMs = async () => {
    try {
      const d = await api('/api/chat/state')
      setLLMs(d.llms || [])
    } catch (e) {
      console.error('加载模型列表失败:', e)
    }
  }

  const serviceAction = async (name, action, params = null) => {
    if (!await confirmDanger(`service-${action}`, t.service.confirmAction(action, name))) return false
    setActionStates(current => ({ ...current, [name]: { status: 'pending', action, message: t.service.pending(action, name) } }))
    try {
      const body = { name }
      if (params) body.params = params
      await api(`/api/services/${action}`, { dangerous:true, method:'POST', body: JSON.stringify(body) })
      await refresh()
      setActionStates(current => ({ ...current, [name]: { status: 'success', action, message: t.service.success(action, name) } }))
      return true
    } catch (e) {
      setActionStates(current => ({ ...current, [name]: { status: 'error', action, message: t.service.failed(action, name, e.message) } }))
      return false
    }
  }

  const toggleAutostart = async (name, enabled) => {
    if (!await confirmDanger('service-autostart', t.service.autostartConfirm(name, enabled))) return
    setBusy(true)
    try {
      const d = await api('/api/services/autostart', { dangerous:true, method:'POST', body: JSON.stringify({ name, enabled }) })
      setServices(d.services || [])
      setMsg(enabled ? t.enabled : t.disabled)
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const setServiceModel = async (name, llm_no) => {
    if (!await confirmDanger('service-model', t.service.modelConfirm(name))) return
    setBusy(true)
    try {
      const d = await api('/api/services/model', { dangerous:true, method:'POST', body: JSON.stringify({ name, llm_no }) })
      setServices(d.services || [])
      setMsg(t.service.modelUpdated)
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const startReflectService = (name) => {
    const fallbackModel = llms.find(m => m?.index !== undefined && m?.index !== null)
    setReflectLLMNo(current => current !== '' ? current : (fallbackModel?.index?.toString() || '0'))
    setPendingServiceName(name)
    setPickerOpen(true)
  }

  const confirmReflectStart = async () => {
    const selectedLLMNo = String(reflectLLMNo || '').trim()
    if (!/^\d+$/.test(selectedLLMNo)) {
      setMsg(t.service.invalidModel)
      return
    }
    setPickerOpen(false)
    await serviceAction(pendingServiceName, 'start', { llm_no: selectedLLMNo })
    setPendingServiceName('')
  }

  const taskSvcs = useMemo(() => group(services, s => s.kind === 'task' || s.name?.includes('task') || s.name?.includes('scheduler')), [services])
  const frontendSvcs = useMemo(() => group(services, s => s.kind === 'frontend'), [services])
  const reflectSvcs = useMemo(() => group(services, s => s.name?.includes('scheduler') || s.name?.includes('autonomous')), [services])

  return {
    services, setServices, refresh,
    llms, loadLLMs,
    actionStates,
    serviceAction, toggleAutostart, setServiceModel,
    startReflectService, confirmReflectStart,
    pickerOpen, closePicker: () => setPickerOpen(false),
    reflectLLMNo, setReflectLLMNo, pendingServiceName,
    taskSvcs, frontendSvcs, reflectSvcs,
  }
}
