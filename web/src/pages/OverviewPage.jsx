import React from 'react'
import { Activity, Download, GitPullRequest, RefreshCw, ShieldAlert } from 'lucide-react'
import { SettingFooter, SettingNote, SettingRow, SettingStat, SettingsPage, SettingsSection } from '../components/settings'

const countOf = (items) => Array.isArray(items) ? items.length : 0

export function OverviewPage({
  t, text, services, schedule, observability, observabilityError, onRefreshObservability, version, root,
}) {
  const copy = t.overview
  const { info, check, status, busy, gitBusy, gitResult, gitStatus } = version
  const updateMessage = status?.error || (status?.stage === 'queued' ? copy.updateQueued : (status?.message || status?.stage))
  const sourceState = gitStatus?.error
    ? copy.checkFailed
    : (gitStatus
        ? (gitStatus.upstream_configured === false
            ? copy.upstreamMissing
            : (gitStatus.latest ? copy.current : copy.sourceStatusBehind(gitStatus.behind || 0)))
        : copy.notChecked)
  const missingCore = observability?.missingCore || []

  return <SettingsPage>
    <SettingsSection
      title={copy.observability}
      description={text.about.statusDesc}
      icon={<Activity size={17}/>}
      actions={<button type="button" onClick={onRefreshObservability}><RefreshCw size={14}/>{t.refresh}</button>}
    >
      {observabilityError
        ? <p className="err-text">{observabilityError}</p>
        : <>
          <div className="set-stats">
            <SettingStat label={t.cards.processes} value={services.length}/>
            <SettingStat label={t.cards.running} value={services.filter(service => service.running).length}/>
            <SettingStat label={t.cards.schedule} value={schedule.task_count || 0}/>
            <SettingStat label={copy.healthChecks} value={countOf(observability?.checks)}/>
            <SettingStat label={copy.coreFiles} value={countOf(observability?.coreFiles?.filter?.(item => item?.exists) || [])}/>
            <SettingStat label={copy.riskRules} value={countOf(observability?.riskItems)}/>
          </div>
          <SettingRow label={root || copy.observability} hint={observability?.generatedAt ? `${copy.generatedAt}: ${observability.generatedAt}` : ''}>
            <span className={`set-state ${observability?.ok ? 'is-on' : 'is-off'}`}>
              {observability ? (observability.ok ? copy.healthy : copy.needsAttention) : copy.awaitingSnapshot}
            </span>
          </SettingRow>
          {missingCore.length > 0 && <SettingNote tone="warn">{copy.missingCore}: {missingCore.map(x => x.path || x.name).join(', ')}</SettingNote>}
          <SettingNote tone="muted" icon={<ShieldAlert size={14}/>}>{text.about.backupNote}</SettingNote>
        </>}
    </SettingsSection>

    <SettingsSection title={t.cards.version} description={text.about.releaseDesc} icon={<Download size={17}/>}>
      <SettingRow
        label={`GA Admin ${info?.version || 'dev'}`}
        hint={`${copy.commit} ${info?.commit || copy.unknown} · ${info?.date || copy.unknown} · ${copy.runtime} ${info?.runtime || '-'}`}
      >
        <span className={check?.update ? 'set-state is-off' : 'set-state is-on'}>
          {check ? (check.update ? copy.updateAvailable : copy.current) : (info?.goos ? `${info.goos}/${info.goarch}` : t.empty)}
        </span>
      </SettingRow>
      {info && !info.update_supported && <SettingNote tone="warn">{copy.updateUnavailable}: {info.update_unsupported_reason || copy.platformUnsupported}</SettingNote>}
      {check?.latest && <SettingRow label={copy.latestVersion}>
        <a href={check.latest.html_url} target="_blank" rel="noreferrer">{check.latest.tag_name}</a>
      </SettingRow>}
      {status?.stage && <div className="update-progress">
        <div className="update-progress-head">
          <span>{status.running ? copy.updateRunning : (status.error ? copy.updateFailed : copy.updateStatus)}</span>
          <b>{status.progress || 0}%</b>
        </div>
        <div className="progress-bar"><span style={{ width: `${Math.max(0, Math.min(100, status.progress || 0))}%` }}/></div>
        <p className={status.error ? 'err' : 'muted'}>{updateMessage}</p>
        <code>{status.stage}</code>
      </div>}
      <SettingFooter>
        <button type="button" onClick={version.checkVersion} disabled={busy || status?.running}>{busy ? t.busy : copy.checkUpdate}</button>
        <button className="primary" type="button" onClick={version.updateVersion} disabled={busy || status?.running || !check?.update}>
          {status?.running ? `${copy.updateRunning}…` : copy.oneClickUpdate}
        </button>
      </SettingFooter>
    </SettingsSection>

    <SettingsSection title={copy.sourceTitle} description={copy.sourceDescription} icon={<GitPullRequest size={17}/>}>
      <SettingRow label={copy.gitUpdate} hint={gitStatus?.root || ''}>
        <span className={`set-state ${gitStatus?.error ? 'is-off' : (gitStatus?.latest ? 'is-on' : '')}`}>{sourceState}</span>
      </SettingRow>
      <SettingRow label={copy.branch} hint={gitStatus?.upstream ? `${copy.upstream}: ${gitStatus.upstream} · ${copy.ahead} ${gitStatus.ahead || 0} / ${copy.behind} ${gitStatus.behind || 0}` : ''}>
        <code>{gitStatus?.branch || '-'} · {gitStatus?.commit || gitResult?.after || '-'}</code>
      </SettingRow>
      {gitStatus?.upstream_configured === false && <SettingNote tone="warn">{copy.upstreamHelp}</SettingNote>}
      {gitStatus?.dirty && <SettingNote tone="warn">{copy.dirty}</SettingNote>}
      {gitStatus?.error && <SettingNote tone="error">{gitStatus.error}</SettingNote>}
      {gitStatus?.fetch_error && <pre className="mini-log">{gitStatus.fetch_error}</pre>}
      {gitResult?.pull && <pre className="mini-log">{gitResult.pull}</pre>}
      <SettingFooter>
        <button type="button" onClick={version.checkSource} disabled={gitBusy}>{gitBusy ? t.busy : copy.checkLatest}</button>
        <button className="primary" type="button" onClick={version.updateSource} disabled={gitBusy || gitStatus?.latest || gitStatus?.upstream_configured === false}>
          {gitBusy ? t.busy : copy.updateSource}
        </button>
      </SettingFooter>
    </SettingsSection>
  </SettingsPage>
}

export default OverviewPage
