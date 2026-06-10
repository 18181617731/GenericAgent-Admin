import json, os, sys, time, traceback, threading, queue
from pathlib import Path


def _force_utf8_stdio():
    # Windows pipes otherwise may inherit the active ANSI code page and corrupt CJK text.
    for stream in (sys.stdin, sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding='utf-8', errors='replace')
        except Exception:
            pass


def _venv_paths_for(root: Path):
    venvs = [root / '.venv', root / 'venv']
    for venv in venvs:
        if not venv.exists():
            continue
        scripts = venv / ('Scripts' if os.name == 'nt' else 'bin')
        sites = []
        if os.name == 'nt':
            sites.append(venv / 'Lib' / 'site-packages')
        else:
            lib = venv / 'lib'
            try:
                sites.extend(sorted(lib.glob('python*/site-packages')))
            except Exception:
                pass
        sites = [p.resolve() for p in sites if p.exists()]
        if sites:
            return venv.resolve(), scripts.resolve(), sites
    return None, None, []


def _inject_ga_venv(root: Path):
    """Make GA virtualenv packages visible even if launched by bare uv/system Python.

    Admin chat is normally started with GA_ROOT and root/.venv Python.  If a
    nested/old launch uses a bare uv Python, importing agentmain eventually
    fails on dependencies such as requests/TMWebDriver deps.  Avoid re-exec on
    Windows (can crash and risks stdin handling); inject the venv site-packages
    before importing GA modules instead.
    """
    venv, scripts, sites = _venv_paths_for(root)
    if not sites:
        return
    os.environ.setdefault('VIRTUAL_ENV', str(venv))
    if scripts:
        path = os.environ.get('PATH') or ''
        sp = str(scripts)
        parts = path.split(os.pathsep) if path else []
        if not parts or parts[0].lower() != sp.lower():
            os.environ['PATH'] = sp + (os.pathsep + path if path else '')
    for site in reversed(sites):
        s = str(site)
        if s not in sys.path:
            sys.path.insert(0, s)


_force_utf8_stdio()
# stdout is the Go<->worker NDJSON protocol channel.  GA core/tools may
# print diagnostics while executing (including browser helpers).  Keep a
# private duplicate of the original stdout for protocol events, then point fd 1
# itself at stderr so Python prints, os.write(1, ...), C extensions, and child
# processes cannot interleave ordinary output with protocol JSON.
_PROTOCOL_STDOUT_LOCK = threading.Lock()


def _isolate_protocol_stdout():
    protocol = sys.stdout
    if sys.stderr is None:
        return protocol
    try:
        stdout_fd = sys.stdout.fileno()
        stderr_fd = sys.stderr.fileno()
        protocol_fd = os.dup(stdout_fd)
        try:
            os.set_inheritable(protocol_fd, False)
        except Exception:
            pass
        encoding = getattr(sys.stdout, 'encoding', None) or 'utf-8'
        protocol = os.fdopen(protocol_fd, 'w', encoding=encoding, errors='replace', buffering=1)
        os.dup2(stderr_fd, stdout_fd)
    except Exception:
        pass
    sys.stdout = sys.stderr
    return protocol


_PROTOCOL_STDOUT = _isolate_protocol_stdout()


def emit(ev):
    line = json.dumps(ev, ensure_ascii=False)
    with _PROTOCOL_STDOUT_LOCK:
        _PROTOCOL_STDOUT.write(line + '\n')
        _PROTOCOL_STDOUT.flush()


def new_id():
    import uuid
    return str(uuid.uuid4())


def _chat_content_text(value):
    if value is None:
        return ''
    if isinstance(value, str):
        return value
    try:
        return json.dumps(value, ensure_ascii=False)
    except Exception:
        return str(value)


def _admin_history_to_backend(history):
    """Convert persisted Admin chat messages to GA llmcore BaseSession.history format."""
    out = []
    for msg in history or []:
        if not isinstance(msg, dict):
            continue
        role = str(msg.get('role') or '').lower()
        if role not in ('user', 'assistant'):
            continue
        text = _chat_content_text(msg.get('content')).strip()
        if not text:
            continue
        out.append({'role': role, 'content': [{'type': 'text', 'text': text}]})
    return out


def _snapshot_backend_history(agent):
    try:
        history = getattr(agent.llmclient.backend, 'history', [])
        if not isinstance(history, list):
            return []
        return json.loads(json.dumps(history, ensure_ascii=False, default=str))
    except Exception:
        return []


def _json_clone(value, fallback):
    try:
        return json.loads(json.dumps(value, ensure_ascii=False, default=str))
    except Exception:
        return fallback


def _snapshot_ga_state(agent):
    """Persist the GA official lightweight context state in addition to raw LLM history."""
    state = {'history_info': [], 'working': {}}
    try:
        h = getattr(agent, 'history', [])
        if isinstance(h, list):
            state['history_info'] = _json_clone(h, [])
    except Exception:
        pass
    try:
        handler = getattr(agent, 'handler', None)
        working = getattr(handler, 'working', None) if handler is not None else None
        if isinstance(working, dict):
            state['working'] = _json_clone(working, {})
    except Exception:
        pass
    return state


