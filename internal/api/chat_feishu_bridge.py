"""Bridge Feishu/Lark chats to durable GenericAgent Admin chat sessions.

The process owns no chat state. It only keeps Feishu chat-to-Admin session bindings;
all conversation reads, writes, busy state, and persistence remain inside Admin's
private loopback API.
"""
import ast
import importlib.util
import json
import os
import re
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


HELP = (
    "GA Admin \u4f1a\u8bdd\u540c\u6b65\u6307\u4ee4:\n"
    "/list [\u9875\u7801] - \u5206\u9875\u5217\u51fa Admin \u4f1a\u8bdd\uff08\u6bcf\u9875 10 \u6761\uff09\n"
    "/new - \u65b0\u5efa\u5e76\u5207\u6362\u5230 Admin \u4f1a\u8bdd\n"
    "/switch <\u5168\u5c40\u5e8f\u53f7|session_id|instance/session_id> - \u5207\u6362\u4f1a\u8bdd\n"
    "/current - \u67e5\u770b\u5f53\u524d\u4f1a\u8bdd\n"
    "/stop - \u4e2d\u6b62\u5f53\u524d\u8f93\u51fa\n"
    "/unbind - \u89e3\u9664\u7ed1\u5b9a\n"
    "/help - \u663e\u793a\u5e2e\u52a9\n\n"
    "\u4e5f\u53ef\u4f7f\u7528 /ga list 2\u3001/ga switch <...>\u3002"
)

LIST_PAGE_SIZE = 10


def _request(base, token, path, method="GET", data=None):
    body = None if data is None else json.dumps(data).encode("utf-8")
    req = urllib.request.Request(
        base + path,
        data=body,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=20) as response:
            raw = response.read()
            return json.loads(raw) if raw else {"ok": True}
    except urllib.error.HTTPError as exc:
        try:
            detail = json.loads(exc.read()).get("error", str(exc))
        except Exception:
            detail = str(exc)
        raise RuntimeError(detail) from exc


def _load_config(path):
    path = Path(path)
    if not path.is_file():
        return {}
    if path.suffix.lower() == ".json":
        data = json.loads(path.read_text(encoding="utf-8"))
    else:
        spec = importlib.util.spec_from_file_location("_ga_admin_feishu_config", str(path))
        if spec is None or spec.loader is None:
            return {}
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        data = vars(module)
    return data if isinstance(data, dict) else {}


def _allowed_users(value):
    if isinstance(value, str):
        value = re.split(r"[,;\s]+", value)
    if not isinstance(value, (list, tuple, set)):
        return set()
    return {str(item).strip() for item in value if str(item).strip()}


def _events(tasks):
    result = []
    for task in tasks or []:
        text = str(task.get("input") or "").strip()
        if text:
            result.append(("user", text))
        for output in task.get("outputs") or []:
            text = str(output or "").strip()
            if text:
                result.append(("assistant", text))
    return result


def _split_text(text, limit=3500):
    text = str(text or "").strip()
    if not text:
        return []
    chunks = []
    while len(text) > limit:
        cut = text.rfind("\n", 0, limit + 1)
        if cut < limit // 2:
            cut = limit
        chunks.append(text[:cut].strip())
        text = text[cut:].strip()
    if text:
        chunks.append(text)
    return chunks


def _card(elements):
    return json.dumps({
        "schema": "2.0",
        "config": {"streaming_mode": False, "width_mode": "fill"},
        "body": {"elements": elements},
    }, ensure_ascii=False)


def _info_card(text):
    return _card([{"tag": "markdown", "content": str(text or "")}])


def _task_card(text):
    return _card([
        {"tag": "markdown", "content": "**\u2705 \u5df2\u5b8c\u6210**"},
        {"tag": "hr"},
        {"tag": "markdown", "content": str(text or "_(\u65e0\u6587\u672c\u8f93\u51fa)_")},
    ])


def _send_with_fallback(send_once, text, completed=False):
    attempts = [
        ("interactive", _task_card(text) if completed else _info_card(text)),
        ("text", json.dumps({"text": text}, ensure_ascii=False)),
    ]
    for msg_type, content in attempts:
        try:
            response = send_once(msg_type, content)
            if response.success():
                return True
            print("[feishu_admin_bridge] %s send failed: %s %s" % (
                msg_type, response.code, response.msg,
            ), flush=True)
        except Exception as exc:
            print("[feishu_admin_bridge] %s send failed: %s" % (msg_type, exc), flush=True)
    return False


