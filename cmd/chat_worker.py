import json, os, sys, time, traceback
from pathlib import Path


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
    llm_no = int(req.get('llm_no') or 0)
    agent = GeneraticAgent()
    try:
        agent.next_llm(llm_no)
    except Exception:
        pass
    agent.verbose = True
    try:
        agent.stream_output = True
    except Exception:
        pass

    chunks = []
    def cb(text):
        if text is None:
            return
        s = str(text)
        chunks.append(s)
        emit({'type': 'delta', 'delta': s})
    try:
        agent.on_stream = cb
    except Exception:
        pass

    try:
        result = agent.Run(prompt)
        text = ''.join(chunks) or ('' if result is None else str(result))
        msg = {'id': new_id(), 'role': 'assistant', 'content': text, 'created_at': int(time.time())}
        emit({'type': 'done', 'message': msg})
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        emit({'type': 'error', 'message': msg})


def new_id():
    import uuid
    return str(uuid.uuid4())

if __name__ == '__main__':
    main()
