export const gitSyncPresentation = status => {
  if (!status) return { state: 'unchecked', label: '未检查', summary: '尚未检查 origin', canSync: false }
  if (status.error || status.fetch_error) {
    return { state: 'error', label: '检查失败', summary: status.error || status.fetch_error, canSync: false }
  }
  if (status.conflicts) {
    return { state: 'blocked', label: '存在冲突', summary: '请先人工解决未合并文件', canSync: false }
  }
  if (!status.remote_checked) {
    return { state: 'unchecked', label: '未检查', summary: '点击检查后获取 origin 最新状态', canSync: false }
  }
  if (status.tracking_matches_origin === false) {
    const target = status.expected_origin || 'origin/<当前分支>'
    return { state: 'blocked', label: '无法同步', summary: `当前分支必须跟踪 ${target}`, canSync: false }
  }
  if (Number(status.behind || 0) > 0) {
    const behind = Number(status.behind || 0)
    return { state: 'behind', label: `远端领先 ${behind}`, summary: `需要合并 origin 的 ${behind} 个提交`, canSync: status.strategy_available === true }
  }
  if (status.dirty || Number(status.ahead || 0) > 0) {
    const parts = []
    if (status.dirty) parts.push(`${Number(status.changed_files || 0)} 个本地变更`)
    if (Number(status.ahead || 0) > 0) parts.push(`${Number(status.ahead)} 个待推送提交`)
    return { state: 'pending', label: '待同步', summary: parts.join('，'), canSync: status.strategy_available === true }
  }
  if (status.synchronized) return { state: 'synced', label: '已同步', summary: '当前分支已与 origin 完全同步', canSync: false }
  return { state: 'blocked', label: '无法同步', summary: '当前分支未配置 origin 跟踪分支', canSync: false }
}