def _task_snapshot(value):
    if isinstance(value, dict):
        return (
            value.get("tasks", []) or [],
            bool(value.get("run")),
            str(value.get("partial") or "").strip(),
            value.get("turns", []) or [],
            str(value.get("run_id") or ""),
        )
    return value or [], False, "", [], ""


def _build_turn_detail(turn):
    parts = []
    thinking = str(turn.get("thinking") or "").strip()
    if thinking:
        parts.append("### \U0001f4ad Thinking\n" + thinking)
    tool_calls = turn.get("tool_calls") or []
    if tool_calls:
        lines = []
        for tool_call in tool_calls:
            name = tool_call.get("tool_name", "?")
            args = {key: value for key, value in (tool_call.get("args") or {}).items()
                    if not str(key).startswith("_")}
            lines.append("- `%s`(%s)" % (
                name, json.dumps(args, ensure_ascii=False)[:200],
            ))
        parts.append("### \U0001f6e0 Tool Calls\n" + "\n".join(lines))
    content = str(turn.get("content") or "").strip()
    if content and content != "...":
        parts.append("### \U0001f4dd Output\n" + content)
    return "\n\n".join(parts)


def _load_official_task_card(ga_root, send_raw, patch_card, send_message):
    """Load only fsapp._TaskCard, without importing fsapp or its credentials."""
    source_path = Path(ga_root) / "frontends" / "fsapp.py"
    source = source_path.read_text(encoding="utf-8")
    tree = ast.parse(source, str(source_path))
    class_node = next(
        (node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "_TaskCard"),
        None,
    )
    if class_node is None:
        raise RuntimeError("official fsapp _TaskCard not found: %s" % source_path)
    isolated = ast.Module(body=[class_node], type_ignores=[])
    ast.fix_missing_locations(isolated)
    namespace = {
        "_card_raw": _card,
        "_send_raw": send_raw,
        "_patch_card": patch_card,
        "send_message": send_message,
        "_display_text": lambda text: str(text or ""),
    }
    exec(compile(isolated, str(source_path), "exec"), namespace)
    return namespace["_TaskCard"]


class _LiveTaskCard:
    """Adapt live Admin text to the official fsapp task-card lifecycle."""
    def __init__(self, official_card):
        self.card = official_card
        self.last_partial = ""
        self.last_final_candidate = ""

    def start(self):
        self.card.start()

    def update(self, partial):
        partial = str(partial or "").strip()
        if not partial or partial == self.last_partial:
            return True
        summary = "\u5b9e\u65f6\u8f93\u51fa"
        if self.card.steps:
            self.card.steps[-1] = (summary, partial)
            self.card.status = "\u23f3 \u5de5\u4f5c\u4e2d \u00b7 Turn %d" % len(self.card.steps)
        else:
            self.card.steps.append((summary, partial))
            self.card.status = "\u23f3 \u5de5\u4f5c\u4e2d \u00b7 Turn %d" % len(self.card.steps)
        if not self.card._push():
            return False
        self.last_partial = partial
        return True

    def step(self, summary, detail="", final_candidate=""):
        self.card.step(summary, detail)
        self.last_partial = ""
        self.last_final_candidate = str(final_candidate or "").strip()

    def done(self, text):
        final = str(text or "").strip()
        comparable = final
        match = re.fullmatch(
            r"\*\*LLM Running \(Turn \d+\) \.\.\.\*\*\s*(.*)",
            final,
            flags=re.DOTALL,
        )
        if match:
            comparable = match.group(1).strip()
        if self.last_final_candidate and comparable == self.last_final_candidate:
            self.card.status = "\u2705 \u5df2\u5b8c\u6210"
            self.card.final = ""
            if not self.card._push():
                fallback = getattr(self.card, "_fallback_text", None)
                if fallback:
                    fallback("\u2705 \u5df2\u5b8c\u6210", final=True)
            return
        self.card.done(text)

    def fail(self, message):
        self.card.fail(message)


