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


# Intercept stderr to capture GA core's token usage prints.
# GA core's _record_usage prints lines like "[Cache] input=N cached=M" and "[Output] tokens=N".
# We parse these to accumulate usage stats for the current turn.
_USAGE_LOCK = threading.Lock()
_CURRENT_USAGE = {'input_tokens': 0, 'output_tokens': 0, 'cached_tokens': 0}
# Per-internal-turn usage snapshots for the current request. GA core prints a
# "[Cache] ..." then "[Output] tokens=N" pair per internal LLM call; the
# "[Output]" line marks the end of one turn, so we snapshot and reset there.
_TURN_USAGES = []


class _UsageCapturingStderr:
    """Tee stderr writes, parse token usage lines, and accumulate stats."""
    def __init__(self, original):
        self._original = original
        self._encoding = getattr(original, 'encoding', 'utf-8')
    
    def write(self, text):
        # Forward to original stderr first
        try:
            self._original.write(text)
        except Exception:
            pass
        # Parse token usage lines
        if not text:
            return
        import re
        with _USAGE_LOCK:
            # [Cache] input=123 cached=45  or  [Cache] input=123 creation=10 read=20
            m = re.search(r'\[Cache\]\s+input=(\d+)', text)
            if m:
                _CURRENT_USAGE['input_tokens'] = int(m.group(1))
            m = re.search(r'cached=(\d+)', text)
            if m:
                _CURRENT_USAGE['cached_tokens'] = int(m.group(1))
            m = re.search(r'read=(\d+)', text)
            if m:
                _CURRENT_USAGE['cached_tokens'] = int(m.group(1))
            # [Output] tokens=456  -- marks the end of one internal LLM turn.
            m = re.search(r'\[Output\]\s+tokens=(\d+)', text)
            if m:
                _CURRENT_USAGE['output_tokens'] = int(m.group(1))
                # Snapshot this completed turn and reset the buffer for the next.
                turn_snapshot = dict(_CURRENT_USAGE)
                _TURN_USAGES.append(turn_snapshot)
                turn_index = len(_TURN_USAGES) - 1
                _CURRENT_USAGE['input_tokens'] = 0
                _CURRENT_USAGE['output_tokens'] = 0
                _CURRENT_USAGE['cached_tokens'] = 0
                # Push this turn's usage live so the UI can render it the moment
                # the internal turn finishes, instead of waiting for `done`.
                try:
                    emit({'type': 'turn_usage', 'index': turn_index, 'usage': turn_snapshot})
                except Exception:
                    pass
    
    def flush(self):
        try:
            self._original.flush()
        except Exception:
            pass
    
    def __getattr__(self, name):
        return getattr(self._original, name)


sys.stderr = _UsageCapturingStderr(sys.stderr)
# _isolate_protocol_stdout() above pointed sys.stdout at the ORIGINAL stderr
# object (captured before this wrapper was installed).  Re-point sys.stdout at
# the wrapper so GA core's `print()` usage lines flow through the parser too.
sys.stdout = sys.stderr


def _reset_usage():
    """Clear usage accumulator for a new request."""
    with _USAGE_LOCK:
        _CURRENT_USAGE['input_tokens'] = 0
        _CURRENT_USAGE['output_tokens'] = 0
        _CURRENT_USAGE['cached_tokens'] = 0
        _TURN_USAGES.clear()


def _snapshot_usage():
    """Snapshot current usage stats (last turn's running total)."""
    with _USAGE_LOCK:
        return dict(_CURRENT_USAGE)


def _snapshot_turn_usages():
    """Snapshot the list of per-internal-turn usage stats for this request.

    Each completed turn is captured when GA core prints its "[Output]" line.
    If a trailing turn produced cache/input counts without an "[Output]" line
    yet, include it so no usage is dropped.
    """
    with _USAGE_LOCK:
        usages = [dict(u) for u in _TURN_USAGES]
        if (_CURRENT_USAGE['input_tokens'] or _CURRENT_USAGE['output_tokens']
                or _CURRENT_USAGE['cached_tokens']):
            usages.append(dict(_CURRENT_USAGE))
        return usages


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
    _reset_usage()  # Clear usage accumulator for this turn
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
                usage = _snapshot_usage()
                usages = _snapshot_turn_usages()
                emit({'type': 'done', 'message': msg, 'usage': usage, 'usages': usages, 'raw_history': _snapshot_backend_history(agent), 'history_info': state.get('history_info') or [], 'working': state.get('working') or {}})
                return
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        usage = _snapshot_usage()
        usages = _snapshot_turn_usages()
        emit({'type': 'error', 'message': msg, 'usage': usage, 'usages': usages, 'raw_history': _snapshot_backend_history(agent)})


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
