import { Save, Search } from 'lucide-react'
import { Panel } from '../components/common'

export function FilesPage({
  t,
  filePath,
  setFilePath,
  fileList,
  fileContent,
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
  runSearch,
}) {
  return (
    <section>
      <div className="workspace">
        <Panel title={t.lists.fileList}>
          <div className="inline-form">
            <input value={filePath} onChange={e => setFilePath(e.target.value)} placeholder={t.hints.filePath}/>
            <button onClick={() => loadFiles(filePath)}>{t.read}</button>
          </div>
          <div className="inline-form">
            <input value={fileSearch} onChange={e => setFileSearch(e.target.value)} placeholder={t.hints.searchText}/>
            <button onClick={runSearch}><Search size={14}/>{t.search}</button>
          </div>
          <div className="inline-form">
            <input type="number" value={tailLines} onChange={e => setTailLines(Number(e.target.value))}/>
            <span>{t.hints.tailLines}</span>
            <button onClick={() => tailFile(filePath)}>{t.tail || 'Tail'}</button>
            <button onClick={saveFile} disabled={!filePath}><Save size={14}/>{t.save}</button>
          </div>
          <div className="file-list">
            {fileList.map(e => <button key={e.path} onClick={() => e.kind === 'dir' ? loadFiles(e.path) : readFile(e.path)}>{e.kind === 'dir' ? '📁' : '📄'} {e.path}</button>)}
          </div>
          <h4>{t.lists.searchResults}</h4>
          {searchHits.map(h => <button className="hit" key={`${h.path}:${h.line}`} onClick={() => readFile(h.path)}>{h.path}:{h.line} · {h.preview}</button>)}
        </Panel>
        <Panel title={t.lists.filePreview} className="log-panel">
          <textarea className="file-editor" value={fileContent} onChange={e => setFileContent(e.target.value)} placeholder={t.empty}/>
        </Panel>
      </div>
    </section>
  )
}
