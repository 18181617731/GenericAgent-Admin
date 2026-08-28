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

    def test_official_task_card_is_loaded_without_importing_fsapp(self):
        with tempfile.TemporaryDirectory() as temp:
            frontends = Path(temp) / "frontends"
            frontends.mkdir()
            (frontends / "fsapp.py").write_text(
                "raise RuntimeError('fsapp module must not be imported')\n"
                "class _TaskCard:\n"
                "    marker = 'official-source'\n"
                "    def __init__(self, receive_id, receive_id_type):\n"
                "        self.receive_id = receive_id\n"
                "        self.receive_id_type = receive_id_type\n"
                "    def start(self):\n"
                "        return _send_raw(self.receive_id, _card_raw([]), 'interactive', self.receive_id_type)\n",
                encoding="utf-8",
            )
            calls = []
            task_card = bridge_module._load_official_task_card(
                temp,
                lambda *args: calls.append(args) or "message-id",
                lambda *args: True,
                lambda *args, **kwargs: True,
            )
            card = task_card("chat-a", "chat_id")
            self.assertEqual(card.marker, "official-source")
            self.assertEqual(card.start(), "message-id")
            self.assertEqual(calls[0][0], "chat-a")
            self.assertEqual(calls[0][2:], ("interactive", "chat_id"))

    def test_live_task_card_retries_failed_partial_patch(self):
        class OfficialCard:
            def __init__(self):
                self.steps = []
                self.status = ""
                self.push_results = [False, True]
                self.pushes = 0

            def _push(self):
                self.pushes += 1
                return self.push_results.pop(0)

        official = OfficialCard()
        card = bridge_module._LiveTaskCard(official)
        self.assertFalse(card.update("draft"))
        self.assertTrue(card.update("draft"))
        self.assertEqual(official.pushes, 2)
        self.assertEqual(official.steps, [("\u5b9e\u65f6\u8f93\u51fa", "draft")])


class FakeAPI:
    def __init__(self):
        self.items = [
            {"peer": "ga-admin/default/s1", "instance_id": "_", "session_id": "s1", "title": "First"},
            {"peer": "ga-admin/work/s2", "instance_id": "work", "session_id": "s2", "title": "Second"},
        ]
        self.tasks_by_sid = {"s1": [{"input": "old", "outputs": ["done"]}], "s2": []}
        self.puts = []
        self.snapshot_value = None

    def sessions(self):
        return list(self.items)

    def tasks(self, item):
        return self.tasks_by_sid[item["session_id"]]

    def snapshot(self, item):
        if self.snapshot_value is None:
            return {"tasks": self.tasks(item), "run": False, "partial": ""}
        return self.snapshot_value

    def put(self, item, text):
        self.puts.append((item["session_id"], text))
        self.tasks_by_sid[item["session_id"]].append({"input": text, "outputs": []})


class RecordingCard:
    def __init__(self, update_results=None):
        self.events = []
        self.update_results = list(update_results or [])

    def start(self):
        self.events.append(("start", ""))

    def update(self, text):
        self.events.append(("update", text))
        return self.update_results.pop(0) if self.update_results else True

    def done(self, text):
        self.events.append(("done", text))

    def fail(self, text):
        self.events.append(("fail", text))


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

    def test_streaming_card_updates_then_completes_without_phantom_card(self):
        cards = []

        def new_card(chat_id):
            card = RecordingCard()
            cards.append((chat_id, card))
            return card

        self.bridge.card_factory = new_card
        self.bridge.handle("chat-a", "/switch s1")
        self.assertIsNone(self.bridge.handle("chat-a", "hello"))
        self.api.snapshot_value = {
            "tasks": self.api.tasks_by_sid["s1"],
            "run": True,
            "partial": "draft",
        }
        self.bridge.poll_once()
        self.assertEqual(cards[0][1].events, [("start", ""), ("update", "draft")])

        self.api.tasks_by_sid["s1"][-1]["outputs"] = ["final"]
        self.bridge.poll_once()
        self.assertEqual(cards[0][1].events, [
            ("start", ""),
            ("update", "draft"),
            ("done", "final"),
        ])
        self.assertEqual(len(cards), 1)
        self.assertEqual(self.bridge.active_cards, {})
        self.assertEqual(self.store.get("chat-a")["cursor"], 4)
        self.assertEqual(self.sent, [])

    def test_failed_partial_update_is_retried_on_next_poll(self):
        card = RecordingCard(update_results=[False, True])
        self.bridge.card_factory = lambda chat_id: card
        self.bridge.handle("chat-a", "/switch s1")
        self.api.snapshot_value = {
            "tasks": self.api.tasks_by_sid["s1"],
            "run": True,
            "partial": "draft",
        }
        self.bridge.poll_once()
        self.bridge.poll_once()
        self.assertEqual(card.events, [
            ("start", ""),
            ("update", "draft"),
            ("update", "draft"),
        ])
        self.assertEqual(len(self.bridge.active_cards), 1)

    def test_switch_retires_active_card_and_clears_pending_echo(self):
        card = RecordingCard()
        self.bridge.card_factory = lambda chat_id: card
        self.bridge.handle("chat-a", "/switch s1")
        self.api.snapshot_value = {
            "tasks": self.api.tasks_by_sid["s1"],
            "run": True,
            "partial": "draft",
        }
        self.bridge.poll_once()
        self.assertIsNone(self.bridge.handle("chat-a", "pending"))

        response = self.bridge.handle("chat-a", "/switch work/s2")
        self.assertIn("Second", response)
        self.assertEqual(card.events[-1], ("fail", "\u5df2\u5207\u6362 Admin \u4f1a\u8bdd"))
        self.assertEqual(self.bridge.active_cards, {})
        self.assertNotIn("chat-a", self.bridge.pending_inputs)
        self.assertEqual(self.store.get("chat-a")["session_id"], "s2")

    def test_binding_survives_reload(self):
        self.bridge.handle("chat-a", "/switch work/s2")
        reloaded = bridge_module.BindingStore(Path(self.temp.name) / "bindings.json")
        self.assertEqual(reloaded.get("chat-a")["session_id"], "s2")


if __name__ == "__main__":
    unittest.main()
