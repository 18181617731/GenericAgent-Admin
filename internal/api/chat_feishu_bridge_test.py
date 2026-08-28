import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("chat_feishu_bridge.py")
spec = importlib.util.spec_from_file_location("chat_feishu_bridge", SCRIPT)
bridge_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge_module)


class FakeAPI:
    def __init__(self):
        self.items = [
            {"peer": "ga-admin/default/s1", "instance_id": "_", "session_id": "s1", "title": "First"},
            {"peer": "ga-admin/work/s2", "instance_id": "work", "session_id": "s2", "title": "Second"},
        ]
        self.tasks_by_sid = {"s1": [{"input": "old", "outputs": ["done"]}], "s2": []}
        self.puts = []

    def sessions(self):
        return list(self.items)

    def tasks(self, item):
        return self.tasks_by_sid[item["session_id"]]

    def put(self, item, text):
        self.puts.append((item["session_id"], text))
        self.tasks_by_sid[item["session_id"]].append({"input": text, "outputs": []})


class FeishuAdminBridgeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.store = bridge_module.BindingStore(Path(self.temp.name) / "bindings.json")
        self.api = FakeAPI()
        self.sent = []
        self.bridge = bridge_module.FeishuAdminBridge(
            self.api, self.store, lambda chat_id, text: self.sent.append((chat_id, text)) or True
        )

    def tearDown(self):
        self.temp.cleanup()

    def test_list_switch_and_current(self):
        listing = self.bridge.handle("chat-a", "/list")
        self.assertIn("1. First", listing)
        self.assertIn("2. Second", listing)
        result = self.bridge.handle("chat-a", "/switch 1")
        self.assertIn("First", result)
        self.assertEqual(self.store.get("chat-a")["cursor"], 2)
        self.assertIn("First", self.bridge.handle("chat-a", "/current"))

    def test_feishu_input_is_persisted_but_not_echoed(self):
        self.bridge.handle("chat-a", "/switch 1")
        self.assertIsNone(self.bridge.handle("chat-a", "hello"))
        self.assertEqual(self.api.puts, [("s1", "hello")])
        self.bridge.poll_once()
        self.assertEqual(self.sent, [])
        self.api.tasks_by_sid["s1"][-1]["outputs"] = ["world"]
        self.bridge.poll_once()
        self.assertEqual(self.sent, [("chat-a", "world")])

    def test_admin_side_turn_is_forwarded(self):
        self.bridge.handle("chat-a", "/switch s1")
        self.api.tasks_by_sid["s1"].append({"input": "from admin", "outputs": ["answer"]})
        self.bridge.poll_once()
        self.assertEqual(self.sent, [("chat-a", "[Admin]\nfrom admin"), ("chat-a", "answer")])

    def test_failed_send_is_retried_without_advancing_cursor(self):
        self.bridge.handle("chat-a", "/switch s1")
        initial_cursor = self.store.get("chat-a")["cursor"]
        self.api.tasks_by_sid["s1"].append({"input": "from admin", "outputs": ["answer"]})
        attempts = []

        def flaky_send(chat_id, text):
            attempts.append((chat_id, text))
            return len(attempts) > 1

        self.bridge.send = flaky_send
        self.bridge.poll_once()
        self.assertEqual(self.store.get("chat-a")["cursor"], initial_cursor)
        self.bridge.poll_once()
        self.assertEqual(attempts, [
            ("chat-a", "[Admin]\nfrom admin"),
            ("chat-a", "[Admin]\nfrom admin"),
            ("chat-a", "answer"),
        ])
        self.assertEqual(self.store.get("chat-a")["cursor"], initial_cursor + 2)

    def test_binding_survives_reload(self):
        self.bridge.handle("chat-a", "/switch work/s2")
        reloaded = bridge_module.BindingStore(Path(self.temp.name) / "bindings.json")
        self.assertEqual(reloaded.get("chat-a")["session_id"], "s2")


if __name__ == "__main__":
    unittest.main()
