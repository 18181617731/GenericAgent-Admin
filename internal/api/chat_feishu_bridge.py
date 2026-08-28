"""Bridge Feishu/Lark chats to durable GenericAgent Admin chat sessions.

The process owns no chat state. It only keeps Feishu chat-to-Admin session bindings;
all conversation reads, writes, busy state, and persistence remain inside Admin's
private loopback API.
"""
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
    "/list - \u5217\u51fa Admin \u4f1a\u8bdd\n"
    "/switch <\u5e8f\u53f7|session_id|instance/session_id> - \u5207\u6362\u4f1a\u8bdd\n"
    "/current - \u67e5\u770b\u5f53\u524d\u4f1a\u8bdd\n"
    "/unbind - \u89e3\u9664\u7ed1\u5b9a\n"
    "/help - \u663e\u793a\u5e2e\u52a9\n\n"
    "\u4e5f\u53ef\u4f7f\u7528 /ga list\u3001/ga switch <...>\u3002"
)


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


class AdminAPI:
    def __init__(self, base, token):
        self.base = base.rstrip("/")
        self.token = token

    def sessions(self):
        return _request(self.base, self.token, "/sessions?all=1").get("sessions", [])

    @staticmethod
    def _path(item, operation):
        instance_id = urllib.parse.quote(str(item["instance_id"]), safe="")
        session_id = urllib.parse.quote(str(item["session_id"]), safe="")
        return "/session/%s/%s/%s" % (instance_id, session_id, operation)

    def tasks(self, item):
        path = self._path(item, "outputs") + "?live=0"
        return _request(self.base, self.token, path).get("tasks", [])

    def put(self, item, text):
        return _request(self.base, self.token, self._path(item, "put"), "POST", {"text": text})


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
    def __init__(self, api, store, send):
        self.api = api
        self.store = store
        self.send = send
        self.pending_inputs = {}
        self.pending_lock = threading.Lock()

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

    def _list(self):
        sessions = self.api.sessions()
        if not sessions:
            return "Admin \u4e2d\u6682\u65e0\u53ef\u7528\u4f1a\u8bdd\u3002"
        lines = ["Admin \u4f1a\u8bdd\uff08\u6309\u6700\u8fd1\u66f4\u65b0\u6392\u5e8f\uff09:"]
        for index, item in enumerate(sessions, 1):
            lines.append("%d. %s" % (index, self._label(item)))
        lines.append("\n\u4f7f\u7528 /switch <\u5e8f\u53f7> \u5207\u6362\u3002")
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
                return self._list()
            if command == "/current":
                binding = self.store.get(chat_id)
                return ("\u5f53\u524d: " + self._label(binding)) if binding else "\u5f53\u524d\u672a\u7ed1\u5b9a Admin \u4f1a\u8bdd\u3002"
            if command == "/unbind":
                return "\u5df2\u89e3\u9664\u4f1a\u8bdd\u7ed1\u5b9a\u3002" if self.store.remove(chat_id) else "\u5f53\u524d\u672a\u7ed1\u5b9a Admin \u4f1a\u8bdd\u3002"
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
                self.store.set(chat_id, binding)
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

    def poll_once(self):
        for chat_id, binding in self.store.snapshot():
            try:
                events = _events(self.api.tasks(binding))
                cursor = max(0, int(binding.get("cursor", 0)))
                if cursor > len(events):
                    cursor = len(events)
                changed = cursor != int(binding.get("cursor", 0))
                while cursor < len(events):
                    role, text = events[cursor]
                    if role == "user" and self._consume_pending(chat_id, text):
                        delivered = True
                    else:
                        payload = ("[Admin]\n" + text) if role == "user" else text
                        delivered = all(self.send(chat_id, chunk) for chunk in _split_text(payload))
                    if not delivered:
                        break
                    cursor += 1
                    changed = True
                if changed:
                    latest = self.store.get(chat_id)
                    if latest and latest.get("session_id") == binding.get("session_id"):
                        latest["cursor"] = cursor
                        self.store.set(chat_id, latest)
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
        from lark_oapi.api.im.v1 import CreateMessageRequest, CreateMessageRequestBody
    except ImportError as exc:
        print("[feishu_admin_bridge] lark_oapi unavailable: %s" % exc, flush=True)
        return

    rest_client = lark.Client.builder().app_id(app_id).app_secret(app_secret).log_level(lark.LogLevel.INFO).build()

    def send(chat_id, text):
        try:
            request = CreateMessageRequest.builder().receive_id_type("chat_id").request_body(
                CreateMessageRequestBody.builder().receive_id(chat_id).msg_type("text").content(
                    json.dumps({"text": text}, ensure_ascii=False)
                ).build()
            ).build()
            response = rest_client.im.v1.message.create(request)
            if response.success():
                return True
            print("[feishu_admin_bridge] send failed: %s %s" % (response.code, response.msg), flush=True)
        except Exception as exc:
            print("[feishu_admin_bridge] send failed: %s" % exc, flush=True)
        return False

    bridge = FeishuAdminBridge(AdminAPI(base, token), BindingStore(state_path), send)
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
