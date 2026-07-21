import importlib.util
import os
import queue
import sys
import unittest
from unittest import mock
from pathlib import Path
from types import ModuleType, SimpleNamespace


# Import production worker code without rewriting its pre-existing tracked cache.
sys.dont_write_bytecode = True
GA_ROOT = Path(__file__).resolve().parents[4]
if str(GA_ROOT) not in sys.path:
    sys.path.insert(0, str(GA_ROOT))
WORKER_PATH = Path(__file__).with_name("chat_worker.py")
SPEC = importlib.util.spec_from_file_location("ga_admin_chat_worker_under_test", WORKER_PATH)
chat_worker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(chat_worker)


class FakeWorker:
    def is_alive(self):
        return True


class FakeAgent:
    def __init__(self, fail=False):
        self.fail = fail
        self.prompts = []
        self.llm_no = 0
        self.history = []
        self.handler = SimpleNamespace(working={})
        backend = SimpleNamespace(history=[], model="official-model", reasoning_effort=None)
        self.llmclient = SimpleNamespace(backend=backend, last_tools="cached")

    def next_llm(self, llm_no):
        self.llm_no = llm_no

    def put_task(self, prompt, source=None):
        self.prompts.append((prompt, source))
        if self.fail:
            raise RuntimeError("official task failed")
        self.llmclient.backend.history.append(
            {"role": "assistant", "content": [{"type": "text", "text": "resumed"}]}
        )
        self.history = [{"role": "assistant", "summary": "official resume state"}]
        self.handler = SimpleNamespace(working={"checkpoint": "restored"})
        output = queue.Queue()
        output.put({"next": "res"})
        output.put({"done": "resumed"})
        return output


class ChatWorkerProtocolTest(unittest.TestCase):
    def setUp(self):
        self.events = []
        self.old_emit = chat_worker.emit
        self.old_cwd = os.getcwd()
        chat_worker.emit = self.events.append

    def tearDown(self):
        chat_worker.emit = self.old_emit
        os.chdir(self.old_cwd)

    def request(self, prompt="/resume"):
        return {
            "prompt": prompt,
            "history": [{"role": "user", "content": "before"}],
            "raw_history": [{"role": "user", "content": [{"type": "text", "text": "raw-before"}]}],
            "history_info": [{"role": "user", "summary": "before"}],
            "working": {"checkpoint": "before"},
            "llm_no": 0,
            "ga_root": str(WORKER_PATH.parents[1]),
            "project_mode": "",
            "reasoning_effort": "high",
        }

    def test_resume_reaches_official_put_task_literal_and_done_state_sync(self):
        agent = FakeAgent()

        chat_worker.handle_request(agent, FakeWorker(), self.request())

        self.assertEqual(agent.prompts, [("/resume", "admin_chat")])
        self.assertFalse(any(event.get("type") == "btw_done" for event in self.events))
        done = next(event for event in self.events if event.get("type") == "done")
        self.assertEqual(done["message"]["content"], "resumed")
        self.assertEqual(done["message"]["model_id"], "official-model")
        self.assertEqual(done["raw_history"][-1]["content"][0]["text"], "resumed")
        self.assertEqual(done["history_info"], [{"role": "assistant", "summary": "official resume state"}])
        self.assertEqual(done["working"], {"checkpoint": "restored"})
        self.assertEqual(done["reasoning_effort"], "high")
        self.assertIn("usage", done)
        self.assertIn("usages", done)

    def test_ordinary_request_does_not_initialize_worldline(self):
        agent = FakeAgent()
        with mock.patch.object(chat_worker, "_ensure_worldline_store") as ensure:
            chat_worker.handle_request(agent, FakeWorker(), self.request("ordinary prompt"))
        ensure.assert_not_called()

    def test_extra_system_prompts_are_replaced_and_cleared_each_turn(self):
        agent = FakeAgent()
        first = self.request("first prompt")
        first["extra_sys_prompts"] = ["  be concise  ", "", "use JSON"]

        chat_worker.handle_request(agent, FakeWorker(), first)
        self.assertEqual(agent.extra_sys_prompts, ["be concise", "use JSON"])

        chat_worker.handle_request(agent, FakeWorker(), self.request("second prompt"))
        self.assertEqual(agent.extra_sys_prompts, [])

    def test_official_task_error_keeps_ordinary_error_protocol_shape(self):
        agent = FakeAgent(fail=True)

        chat_worker.handle_request(agent, FakeWorker(), self.request("ordinary prompt"))

        self.assertEqual(agent.prompts, [("ordinary prompt", "admin_chat")])
        error = next(event for event in self.events if event.get("type") == "error")
        self.assertTrue(error["message"]["error"])
        self.assertEqual(error["message"]["model_id"], "official-model")
        self.assertIn("official task failed", error["message"]["content"])
        self.assertEqual(error["raw_history"][0]["content"][0]["text"], "raw-before")
        self.assertEqual(error["reasoning_effort"], "high")
        self.assertIn("usage", error)
        self.assertIn("usages", error)
    def test_ultraplan_alias_is_an_ordinary_agent_task_and_preserves_raw_delta(self):
        for command in ("/ultraplan ship feature", "/ultralplan ship feature"):
            with self.subTest(command=command):
                self.events.clear()
                agent = FakeAgent()
                with mock.patch.object(
                    chat_worker, "_capture_ultraplan_dashboard_baseline", return_value={}
                ) as capture, mock.patch.object(
                    chat_worker, "_observe_ultraplan_daemon", return_value=None
                ) as observe:
                    chat_worker.handle_request(agent, FakeWorker(), self.request(command))

                self.assertEqual(len(agent.prompts), 1)
                rendered, source = agent.prompts[0]
                self.assertEqual(source, "admin_chat")
                self.assertIn("Objective: ship feature", rendered)
                self.assertIn("memory", rendered)
                self.assertIn("ultraplan_sop.md", rendered)
                self.assertNotIn("admin_chat_ultraplan.py", rendered)
                self.assertNotIn("/exec", rendered)
                self.assertEqual(
                    [event["delta"] for event in self.events if event.get("type") == "delta"],
                    ["res"],
                )
                capture.assert_called_once_with()
                observe.assert_called_once()