class AdminAPI:
    def __init__(self, base, token):
        self.base = base.rstrip("/")
        self.token = token

    def sessions(self):
        return _request(self.base, self.token, "/sessions?all=1").get("sessions", [])

    def create(self, instance_id="_"):
        instance_id = urllib.parse.quote(str(instance_id or "_"), safe="")
        return _request(self.base, self.token, "/session/%s/new" % instance_id, "POST")

    @staticmethod
    def _path(item, operation):
        instance_id = urllib.parse.quote(str(item["instance_id"]), safe="")
        session_id = urllib.parse.quote(str(item["session_id"]), safe="")
        return "/session/%s/%s/%s" % (instance_id, session_id, operation)

    def tasks(self, item):
        path = self._path(item, "outputs") + "?live=0"
        return _request(self.base, self.token, path).get("tasks", [])

    def snapshot(self, item):
        return _request(self.base, self.token, self._path(item, "snapshot"))

    def put(self, item, text):
        return _request(self.base, self.token, self._path(item, "put"), "POST", {"text": text})

    def cancel(self, item):
        return _request(self.base, self.token, self._path(item, "abort"), "POST")


class BindingStore:
    def __init__(self, path):
        self.path = Path(path)
        self.lock = threading.RLock()
        self.bindings = {}
        self._load()

    def _load(self):
        try:
            data = json.loads(self.path.read_text(encoding="utf-8"))
            bindings = data.get("bindings", {})
            if isinstance(bindings, dict):
                self.bindings = bindings
        except (OSError, ValueError, TypeError):
            self.bindings = {}

    def save(self):
        self.path.parent.mkdir(parents=True, exist_ok=True)
        temp = self.path.with_name(self.path.name + ".tmp")
        temp.write_text(json.dumps({"bindings": self.bindings}, ensure_ascii=False, indent=2), encoding="utf-8")
        os.replace(str(temp), str(self.path))

    def get(self, chat_id):
        with self.lock:
            item = self.bindings.get(chat_id)
            return dict(item) if isinstance(item, dict) else None

    def set(self, chat_id, item):
        with self.lock:
            self.bindings[chat_id] = dict(item)
            self.save()

    def remove(self, chat_id):
        with self.lock:
            removed = self.bindings.pop(chat_id, None) is not None
            if removed:
                self.save()
            return removed

    def snapshot(self):
        with self.lock:
            return [(chat_id, dict(item)) for chat_id, item in self.bindings.items() if isinstance(item, dict)]


