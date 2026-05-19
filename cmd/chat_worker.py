import json, os, sys, time, traceback, threading, queue
from pathlib import Path


def _force_utf8_stdio():
    # Windows pipes otherwise may inherit the active ANSI code page and corrupt CJK text.
    for stream in (sys.stdin, sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding='utf-8', errors='replace')
        except Exception:
            pass


_force_utf8_stdio()


def _compact_text(value, limit=4000):
    text = str(value or '').replace('\r\n', '\n').strip()
    if len(text) > limit:
        return text[:limit] + '\n...[truncated]'
    return text


def _message_label(role):
    role = str(role or '').lower()
    if role == 'user':
        return 'USER'
    if role == 'assistant':
        return 'ASSISTANT'
    return role.upper() or 'MESSAGE'


def build_prompt_with_history(prompt, history):
    """GA core owns model memory only inside one worker process; admin chat starts a fresh worker per send.
    Therefore inject prior persisted session messages into the current user_input explicitly.
    """
    prompt = str(prompt or '')
    if not isinstance(history, list):
        return prompt
    previous = []
    # chatPost already appends the current user message before sending history.
    for msg in history[:-1]:
        if not isinstance(msg, dict):
            continue
        role = _message_label(msg.get('role'))
        content = _compact_text(msg.get('content'), 5000 if role == 'ASSISTANT' else 3000)
        if content:
            previous.append(f'[{role}]: {content}')
    if not previous:
        return prompt
    # Bound context size to avoid flooding the selected model while still preserving recent turns.
    text = '\n\n'.join(previous[-24:])
    if len(text) > 28000:
        text = '...[older history omitted]\n' + text[-28000:]
    return (
        '以下是当前会话的历史上下文，请在回答时延续这些上下文，不要把它当作用户的新问题。\n'
        '<history>\n' + text + '\n</history>\n\n'
        '### 用户当前消息\n' + prompt
    )


def emit(ev):
    print(json.dumps(ev, ensure_ascii=False), flush=True)


def main():
    req = json.load(sys.stdin)
    root = Path(req.get('ga_root') or os.environ.get('GA_ROOT') or '.').resolve()
    if str(root) not in sys.path:
        sys.path.insert(0, str(root))
    os.chdir(root)
    from agentmain import GeneraticAgent

    prompt = req.get('prompt') or ''
    prompt = build_prompt_with_history(prompt, req.get('history'))
    llm_no = int(req.get('llm_no') or 0)
    agent = GeneraticAgent()
    try:
        agent.next_llm(llm_no)
    except Exception:
        pass
    # GA core supports incremental display through inc_out + put_task display queue.
    agent.verbose = True
    agent.inc_out = True

    try:
        worker = threading.Thread(target=agent.run, name='ga-admin-chat-worker', daemon=True)
        worker.start()
        display_queue = agent.put_task(prompt, source='admin_chat')
        chunks = []
        while True:
            try:
                item = display_queue.get(timeout=1.0)
            except queue.Empty:
                if not worker.is_alive():
                    raise RuntimeError('GA core worker exited unexpectedly')
                continue
            if 'next' in item:
                delta = str(item.get('next') or '')
                if delta:
                    chunks.append(delta)
                    emit({'type': 'delta', 'delta': delta})
            if 'done' in item:
                text = str(item.get('done') or ''.join(chunks))
                msg = {'id': new_id(), 'role': 'assistant', 'content': text, 'created_at': int(time.time())}
                emit({'type': 'done', 'message': msg})
                return
    except Exception as e:
        try:
            agent.abort()
        except Exception:
            pass
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        emit({'type': 'error', 'message': msg})


def new_id():
    import uuid
    return str(uuid.uuid4())

if __name__ == '__main__':
    main()
