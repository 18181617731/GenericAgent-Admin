import { useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { ChevronRight, Download, ExternalLink, Eye, FileCode2, FileImage, FileText, Folder, FolderOpen, HardDrive, Maximize2, Minimize2, Pencil, RefreshCw, Save, Search, Trash2, Undo2, X } from 'lucide-react'
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

const isMarkdownPath = (path) => /\.(?:md|markdown)$/i.test(String(path || '').trim())

const pathSegments = (path) => {
  const parts = String(path || '').replace(/\\/g, '/').split('/').filter(Boolean)
  return parts.map((name, index) => ({ name, path: parts.slice(0, index + 1).join('/') }))
}

const fileExtension = (path) => pathName(path).split('.').pop()?.toUpperCase() || 'FILE'

const formatFileSize = (size) => {
  const bytes = Number(size)
  if (!Number.isFinite(bytes) || bytes < 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB']
  const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)) - 1, units.length - 1)
  return `${(bytes / (1024 ** (unit + 1))).toFixed(unit ? 1 : 0)} ${units[unit]}`
}

const formatFileDate = (value) => {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString([], { dateStyle: 'short', timeStyle: 'short' })
}

const entryIcon = (entry) => {
  if (entry.kind === 'dir') return <Folder size={18}/>
  if (/\.(?:png|jpe?g|gif|webp|svg|bmp)$/i.test(entry.path)) return <FileImage size={18}/>
  if (/\.(?:py|js|jsx|ts|tsx|json|ya?ml|toml|ini|sh|bat)$/i.test(entry.path)) return <FileCode2 size={18}/>
  return <FileText size={18}/>
}

