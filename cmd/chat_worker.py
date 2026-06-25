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


EFFORT_LEVELS = ('none', 'minimal', 'low', 'medium', 'high', 'xhigh')


def _snapshot_reasoning_effort(agent):
    try:
        backend = getattr(getattr(agent, 'llmclient', None), 'backend', None)
        value = getattr(backend, 'reasoning_effort', None) if backend is not None else None
    except Exception:
        value = None
    value = str(value or '').strip().lower()
    if value == 'max':
        return 'xhigh'
    if value in EFFORT_LEVELS and value != 'none':
        return value
    return 'off'


def _agent_protocols(agent):
    try:
        b = getattr(getattr(agent, 'llmclient', None), 'backend', None)
        backs = getattr(b, 'backends', None)
        if isinstance(backs, (list, tuple)):
            return {str(getattr(x, 'protocol', '') or '').lower() for x in backs}
        return {str(getattr(b, 'protocol', '') or '').lower()} if b is not None else set()
    except Exception:
        return set()


def _effort_note(level, protocols):
    if level and 'claude' in protocols:
        if level in ('none', 'minimal'):
            return 'Claude 渠道忽略'
        if level == 'xhigh':
            return 'Claude 对应 max'
    return ''


def _maybe_handle_effort_command(agent, prompt):
    s = (prompt or '').strip()
    if s != '/effort' and not s.startswith('/effort ') and not s.startswith('/effort\t'):
        return None
    try:
        backend = getattr(getattr(agent, 'llmclient', None), 'backend', None)
        if backend is None:
            return '无法读取当前 LLM backend，不能设置 reasoning_effort。'
        if s == '/effort':
            cur = getattr(backend, 'reasoning_effort', None) or '(未设置)'
            return '当前 reasoning_effort: %s\n\n可选: %s；`off` 清除。' % (cur, '/'.join(EFFORT_LEVELS))
        value = s[len('/effort'):].strip().lower()
        old = getattr(backend, 'reasoning_effort', None)
        if value in ('', 'off', 'clear', 'unset'):
            effort = None
        elif value in EFFORT_LEVELS:
            effort = value
        else:
            return "无效 effort: %r (可选 %s, 留空或 off 清除)" % (value, '/'.join(EFFORT_LEVELS))
        setattr(backend, 'reasoning_effort', effort)
        note = _effort_note(effort, _agent_protocols(agent))
        tail = ' (%s)' % note if note else ''
        return 'reasoning_effort: %s → %s%s' % (old or '(未设置)', effort or '(清除)', tail)
    except Exception as e:
        return '设置 reasoning_effort 失败：%s' % e


def _apply_reasoning_effort_setting(agent, value):
    raw = str(value or '').strip().lower()
    if raw in ('', 'off', 'none', 'clear', 'unset'):
        effort = None
    elif raw in EFFORT_LEVELS:
        effort = raw
    elif raw == 'max':
        effort = 'xhigh'
    else:
        return
    try:
        backend = getattr(getattr(agent, 'llmclient', None), 'backend', None)
        if backend is not None:
            setattr(backend, 'reasoning_effort', effort)
    except Exception:
        pass


