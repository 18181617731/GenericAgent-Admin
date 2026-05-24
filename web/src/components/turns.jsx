export function TurnBubble({ message }) {
  const m = message || {}
  return <div className={`bubble ${m.role || 'assistant'} ${m.type || ''} ${m.error?'error':''}`}>
    <div className="role">{m.title || m.role || 'assistant'}</div>
    <div className="content">{m.content || ''}</div>
  </div>
}

export function TurnList({ messages, empty, className = '' }) {
  const items = messages || []
  return <div className={`chat-messages turn-list ${className}`}>
    {items.length===0 && <div className="empty-chat">{empty}</div>}
    {items.map((m, i) => <TurnBubble key={m.id || i} message={m} />)}
  </div>
}