class FeishuAdminBridge:
    def __init__(self, api, store, send, card_factory=None):
        self.api = api
        self.store = store
        self.send = send
        self.card_factory = card_factory
        self.active_cards = {}
        self.turn_cursors = {}
        self.active_cards_lock = threading.RLock()
        self.pending_inputs = {}
        self.pending_lock = threading.Lock()

    def _snapshot(self, binding):
        snapshot = getattr(self.api, "snapshot", None)
        if callable(snapshot):
            return _task_snapshot(snapshot(binding))
        return self.api.tasks(binding), False, ""

    @staticmethod
    def _command(text):
        text = text.strip()
        if not text.startswith("/"):
            return "", ""
        parts = text.split(None, 2)
        command = parts[0].lower()
        if command == "/ga":
            command = ("/" + parts[1].lower()) if len(parts) > 1 else "/help"
            argument = parts[2].strip() if len(parts) > 2 else ""
        else:
            argument = " ".join(parts[1:]).strip()
        aliases = {
            "/sessions": "/list", "/chats": "/list", "/\u4f1a\u8bdd": "/list",
            "/use": "/switch", "/\u5207\u6362": "/switch",
            "/status": "/current", "/\u5f53\u524d": "/current",
            "/\u89e3\u7ed1": "/unbind", "/\u5e2e\u52a9": "/help",
        }
        return aliases.get(command, command), argument

    @staticmethod
    def _label(item):
        title = str(item.get("title") or "(untitled)").replace("\n", " ").strip()
        instance_id = str(item.get("instance_id") or "_")
        display_instance = "default" if instance_id == "_" else instance_id
        return "%s [%s/%s]" % (title, display_instance, item.get("session_id", ""))

    def _list(self, page_text=""):
        page_text = str(page_text or "").strip()
        if page_text and not page_text.isdigit():
            return "\u9875\u7801\u5fc5\u987b\u662f\u6b63\u6574\u6570\uff0c\u4f8b\u5982 /list 2\u3002"
        page = int(page_text or "1")
        if page < 1:
            return "\u9875\u7801\u5fc5\u987b\u4ece 1 \u5f00\u59cb\u3002"
        sessions = self.api.sessions()
        if not sessions:
            return "Admin \u4e2d\u6682\u65e0\u53ef\u7528\u4f1a\u8bdd\u3002"
        page_count = (len(sessions) + LIST_PAGE_SIZE - 1) // LIST_PAGE_SIZE
        if page > page_count:
            return "\u9875\u7801\u8d85\u51fa\u8303\u56f4\uff0c\u5f53\u524d\u5171 %d \u9875\u3002" % page_count
        start = (page - 1) * LIST_PAGE_SIZE
        end = min(start + LIST_PAGE_SIZE, len(sessions))
        lines = [
            "Admin \u4f1a\u8bdd\uff08\u7b2c %d/%d \u9875\uff0c\u5171 %d \u4e2a\uff1b\u987a\u5e8f\u540c Admin \u6700\u8fd1\u5bf9\u8bdd\uff09:"
            % (page, page_count, len(sessions))
        ]
        for index, item in enumerate(sessions[start:end], start + 1):
            lines.append("%d. %s" % (index, self._label(item)))
        navigation = []
        if page > 1:
            navigation.append("/list %d" % (page - 1))
        if page < page_count:
            navigation.append("/list %d" % (page + 1))
        if navigation:
            lines.append("\n\u7ffb\u9875: " + " | ".join(navigation))
        lines.append("\u4f7f\u7528 /switch <\u5168\u5c40\u5e8f\u53f7> \u5207\u6362\u3002")
        return "\n".join(lines)

    def _select(self, selector):
        sessions = self.api.sessions()
        if selector.isdigit():
            index = int(selector) - 1
            return sessions[index] if 0 <= index < len(sessions) else None
        selector = selector.strip()
        matches = []
        for item in sessions:
            instance_id = str(item.get("instance_id") or "_")
            sid = str(item.get("session_id") or "")
            peer = str(item.get("peer") or "")
            candidates = {sid, peer, instance_id + "/" + sid}
            if instance_id == "_":
                candidates.add("default/" + sid)
            if selector in candidates:
                matches.append(item)
        return matches[0] if len(matches) == 1 else None

    def handle(self, chat_id, text):
        command, argument = self._command(text)
        try:
            if command == "/help":
                return HELP
            if command == "/list":
                return self._list(argument)
            if command == "/current":
                binding = self.store.get(chat_id)
                return ("\u5f53\u524d: " + self._label(binding)) if binding else "\u5f53\u524d\u672a\u7ed1\u5b9a Admin \u4f1a\u8bdd\u3002"
            if command == "/stop":
                binding = self.store.get(chat_id)
                if not binding:
                    return "\u8bf7\u5148\u4f7f\u7528 /switch <\u5e8f\u53f7> \u7ed1\u5b9a Admin \u4f1a\u8bdd\u3002"
                self.api.cancel(binding)
                return "\u5df2\u8bf7\u6c42\u4e2d\u6b62\u5f53\u524d\u8f93\u51fa\u3002"
            if command == "/unbind":
                with self.active_cards_lock:
                    removed = self.store.remove(chat_id)
                    cards = self._take_chat_cards(chat_id)
                    self._clear_pending(chat_id)
                self._retire_cards(cards, "\u5df2\u89e3\u9664\u4f1a\u8bdd\u7ed1\u5b9a")
                return "\u5df2\u89e3\u9664\u4f1a\u8bdd\u7ed1\u5b9a\u3002" if removed else "\u5f53\u524d\u672a\u7ed1\u5b9a Admin \u4f1a\u8bdd\u3002"
            if command == "/new":
                current = self.store.get(chat_id)
                instance_id = str((current or {}).get("instance_id") or "_")
                selected = self.api.create(instance_id)
                binding = {
                    "instance_id": selected.get("instance_id") or instance_id,
                    "session_id": selected["session_id"],
                    "title": selected.get("title") or "\u65b0\u4f1a\u8bdd",
                    "peer": selected.get("peer", ""),
                    "cursor": 0,
                }
                with self.active_cards_lock:
                    cards = self._take_chat_cards(chat_id)
                    self._clear_pending(chat_id)
                    self.store.set(chat_id, binding)
                self._retire_cards(cards, "\u5df2\u65b0\u5efa Admin \u4f1a\u8bdd")
                return "\u5df2\u65b0\u5efa\u5e76\u5207\u6362\u5230: " + self._label(binding)
            if command == "/switch":
                if not argument:
                    return self._list()
                selected = self._select(argument)
                if not selected:
                    return "\u672a\u627e\u5230\u552f\u4e00\u4f1a\u8bdd\uff0c\u8bf7\u5148\u7528 /list \u67e5\u770b\u5e8f\u53f7\u3002"
                binding = {
                    "instance_id": selected["instance_id"],
                    "session_id": selected["session_id"],
                    "title": selected.get("title", ""),
                    "peer": selected.get("peer", ""),
                    "cursor": len(_events(self.api.tasks(selected))),
                }
                with self.active_cards_lock:
                    cards = self._take_chat_cards(chat_id)
                    self._clear_pending(chat_id)
                    self.store.set(chat_id, binding)
                self._retire_cards(cards, "\u5df2\u5207\u6362 Admin \u4f1a\u8bdd")
                return "\u5df2\u5207\u6362\u5230: " + self._label(binding)
            if command:
                return "\u672a\u77e5\u6307\u4ee4\u3002\n\n" + HELP

            binding = self.store.get(chat_id)
            if not binding:
                return "\u8bf7\u5148\u7528 /list \u67e5\u770b\u4f1a\u8bdd\uff0c\u518d\u7528 /switch <\u5e8f\u53f7> \u7ed1\u5b9a\u3002"
            with self.pending_lock:
                self.pending_inputs.setdefault(chat_id, []).append(text.strip())
            try:
                self.api.put(binding, text)
            except Exception:
                with self.pending_lock:
                    pending = self.pending_inputs.get(chat_id, [])
                    if text.strip() in pending:
                        pending.remove(text.strip())
                raise
            return None
        except Exception as exc:
            return "GA Admin \u540c\u6b65\u5931\u8d25: %s" % exc

    def _consume_pending(self, chat_id, text):
        with self.pending_lock:
            pending = self.pending_inputs.get(chat_id, [])
            if pending and pending[0] == text:
                pending.pop(0)
                return True
        return False

    @staticmethod
    def _card_key(chat_id, binding):
        return (
            str(chat_id),
            str(binding.get("instance_id") or "_"),
            str(binding.get("session_id") or ""),
        )

    def _new_card(self, chat_id):
        card = self.card_factory(chat_id)
        card.start()
        return card

    def _take_chat_cards(self, chat_id):
        chat_id = str(chat_id)
        with self.active_cards_lock:
            keys = [key for key in self.active_cards if key[0] == chat_id]
            cards = [self.active_cards.pop(key) for key in keys]
            turn_keys = [key for key in self.turn_cursors if key[0] == chat_id]
            for key in turn_keys:
                self.turn_cursors.pop(key, None)
            return cards

    @staticmethod
    def _retire_cards(cards, message):
        for card in cards:
            try:
                card.fail(message)
            except Exception as exc:
                print("[feishu_admin_bridge] retire card failed: %s" % exc, flush=True)

    def _clear_pending(self, chat_id):
        with self.pending_lock:
            self.pending_inputs.pop(chat_id, None)

    def _save_cursor(self, chat_id, binding, cursor):
        latest = self.store.get(chat_id)
        if latest and self._card_key(chat_id, latest) == self._card_key(chat_id, binding):
            latest["cursor"] = cursor
            self.store.set(chat_id, latest)

    def poll_once(self):
        for chat_id, binding in self.store.snapshot():
            try:
                tasks, running, partial, turns, run_id = self._snapshot(binding)
                events = _events(tasks)
                with self.active_cards_lock:
                    latest = self.store.get(chat_id)
                    if not latest or self._card_key(chat_id, latest) != self._card_key(chat_id, binding):
                        continue
                    binding = latest
                    cursor = max(0, int(binding.get("cursor", 0)))
                    if cursor > len(events):
                        cursor = len(events)
                    changed = cursor != int(binding.get("cursor", 0))
                    completed_in_snapshot = False
                    key = self._card_key(chat_id, binding)
                    saved_turn_cursor = self.turn_cursors.get(key)
                    if (isinstance(saved_turn_cursor, tuple) and len(saved_turn_cursor) == 2
                            and saved_turn_cursor[0] == run_id):
                        turn_cursor = max(0, int(saved_turn_cursor[1]))
                    else:
                        turn_cursor = 0
                    if turn_cursor > len(turns):
                        turn_cursor = 0
                    if self.card_factory:
                        while turn_cursor < len(turns):
                            turn = turns[turn_cursor]
                            summary = str(turn.get("summary") or "").strip()
                            if summary:
                                card = self.active_cards.get(key)
                                if card is None:
                                    card = self._new_card(chat_id)
                                    self.active_cards[key] = card
                                card.step(
                                    summary,
                                    _build_turn_detail(turn),
                                    final_candidate=turn.get("content"),
                                )
                            turn_cursor += 1
                            self.turn_cursors[key] = (run_id, turn_cursor)
                    while cursor < len(events):
                        role, text = events[cursor]
                        terminal_assistant = (
                            role == "assistant"
                            and (cursor + 1 == len(events) or events[cursor + 1][0] != "assistant")
                        )
                        if role == "assistant" and not terminal_assistant and self.card_factory:
                            delivered = True
                        elif role == "user" and self._consume_pending(chat_id, text):
                            delivered = True
                        elif role == "assistant" and self.card_factory:
                            card = self.active_cards.get(key)
                            if card is None:
                                card = self._new_card(chat_id)
                            card.done(text)
                            self.active_cards.pop(key, None)
                            delivered = True
                        else:
                            payload = ("[Admin]\n" + text) if role == "user" else text
                            completed = role == "assistant"
                            delivered = all(self.send(chat_id, chunk, completed) for chunk in _split_text(payload))
                        if not delivered:
                            break
                        if terminal_assistant:
                            completed_in_snapshot = True
                        cursor += 1
                        changed = True
                    if changed:
                        self._save_cursor(chat_id, binding, cursor)
                    if running and not completed_in_snapshot and cursor == len(events) and self.card_factory:
                        if key not in self.active_cards:
                            self.active_cards[key] = self._new_card(chat_id)
            except Exception as exc:
                print("[feishu_admin_bridge] poll %s failed: %s" % (chat_id, exc), flush=True)