def _render_review_prompt(root, body):
    """Render GA official /review inline prompt for Admin Chat.

    Keep this logic in the worker so Admin reuses the current in-session agent
    instead of treating /review as an autocomplete-only text snippet.
    """
    lang = os.environ.get('GA_LANG', '').strip().lower()
    en = lang == 'en'
    fname = 'review_inline_prompt.en.txt' if en else 'review_inline_prompt.txt'
    fpath = Path(root) / 'memory' / 'review_sop' / fname
    default_request = (
        '(no specific request — default to uncommitted diff: run `git diff --stat HEAD` and `git diff HEAD`)'
        if en else
        '(无具体请求 — 默认审本次 uncommitted 改动:用 code_run 跑 `git diff --stat HEAD` 与 `git diff HEAD`)'
    )
    user_request = body or default_request
    header = (
        '> 🔍 /review (in-session) → main agent reviews here, echoes the report inline\n\n'
        if en else
        '> 🔍 /review (in-session) → 主 agent 当场审,直接 echo 报告\n\n'
    )
    fallback = (
        '[/review in-session] (⚠️ prompt 文件缺失: {fpath} → {err})\n\n'
        '# 本轮用户请求\n{user_request}\n\n'
        '请按 memory/code_review_principles.md 评审,直接 echo 报告到对话。\n'
        '不要写 review.md,不要打 [ROUND END]。'
    )
    try:
        template = fpath.read_text(encoding='utf-8')
        rendered = template.format(user_request=user_request, ga_root=str(Path(root)).replace('\\', '/'))
    except Exception as e:
        rendered = fallback.format(fpath=str(fpath), err=e, user_request=user_request)
    return header + rendered


def _review_help_text():
    return '## /review\n\n**用途**：在当前会话内执行对抗式代码审阅。\n\n**用法**\n\n```text\n/review\n/review <自然语言请求>\n/review help\n```\n\n- `/review`：默认审阅本次 uncommitted 改动，由主 agent 在会话内读取 `git diff`。\n- `/review <自然语言请求>`：按你描述的范围或关注点审阅。\n- `/review help`：显示这份帮助，不启动审阅。\n\n**示例**\n\n```text\n/review\n/review 我刚改了 review_cmd.py 和 tuiapp_v2.py，关注 prompt 注入\n/review 审 frontends 目录下所有改过的文件\n```\n\n**产出**：直接在对话中返回 Markdown；不写文件、不开 subagent。\n\n**协议**：`memory/review_sop/review_inline_prompt.txt` + `memory/code_review_principles.md`'


def _improve_help_text():
    return '## /improve\n\n**用途**：将 `/improve` 转换为内置记忆提炼请求。\n\n**等价消息**\n\n```text\n依据 memory_management_sop.md，提取成功经验总结为 skill 写入 L3，并更新 L1 索引\n```'


def _maybe_handle_review_command(root, prompt):
    s = (prompt or '').strip()
    if s == '/review':
        return _render_review_prompt(root, ''), None
    if s.startswith('/review ') or s.startswith('/review\t'):
        body = s[len('/review'):].strip()
        if body in ('help', '?', '-h', '--help'):
            return None, _review_help_text()
        return _render_review_prompt(root, body), None
    return prompt, None


def _maybe_handle_improve_command(prompt):
    s = (prompt or '').strip()
    if s == '/improve':
        return '依据 memory_management_sop.md，提取成功经验总结为 skill 写入 L3，并更新 L1 索引', None
    if s.startswith('/improve ') or s.startswith('/improve\t'):
        body = s[len('/improve'):].strip()
        if body in ('help', '?', '-h', '--help'):
            return None, _improve_help_text()
    return prompt, None


def _maybe_handle_continue_command(root, agent, prompt):
    s = (prompt or '').strip()
    if s != '/continue' and not s.startswith('/continue ') and not s.startswith('/continue\t'):
        return None
    try:
        root = Path(root or Path.cwd()).resolve()
        if str(root) not in sys.path:
            sys.path.insert(0, str(root))
        from frontends import continue_cmd
        return continue_cmd.handle_frontend_command(agent, s, exclude_pid=os.getpid())
    except Exception as e:
        return '❌ /continue 执行失败：%s\n%s' % (e, traceback.format_exc())


def _emit_immediate_done(agent, content, history_info=None, working=None):
    msg = {'id': new_id(), 'role': 'assistant', 'content': content, 'created_at': int(time.time())}
    state = _snapshot_ga_state(agent)
    emit({'type': 'done', 'message': msg, 'usage': _snapshot_usage(), 'usages': _snapshot_turn_usages(), 'raw_history': _snapshot_backend_history(agent), 'history_info': state.get('history_info') or history_info or [], 'working': state.get('working') or working or {}, 'reasoning_effort': _snapshot_reasoning_effort(agent)})



