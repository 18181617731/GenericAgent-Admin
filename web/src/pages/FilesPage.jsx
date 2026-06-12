import { Download, Save, Search, Trash2 } from 'lucide-react'
import { Panel } from '../components/common'
import { fileEditorDirty, saveReviewText } from '../lib/filesSafety'

export function FilesPage({
  t,
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
  busy = false,
}) {
  const dirty = fileEditorDirty(fileContent, loadedFileContent)
  const retargeted = Boolean(loadedFilePath && filePath && loadedFilePath !== filePath)
  const saveReview = saveReviewText({ path: filePath, loadedPath: loadedFilePath, dirty })
  const saveDisabled = !filePath || !dirty
  const fileListEmpty = !fileList?.length
  const searchEmpty = !searchHits?.length
  const hasFilePath = Boolean(String(filePath || '').trim())
  const searchHint = fileSearch ? 'No matches found. Check the path filter or try a broader term.' : 'Enter search text, then run search.'
  const fileListHint = hasFilePath
    ? 'No files returned for this path. Confirm the GA root or choose a folder and read again.'
    : 'No GA root selected yet. Paste the project root or a folder path, then click Read.'
  return (
    <section>
      <div className="workspace">
        <Panel title={t.lists.fileList}>
          <div className="inline-form">
            <input value={filePath} onChange={e => setFilePath(e.target.value)} placeholder={t.hints.filePath}/>
            <button onClick={() => loadFiles(filePath)} disabled={busy || !hasFilePath}>{t.read}</button>
          </div>
          <div className="inline-form">
            <input value={fileSearch} onChange={e => setFileSearch(e.target.value)} placeholder={t.hints.searchText}/>
            <button onClick={runSearch}><Search size={14}/>{t.search}</button>
          </div>
          <div className="inline-form">
            <input type="number" value={tailLines} onChange={e => setTailLines(Number(e.target.value))}/>
            <span>{t.hints.tailLines}</span>
            <button onClick={() => tailFile(filePath)} disabled={!filePath || busy}>{t.tail || 'Tail'}</button>
            <button onClick={() => downloadFile(filePath)} disabled={!filePath || busy} title="Read-only: downloads the selected file without changing it."><Download size={14}/>{t.download || 'Download'}</button>
            <button onClick={() => deleteFile(filePath)} disabled={!filePath || busy} title="Destructive: removes the selected file after confirmation."><Trash2 size={14}/>{t.delete || 'Delete'}</button>
            <button onClick={saveFile} disabled={saveDisabled || busy} title={saveReview}><Save size={14}/>{t.save}</button>
          </div>
          <p className="operation-note" role="note">读取、尾读、搜索和下载都是安全的只读操作。保存会把当前编辑器文本写入目标路径；删除是破坏性操作，只能在确认选中路径后使用。</p>
          <div className="file-list">
            {fileListEmpty && <div className="empty-card" role="status"><b>{hasFilePath ? 'Folder is empty or unavailable' : 'Choose a GA root to browse files'}</b><span>{t.hints?.fileListEmpty || fileListHint}</span></div>}
            {fileList.map(e => <button key={e.path} onClick={() => e.kind === 'dir' ? loadFiles(e.path) : readFile(e.path)}>{e.kind === 'dir' ? '📁' : '📄'} {e.path}</button>)}
          </div>
          <h4>{t.lists.searchResults}</h4>
          {searchEmpty && <p className="muted">{t.hints?.searchEmpty || searchHint}</p>}
          {searchHits.map(h => <button className="hit" key={`${h.path}:${h.line}`} onClick={() => readFile(h.path)}>{h.path}:{h.line} · {h.preview}</button>)}
        </Panel>
        <Panel title={t.lists.filePreview} className="log-panel">
          <div className="file-editor-toolbar">
            <span className={dirty ? 'status-pill warn' : 'status-pill ok'}>{dirty ? '有未保存更改' : '已保存/干净'}</span>
            {loadedFilePath && <span className="muted">Loaded: {loadedFilePath}</span>}
            {retargeted && <span className="status-pill bad">Save target changed</span>}
          </div>
          <div className={`file-save-review ${retargeted ? 'bad' : dirty ? 'warn' : 'ok'}`} role="status" aria-live="polite">
            {saveReview}
          </div>
          {!loadedFilePath && !fileContent && <div className="empty-card" role="status"><b>尚未加载文件</b><span>请先从列表选择文件、尾读日志，或输入路径并读取后再编辑。</span></div>}
          <textarea className="file-editor" value={fileContent} onChange={e => setFileContent(e.target.value)} placeholder={t.empty}/>
        </Panel>
      </div>
    </section>
  )
}
