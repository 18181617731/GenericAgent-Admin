"""Expose GA Admin chat sessions as peers on the official GenericAgent Hub.

This process is intentionally separate from chat_worker.py: aborting a chat worker must
not disconnect the session from Hub. All mutations go through Admin's private loopback
API, so Admin remains the owner of persistence, busy state, and cancellation.
"""
import asyncio
import importlib.util
import json
import os
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def _request(base, token, path, method="GET", data=None):
    body = None if data is None else json.dumps(data).encode("utf-8")
    req = urllib.request.Request(
        base + path,
        data=body,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=15) as response:
            raw = response.read()
            return json.loads(raw) if raw else {"ok": True}
    except urllib.error.HTTPError as exc:
        try:
            detail = json.loads(exc.read())
        except Exception:
            detail = {"error": str(exc)}
        if isinstance(detail, dict):
            detail.setdefault("code", "http_%d" % exc.code)
            return detail
        return {"error": str(detail), "code": "http_%d" % exc.code}
    except Exception as exc:
        return {"error": str(exc), "code": "offline"}


def _load_hub(ga_root):
    path = Path(ga_root) / "frontends" / "hub.py"
    spec = importlib.util.spec_from_file_location("ga_official_hub", path)
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load official Hub client from %s" % path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main():
    base = os.environ["GA_ADMIN_HUB_API"].rstrip("/")
    token = os.environ["GA_ADMIN_HUB_TOKEN"]
    hub = _load_hub(os.environ["GA_ROOT"])

    class ManagedHubClient(hub.HubClient):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, **kwargs)
            self._stopped = False

        def stop(self):
            self._stopped = True
            if self._ws is not None and self._lp is not None:
                asyncio.run_coroutine_threadsafe(self._ws.close(), self._lp)

        async def _loop(self):
            try:
                import websockets
            except ImportError as exc:
                print("[chat_hub_bridge:%s] websockets unavailable: %s" % (self.name, exc), flush=True)
                return
            while not self._stopped:
                try:
                    async with websockets.connect(hub.URL, open_timeout=3, max_size=None) as ws:
                        if self._stopped:
                            return
                        self._ws, self._lp = ws, asyncio.get_running_loop()
                        caps = [
                            key
                            for key, value in (
                                ("get", self.get_outputs),
                                ("put", self.put_task),
                                ("abort", self.abort),
                            )
                            if value
                        ]
                        await ws.send(json.dumps({
                            "op": "hello", "name": self.name, "pid": os.getpid(),
                            "fixed": self.fixed, "caps": caps, "sub": self.sub,
                        }))
                        async for raw in ws:
                            await self._on_cmd(ws, json.loads(raw))
                except Exception as exc:
                    print("[chat_hub_bridge:%s] connection failed: %s" % (self.name, exc), flush=True)
                self._ws = None
                if not self._stopped:
                    await asyncio.sleep(5)

    clients = {}
    lock = threading.Lock()

    def call(instance_id, session_id, op, data=None):
        instance_key = urllib.parse.quote(instance_id, safe="")
        session_key = urllib.parse.quote(session_id, safe="")
        path = "/session/%s/%s/%s" % (instance_key, session_key, op)
        return _request(base, token, path, "POST" if op in ("put", "abort") else "GET", data)

    def connect(item):
        peer = item["peer"]
        instance_id, session_id = item["instance_id"], item["session_id"]
        client = ManagedHubClient(
            peer,
            put_task=lambda text, i=instance_id, s=session_id: call(i, s, "put", {"text": text}),
            get_outputs=lambda i=instance_id, s=session_id: call(i, s, "outputs").get("tasks", []),
            abort=lambda i=instance_id, s=session_id: call(i, s, "abort"),
            state=lambda i=instance_id, s=session_id: call(i, s, "state"),
            fixed=True,
        )
        client.start()
        clients[peer] = client

    while True:
        listing = _request(base, token, "/sessions")
        wanted = {item["peer"]: item for item in listing.get("sessions", []) if item.get("peer")}
        with lock:
            for peer in set(clients) - set(wanted):
                clients.pop(peer).stop()
            for peer, item in wanted.items():
                if peer not in clients:
                    try:
                        connect(item)
                    except Exception as exc:
                        print("[chat_hub_bridge:%s] registration failed: %s" % (peer, exc), flush=True)
        time.sleep(3)


if __name__ == "__main__":
    main()
