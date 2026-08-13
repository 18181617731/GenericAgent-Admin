// Copy for the codes /api/chat/state reports when the chat model list comes back
// empty. The backend hint stays Chinese-only because it also serves logs and API
// clients; the UI owns the bilingual wording and the concrete next step, since a
// bare "no models found" is what used to send first-time users into the logs.

const TITLES = {
  missing_python_module: ['缺少 Python 依赖，读不到模型', 'A Python dependency is missing, so no models load'],
  ga_root_unusable: ['GA 目录未配置或不可用', 'The GA directory is missing or unusable'],
  python_unusable: ['Python 解释器无法启动', 'The Python interpreter cannot be started'],
  no_models_configured: ['GA 里还没有配置模型', 'GA has no models configured yet'],
  unknown: ['读取模型列表失败', 'Reading the model list failed'],
}

export const modelDiagnosisTitle = (diagnosis, translate) => {
  const [zh, en] = TITLES[diagnosis?.code] || TITLES.unknown
  return translate(zh, en)
}

export const modelDiagnosisAdvice = (diagnosis, translate) => {
  if (!diagnosis) return ''
  const packages = (diagnosis.install_packages || []).join(', ')
  const python = diagnosis.python || 'python'
  const root = diagnosis.ga_root || ''
  switch (diagnosis.code) {
    case 'missing_python_module':
      return translate(
        `当前使用的 Python（${python}）缺少 ${packages}，GA 因此列不出任何模型。点“一键安装依赖”即可修复，无需重启。`,
        `The Python in use (${python}) is missing ${packages}, so GA cannot list any model. Install dependencies fixes it without a restart.`,
      )
    case 'ga_root_unusable':
      return root
        ? translate(
          `GA 目录 ${root} 里找不到 agentmain.py。请到「设置」重新指定 GA 目录。`,
          `${root} does not contain agentmain.py. Point the GA directory at a real GenericAgent checkout in Settings.`,
        )
        : translate(
          '还没有配置 GA 目录。请到「设置」完成首次安装向导。',
          'No GA directory is configured yet. Finish the first-run setup wizard in Settings.',
        )
    case 'python_unusable':
      return translate(
        `Python 解释器 ${python} 无法启动。请到「设置」重新选择 Python，或让向导创建虚拟环境。`,
        `Python interpreter ${python} cannot be started. Pick another interpreter in Settings, or let the wizard create a virtualenv.`,
      )
    case 'no_models_configured':
      return translate(
        'GA 已能正常启动，只是还没有配置任何模型。请到「模型」页面导入或填写服务商配置。',
        'GA starts fine but has no model configured. Import or fill in a provider on the Models page.',
      )
    default:
      return diagnosis.hint || translate(
        'GA 返回模型列表失败，展开下面的 Python 输出查看原因。',
        'GA failed to return the model list; expand the Python output below for the reason.',
      )
  }
}
