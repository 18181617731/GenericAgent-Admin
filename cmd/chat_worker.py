import json, os, sys, time, traceback, threading, queue, subprocess, re, socket
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
_ultraplan_ctx = [None]  # [objective] when in ultraplan mode, else [None]


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


def _snapshot_model_id(agent):
    """Return the active backend's concrete model ID for this reply."""
    try:
        value = getattr(agent.llmclient.backend, 'model', '')
        if not isinstance(value, str):
            return ''
        return ' '.join(value.split())[:256]
    except Exception:
        return ''


def _json_clone(value, fallback):
    try:
        return json.loads(json.dumps(value, ensure_ascii=False, default=str))
    except Exception:
        return fallback


def _coerce_llm_no(value):
    """Return a safe non-negative model index for untrusted Admin input."""
    if value is None or isinstance(value, bool):
        return 0
    try:
        return max(0, int(value))
    except (TypeError, ValueError, OverflowError):
        return 0


def _resolve_request_root(value, fallback):
    """Resolve a request root without letting malformed JSON break the worker."""
    candidate = value or fallback
    if not isinstance(candidate, (str, os.PathLike)):
        candidate = fallback
    try:
        return Path(candidate).resolve()
    except (OSError, TypeError, ValueError):
        return Path(fallback).resolve()


def _normalize_request(req):
    """Normalize the JSON boundary while preserving the existing chat contract."""
    if not isinstance(req, dict):
        raise ValueError('chat worker request must be a JSON object')
    normalized = dict(req)
    normalized['prompt'] = normalized.get('prompt') if isinstance(normalized.get('prompt'), str) else ''
    for name in ('history', 'raw_history', 'history_info'):
        value = normalized.get(name)
        normalized[name] = value if isinstance(value, list) else []
    working = normalized.get('working')
    normalized['working'] = working if isinstance(working, dict) else {}
    normalized['llm_no'] = _coerce_llm_no(normalized.get('llm_no'))
    if normalized.get('ga_root') is not None and not isinstance(normalized.get('ga_root'), (str, os.PathLike)):
        normalized['ga_root'] = None
    if not isinstance(normalized.get('project_mode'), str):
        normalized['project_mode'] = ''
    prompts = normalized.get('extra_sys_prompts')
    if not isinstance(prompts, list):
        prompts = []
    normalized['extra_sys_prompts'] = [str(value).strip() for value in prompts if str(value).strip()]
    if normalized.get('reasoning_effort') is not None and not isinstance(normalized.get('reasoning_effort'), str):
        normalized['reasoning_effort'] = None
    return normalized


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


EFFORT_LEVELS = ('off', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max')


def _snapshot_reasoning_effort(agent):
    try:
        backend = getattr(getattr(agent, 'llmclient', None), 'backend', None)
        value = getattr(backend, 'reasoning_effort', None) if backend is not None else None
    except Exception:
        value = None
    value = str(value or '').strip().lower()
    if value in EFFORT_LEVELS:
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
    if raw in ('', 'off', 'clear', 'unset'):
        effort = None
    elif raw in EFFORT_LEVELS:
        effort = raw
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


def _maybe_expand_official_slash_command(root, prompt):
    s = str(prompt or '').strip()
    if not s.startswith('/'):
        return prompt
    parts = s.split(maxsplit=1)
    cmd = parts[0].lower()
    args = parts[1] if len(parts) > 1 else ''
    if cmd not in ('/update', '/autorun', '/morphling', '/goal', '/hive', '/conductor'):
        return prompt
    try:
        if str(root) not in sys.path:
            sys.path.insert(0, str(root))
        from frontends import slash_cmds
        rendered = slash_cmds.prompt_for(cmd, args)
        return rendered or prompt
    except Exception:
        return prompt


def _emit_immediate_done(agent, content, history_info=None, working=None):
    msg = {'id': new_id(), 'role': 'assistant', 'content': content, 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent)}
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

def _ultraplan_slug(text):
    slug = re.sub(r'[^a-zA-Z0-9]+', '_', str(text or '')).strip('_').lower()
    return (slug[:60] or 'objective')


def _ultraplan_task_match_key(text):
    """Normalize task/file text for Admin-side UltraPlan output matching."""
    return re.sub(r'[^a-z0-9]+', '_', str(text or '').lower()).strip('_')


def _build_ultraplan_output_index(run_dir_str):
    """Map dashboard task descriptions to live .out.txt files without touching ga_ultraplan.py."""
    index = {}
    try:
        run_dir = Path(run_dir_str)
        if not run_dir.exists():
            return index
        for out_file in sorted(run_dir.glob('*.out.txt')):
            name = out_file.name
            task_id = name[:-8] if name.endswith('.out.txt') else out_file.stem
            clean = re.sub(r'^\d+_', '', task_id)
            aliases = {task_id, clean}
            parts = [p for p in clean.split('_') if p]
            # Stems often include a source prefix (admin_chat_*).  Add suffix aliases so
            # dashboard labels like "architecture lens" match 001_admin_chat_architecture_lens.
            for i in range(len(parts)):
                suffix = '_'.join(parts[i:])
                if suffix:
                    aliases.add(suffix)
            meta = {
                'id': task_id,
                'task_id': task_id,
                'output_file': str(out_file),
                'file': str(out_file),
                'path': str(out_file),
            }
            for alias in aliases:
                key = _ultraplan_task_match_key(alias)
                if key:
                    index.setdefault(key, meta)
    except Exception:
        pass
    return index


def _enrich_ultraplan_task_with_output(task, output_index):
    """Attach id/output_file to parsed dashboard tasks when a matching .out.txt exists."""
    if not isinstance(task, dict) or not output_index:
        return task
    candidates = []
    desc = task.get('desc') or task.get('name') or task.get('title') or ''
    key = _ultraplan_task_match_key(desc)
    if key:
        candidates.append(key)
        words = [p for p in key.split('_') if p]
        for i in range(len(words)):
            suffix = '_'.join(words[i:])
            if suffix:
                candidates.append(suffix)
    # Prefer exact/desc-suffix match, then file aliases that end with the desc key.
    meta = None
    for candidate in candidates:
        meta = output_index.get(candidate)
        if meta:
            break
    if meta is None and key:
        suffix = '_' + key
        for alias, alias_meta in output_index.items():
            if alias == key or alias.endswith(suffix):
                meta = alias_meta
                break
    if not meta:
        return task
    enriched = dict(task)
    # A dashboard task may already carry its input prompt (`<stem>.txt`). Once
    # the matching generated output exists, every path alias must point at that
    # `.out.txt`; setdefault would preserve the input path in output_file.
    for k, v in meta.items():
        enriched[k] = v
    return enriched


