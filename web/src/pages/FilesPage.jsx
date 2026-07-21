import { useEffect, useRef, useState } from 'react'
import { ChevronRight, Download, FileText, Folder, FolderOpen, Save, Search, Trash2, Undo2, X } from 'lucide-react'
import { Panel } from '../components/common'
import { StatusNotice } from '../components/feedback'
import { fileEditorDirty, saveReviewText } from '../lib/filesSafety'
import { shouldConfirmFileReplacement } from '../lib/ux'

const parentPath = (path) => {
  const normalized = String(path || '').replace(/\\/g, '/').replace(/\/+$/, '')
  const index = normalized.lastIndexOf('/')
  return index > 0 ? normalized.slice(0, index) : ''
}

const pathName = (path) => {
  const normalized = String(path || '').replace(/\\/g, '/').replace(/\/+$/, '')
  return normalized.slice(normalized.lastIndexOf('/') + 1) || normalized || '/'
}

export function FilesPage({
  t,
  browsePath,
  setBrowsePath,
  filePath,
  setFilePath,
  fileList,
  fileContent,
  loadedFileContent = '',
  loadedFilePath = '',
  setFileContent,
  fileSearch,
  setFileSearch,
  searchHits,
  tailLines,
  setTailLines,
  loadFiles,
  readFile,
  tailFile,
  saveFile,
  deleteFile,
  downloadFile,
  runSearch,
  clearSearch,
  discardChanges,
  fileStatus = {},
  dismissFileStatus,
  busy = false,
}) {
  const [mobileView, setMobileView] = useState('browse')
  const mobileTabsRef = useRef(null)
  const dirty = fileEditorDirty(fileContent, loadedFileContent)
  const retargeted = Boolean(loadedFilePath && filePath && loadedFilePath !== filePath)
  const saveReview = saveReviewText({ path: filePath, loadedPath: loadedFilePath, dirty })
  const hasLoadedTarget = Boolean(String(loadedFilePath || '').trim())
  const saveDisabled = !hasLoadedTarget || !filePath || !dirty
  const saveDisabledReason = !hasLoadedTarget
    ? 'Read a file before saving. The editor has no loaded target.'
    : !filePath
      ? 'Choose a save target before saving.'
      : !dirty
        ? `Save is disabled because ${loadedFilePath} has no unsaved changes.`
        : ''
  const fileListEmpty = !fileList?.length
  const searchEmpty = !searchHits?.length
  const searchAttempted = fileStatus?.action === 'search' && fileStatus?.kind === 'success'
  const hasBrowsePath = Boolean(String(browsePath || '').trim())
  const hasFilePath = Boolean(String(filePath || '').trim())
  const parent = parentPath(browsePath)
  const searchHint = fileSearch ? 'No matches found. Check the path filter or try a broader term.' : 'Enter search text, then run search.'
  const fileListHint = hasBrowsePath
    ? 'No files returned for this path. Confirm the GA root or choose a folder and read again.'
    : 'No GA root selected yet. Paste the project root or a folder path, then click Read.'

  useEffect(() => {
    if (!loadedFilePath) return
    setMobileView('preview')
    if (typeof window !== 'undefined' && window.matchMedia?.('(max-width: 680px)')?.matches) {
      window.requestAnimationFrame(() => {
        mobileTabsRef.current?.scrollIntoView({ block: 'start' })
        window.requestAnimationFrame(() => {
          const shellHeight = document.querySelector('.sidebar')?.getBoundingClientRect().height || 0
          window.scrollBy(0, -(shellHeight + 8))
        })
      })
    }
  }, [loadedFilePath])

  useEffect(() => {
    if (!dirty) return undefined
    const warn = event => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const guardedReadFile = (path) => {
    if (shouldConfirmFileReplacement({ dirty, loadedPath: loadedFilePath, nextPath: path })
      && !window.confirm(`“${pathName(loadedFilePath) || '当前文件'}”有未保存更改。放弃更改并打开“${pathName(path)}”？`)) return
    readFile(path)
  }

  const openEntry = (entry) => {
    if (entry.kind === 'dir') {
      setMobileView('browse')
      loadFiles(entry.path)
      return
    }
    guardedReadFile(entry.path)
  }

  const openSearchHit = (path) => guardedReadFile(path)

  return (
    <section className="files-page">
      <StatusNotice
        kind={fileStatus?.kind}
        message={fileStatus?.message}
        onRetry={fileStatus?.onRetry}
        onDismiss={dismissFileStatus}
        retryLabel="Retry file action"
      />
      <div className="files-mobile-tabs" ref={mobileTabsRef} role="tablist" aria-label="文件视图">
        <button type="button" role="tab" aria-selected={mobileView === 'browse'} className={mobileView === 'browse' ? 'active' : ''} onClick={() => setMobileView('browse')}><FolderOpen size={16}/>文件</button>
        <button type="button" role="tab" aria-selected={mobileView === 'preview'} className={mobileView === 'preview' ? 'active' : ''} onClick={() => setMobileView('preview')}><FileText size={16}/>预览{dirty ? ' *' : ''}</button>
      </div>
      <div className={`workspace files-workspace files-view-${mobileView}`}>
        <Panel title={t.lists.fileList} className="files-browser-panel">
          <div className="files-path-row">
            <input aria-label="浏览路径" value={browsePath} onChange={e => setBrowsePath(e.target.value)} placeholder={t.hints.filePath}/>
            <button type="button" onClick={() => loadFiles(browsePath)} disabled={busy}><FolderOpen size={15}/>{t.read}</button>
          </div>
          <div className="files-browse-actions">
            <button type="button" onClick={() => loadFiles(parent)} disabled={busy || !browsePath} title="返回上级目录"><Undo2 size={15}/>上级</button>
            <span className="files-current-path" title={browsePath || '/'}>{browsePath || '/'} · {fileList?.length || 0} 项</span>
          </div>
          <div className="files-search-row">
            <input aria-label="文件搜索文本" value={fileSearch} onChange={e => setFileSearch(e.target.value)} placeholder={t.hints.searchText} onKeyDown={e => e.key === 'Enter' && fileSearch.trim() && runSearch()}/>
            <button type="button" onClick={runSearch} disabled={busy || !fileSearch.trim()}><Search size={15}/>{t.search}</button>
          </div>
          <div className="file-list">
            {fileListEmpty && <div className="empty-card" role="status"><b>{hasBrowsePath ? 'Folder is empty or unavailable' : 'Choose a GA root to browse files'}</b><span>{t.hints?.fileListEmpty || fileListHint}</span></div>}
            {fileList.map(entry => <button type="button" className={`file-entry file-entry-${entry.kind}`} key={entry.path} onClick={() => openEntry(entry)} title={entry.path}>
              <span className="file-entry-icon">{entry.kind === 'dir' ? <Folder size={18}/> : <FileText size={18}/>}</span>
              <span className="file-entry-label"><b>{pathName(entry.path)}</b><small>{entry.path}</small></span>
              {entry.kind === 'dir' && <ChevronRight className="file-entry-next" size={16}/>}
            </button>)}
          </div>
          {(fileSearch || searchHits.length > 0) && <div className="files-search-results">
            <div className="files-search-results-head"><h4>{t.lists.searchResults} <span>{searchHits.length}</span></h4><button type="button" onClick={() => clearSearch?.()} aria-label="清空文件搜索"><X size={14}/>清空</button></div>
            {searchEmpty && searchAttempted
              ? <div className="empty-card" role="status"><b>No matches found</b><span>Check the path filter or try a broader term.</span></div>
              : searchEmpty && <p className="muted">{t.hints?.searchEmpty || searchHint}</p>}
            {searchHits.map(hit => <button type="button" className="hit" key={`${hit.path}:${hit.line}`} onClick={() => openSearchHit(hit.path)} title={`${hit.path}:${hit.line} · ${hit.preview}`}><b>{pathName(hit.path)}:{hit.line}</b><span>{hit.preview}</span></button>)}
          </div>}
        </Panel>
        <Panel title={t.lists.filePreview} className="log-panel files-preview-panel">
          <div className="file-editor-toolbar">
            <span className={dirty ? 'status-pill warn' : 'status-pill ok'}>{dirty ? '有未保存更改' : '已保存/干净'}</span>
            {loadedFilePath && <span className="muted" title={loadedFilePath}>已加载：{loadedFilePath}</span>}
            {retargeted && <span className="status-pill bad">Save target changed</span>}
          </div>
          <div className="files-target-row">
            <input aria-label="当前文件路径" value={filePath} onChange={e => setFilePath(e.target.value)} placeholder="输入要读取或保存的文件路径"/>
            <button type="button" onClick={() => guardedReadFile(filePath)} disabled={!hasFilePath || busy}><FileText size={15}/>{t.read}</button>
          </div>
          <div className="files-editor-actions">
            <label className="files-tail-field"><span>{t.hints.tailLines}</span><input aria-label={t.hints.tailLines} type="number" min="1" max="2000" value={tailLines} onChange={e => setTailLines(Number(e.target.value))}/></label>
            <button type="button" onClick={() => tailFile(filePath)} disabled={!hasFilePath || busy}>{t.tail || 'Tail'}</button>
            <button type="button" onClick={() => downloadFile(filePath)} disabled={!hasFilePath || busy} title="下载当前文件"><Download size={15}/><span>{t.download || 'Download'}</span></button>
            <button type="button" className="danger-subtle" onClick={() => deleteFile(filePath)} disabled={!hasFilePath || busy} title="删除当前文件，需要再次确认"><Trash2 size={15}/><span>{t.delete || 'Delete'}</span></button>
            {dirty && <button type="button" onClick={discardChanges} disabled={busy || !discardChanges}><Undo2 size={15}/><span>Discard changes</span></button>}
            <button type="button" className="primary" onClick={saveFile} disabled={saveDisabled || busy} title={saveReview} aria-describedby={saveDisabledReason ? 'file-save-reason' : undefined}><Save size={15}/><span>{t.save}</span></button>
          </div>
          {saveDisabledReason && <p id="file-save-reason" className="muted">{saveDisabledReason}</p>}
          <div className={`file-save-review ${retargeted ? 'bad' : dirty ? 'warn' : 'ok'}`} role="status" aria-live="polite">
            {saveReview}
          </div>
          {!hasFilePath && !fileContent && <div className="empty-card files-editor-empty" role="status"><b>尚未加载文件</b><span>从文件列表选择一个文件，或输入文件路径后读取。</span></div>}
          {(hasFilePath || fileContent) && <textarea aria-label="文件内容编辑器" className="file-editor" value={fileContent} onChange={e => setFileContent(e.target.value)} placeholder={t.empty}/>}
        </Panel>
      </div>
    </section>
  )
}