def _restore_ga_state(agent, history_info=None, working=None):
    """Restore GA's own WORKING MEMORY inputs so Admin matches official long-running GA."""
    try:
        if isinstance(history_info, list):
            agent.history = _json_clone(history_info, [])
    except Exception:
        pass
    try:
        if isinstance(working, dict):
            restored_working = _json_clone(working, {})
            agent._admin_restore_working = restored_working
            # GenericAgent.run copies working memory only from self.handler into the
            # freshly-created handler; provide an Admin-side previous handler without
            # modifying GA core code.
            agent.handler = type('AdminRestoredHandler', (), {'working': restored_working})()
    except Exception:
        pass


def _restore_admin_history(agent, history, raw_history=None):
    try:
        restored = raw_history if isinstance(raw_history, list) and raw_history else _admin_history_to_backend(history)
        restored = json.loads(json.dumps(restored, ensure_ascii=False, default=str)) if isinstance(restored, list) else []
        sticky_tools_history = getattr(agent, '_admin_sticky_tools_history', []) or []
        if sticky_tools_history:
            restored.extend(json.loads(json.dumps(sticky_tools_history, ensure_ascii=False, default=str)))
        agent.llmclient.backend.history = restored
    except Exception:
        pass


def _select_llm_if_needed(agent, llm_no):
    """Keep GA official lazy tool injection cache unless the user switches models."""
    try:
        current = getattr(agent, 'llm_no', None)
        if current == llm_no:
            return
    except Exception:
        pass
    try:
        agent.next_llm(llm_no)
    except Exception:
        pass


def _load_tools_history(agent):
    hist_path = Path(getattr(agent, 'script_dir', os.getcwd())) / 'assets' / 'tool_usable_history.json'
    with hist_path.open('r', encoding='utf-8') as f:
        items = json.load(f)
    return items if isinstance(items, list) else []


def _inject_tools_history(agent, sticky=True):
    items = _load_tools_history(agent)
    if sticky:
        agent._admin_sticky_tools_history = items
    if items:
        agent.llmclient.backend.history.extend(items)
    return len(items)


def _reset_tools_schema(agent):
    try:
        agent.llmclient.last_tools = ''
    except Exception:
        pass


def _reinject_tools(agent):
    """Mirror GA Streamlit's manual Tools reinjection button.

    Official GA does two things: clear llmclient.last_tools so the next model
    request resends the tool schemas, then append assets/tool_usable_history.json
    into backend history as a reminder of available tool usage.
    """
    _reset_tools_schema(agent)
    added = 0
    try:
        added = _inject_tools_history(agent, sticky=True)
    except Exception as e:
        return {'ok': False, 'message': '工具历史注入失败：%s' % e, 'added': added}
    return {'ok': True, 'message': '已重新注入 Tools，下一次请求会重新发送工具定义', 'added': added}


def _apply_tools_mode(agent, mode):
    if mode != 'fixed':
        return None
    _reset_tools_schema(agent)
    try:
        added = _inject_tools_history(agent, sticky=False)
        return {'ok': True, 'message': '固定模式：本次请求已注入 Tools', 'added': added}
    except Exception as e:
        return {'ok': False, 'message': '固定模式 Tools 注入失败：%s' % e, 'added': 0}


def handle_request(agent, worker, req):
    prompt = req.get('prompt') or ''
    history = req.get('history') or []
    raw_history = req.get('raw_history') or []
    history_info = req.get('history_info') or []
    working = req.get('working') or {}
    llm_no = int(req.get('llm_no') or 0)
    tools_mode = str(req.get('tools_mode') or 'official')
    _select_llm_if_needed(agent, llm_no)
    _restore_ga_state(agent, history_info, working)
    _restore_admin_history(agent, history, raw_history)
    mode_status = _apply_tools_mode(agent, tools_mode)
    if mode_status and not mode_status.get('ok'):
        emit({'type': 'notice', 'message': mode_status})
    chunks = []
    try:
        display_queue = agent.put_task(prompt, source='admin_chat')
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
                state = _snapshot_ga_state(agent)
                emit({'type': 'done', 'message': msg, 'raw_history': _snapshot_backend_history(agent), 'history_info': state.get('history_info') or [], 'working': state.get('working') or {}})
                return
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        emit({'type': 'error', 'message': msg, 'raw_history': _snapshot_backend_history(agent)})


def main():
    root = Path(os.environ.get('GA_ROOT') or '.').resolve()
    _inject_ga_venv(root)
    first = True
    agent = None
    worker = None
    agent_lock = threading.RLock()
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            if first:
                first = False
                root = Path(req.get('ga_root') or root).resolve()
                if str(root) not in sys.path:
                    sys.path.insert(0, str(root))
                os.chdir(root)
                from agentmain import GeneraticAgent
                agent = GeneraticAgent()
                agent.verbose = True
                agent.inc_out = True
                worker = threading.Thread(target=agent.run, name='ga-admin-chat-worker', daemon=True)
                worker.start()
            if req.get('op') == 'reinject_tools':
                with agent_lock:
                    emit({'type': 'reinject_tools', **_reinject_tools(agent)})
                continue
            with agent_lock:
                handle_request(agent, worker, req)
        except Exception as e:
            msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
            emit({'type': 'error', 'message': msg})


if __name__ == '__main__':
    main()