def _enrich_ultraplan_phase_outputs(phase, output_index):
    if not isinstance(phase, dict):
        return phase
    phase['tasks'] = [_enrich_ultraplan_task_with_output(t, output_index)
                      for t in phase.get('tasks', [])]
    phase['children'] = [_enrich_ultraplan_phase_outputs(ch, output_index)
                         for ch in phase.get('children', [])]
    return phase


def _emit_ultraplan_line(emit, line, state):
    """Parse a single UltraPlan marker line and update state dict, then emit ultraplan_event."""
    import re
    s = line.strip()
    # [ultraplan] objective: xxx
    m = re.match(r'^\[ultraplan\]\s*objective[:\s]+(.+)$', s, re.IGNORECASE)
    if m:
        state.setdefault('objective', m.group(1).strip())
        emit({'type': 'ultraplan_event', 'state': dict(state)})
        return
    # [phase] name - desc
    m = re.match(r'^\[phase\]\s*(.+)$', s, re.IGNORECASE)
    if m:
        rest = m.group(1).strip()
        parts = rest.split(' - ', 1)
        name = parts[0].strip()
        desc = parts[1].strip() if len(parts) > 1 else ''
        phases = state.setdefault('phases', [])
        if not any(p['name'] == name for p in phases):
            phases.append({'name': name, 'desc': desc, 'status': 'running', 'tasks': []})
        else:
            for p in phases:
                if p['name'] == name:
                    p['status'] = 'running'
        emit({'type': 'ultraplan_event', 'state': dict(state)})
        return
    # [done] name (Xs)
    m = re.match(r'^\[done\]\s*(.+?)(?:\s*\([^)]*\))?\s*$', s, re.IGNORECASE)
    if m:
        name = m.group(1).strip()
        for p in state.get('phases', []):
            if p['name'] == name:
                p['status'] = 'done'
            for t in p.get('tasks', []):
                if t['desc'] == name:
                    t['status'] = 'done'
        emit({'type': 'ultraplan_event', 'state': dict(state)})
        return
    # [fail] name (Xs)
    m = re.match(r'^\[fail\]\s*(.+?)(?:\s*\([^)]*\))?\s*$', s, re.IGNORECASE)
    if m:
        name = m.group(1).strip()
        for p in state.get('phases', []):
            if p['name'] == name:
                p['status'] = 'fail'
            for t in p.get('tasks', []):
                if t['desc'] == name:
                    t['status'] = 'fail'
        emit({'type': 'ultraplan_event', 'state': dict(state)})
        return
    # [subagent] desc -> path
    m = re.match(r'^\[subagent\]\s*(.+)$', s, re.IGNORECASE)
    if m:
        rest = m.group(1).strip()
        parts = rest.split('->')
        desc = parts[0].strip()
        task_entry = {'desc': desc, 'status': 'running'}
        if len(parts) > 1:
            path_part = parts[1].strip()
            # Extract file stem (e.g. "001_failure_modes_lens" from "001_failure_modes_lens.out.txt")
            basename = path_part.replace('\\', '/').rsplit('/', 1)[-1]
            stem = basename
            if stem.endswith('.out.txt'):
                stem = stem[:-8]
            elif stem.endswith('.txt'):
                stem = stem[:-4]
            elif '.' in stem:
                stem = stem.rsplit('.', 1)[0]
            if stem:
                task_entry['id'] = stem
            if path_part:
                # Derive .out.txt (actual output) from .txt (input/prompt file)
                if path_part.endswith('.out.txt'):
                    out_path = path_part
                elif path_part.endswith('.txt'):
                    out_path = path_part[:-4] + '.out.txt'
                else:
                    out_path = path_part + '.out.txt'
                task_entry['output_file'] = out_path
        phases = state.get('phases', [])
        if phases:
            phases[-1].setdefault('tasks', []).append(task_entry)
        emit({'type': 'ultraplan_event', 'state': dict(state)})
        return


def _maybe_handle_ultraplan_command(root, prompt):
    s = (prompt or '').strip()
    if s != '/ultraplan' and not s.startswith('/ultraplan ') and not s.startswith('/ultraplan\t'):
        _ultraplan_ctx[0] = None
        return None, prompt, None
    objective = s[len('/ultraplan'):].strip()
    if not objective:
        _ultraplan_ctx[0] = None
        return None, None, (
            'UltraPlan mode is explicit opt-in only.\n\n'
            'Usage: `/ultraplan <objective>`\n\n'
            'Normal chat is unchanged; only this slash command invokes UltraPlan.'
        )
    _ultraplan_ctx[0] = objective
    return objective, None, None


def _is_ultraplan_marker_line(line):
    s = str(line or '').strip().lower()
    return s.startswith(('[phase]', '[done]', '[fail]', '[subagent]', '[ultraplan]', '[next]'))


def _emit_ultraplan_text(emit, text, state, chunks, buf):
    if not text:
        return
    buf[0] += text
    while '\n' in buf[0]:
        line, buf[0] = buf[0].split('\n', 1)
        stripped = line.strip()
        if stripped and _is_ultraplan_marker_line(stripped):
            _emit_ultraplan_line(emit, stripped, state)
        else:
            visible = line + '\n'
            chunks.append(visible)
            emit({'type': 'delta', 'delta': visible})


def _drain_ultraplan_buf(emit, state, chunks, buf):
    if not buf[0]:
        return
    stripped = buf[0].strip()
    if stripped and _is_ultraplan_marker_line(stripped):
        _emit_ultraplan_line(emit, stripped, state)
    else:
        chunks.append(buf[0])
        emit({'type': 'delta', 'delta': buf[0]})
    buf[0] = ''


