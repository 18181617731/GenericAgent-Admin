import importlib.util
import json
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("chat_feishu_bridge.py")
spec = importlib.util.spec_from_file_location("chat_feishu_bridge", SCRIPT)
bridge_module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge_module)


class FeishuCardPayloadTest(unittest.TestCase):
    def test_task_card_matches_official_fsapp_shape(self):
        payload = json.loads(bridge_module._task_card("### Result\n\n**done**"))
        self.assertEqual(payload["schema"], "2.0")
        self.assertEqual(payload["config"], {"streaming_mode": False, "width_mode": "fill"})
        self.assertEqual(payload["body"]["elements"], [
            {"tag": "markdown", "content": "**✅ 已完成**"},
            {"tag": "hr"},
            {"tag": "markdown", "content": "### Result\n\n**done**"},
        ])

    def test_info_card_uses_markdown(self):
        payload = json.loads(bridge_module._info_card("**GA Admin**\n\n/list"))
        self.assertEqual(payload["body"]["elements"], [
            {"tag": "markdown", "content": "**GA Admin**\n\n/list"},
        ])

    def test_send_prefers_interactive_completion_card(self):
        attempts = []

        class Response:
            def success(self):
                return True

        def send_once(msg_type, content):
            attempts.append((msg_type, content))
            return Response()

        self.assertTrue(bridge_module._send_with_fallback(send_once, "answer", completed=True))
        self.assertEqual([item[0] for item in attempts], ["interactive"])
        self.assertEqual(
            json.loads(attempts[0][1])["body"]["elements"][0]["content"],
            "**\u2705 \u5df2\u5b8c\u6210**",
        )

    def test_send_falls_back_to_plain_text(self):
        attempts = []

        class Response:
            code = 230001
            msg = "card rejected"

            def __init__(self, ok):
                self.ok = ok

            def success(self):
                return self.ok

        responses = iter([Response(False), Response(True)])

        def send_once(msg_type, content):
            attempts.append((msg_type, content))
            return next(responses)

        self.assertTrue(bridge_module._send_with_fallback(send_once, "plain **text**"))
        self.assertEqual([item[0] for item in attempts], ["interactive", "text"])
        self.assertEqual(json.loads(attempts[1][1]), {"text": "plain **text**"})


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
            self.api,
            self.store,
            lambda chat_id, text, completed=False: self.sent.append((chat_id, text, completed)) or True,
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

    def test_list_paginates_with_global_switch_numbers(self):
        self.api.items = [
            {
                "peer": "ga-admin/default/s%d" % index,
                "instance_id": "_",
                "session_id": "s%d" % index,
                "title": "Session %d" % index,
            }
            for index in range(1, 13)
        ]
        self.api.tasks_by_sid = {"s%d" % index: [] for index in range(1, 13)}

        first = self.bridge.handle("chat-a", "/list")
        self.assertIn("\u7b2c 1/2 \u9875", first)
        self.assertIn("1. Session 1", first)
        self.assertIn("10. Session 10", first)
        self.assertNotIn("11. Session 11", first)
        self.assertIn("/list 2", first)

        second = self.bridge.handle("chat-a", "/ga list 2")
        self.assertIn("\u7b2c 2/2 \u9875", second)
        self.assertNotIn("10. Session 10", second)
        self.assertIn("11. Session 11", second)
        self.assertIn("12. Session 12", second)
        self.assertIn("/list 1", second)
        self.assertNotIn("/list 3", second)

        switched = self.bridge.handle("chat-a", "/switch 11")
        self.assertIn("Session 11", switched)
        self.assertEqual(self.store.get("chat-a")["session_id"], "s11")

    def test_list_rejects_invalid_page(self):
        self.assertIn("\u6b63\u6574\u6570", self.bridge.handle("chat-a", "/list nope"))
        self.assertIn("\u4ece 1 \u5f00\u59cb", self.bridge.handle("chat-a", "/list 0"))
        self.assertIn("\u5171 1 \u9875", self.bridge.handle("chat-a", "/list 2"))

    def test_feishu_input_is_persisted_but_not_echoed(self):
        self.bridge.handle("chat-a", "/switch 1")
        self.assertIsNone(self.bridge.handle("chat-a", "hello"))
        self.assertEqual(self.api.puts, [("s1", "hello")])
        self.bridge.poll_once()
        self.assertEqual(self.sent, [])
        self.api.tasks_by_sid["s1"][-1]["outputs"] = ["world"]
        self.bridge.poll_once()
        self.assertEqual(self.sent, [("chat-a", "world", True)])

    def test_admin_side_turn_is_forwarded(self):
        self.bridge.handle("chat-a", "/switch s1")
        self.api.tasks_by_sid["s1"].append({"input": "from admin", "outputs": ["answer"]})
        self.bridge.poll_once()
        self.assertEqual(self.sent, [
            ("chat-a", "[Admin]\nfrom admin", False),
            ("chat-a", "answer", True),
        ])

    def test_failed_send_is_retried_without_advancing_cursor(self):
        self.bridge.handle("chat-a", "/switch s1")
        initial_cursor = self.store.get("chat-a")["cursor"]
        self.api.tasks_by_sid["s1"].append({"input": "from admin", "outputs": ["answer"]})
        attempts = []

        def flaky_send(chat_id, text, completed=False):
            attempts.append((chat_id, text, completed))
            return len(attempts) > 1

        self.bridge.send = flaky_send
        self.bridge.poll_once()
        self.assertEqual(self.store.get("chat-a")["cursor"], initial_cursor)
        self.bridge.poll_once()
        self.assertEqual(attempts, [
            ("chat-a", "[Admin]\nfrom admin", False),
            ("chat-a", "[Admin]\nfrom admin", False),
            ("chat-a", "answer", True),
        ])
        self.assertEqual(self.store.get("chat-a")["cursor"], initial_cursor + 2)

    def test_binding_survives_reload(self):
        self.bridge.handle("chat-a", "/switch work/s2")
        reloaded = bridge_module.BindingStore(Path(self.temp.name) / "bindings.json")
        self.assertEqual(reloaded.get("chat-a")["session_id"], "s2")


if __name__ == "__main__":
    unittest.main()