function MarkdownPreview({ content }) {
  return <article className="file-markdown-preview" aria-label="Markdown 格式化预览">
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        a: ({ children, href, title }) => <a href={href} title={title} target="_blank" rel="noreferrer">{children}</a>,
      }}
    >{content}</ReactMarkdown>
  </article>
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
  revealFileInExplorer,
  runSearch,
  clearSearch,
  discardChanges,
  fileStatus = {},
  dismissFileStatus,
  busy = false,
}) {
  const [mobileView, setMobileView] = useState('browse')
  const [contentMode, setContentMode] = useState('edit')
  const [previewExpanded, setPreviewExpanded] = useState(false)
  const mobileTabsRef = useRef(null)
  const dirty = fileEditorDirty(fileContent, loadedFileContent)
  const text = t.files
  const explorer = t.fileExplorer || (String(text?.workflow || '').includes('文件')
    ? {
        root: 'GA 根目录', location: '当前目录', folderActions: '目录操作', up: '返回上级目录', refresh: '刷新当前目录', openFolder: '在本机资源管理器中打开当前目录', showFile: '在资源管理器中显示', go: '前往', items: count => `${count} 项`, name: '名称', modified: '修改日期', type: '类型', size: '大小', folder: '文件夹',
      }
    : {
        root: 'GA root', location: 'Current folder', folderActions: 'Folder actions', up: 'Go up', refresh: 'Refresh folder', openFolder: 'Open current folder on this computer', showFile: 'Show in folder', go: 'Go', items: count => `${count} items`, name: 'Name', modified: 'Modified', type: 'Type', size: 'Size', folder: 'Folder',
      })
  const retargeted = Boolean(loadedFilePath && filePath && loadedFilePath !== filePath)
  const saveReview = !filePath ? text.chooseBeforeSave : retargeted ? text.reviewRetargeted(loadedFilePath, filePath) : dirty ? text.reviewDirty(filePath) : text.reviewClean(filePath)
  const hasLoadedTarget = Boolean(String(loadedFilePath || '').trim())
  const saveDisabled = !hasLoadedTarget || !filePath || !dirty
  const saveDisabledReason = !hasLoadedTarget
    ? text.readFirst
    : !filePath
      ? text.chooseSaveTarget
      : !dirty
        ? text.noChanges(loadedFilePath)
        : ''
  const fileListEmpty = !fileList?.length
  const searchEmpty = !searchHits?.length
  const searchAttempted = fileStatus?.action === 'search' && fileStatus?.kind === 'success'
  const hasBrowsePath = Boolean(String(browsePath || '').trim())
  const hasFilePath = Boolean(String(filePath || '').trim())
  const markdownFile = isMarkdownPath(loadedFilePath || filePath)
  const parent = parentPath(browsePath)
  const segments = pathSegments(browsePath)
  const searchHint = fileSearch ? `${text.noMatches}. ${text.broaderSearch}` : text.searchPrompt
  const fileListHint = hasBrowsePath
    ? text.noFilesPath
    : text.noRootPath

  useEffect(() => {
    if (!loadedFilePath) return
    setMobileView('preview')
    setContentMode(isMarkdownPath(loadedFilePath) ? 'preview' : 'edit')
    if (typeof window !== 'undefined' && window.matchMedia?.('(max-width: 680px)')?.matches) {
      window.requestAnimationFrame(() => {
        mobileTabsRef.current?.scrollIntoView({ block: 'start' })
        window.requestAnimationFrame(() => {
          const shellHeight = document.querySelector('.sidebar')?.getBoundingClientRect().height || 0
          window.scrollBy(0, -(shellHeight + 8))
        })
      })
    }
  }, [loadedFileContent, loadedFilePath])

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
        retryLabel={text.retryAction}
      />
      <div className="files-mobile-tabs" ref={mobileTabsRef} role="tablist" aria-label="文件视图">
        <button type="button" role="tab" aria-selected={mobileView === 'browse'} className={mobileView === 'browse' ? 'active' : ''} onClick={() => setMobileView('browse')}><FolderOpen size={16}/>文件</button>
        <button type="button" role="tab" aria-selected={mobileView === 'preview'} className={mobileView === 'preview' ? 'active' : ''} onClick={() => setMobileView('preview')}><FileText size={16}/>预览{dirty ? ' *' : ''}</button>
      </div>
      <div className={`workspace files-workspace files-view-${mobileView}${previewExpanded ? ' files-preview-expanded' : ''}`}>
        <Panel title={t.lists.fileList} className="files-browser-panel">
          <div className="files-explorer-toolbar">
            <nav className="files-breadcrumbs" aria-label={explorer.location}>
              <button type="button" className="files-root-crumb" onClick={() => loadFiles('')} disabled={busy}><HardDrive size={15}/>{explorer.root}</button>
              {segments.map(segment => <span className="files-breadcrumb-part" key={segment.path}><ChevronRight size={14}/><button type="button" onClick={() => loadFiles(segment.path)} disabled={busy}>{segment.name}</button></span>)}
            </nav>
            <div className="files-directory-tools" role="toolbar" aria-label={explorer.folderActions}>
              <button type="button" onClick={() => loadFiles(parent)} disabled={busy || !browsePath} title={explorer.up} aria-label={explorer.up}><Undo2 size={16}/></button>
              <button type="button" onClick={() => loadFiles(browsePath)} disabled={busy} title={explorer.refresh} aria-label={explorer.refresh}><RefreshCw size={16}/></button>
              <button type="button" className="files-host-open" onClick={() => revealFileInExplorer?.(browsePath || '.', 'folder')} disabled={busy} title={explorer.openFolder} aria-label={explorer.openFolder}><ExternalLink size={16}/></button>
            </div>
          </div>
          <div className="files-search-row">
            <input aria-label="文件搜索文本" value={fileSearch} onChange={e => setFileSearch(e.target.value)} placeholder={t.hints.searchText} onKeyDown={e => e.key === 'Enter' && fileSearch.trim() && runSearch()}/>
            <button type="button" onClick={runSearch} disabled={busy || !fileSearch.trim()}><Search size={15}/>{t.search}</button>
          </div>
          <div className="files-directory-summary"><span title={browsePath || explorer.root}>{browsePath || explorer.root}</span><b>{explorer.items(fileList?.length || 0)}</b></div>
          <div className="file-list-columns" aria-hidden="true"><span></span><span>{explorer.name}</span><span>{explorer.modified}</span><span>{explorer.type}</span><span>{explorer.size}</span><span></span></div>
          <div className="file-list">
            {fileListEmpty && <div className="empty-card" role="status"><b>{hasBrowsePath ? text.folderEmpty : text.chooseRoot}</b><span>{t.hints?.fileListEmpty || fileListHint}</span></div>}
            {fileList.map(entry => <button type="button" className={`file-entry file-entry-${entry.kind}`} key={entry.path} onClick={() => openEntry(entry)} title={entry.path}>
              <span className="file-entry-icon">{entryIcon(entry)}</span>
              <span className="file-entry-label"><b>{pathName(entry.path)}</b><small>{entry.path}</small></span>
              <time className="file-entry-date" dateTime={entry.mod_time || undefined}>{formatFileDate(entry.mod_time)}</time>
              <span className="file-entry-type">{entry.kind === 'dir' ? explorer.folder : fileExtension(entry.path)}</span>
              <span className="file-entry-size">{entry.kind === 'dir' ? '-' : formatFileSize(entry.size)}</span>
              {entry.kind === 'dir' && <ChevronRight className="file-entry-next" size={16}/>}
            </button>)}
          </div>
          {(fileSearch || searchHits.length > 0) && <div className="files-search-results">
            <div className="files-search-results-head"><h4>{t.lists.searchResults} <span>{searchHits.length}</span></h4><button type="button" onClick={() => clearSearch?.()} aria-label="清空文件搜索"><X size={14}/>清空</button></div>
            {searchEmpty && searchAttempted
              ? <div className="empty-card" role="status"><b>{text.noMatches}</b><span>{text.broaderSearch}</span></div>
              : searchEmpty && <p className="muted">{t.hints?.searchEmpty || searchHint}</p>}
            {searchHits.map(hit => <button type="button" className="hit" key={`${hit.path}:${hit.line}`} onClick={() => openSearchHit(hit.path)} title={`${hit.path}:${hit.line} · ${hit.preview}`}><b>{pathName(hit.path)}:{hit.line}</b><span>{hit.preview}</span></button>)}
          </div>}
        </Panel>
        <Panel title={t.lists.filePreview} className="log-panel files-preview-panel">
          <div className="file-editor-toolbar">
            <span className={dirty ? 'status-pill warn' : 'status-pill ok'}>{dirty ? text.dirty : text.clean}</span>
            {loadedFilePath && <span className="muted" title={loadedFilePath}>{text.loaded}: {loadedFilePath}</span>}
            {retargeted && <span className="status-pill bad">{text.targetChanged}</span>}
            {hasLoadedTarget && <button type="button" className="file-preview-size-toggle" aria-pressed={previewExpanded} onClick={() => setPreviewExpanded(value => !value)} title={previewExpanded ? '恢复文件列表与预览的分栏显示' : '收起文件列表，扩大预览或编辑空间'}>
              {previewExpanded ? <Minimize2 size={14}/> : <Maximize2 size={14}/>}<span>{previewExpanded ? '恢复分栏' : '扩大预览'}</span>
            </button>}
            {loadedFilePath && <button type="button" className="file-reveal-action" onClick={() => revealFileInExplorer?.(loadedFilePath, 'folder')} disabled={busy} title={explorer.showFile} aria-label={explorer.showFile}><ExternalLink size={14}/><span>{explorer.showFile}</span></button>}
            {markdownFile && hasLoadedTarget && <div className="file-content-mode" role="group" aria-label="Markdown 查看模式">
              <button type="button" className={contentMode === 'preview' ? 'active' : ''} aria-pressed={contentMode === 'preview'} onClick={() => setContentMode('preview')}><Eye size={14}/>预览</button>
              <button type="button" className={contentMode === 'edit' ? 'active' : ''} aria-pressed={contentMode === 'edit'} onClick={() => setContentMode('edit')}><Pencil size={14}/>编辑</button>
            </div>}
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
            {dirty && <button type="button" onClick={discardChanges} disabled={busy || !discardChanges}><Undo2 size={15}/><span>{text.discard}</span></button>}
            <button type="button" className="primary" onClick={saveFile} disabled={saveDisabled || busy} title={saveReview} aria-describedby={saveDisabledReason ? 'file-save-reason' : undefined}><Save size={15}/><span>{t.save}</span></button>
          </div>
          {saveDisabledReason && <p id="file-save-reason" className="muted">{saveDisabledReason}</p>}
          <div className={`file-save-review ${retargeted ? 'bad' : dirty ? 'warn' : 'ok'}`} role="status" aria-live="polite">
            {saveReview}
          </div>
          <div className="files-preview-content">
            {(hasFilePath || fileContent) && markdownFile && contentMode === 'preview' &&
              <MarkdownPreview content={fileContent}/>
            }
            {!hasFilePath && !fileContent && <div className="empty-card files-editor-empty" role="status"><b>{text.noFileLoaded}</b><span>{text.noFileLoadedHelp}</span></div>}
            {(hasFilePath || fileContent) && (!markdownFile || contentMode === 'edit') && (
              <textarea aria-label={text.editorLabel} className="file-editor" value={fileContent} onChange={e => setFileContent(e.target.value)} placeholder={t.empty}/>
            )}
          </div>
        </Panel>
      </div>
    </section>
  )
}