def _build_ultraplan_script(root, run_dir, objective, llm_no=0):
    root = Path(root).resolve()
    run_dir = Path(run_dir).resolve()
    lines = [
        '# Auto-generated by GA Admin Chat /ultraplan.',
        'import os, sys',
        'ROOT = ' + json.dumps(str(root), ensure_ascii=False),
        'RUN_DIR = ' + json.dumps(str(run_dir), ensure_ascii=False),
        'OBJECTIVE = ' + json.dumps(str(objective or ''), ensure_ascii=False),
        'LLM_NO = ' + json.dumps(int(llm_no or 0)),
        'BOUNDARY = "Do not start UltraPlan. Do not delegate. If decomposition is needed, report blocker only."',
        'ARTIFACT = f"Save any artifacts under {RUN_DIR}; return paths."',
        'os.makedirs(RUN_DIR, exist_ok=True)',
        'if ROOT not in sys.path:',
        '    sys.path.insert(0, ROOT)',
        'from assets.ga_ultraplan import plan, phase, parallel',
        'plan(RUN_DIR)',
        "print('[ultraplan] objective: ' + OBJECTIVE, flush=True)",
        '',
        "# Phase 1: Explore - fan out by independent lenses",
        "with phase('explore', 'analyze objective from multiple independent angles'):",
        "    explore_lenses = parallel([",
        "        {'desc': 'architecture lens', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nAnalyze the architecture/technical stack/core components needed. What systems/files/APIs are involved? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "        {'desc': 'failure modes lens', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nList potential failure scenarios, edge cases, and risks. What could go wrong? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "        {'desc': 'user intent lens', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nWhat is the user truly trying to achieve? What is the background/context/real need? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "        {'desc': 'data evidence lens', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nWhat data/files/external information is needed? Where to find it? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "        {'desc': 'constraints lens', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nIdentify time/resource/security constraints and boundaries. What are the limits? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "    ], max_workers=5)",
        "    print('[explore] completed ' + str(len(explore_lenses)) + ' lenses', flush=True)",
        '',
        "# Phase 2: Execute - decompose into parallel subtasks",
        "with phase('execute', 'break down and run independent subtasks'):",
        "    # Reducer: synthesize exploration results and design subtasks",
        "    synthesis_prompt = f'Objective: {OBJECTIVE}\\\\n\\\\nExploration results:\\\\n' + '\\\\n---\\\\n'.join([f'Lens {i+1}: {r}' for i, r in enumerate(explore_lenses)]) + f'\\\\n\\\\nBased on these lenses, break the objective into 3-5 independent, concrete subtasks that can run in parallel. For each subtask, provide: (1) ID, (2) description, (3) what it will do, (4) expected output. Return as structured list. Be concise. {BOUNDARY}'",
        "    subtasks_design = parallel([{'desc': 'synthesize subtasks', 'prompt': synthesis_prompt, 'llm_no': LLM_NO, 'timeout': 1800}], max_workers=1)[0]",
        "    print('[execute] subtasks designed: ' + str(subtasks_design)[:200], flush=True)",
        "    ",
        "    # Execute subtasks in parallel (static 4 workers for now; ideally parse subtasks_design)",
        "    exec_results = parallel([",
        "        {'desc': 'subtask 1', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nSubtasks plan:\\\\n{subtasks_design}\\\\n\\\\nExecute ONLY subtask 1. {ARTIFACT} Return results. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 3600},",
        "        {'desc': 'subtask 2', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nSubtasks plan:\\\\n{subtasks_design}\\\\n\\\\nExecute ONLY subtask 2. {ARTIFACT} Return results. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 3600},",
        "        {'desc': 'subtask 3', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nSubtasks plan:\\\\n{subtasks_design}\\\\n\\\\nExecute ONLY subtask 3. {ARTIFACT} Return results. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 3600},",
        "        {'desc': 'subtask 4', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nSubtasks plan:\\\\n{subtasks_design}\\\\n\\\\nExecute ONLY subtask 4 if exists, otherwise return empty. {ARTIFACT} Return results. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 3600},",
        "    ], max_workers=4)",
        "    print('[execute] completed ' + str(len(exec_results)) + ' subtasks', flush=True)",
        '',
        "# Phase 3: Verify & Summarize",
        "with phase('verify', 'validate results and produce final report'):",
        "    verify_results = parallel([",
        "        {'desc': 'completeness check', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nExecution results:\\\\n' + '\\\\n---\\\\n'.join([f'Result {i+1}: {r}' for i, r in enumerate(exec_results)]) + f'\\\\n\\\\nCheck: are all parts of the objective covered? Any gaps/errors? {ARTIFACT} Return findings. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "        {'desc': 'final integration', 'prompt': f'Objective: {OBJECTIVE}\\\\n\\\\nExecution results:\\\\n' + '\\\\n---\\\\n'.join([f'Result {i+1}: {r}' for i, r in enumerate(exec_results)]) + f'\\\\n\\\\nIntegrate all results into a coherent final report. {ARTIFACT} Return report. Be concise. {BOUNDARY}', 'llm_no': LLM_NO, 'timeout': 1800},",
        "    ], max_workers=2)",
        "    print('[verify] validation complete', flush=True)",
        "    print('[summary] run_dir: ' + RUN_DIR, flush=True)",
        "    print('[summary] final_report: ' + str(verify_results[1])[:500], flush=True)",
        '',
    ]
    return '\n'.join(lines)


def _parse_ultraplan_dashboard(html_text, run_dir_str):
    """Parse 47831 dashboard HTML <pre> text, extract state for matching run_dir."""
    import html as _html, re as _re
    m = _re.search(r'<pre>(.*?)</pre>', html_text, _re.DOTALL)
    if not m:
        return None
    text = _html.unescape(m.group(1))

    # Find our session block by matching rundir:
    in_our = False
    session_lines = []
    for raw in text.split('\n'):
        line = raw.rstrip()
        if line.startswith('rundir:'):
            rd = line[len('rundir:'):].strip()
            in_our = (rd.replace('\\', '/') == run_dir_str.replace('\\', '/'))
            session_lines = []
            continue
        if line.startswith('== ') and line.endswith(' =='):
            if in_our:
                break  # next session started
            in_our = False
            continue
        if in_our:
            session_lines.append(line)

    if not session_lines:
        return None

    result = {'phases': [], 'current': '', 'tasks': [], 'events': [], 'done': False}
    section = None
    phase_stack = []  # list of (indent_level, phase_dict)

    for line in session_lines:
        if line.startswith('current:'):
            result['current'] = line[len('current:'):].strip()
            continue
        ls = line.strip()
        if ls == 'phases:':
            section = 'phases'
            phase_stack = []
            continue
        if ls == 'recent tasks:':
            section = 'tasks'
            continue
        if ls == 'events:':
            section = 'events'
            continue
        if not ls:
            continue

        if section == 'phases':
            # Phase line: "{indent}(>>|  ) {status:<7} {name}[ - {desc}][ | parallel: N tasks]"
            pm = _re.match(r'^(\s*)(>>|  ) (\w+)\s+(.+)$', line)
            if pm:
                indent = len(pm.group(1))
                active = pm.group(2).strip() == '>>'
                status_raw = pm.group(3)
                rest = _re.sub(r'\s*\|\s*parallel:.*$', '', pm.group(4)).strip()
                parts = rest.split(' - ', 1)
                name = parts[0].strip()
                desc = parts[1].strip() if len(parts) > 1 else ''
                status = 'running' if status_raw == 'run' else status_raw
                phase = {'name': name, 'desc': desc, 'status': status,
                         'active': active, 'tasks': [], 'children': []}
                while phase_stack and phase_stack[-1][0] >= indent:
                    phase_stack.pop()
                if phase_stack:
                    phase_stack[-1][1]['children'].append(phase)
                else:
                    result['phases'].append(phase)
                phase_stack.append((indent, phase))
                continue
            # Task line: "{indent}   - {status:<5} {desc}"
            tm = _re.match(r'^(\s+)- ?(\w+)\s+(.+)$', line)
            if tm and phase_stack:
                status_raw = tm.group(2)
                status = 'running' if status_raw == 'run' else status_raw
                phase_stack[-1][1]['tasks'].append(
                    {'desc': tm.group(3).strip(), 'status': status})

        elif section == 'tasks':
            tm = _re.match(r'^\s+(\w+)\s+(.+)$', line)
            if tm:
                result['tasks'].append({'status': tm.group(1), 'desc': tm.group(2).strip()})

        elif section == 'events':
            em = _re.match(r'^\s+([\d.]+)s\s+(.+)$', line)
            if em:
                result['events'].append({'time': float(em.group(1)), 'msg': em.group(2).strip()})

    output_index = _build_ultraplan_output_index(run_dir_str)
    if output_index:
        result['phases'] = [_enrich_ultraplan_phase_outputs(p, output_index)
                            for p in result.get('phases', [])]
        result['tasks'] = [_enrich_ultraplan_task_with_output(t, output_index)
                           for t in result.get('tasks', [])]

    # All phases done when none are in 'run'/'running' state
    if result['phases'] and all(
            p.get('status') not in ('run', 'running') for p in result['phases']):
        result['done'] = True

    return result