class UltraPlanReadOnlyObserverTests(unittest.TestCase):
    class _Response:
        def __init__(self, body):
            self.body = body

        def __enter__(self):
            return self

        def __exit__(self, exc_type, exc, tb):
            return False

        def read(self):
            return self.body

    def test_dashboard_fetch_uses_default_daemon_get_without_request_body(self):
        body = b"<html><pre>rundir: C:/temp/official-run\n</pre></html>"
        with mock.patch(
            "urllib.request.urlopen", return_value=self._Response(body)
        ) as urlopen:
            sessions = chat_worker._fetch_ultraplan_dashboard_sessions(timeout=0.42)

        urlopen.assert_called_once_with(
            "http://127.0.0.1:47831/", timeout=0.42
        )
        self.assertIsInstance(sessions, dict)

    def test_session_selection_detects_new_and_changed_official_runs(self):
        old = {"current": "phase-a", "phases": [], "tasks": [], "events": []}
        changed = {"current": "phase-b", "phases": [], "tasks": [], "events": []}
        baseline = {"C:/runs/existing": chat_worker._ultraplan_session_signature(old)}

        self.assertEqual(
            chat_worker._select_ultraplan_session(
                {"C:/runs/existing": old, "C:/runs/new-objective": changed},
                baseline,
                objective="new objective",
            ),
            "C:/runs/new-objective",
        )
        self.assertEqual(
            chat_worker._select_ultraplan_session(
                {"C:/runs/existing": changed}, baseline
            ),
            "C:/runs/existing",
        )
        self.assertIsNone(
            chat_worker._select_ultraplan_session(
                {"C:/runs/existing": old}, baseline
            )
        )

    def test_observer_projects_official_session_without_starting_it(self):
        parsed = {
            "current": "phase-b",
            "phases": [{"name": "phase-b", "status": "running"}],
            "tasks": [{"desc": "lens", "status": "running"}],
            "events": [{"time": 0.2, "msg": "started"}],
            "done": False,
        }
        stop = chat_worker.threading.Event()
        events = []
        state = {"objective": "ship feature"}

        def fetch_once():
            stop.set()
            return {"C:/runs/official": parsed}

        with mock.patch.object(
            chat_worker, "_fetch_ultraplan_dashboard_sessions", side_effect=fetch_once
        ) as fetch, mock.patch.object(chat_worker, "_tail_ultraplan_outputs") as tail:
            chat_worker._observe_ultraplan_daemon(
                "ship feature", {}, state, events.append, stop
            )

        fetch.assert_called_once_with()
        tail.assert_called_once()
        self.assertEqual(state["run_dir"], "C:/runs/official")
        self.assertEqual(state["dashboard_port"], 47831)
        self.assertEqual(state["dashboard_url"], "http://127.0.0.1:47831/")
        self.assertEqual(state["phases"], parsed["phases"])
        self.assertEqual([event["type"] for event in events], ["ultraplan_event"])

    def test_output_tail_emits_file_lines_unchanged_and_only_once(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "001_lens.out.txt"
            output.write_bytes(b"alpha\nbeta\n")
            state = {}
            events = []
            tail_state = {}

            chat_worker._tail_ultraplan_outputs(
                tmp, state, events.append, tail_state
            )
            with output.open("ab") as stream:
                stream.write(b"gamma\n")
            chat_worker._tail_ultraplan_outputs(
                tmp, state, events.append, tail_state
            )

        self.assertEqual(
            events,
            [
                {
                    "type": "ultraplan_output",
                    "task_id": "001_lens",
                    "lines": ["alpha", "beta"],
                },
                {
                    "type": "ultraplan_output",
                    "task_id": "001_lens",
                    "lines": ["gamma"],
                },
            ],
        )
        self.assertEqual(
            state["task_outputs"]["001_lens"], ["alpha", "beta", "gamma"]
        )


class PlanPayloadAdapterTests(unittest.TestCase):
    def test_spaced_delegate_and_done_markers_are_consumed(self):
        payload = {
            "active": True,
            "placeholder": False,
            "done": 0,
            "total": 1,
            "complete": False,
            "items": [{
                # GA already consumed the first [D] from
                # `- [D] [\u2713] [VERIFY] ...`; this is its canonical output.
                "content": "[\u2713] [VERIFY] \u4e3bagent\u6267\u884c\u9a8c\u6536",
                "status": "open",
            }],
        }

        adapted = chat_worker._adapt_plan_payload(payload)

        self.assertEqual(adapted["items"], [{
            "content": "[VERIFY] \u4e3bagent\u6267\u884c\u9a8c\u6536",
            "status": "done",
        }])
        self.assertEqual((adapted["done"], adapted["total"], adapted["complete"]), (1, 1, True))
        self.assertEqual(payload["items"][0]["content"], "[\u2713] [VERIFY] \u4e3bagent\u6267\u884c\u9a8c\u6536")

    def test_semantic_bracket_tag_is_preserved(self):
        payload = {
            "active": True,
            "done": 0,
            "total": 1,
            "complete": False,
            "items": [{"content": "[VERIFY] smoke test", "status": "open"}],
        }

        adapted = chat_worker._adapt_plan_payload(payload)

        self.assertEqual(adapted["items"], payload["items"])
        self.assertEqual((adapted["done"], adapted["total"], adapted["complete"]), (0, 1, False))

    def test_snapshot_plan_adapts_canonical_payload(self):
        payload = {
            "active": True,
            "placeholder": False,
            "done": 0,
            "total": 1,
            "complete": False,
            "items": [{"content": "[\u2713] [VERIFY] wired", "status": "open"}],
        }
        plan_state = ModuleType("frontends.plan_state")
        plan_state.desktop_plan_payload_from_session = lambda sess, root: payload
        frontends = ModuleType("frontends")
        frontends.__path__ = []
        frontends.plan_state = plan_state

        with mock.patch.dict(sys.modules, {
            "frontends": frontends,
            "frontends.plan_state": plan_state,
        }), mock.patch.object(chat_worker, "_snapshot_ga_state", return_value={"working": {}}):
            adapted = chat_worker._snapshot_plan(FakeAgent(), "C:/ga")

        self.assertEqual(adapted["items"], [{
            "content": "[VERIFY] wired",
            "status": "done",
        }])
        self.assertEqual((adapted["done"], adapted["total"], adapted["complete"]), (1, 1, True))


if __name__ == "__main__":
    unittest.main()