def _safe_workspace(root, value):
    raw = (value or '').strip() if isinstance(value, str) else ''
    if not raw:
        return None
    try:
        p = Path(raw).expanduser().resolve()
    except Exception:
        return None
    if not p.exists() or not p.is_dir():
        return None
    return p


def _apply_workspace(agent, root, workspace):
    ws = _safe_workspace(root, workspace)
    if not ws:
        try:
            os.chdir(root)
        except Exception:
            pass
        os.environ.pop('GA_WORKSPACE', None)
        return ''
    os.environ['GA_WORKSPACE'] = str(ws)
    os.environ['GA_PROJECT_ROOT'] = str(ws)
    try:
        os.chdir(ws)
    except Exception:
        pass
    for name in ('workspace', 'workspace_root', 'project_root', 'cwd'):
        try:
            setattr(agent, name, str(ws))
        except Exception:
            pass
    try:
        w = getattr(agent, 'working', None)
        if isinstance(w, dict):
            w['workspace'] = str(ws)
            w['project_root'] = str(ws)
    except Exception:
        pass
    return str(ws)

def handle_request(agent, worker, req):
    _reset_usage()  # Clear usage accumulator for this turn
    prompt = req.get('prompt') or ''
    history = req.get('history') or []
    raw_history = req.get('raw_history') or []
    history_info = req.get('history_info') or []
    working = req.get('working') or {}
    llm_no = int(req.get('llm_no') or 0)
    tools_mode = str(req.get('tools_mode') or 'official')
    reasoning_effort = req.get('reasoning_effort') if 'reasoning_effort' in req else None
    root_for_req = Path(req.get('ga_root') or Path.cwd()).resolve()
    _select_llm_if_needed(agent, llm_no)
    if str(reasoning_effort or '').strip():
        _apply_reasoning_effort_setting(agent, reasoning_effort)
    _restore_ga_state(agent, history_info, working)
    applied_workspace = _apply_workspace(agent, root_for_req, req.get('workspace'))
    if applied_workspace and isinstance(working, dict):
        working['workspace'] = applied_workspace
        working['project_root'] = applied_workspace
    _restore_admin_history(agent, history, raw_history)
    immediate_done = _maybe_handle_continue_command(root_for_req, agent, prompt)
    if immediate_done is not None:
        _emit_immediate_done(agent, immediate_done, history_info, working)
        return
    prompt, immediate_done = _maybe_handle_review_command(req.get('ga_root') or Path.cwd(), prompt)
    if immediate_done is not None:
        _emit_immediate_done(agent, immediate_done, history_info, working)
        return
    prompt, immediate_done = _maybe_handle_improve_command(prompt)
    if immediate_done is not None:
        _emit_immediate_done(agent, immediate_done, history_info, working)
        return
    immediate_done = _maybe_handle_effort_command(agent, prompt)
    if immediate_done is not None:
        _emit_immediate_done(agent, immediate_done, history_info, working)
        return
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
                emit({'type': 'done', 'message': msg, 'usage': usage, 'usages': usages, 'raw_history': _snapshot_backend_history(agent), 'history_info': state.get('history_info') or [], 'working': state.get('working') or {}, 'reasoning_effort': _snapshot_reasoning_effort(agent)})
                return
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        usage = _snapshot_usage()
        usages = _snapshot_turn_usages()
        emit({'type': 'error', 'message': msg, 'usage': usage, 'usages': usages, 'raw_history': _snapshot_backend_history(agent), 'reasoning_effort': _snapshot_reasoning_effort(agent)})


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
                    root_for_req = Path(req.get('ga_root') or root).resolve()
                    _apply_workspace(agent, root_for_req, req.get('workspace'))
                    emit({'type': 'reinject_tools', **_reinject_tools(agent)})
                continue
            with agent_lock:
                handle_request(agent, worker, req)
        except Exception as e:
            msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
            emit({'type': 'error', 'message': msg})


if __name__ == '__main__':
    main()