def _allocate_ultraplan_port():
    """Return a currently-free localhost port for one UltraPlan dashboard."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(('127.0.0.1', 0))
        return int(s.getsockname()[1])


def _poll_ultraplan_daemon(run_dir_str, state, emit, stop_event, dashboard_port):
    """Background thread: poll a per-run UltraPlan dashboard, parse it, and emit ultraplan_event.
    Also tail .out.txt files and emit ultraplan_output for task details."""
    import urllib.request as _urlreq, hashlib as _md5, json as _j
    from pathlib import Path as _Path
    _up_port = int(dashboard_port or 47831)
    url = f'http://127.0.0.1:{_up_port}/'
    last_hash = None
    # Track tail positions: {filename: (size, mtime)}
    tail_state = {}
    run_dir = _Path(run_dir_str)

    # Wait up to 15s for daemon to come up
    for _ in range(15):
        if stop_event.is_set():
            return
        try:
            with _urlreq.urlopen(url, timeout=2) as r:
                r.read()
            break
        except Exception:
            stop_event.wait(1.0)

    while not stop_event.is_set():
        # Poll dashboard
        try:
            with _urlreq.urlopen(url, timeout=3) as r:
                html_bytes = r.read()
            html_text = html_bytes.decode('utf-8', errors='replace')
            parsed = _parse_ultraplan_dashboard(html_text, run_dir_str)
            if parsed:
                new_state = dict(state)
                new_state['phases'] = parsed['phases']
                new_state['current'] = parsed.get('current', '')
                new_state['recent_tasks'] = parsed.get('tasks', [])
                new_state['events'] = parsed.get('events', [])
                # Keep the shared state object fresh so the final done/error message
                # does not overwrite rich dashboard data with the initial header-only state.
                state.update(new_state)
                h = _md5.md5(_j.dumps(parsed, sort_keys=True, default=str).encode()).hexdigest()
                if h != last_hash:
                    last_hash = h
                    emit({'type': 'ultraplan_event', 'state': dict(state)})
        except Exception:
            pass

        # Tail .out.txt files
        if run_dir.exists():
            try:
                for out_file in run_dir.glob('*.out.txt'):
                    stat = out_file.stat()
                    last_size, last_mtime = tail_state.get(str(out_file), (0, 0))
                    # Read new content if file grew or was modified
                    if stat.st_size > last_size or stat.st_mtime > last_mtime:
                        try:
                            with open(out_file, 'r', encoding='utf-8', errors='ignore') as f:
                                f.seek(last_size)
                                new_lines = [line.rstrip('\r\n') for line in f.readlines()]
                            if new_lines:
                                # Extract task_id from filename: "001_task_name.out.txt" -> "001_task_name"
                                name = out_file.name
                                task_id = name[:-8] if name.endswith('.out.txt') else out_file.stem
                                outputs = state.setdefault('task_outputs', {})
                                outputs.setdefault(task_id, []).extend(new_lines)
                                emit({'type': 'ultraplan_output', 'task_id': task_id, 'lines': new_lines})
                            tail_state[str(out_file)] = (stat.st_size, stat.st_mtime)
                        except Exception:
                            pass
            except Exception:
                pass

        stop_event.wait(1.0)


def _run_ultraplan_command(root, objective, llm_no, agent, history_info, working, emit):
    root = Path(root).resolve()
    temp_root = Path(os.environ.get('GA_TEMP') or (root / 'temp')).resolve()
    stamp = time.strftime('%Y%m%d_%H%M%S')
    run_dir = temp_root / ('ultraplan_%s_%s' % (_ultraplan_slug(objective), stamp))
    run_dir.mkdir(parents=True, exist_ok=True)
    script_path = run_dir / 'admin_chat_ultraplan.py'
    script_path.write_text(_build_ultraplan_script(root, run_dir, objective, llm_no), encoding='utf-8')
    dashboard_port = _allocate_ultraplan_port()
    dashboard_url = 'http://127.0.0.1:%s/' % dashboard_port
    state = {'objective': objective, 'run_dir': str(run_dir), 'script': str(script_path), 'phases': [], 'dashboard_port': dashboard_port, 'dashboard_url': dashboard_url}
    emit({'type': 'ultraplan_event', 'state': dict(state)})
    chunks = []
    buf = ['']
    env = os.environ.copy()
    env['PYTHONIOENCODING'] = 'utf-8'
    # Use a per-run UltraPlan dashboard daemon so a previous long exec on the
    # default 47831 daemon cannot block this run's plan() registration.
    env['GA_ULTRAPLAN_PORT'] = str(dashboard_port)
    env['GA_ULTRAPLAN_BROWSER'] = '0'
    env['GA_ULTRAPLAN_RUNDIR'] = str(run_dir)
    cmd = [sys.executable, str(script_path)]

    # Start dashboard polling thread before launching process
    stop_poll = threading.Event()
    poll_thread = threading.Thread(
        target=_poll_ultraplan_daemon,
        args=(str(run_dir), state, emit, stop_poll, dashboard_port),
        daemon=True,
    )
    poll_thread.start()

    proc = subprocess.Popen(
        cmd, cwd=str(root),
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        stdin=subprocess.DEVNULL,
        text=True, encoding='utf-8', errors='replace',
        bufsize=1, env=env,
    )
    rc = 0
    try:
        while True:
            line = proc.stdout.readline() if proc.stdout else ''
            if line:
                _emit_ultraplan_text(emit, line, state, chunks, buf)
                continue
            if proc.poll() is not None:
                break
            time.sleep(0.1)
        rest = proc.stdout.read() if proc.stdout else ''
        if rest:
            _emit_ultraplan_text(emit, rest, state, chunks, buf)
        rc = proc.wait()
        _drain_ultraplan_buf(emit, state, chunks, buf)
    finally:
        stop_poll.set()
        poll_thread.join(timeout=3)
        _ultraplan_ctx[0] = None

    state['complete'] = True
    if rc != 0:
        state['error'] = True
        text = ''.join(chunks) or ('UltraPlan exited with code %s' % rc)
        msg = {'id': new_id(), 'role': 'assistant', 'content': text, 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent), 'error': True, 'ultraplan_state': dict(state)}
        emit({'type': 'error', 'message': msg, 'usage': _snapshot_usage(), 'usages': _snapshot_turn_usages(), 'raw_history': _snapshot_backend_history(agent), 'history_info': history_info or [], 'working': working or {}, 'reasoning_effort': _snapshot_reasoning_effort(agent)})
        return
    emit({'type': 'ultraplan_event', 'state': dict(state)})
    msg = {'id': new_id(), 'role': 'assistant', 'content': ''.join(chunks), 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent), 'ultraplan_state': dict(state)}
    snap = _snapshot_ga_state(agent)
    emit({'type': 'done', 'message': msg, 'usage': _snapshot_usage(), 'usages': _snapshot_turn_usages(), 'raw_history': _snapshot_backend_history(agent), 'history_info': snap.get('history_info') or history_info or [], 'working': snap.get('working') or working or {}, 'reasoning_effort': _snapshot_reasoning_effort(agent)})



_WORLDLINE_HOOK_INSTALLED = False


def _safe_session_id(value):
    value = re.sub(r'[^A-Za-z0-9_.-]+', '_', str(value or 'session')).strip('._')
    return value[:120] or 'session'


def _worldline_root(ga_root):
    sid = _safe_session_id(os.environ.get('GA_ADMIN_SESSION_ID'))
    return Path(ga_root) / 'temp' / 'rewind_data' / 'ga-admin' / sid


def _install_worldline_hook():
    global _WORLDLINE_HOOK_INSTALLED
    if _WORLDLINE_HOOK_INSTALLED:
        return
    from plugins import hooks as plugin_hooks

    def _before(ctx):
        try:
            if (ctx or {}).get('tool_name') not in ('file_write', 'file_patch'):
                return ctx
            handler = ctx.get('self')
            path = (ctx.get('args') or {}).get('path')
            store = getattr(getattr(handler, 'parent', None), '_admin_worldline_store', None)
            if store is not None and path:
                store.track_pre_edit(handler._get_abs_path(path))
        except Exception:
            pass
        return ctx

    plugin_hooks.register('tool_before')(_before)
    _WORLDLINE_HOOK_INSTALLED = True


def _ensure_worldline_store(agent, ga_root, workspace):
    from frontends.worldline import RewindStore
    cwd = os.path.realpath(str(workspace or ga_root))
    store = getattr(agent, '_admin_worldline_store', None)
    if store is not None:
        if os.path.realpath(store.cwd) != cwd:
            raise RuntimeError('worldline workspace changed within this chat session')
        return store
    store = RewindStore(str(_worldline_root(ga_root)), cwd)
    agent._admin_worldline_store = store
    _install_worldline_hook()
    return store


_WORLDLINE_PROJECT_MODE_BLOCK_RE = re.compile(
    r"\s*-{3,}\s*\[PROJECT MODE:.*?(?:\n-{3,}\s*|$)", re.DOTALL,
)


def _strip_worldline_project_mode(text):
    return _WORLDLINE_PROJECT_MODE_BLOCK_RE.sub('', text or '')


def _worldline_title(store, history, fallback):
    parent = store.head if store.head in store.nodes else store.root_id
    parent_len = len(store.rebuild_history(parent)) if parent is not None else 0
    for item in (history or [])[parent_len:]:
        if isinstance(item, dict) and str(item.get('role') or '').lower() == 'user':
            text = _chat_content_text(item.get('content')).strip()
            if text:
                return _strip_worldline_project_mode(text).replace('\n', ' ').strip()[:160]
    return str(fallback or 'checkpoint').replace('\n', ' ').strip()[:160] or 'checkpoint'


def _commit_worldline(agent, prompt):
    store = getattr(agent, '_admin_worldline_store', None)
    if store is None:
        return None
    history = _snapshot_backend_history(agent)
    parent = store.head if store.head in store.nodes else store.root_id
    parent_len = len(store.rebuild_history(parent)) if parent is not None else 0
    if len(history) <= parent_len:
        store._touched.clear()
        store.save()
        return None
    state = _snapshot_ga_state(agent)
    working = state.get('working') if isinstance(state, dict) else {}
    key_info = working.get('key_info') if isinstance(working, dict) else None
    return store.commit(
        _worldline_title(store, history, prompt), history=history,
        hist_info=state.get('history_info') if isinstance(state, dict) else None,
        key_info=key_info if isinstance(key_info, str) else None,
    )


_WORLDLINE_SID_RE = re.compile(r'^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$')
_WORLDLINE_SIDECAR_SCHEMA = 1
_WORLDLINE_PUBLIC_SCHEMA = 1
_WORLDLINE_PUBLIC_NODE_LIMIT = 1000
_WORLDLINE_SIDECAR_LOCKS = {}
_WORLDLINE_SIDECAR_LOCKS_GUARD = threading.Lock()


def _worldline_sid(value):
    sid = str(value or '')
    if not _WORLDLINE_SID_RE.fullmatch(sid):
        raise ValueError('invalid worldline sid')
    return sid


def _worldline_sidecar_path(ga_root, sid):
    return Path(ga_root) / 'temp' / 'rewind_data' / 'ga-admin' / 'admin_sidecars' / (_worldline_sid(sid) + '.json')


def _worldline_sidecar_lock(path):
    key = os.path.normcase(os.path.abspath(str(path)))
    with _WORLDLINE_SIDECAR_LOCKS_GUARD:
        lock = _WORLDLINE_SIDECAR_LOCKS.get(key)
        if lock is None:
            lock = threading.RLock()
            _WORLDLINE_SIDECAR_LOCKS[key] = lock
        return lock


def _empty_worldline_sidecar(sid):
    return {
        'schema_version': _WORLDLINE_SIDECAR_SCHEMA,
        'sid': sid,
        'next_ordinal': 1,
        'bindings': {},
        # Physical conv/code bridge -> logical conversation node. Bridges remain
        # in the core store; Admin folds them out of its message-version tree.
        'aliases': {},
    }


def _load_worldline_sidecar(ga_root, sid):
    sid = _worldline_sid(sid)
    path = _worldline_sidecar_path(ga_root, sid)
    if not path.exists():
        return _empty_worldline_sidecar(sid), 'missing'
    try:
        with path.open('r', encoding='utf-8') as f:
            data = json.load(f)
    except (OSError, ValueError, TypeError):
        return _empty_worldline_sidecar(sid), 'malformed'
    if not isinstance(data, dict) or data.get('schema_version') != _WORLDLINE_SIDECAR_SCHEMA:
        return _empty_worldline_sidecar(sid), 'legacy'
    if data.get('sid') != sid or not isinstance(data.get('bindings'), dict):
        return _empty_worldline_sidecar(sid), 'malformed'
    clean = _empty_worldline_sidecar(sid)
    next_ordinal = data.get('next_ordinal')
    if isinstance(next_ordinal, int) and not isinstance(next_ordinal, bool) and next_ordinal > 0:
        clean['next_ordinal'] = next_ordinal
    max_ordinal = 0
    for node_id, binding in data['bindings'].items():
        if not isinstance(node_id, str) or not isinstance(binding, dict):
            continue
        user_id = binding.get('user_message_id')
        assistant_id = binding.get('assistant_message_id')
        if not isinstance(user_id, str) or not user_id or not isinstance(assistant_id, str) or not assistant_id:
            continue
        ordinal = binding.get('ordinal')
        if not isinstance(ordinal, int) or isinstance(ordinal, bool) or ordinal < 1:
            ordinal = clean['next_ordinal']
            clean['next_ordinal'] += 1
        created_at = binding.get('created_at')
        if not isinstance(created_at, int) or isinstance(created_at, bool) or created_at < 0:
            created_at = 0
        max_ordinal = max(max_ordinal, ordinal)
        clean['bindings'][node_id] = {
            'user_message_id': user_id,
            'assistant_message_id': assistant_id,
            'display_path': _json_clone(binding.get('display_path'), None),
            'ordinal': ordinal,
            'created_at': created_at,
        }
    clean['next_ordinal'] = max(clean['next_ordinal'], max_ordinal + 1)
    aliases = data.get('aliases', {})
    if isinstance(aliases, dict):
        for physical_id, logical_id in aliases.items():
            if (isinstance(physical_id, str) and physical_id and
                    isinstance(logical_id, str) and logical_id and
                    physical_id != logical_id):
                clean['aliases'][physical_id] = logical_id
    return clean, 'ok'


def _save_worldline_sidecar(ga_root, sid, data):
    path = _worldline_sidecar_path(ga_root, sid)
    path.parent.mkdir(parents=True, exist_ok=True)
    tmp = path.with_name(path.name + '.tmp-' + str(os.getpid()) + '-' + new_id())
    try:
        with tmp.open('w', encoding='utf-8', newline='\n') as f:
            json.dump(data, f, ensure_ascii=False, separators=(',', ':'))
            f.flush()
            os.fsync(f.fileno())
        os.replace(str(tmp), str(path))
    finally:
        try:
            tmp.unlink()
        except FileNotFoundError:
            pass


def _worldline_path(store, node_id):
    if not node_id or node_id not in store.nodes:
        return []
    path, seen, current = [], set(), node_id
    while current in store.nodes and current not in seen:
        seen.add(current)
        path.append(current)
        current = store.nodes[current].get('parent')
    path.reverse()
    return path


def _logical_worldline_node(sidecar, node_id):
    aliases = sidecar.get('aliases', {}) if isinstance(sidecar, dict) else {}
    current, seen = node_id, set()
    while isinstance(current, str) and current:
        if current in seen:
            return node_id
        seen.add(current)
        target = aliases.get(current)
        if not isinstance(target, str) or not target:
            return current
        current = target
    return node_id


def _record_worldline_alias(ga_root, sid, physical_id, logical_id):
    if not isinstance(physical_id, str) or not physical_id:
        return
    if not isinstance(logical_id, str) or not logical_id or physical_id == logical_id:
        return
    path = _worldline_sidecar_path(ga_root, sid)
    with _worldline_sidecar_lock(path):
        sidecar, _ = _load_worldline_sidecar(ga_root, sid)
        logical_id = _logical_worldline_node(sidecar, logical_id)
        if physical_id == logical_id:
            return
        sidecar['aliases'][physical_id] = logical_id
        _save_worldline_sidecar(ga_root, sid, sidecar)


def _bind_worldline_head(store, ga_root, sid, req):
    if req.get('turn_status') != 'completed' or req.get('has_final_answer') is not True:
        raise ValueError('worldline binding requires a completed final answer')
    node_id = str(req.get('node_id') or store.head or '')
    if not node_id or node_id != store.head or node_id not in store.nodes:
        raise ValueError('worldline binding requires the current completed head')
    user_id = str(req.get('user_message_id') or '')
    assistant_id = str(req.get('assistant_message_id') or '')
    if not user_id or not assistant_id:
        raise ValueError('worldline binding requires stable message ids')
    path = _worldline_sidecar_path(ga_root, sid)
    with _worldline_sidecar_lock(path):
        sidecar, _ = _load_worldline_sidecar(ga_root, sid)
        previous = sidecar['bindings'].get(node_id) or {}
        ordinal = previous.get('ordinal')
        created_at = previous.get('created_at')
        if not isinstance(ordinal, int) or isinstance(ordinal, bool) or ordinal < 1:
            ordinal = sidecar['next_ordinal']
            sidecar['next_ordinal'] += 1
        if not isinstance(created_at, int) or isinstance(created_at, bool) or created_at < 1:
            created_at = int(time.time())
        sidecar['bindings'][node_id] = {
            'user_message_id': user_id,
            'assistant_message_id': assistant_id,
            'display_path': _json_clone(req.get('display_path'), None),
            'ordinal': ordinal,
            'created_at': created_at,
        }
        _save_worldline_sidecar(ga_root, sid, sidecar)
    return _json_clone(sidecar['bindings'][node_id], {})


def _worldline_nodes(store, sidecar=None, sidecar_status='missing'):
    from frontends.worldline import tree_from_store
    tree = tree_from_store(store, time.time())
    bindings = sidecar.get('bindings', {}) if isinstance(sidecar, dict) else {}
    aliases = sidecar.get('aliases', {}) if isinstance(sidecar, dict) else {}

    # Only fold aliases that still match the core bridge topology: a bridge and
    # its logical conversation node are siblings. Ignore stale/corrupt entries.
    folded = {}
    for physical_id in aliases:
        if physical_id == tree.root_id or physical_id not in tree.nodes:
            continue
        logical_id = _logical_worldline_node(sidecar, physical_id)
        if logical_id == physical_id or logical_id not in tree.nodes:
            continue
        if tree.nodes[physical_id].parent_id != tree.nodes[logical_id].parent_id:
            continue
        folded[physical_id] = logical_id
    hidden = set(folded)
    visible = [node_id for node_id in tree.nodes if node_id not in hidden]

    public_parent = {}
    for node_id in visible:
        parent_id = tree.nodes[node_id].parent_id
        if parent_id in hidden:
            parent_id = folded[parent_id]
        public_parent[node_id] = parent_id if parent_id in tree.nodes and parent_id not in hidden else None
        if public_parent[node_id] == node_id:
            public_parent[node_id] = None

    # Preserve core traversal order while attaching descendants of an internal
    # bridge to the selected logical node.
    physical_order, visited = [], set()
    stack = [tree.root_id]
    while stack:
        node_id = stack.pop()
        if node_id in visited or node_id not in tree.nodes:
            continue
        visited.add(node_id)
        physical_order.append(node_id)
        stack.extend(reversed(list(tree.nodes[node_id].children)))
    physical_order.extend(node_id for node_id in tree.nodes if node_id not in visited)
    public_children = {node_id: [] for node_id in visible}
    for node_id in physical_order:
        if node_id in hidden or node_id not in public_children:
            continue
        parent_id = public_parent[node_id]
        if parent_id in public_children and node_id not in public_children[parent_id]:
            public_children[parent_id].append(node_id)

    logical_head = folded.get(store.head, store.head)

    def public_path(node_id):
        path, path_seen = [], set()
        while node_id in public_parent and node_id not in path_seen:
            path_seen.add(node_id)
            path.append(node_id)
            node_id = public_parent[node_id]
        path.reverse()
        return path

    current_path = public_path(logical_head)
    ordered, seen = [], set()

    def add(node_id):
        if node_id in seen or node_id not in public_children or len(ordered) >= _WORLDLINE_PUBLIC_NODE_LIMIT:
            return
        seen.add(node_id)
        ordered.append(node_id)

    # Keep the logical active path usable even when a wide tree is truncated.
    for node_id in current_path:
        add(node_id)
    public_root = tree.root_id if tree.root_id in public_children else (visible[0] if visible else None)
    stack = [public_root] if public_root else []
    while stack and len(ordered) < _WORLDLINE_PUBLIC_NODE_LIMIT:
        node_id = stack.pop()
        if node_id in seen or node_id not in public_children:
            continue
        add(node_id)
        stack.extend(reversed(public_children[node_id]))
    if len(ordered) < _WORLDLINE_PUBLIC_NODE_LIMIT:
        for node_id in visible:
            add(node_id)
            if len(ordered) >= _WORLDLINE_PUBLIC_NODE_LIMIT:
                break

    out = []
    for fallback_ordinal, node_id in enumerate(ordered):
        node = tree.nodes[node_id]
        binding = bindings.get(node_id)
        parent_id = public_parent[node_id]
        out.append({
            'id': node_id, 'parent_id': parent_id if parent_id in seen else None,
            'children': [child_id for child_id in public_children[node_id] if child_id in seen],
            'depth': max(0, len(public_path(node_id)) - 1),
            'ordinal': binding.get('ordinal', fallback_ordinal) if binding else fallback_ordinal,
            'title': node.title,
            'created_at': binding.get('created_at', 0) if binding else 0,
            'kind': node.kind, 'files': list(node.files),
            'ago': node.ago, 'rw_tag': node.rw_tag,
            'mapping_status': 'mapped' if binding is not None else 'unmapped',
            'user_message_id': binding.get('user_message_id') if binding else None,
            'assistant_message_id': binding.get('assistant_message_id') if binding else None,
        })
    return {
        'schema_version': _WORLDLINE_PUBLIC_SCHEMA,
        'root_id': public_root if public_root in seen else (ordered[0] if ordered else None),
        'head': logical_head if logical_head in seen else None,
        'current_path': [node_id for node_id in current_path if node_id in seen],
        'sidecar_status': sidecar_status,
        'truncated': len(visible) > len(ordered),
        'nodes': out,
    }


def _apply_worldline_restore(agent, result):
    history = result.get('history')
    if isinstance(history, list):
        agent.llmclient.backend.history = _json_clone(history, [])
    hist_info = result.get('hist_info')
    if isinstance(hist_info, list):
        agent.history = _json_clone(hist_info, [])
    working = _snapshot_ga_state(agent).get('working') or {}
    if result.get('key_info') is not None:
        working['key_info'] = result.get('key_info') or ''
    _restore_ga_state(agent, hist_info if isinstance(hist_info, list) else None, working)


def handle_worldline_request(agent, req):
    req = _normalize_request(req)
    root_for_req = _resolve_request_root(req.get('ga_root'), Path.cwd())
    workspace = _apply_workspace(agent, root_for_req, req.get('workspace'))
    action = str(req.get('action') or 'state').lower()
    sid = _worldline_sid(req.get('sid'))
    store = getattr(agent, '_admin_worldline_store', None)
    if store is None and req.get('activate') is True:
        store = _ensure_worldline_store(agent, root_for_req, workspace)
    elif store is not None:
        cwd = os.path.realpath(str(workspace or root_for_req))
        if os.path.realpath(store.cwd) != cwd:
            raise RuntimeError('worldline workspace changed within this chat session')
    if store is None:
        emit({
            'type': 'worldline', 'action': action,
            'tree': {
                'schema_version': _WORLDLINE_PUBLIC_SCHEMA,
                'root_id': None, 'head': None, 'current_path': [],
                'sidecar_status': 'inactive', 'truncated': False, 'nodes': [],
            },
            'result': None, 'raw_history': _snapshot_backend_history(agent),
            'history_info': _snapshot_ga_state(agent).get('history_info') or [],
            'working': _snapshot_ga_state(agent).get('working') or {},
        })
        return
    from frontends.worldline import restore_plan
    result = None
    if action == 'bind':
        result = _bind_worldline_head(store, root_for_req, sid, req)
    elif action == 'restore_mapped':
        node_id = str(req.get('node_id') or '')
        sidecar, status = _load_worldline_sidecar(root_for_req, sid)
        binding = sidecar['bindings'].get(node_id)
        if status != 'ok' or binding is None:
            raise ValueError('worldline node has no Admin message mapping')
        logical_target = _logical_worldline_node(sidecar, node_id)
        result = restore_plan(store, node_id, mode='conv', to='at')
        if result is None:
            raise ValueError('worldline node not found')
        _apply_worldline_restore(agent, result)
        _record_worldline_alias(root_for_req, sid, result.get('target'), logical_target)
        result['display_path'] = _json_clone(binding.get('display_path'), None)
        result['user_message_id'] = binding['user_message_id']
        result['assistant_message_id'] = binding['assistant_message_id']
    elif action == 'restore':
        node_id = str(req.get('node_id') or '')
        mode = str(req.get('mode') or 'both').lower()
        to = str(req.get('to') or 'at').lower()
        if mode not in ('both', 'conversation', 'code') or to not in ('at', 'before'):
            raise ValueError('invalid worldline restore mode')
        core_mode = 'conv' if mode == 'conversation' else mode
        sidecar_before, _ = _load_worldline_sidecar(root_for_req, sid)
        logical_target = None
        if core_mode == 'conv':
            physical_target = node_id
            if to == 'before' and node_id in store.nodes:
                physical_target = store.nodes[node_id].get('parent')
                if physical_target not in store.nodes:
                    physical_target = node_id
            logical_target = _logical_worldline_node(sidecar_before, physical_target)
        elif core_mode == 'code':
            logical_target = _logical_worldline_node(sidecar_before, store.head)
        result = restore_plan(store, node_id, mode=core_mode, to=to)
        if result is None:
            raise ValueError('worldline node not found')
        _apply_worldline_restore(agent, result)
        if logical_target is not None:
            _record_worldline_alias(root_for_req, sid, result.get('target'), logical_target)
    elif action not in ('state', 'list'):
        raise ValueError('invalid worldline action')
    sidecar, sidecar_status = _load_worldline_sidecar(root_for_req, sid)
    emit({
        'type': 'worldline', 'action': action,
        'tree': _worldline_nodes(store, sidecar, sidecar_status),
        'result': result, 'raw_history': _snapshot_backend_history(agent),
        'history_info': _snapshot_ga_state(agent).get('history_info') or [],
        'working': _snapshot_ga_state(agent).get('working') or {},
    })

def handle_btw_request(agent, req):
    """Run the official side-question command without mutating the main GA history."""
    req = _normalize_request(req)
    prompt = req.get('prompt') or '/btw'
    history = req.get('history') or []
    raw_history = req.get('raw_history') or []
    llm_no = req.get('llm_no', 0)
    reasoning_effort = req.get('reasoning_effort') if 'reasoning_effort' in req else None
    root_for_req = _resolve_request_root(req.get('ga_root'), Path.cwd())
    _select_llm_if_needed(agent, llm_no)
    if str(reasoning_effort or '').strip():
        _apply_reasoning_effort_setting(agent, reasoning_effort)
    _apply_workspace(agent, root_for_req, req.get('workspace'))
    _restore_admin_history(agent, history, raw_history)
    from frontends.btw_cmd import handle_frontend_command
    started = time.time()
    content = handle_frontend_command(agent, prompt)
    msg = {
        'id': new_id(), 'role': 'assistant', 'content': content,
        'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent),
        'elapsed_ms': max(1, int((time.time() - started) * 1000)),
    }
    emit({'type': 'btw_done', 'message': msg})


def handle_request(agent, worker, req):
    req = _normalize_request(req)
    _reset_usage()  # Clear usage accumulator for this turn
    prompt = req.get('prompt') or ''
    history = req.get('history') or []
    raw_history = req.get('raw_history') or []
    history_info = req.get('history_info') or []
    working = req.get('working') or {}
    llm_no = req.get('llm_no', 0)
    reasoning_effort = req.get('reasoning_effort') if 'reasoning_effort' in req else None
    root_for_req = _resolve_request_root(req.get('ga_root'), Path.cwd())
    project_mode = str(req.get('project_mode') or '').strip()
    setattr(agent, '_ga_project_mode_name', project_mode or None)
    extra_sys_prompts = req.get('extra_sys_prompts') or []
    setattr(agent, 'extra_sys_prompts', list(extra_sys_prompts) if isinstance(extra_sys_prompts, list) else [])
    _select_llm_if_needed(agent, llm_no)
    emit({'type': 'model', 'model_id': _snapshot_model_id(agent)})
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
    prompt, immediate_done = _maybe_handle_review_command(root_for_req, prompt)
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
    ultraplan_objective, prompt, immediate_done = _maybe_handle_ultraplan_command(root_for_req, prompt)
    if immediate_done is not None:
        _emit_immediate_done(agent, immediate_done, history_info, working)
        return
    if ultraplan_objective:
        _run_ultraplan_command(root_for_req, ultraplan_objective, llm_no, agent, history_info, working, emit)
        return
    prompt = _maybe_expand_official_slash_command(root_for_req, prompt)
    chunks = []
    _up_state = {'objective': _ultraplan_ctx[0]} if _ultraplan_ctx[0] else {}
    _up_buf = ['']
    _UP_MARKERS = ('[phase]', '[done]', '[fail]', '[subagent]', '[ultraplan]', '[next]')
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
                    if _ultraplan_ctx[0]:
                        _up_buf[0] += delta
                        while '\n' in _up_buf[0]:
                            line, _up_buf[0] = _up_buf[0].split('\n', 1)
                            stripped = line.strip()
                            if stripped and stripped.lower().startswith(_UP_MARKERS):
                                _emit_ultraplan_line(emit, stripped, _up_state)
                            else:
                                emit({'type': 'delta', 'delta': line + '\n'})
                    else:
                        emit({'type': 'delta', 'delta': delta})
            if 'done' in item:
                if _ultraplan_ctx[0] and _up_buf[0].strip():
                    stripped = _up_buf[0].strip()
                    if stripped.lower().startswith(_UP_MARKERS):
                        _emit_ultraplan_line(emit, stripped, _up_state)
                    else:
                        emit({'type': 'delta', 'delta': _up_buf[0]})
                text = str(item.get('done') or ''.join(chunks))
                msg = {'id': new_id(), 'role': 'assistant', 'content': text, 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent)}
                if _up_state.get('phases') or _up_state.get('objective'):
                    _up_state['complete'] = True
                    msg['ultraplan_state'] = dict(_up_state)
                    _ultraplan_ctx[0] = None
                state = _snapshot_ga_state(agent)
                usage = _snapshot_usage()
                usages = _snapshot_turn_usages()
                _commit_worldline(agent, prompt)
                emit({'type': 'done', 'message': msg, 'usage': usage, 'usages': usages, 'raw_history': _snapshot_backend_history(agent), 'history_info': state.get('history_info') or [], 'working': state.get('working') or {}, 'reasoning_effort': _snapshot_reasoning_effort(agent)})
                return
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent), 'error': True}
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
            req = _normalize_request(json.loads(line))
            if first:
                first = False
                root = _resolve_request_root(req.get('ga_root'), root)
                if str(root) not in sys.path:
                    sys.path.insert(0, str(root))
                os.chdir(root)
                from agentmain import GeneraticAgent
                agent = GeneraticAgent()
                agent.verbose = True
                agent.inc_out = True
                worker = threading.Thread(target=agent.run, name='ga-admin-chat-worker', daemon=True)
                worker.start()
            if req.get('op') == 'btw':
                handle_btw_request(agent, req)
                return
            if req.get('op') == 'worldline':
                with agent_lock:
                    handle_worldline_request(agent, req)
                continue
            with agent_lock:
                handle_request(agent, worker, req)
        except Exception as e:
            msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'model_id': _snapshot_model_id(agent), 'error': True}
            emit({'type': 'error', 'message': msg})


if __name__ == '__main__':
    main()
