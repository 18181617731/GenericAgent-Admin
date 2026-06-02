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


def emit(ev):
    print(json.dumps(ev, ensure_ascii=False), flush=True)


def new_id():
    import uuid
    return str(uuid.uuid4())


def _admin_context_hint(root: Path, prompt: str) -> str:
    """Add an Admin-only nudge so GA reads project-specific pet SOPs when relevant.

    GA core already injects the global memory index, but the index is intentionally
    compact.  In the Admin chat surface users often refer to "this pet/SOP" without
    naming the memory file, so keep a short routing hint next to the user prompt.
    """
    text = (prompt or '').lower()
    triggers = [
        'ga admin', 'ga-admin', 'genericagent-admin', 'admin go', 'ga_admin',
        'ga-admin-pets', 'hatch-pet', 'hatch_pet', 'desktop pet', 'pet atlas',
        'spritesheet', 'sprite sheet', 'pet.json', '桌宠', '宠物', '九行', '9行', '图集',
    ]
    if not any(t in text for t in triggers):
        return prompt
    mem = root / 'memory'
    ga_admin_sop = mem / 'ga_admin_pets_sop.md'
    hatch_sop = mem / 'hatch_pet_sop.md'
    hint = (
        "\n\n[SYSTEM TIPS - GA Admin Chat]\n"
        "This request appears related to GA Admin pets / desktop pet assets. "
        "Before answering or editing assets, read and follow the authoritative SOPs if present:\n"
        f"- {ga_admin_sop}\n"
        f"- {hatch_sop}\n"
        "Key contract: ga-admin-pets atlas uses 192x208 cells, 8 columns, 9 rows in order "
        "idle, running-right, running-left, waving, jumping, failed, waiting, running, review; "
        "Web assets live under web/public/ga-admin-pets/<pet_id>/ and embedded desktop assets under "
        "assets/ga-admin-pets/<pet_id>/. If changing the default desktop pet, update pet_assets_windows.go, "
        "then run gofmt/go test/go build.\n"
    )
    return (prompt or '') + hint


def handle_request(agent, worker, req):
    prompt = req.get('prompt') or ''
    root = Path(req.get('ga_root') or os.environ.get('GA_ROOT') or '.').resolve()
    prompt = _admin_context_hint(root, prompt)
    llm_no = int(req.get('llm_no') or 0)
    try:
        agent.next_llm(llm_no)
    except Exception:
        pass
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
                emit({'type': 'done', 'message': msg})
                return
    except Exception as e:
        msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
        emit({'type': 'error', 'message': msg})


def main():
    root = Path(os.environ.get('GA_ROOT') or '.').resolve()
    _inject_ga_venv(root)
    first = True
    agent = None
    worker = None
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
            handle_request(agent, worker, req)
        except Exception as e:
            msg = {'id': new_id(), 'role': 'assistant', 'content': '执行失败：%s\n%s' % (e, traceback.format_exc()), 'created_at': int(time.time()), 'error': True}
            emit({'type': 'error', 'message': msg})


if __name__ == '__main__':
    main()
