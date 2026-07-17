import { RotateCcw, Save } from 'lucide-react'
import { Panel } from '../components/common'

const update = (setConfig, config, field) => event => setConfig({ ...config, [field]: event.target.value })

export function SettingsPage({ t, root, setRoot, config, setConfig, dirty, busy, onSave, onReset }) {
  const proxyMode = config?.proxy_mode || 'off'
  return <section className="settings-page">
    <Panel title={t.nav.settings} className="settings-panel">
      <form className="settings-form" onSubmit={event => { event.preventDefault(); if (dirty && !busy) onSave() }}>
        <section className="settings-section" aria-labelledby="settings-runtime-title">
          <div className="settings-section-head"><h3 id="settings-runtime-title">运行环境</h3><p>配置 GenericAgent 位置及执行环境。</p></div>
          <label className="settings-field" htmlFor="settings-ga-root"><span>{t.root}</span><input id="settings-ga-root" value={root} onChange={event => setRoot(event.target.value)}/></label>
          <label className="settings-field" htmlFor="settings-python"><span>{t.fields.pythonPath}</span><input id="settings-python" value={config?.python_path || ''} onChange={update(setConfig, config, 'python_path')} placeholder={t.fields.pythonAuto}/></label>
          <label className="settings-field" htmlFor="settings-chat-dir"><span>{t.fields.chatDataDir}</span><input id="settings-chat-dir" value={config?.chat_data_dir || ''} onChange={update(setConfig, config, 'chat_data_dir')} placeholder={t.fields.chatDataAuto}/></label>
        </section>

        <section className="settings-section" aria-labelledby="settings-network-title">
          <div className="settings-section-head"><h3 id="settings-network-title">Chat 网络代理</h3><p>仅影响 Chat Python Worker 发出的网络请求。</p></div>
          <label className="settings-field" htmlFor="settings-proxy-mode"><span>代理模式</span><select id="settings-proxy-mode" value={proxyMode} onChange={update(setConfig, config, 'proxy_mode')}><option value="off">关闭</option><option value="system">跟随系统</option><option value="custom">自定义</option></select></label>
          {proxyMode === 'custom' && <div className="settings-proxy-grid">
            <label className="settings-field" htmlFor="settings-http-proxy"><span>HTTP_PROXY</span><input id="settings-http-proxy" value={config?.http_proxy || ''} onChange={update(setConfig, config, 'http_proxy')} placeholder="http://127.0.0.1:7890"/></label>
            <label className="settings-field" htmlFor="settings-https-proxy"><span>HTTPS_PROXY</span><input id="settings-https-proxy" value={config?.https_proxy || ''} onChange={update(setConfig, config, 'https_proxy')} placeholder="http://127.0.0.1:7890"/></label>
            <label className="settings-field" htmlFor="settings-all-proxy"><span>ALL_PROXY</span><input id="settings-all-proxy" value={config?.all_proxy || ''} onChange={update(setConfig, config, 'all_proxy')} placeholder="socks5://127.0.0.1:7890"/></label>
            <label className="settings-field" htmlFor="settings-no-proxy"><span>NO_PROXY</span><input id="settings-no-proxy" value={config?.no_proxy || ''} onChange={update(setConfig, config, 'no_proxy')} placeholder="localhost,127.0.0.1"/></label>
          </div>}
        </section>

        <div className="settings-save-bar">
          <span className={dirty ? 'status-pill warn' : 'status-pill ok'}>{dirty ? '有未保存更改' : '配置已同步'}</span>
          <div className="actions"><button type="button" onClick={onReset} disabled={!dirty || busy}><RotateCcw size={15}/>放弃更改</button><button type="submit" className="primary" disabled={!dirty || busy}><Save size={15}/>{busy ? t.busy : '保存全部配置'}</button></div>
        </div>
      </form>

      <section className="settings-command-list" aria-labelledby="settings-commands-title">
        <h3 id="settings-commands-title">斜杠命令列表</h3>
        <p className="muted">在独立的 Chat 页面管理命令；此处显示当前已配置项。</p>
        {(config?.slash_commands || []).length > 0 ? config.slash_commands.map((item, index) => <div key={`${item.cmd}-${index}`} className="cfg-slash-row"><code>{item.cmd}</code><span className="muted">{item.desc}</span></div>) : <p className="muted">暂无配置命令</p>}
      </section>
    </Panel>
  </section>
}