def main():
    base = os.environ["GA_ADMIN_FEISHU_API"]
    token = os.environ["GA_ADMIN_FEISHU_TOKEN"]
    state_path = os.environ["GA_ADMIN_FEISHU_STATE"]
    config_path = os.environ["GA_ADMIN_FEISHU_CONFIG"]
    config = _load_config(config_path)
    app_id = str(config.get("feishu_admin_app_id") or "").strip()
    app_secret = str(config.get("feishu_admin_app_secret") or "").strip()
    allowed = _allowed_users(config.get("feishu_admin_allowed_users", []))
    if not app_id or not app_secret:
        print("[feishu_admin_bridge] dedicated App ID/App Secret not configured", flush=True)
        return

    try:
        import lark_oapi as lark
        from lark_oapi.api.im.v1 import (
            CreateMessageRequest,
            CreateMessageRequestBody,
            PatchMessageRequest,
            PatchMessageRequestBody,
        )
    except ImportError as exc:
        print("[feishu_admin_bridge] lark_oapi unavailable: %s" % exc, flush=True)
        return

    rest_client = lark.Client.builder().app_id(app_id).app_secret(app_secret).log_level(lark.LogLevel.INFO).build()

    def send_raw(receive_id, payload, msg_type, receive_id_type):
        try:
            request = CreateMessageRequest.builder().receive_id_type(receive_id_type).request_body(
                CreateMessageRequestBody.builder().receive_id(receive_id).msg_type(msg_type).content(payload).build()
            ).build()
            response = rest_client.im.v1.message.create(request)
            if response.success():
                return response.data.message_id if response.data else None
            print("[feishu_admin_bridge] %s send failed: %s %s" % (
                msg_type, response.code, response.msg,
            ), flush=True)
        except Exception as exc:
            print("[feishu_admin_bridge] %s send failed: %s" % (msg_type, exc), flush=True)
        return None

    def patch_card(message_id, payload):
        try:
            request = PatchMessageRequest.builder().message_id(message_id).request_body(
                PatchMessageRequestBody.builder().content(payload).build()
            ).build()
            response = rest_client.im.v1.message.patch(request)
            if not response.success():
                print("[feishu_admin_bridge] card patch failed: %s %s" % (
                    response.code, response.msg,
                ), flush=True)
            return response.success()
        except Exception as exc:
            print("[feishu_admin_bridge] card patch failed: %s" % exc, flush=True)
            return False

    def official_send_message(receive_id, content, msg_type="text", use_card=False,
                              receive_id_type="open_id"):
        if use_card:
            return send_raw(receive_id, _info_card(content), "interactive", receive_id_type)
        if msg_type == "text":
            content = json.dumps({"text": content}, ensure_ascii=False)
        return send_raw(receive_id, content, msg_type, receive_id_type)

    official_card = _load_official_task_card(
        os.environ["GA_ROOT"], send_raw, patch_card, official_send_message,
    )

    def send(chat_id, text, completed=False):
        def send_once(msg_type, content):
            request = CreateMessageRequest.builder().receive_id_type("chat_id").request_body(
                CreateMessageRequestBody.builder().receive_id(chat_id).msg_type(msg_type).content(content).build()
            ).build()
            return rest_client.im.v1.message.create(request)

        return _send_with_fallback(send_once, text, completed)

    bridge = FeishuAdminBridge(
        AdminAPI(base, token),
        BindingStore(state_path),
        send,
        card_factory=lambda chat_id: _LiveTaskCard(official_card(chat_id, "chat_id")),
    )
    seen = set()
    seen_order = []
    seen_lock = threading.Lock()

    def handle_message(data):
        try:
            event = data.event
            message = event.message
            sender = event.sender
            sender_id = str(sender.sender_id.open_id or "")
            if allowed and "*" not in allowed and sender_id not in allowed:
                return
            message_id = str(message.message_id or "")
            with seen_lock:
                if message_id and message_id in seen:
                    return
                if message_id:
                    seen.add(message_id)
                    seen_order.append(message_id)
                    if len(seen_order) > 2000:
                        seen.discard(seen_order.pop(0))
            if str(message.message_type or "") != "text":
                send(message.chat_id, "\u76ee\u524d\u4ec5\u652f\u6301\u6587\u672c\u6d88\u606f\u540c\u6b65\u3002")
                return
            content = json.loads(message.content or "{}")
            text = str(content.get("text") or "").strip()
            text = re.sub(r"^@\S+\s*", "", text).strip()
            if not text:
                return
            response = bridge.handle(str(message.chat_id), text)
            if response:
                for chunk in _split_text(response):
                    send(str(message.chat_id), chunk)
        except Exception as exc:
            print("[feishu_admin_bridge] event failed: %s" % exc, flush=True)

    def poll_loop():
        while True:
            bridge.poll_once()
            time.sleep(2)

    threading.Thread(target=poll_loop, daemon=True).start()
    handler = lark.EventDispatcherHandler.builder("", "").register_p2_im_message_receive_v1(handle_message).build()
    delay = 5
    while True:
        try:
            print("[feishu_admin_bridge] connected for Admin session sync", flush=True)
            ws_client = lark.ws.Client(app_id, app_secret, event_handler=handler, log_level=lark.LogLevel.INFO)
            ws_client.start()
            delay = 5
        except KeyboardInterrupt:
            raise
        except Exception as exc:
            print("[feishu_admin_bridge] connection failed: %s" % exc, flush=True)
        time.sleep(delay)
        delay = min(delay * 2, 120)


if __name__ == "__main__":
    main()
