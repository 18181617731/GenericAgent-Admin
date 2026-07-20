import importlib.util
import os
import queue
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace


# Import production worker code without rewriting its pre-existing tracked cache.
sys.dont_write_bytecode = True
RUNTIME_ROOT = Path(__file__).resolve().parent
if str(RUNTIME_ROOT) not in sys.path:
    sys.path.insert(0, str(RUNTIME_ROOT))
WORKER_PATH = Path(__file__).with_name("chat_worker.py")
SPEC = importlib.util.spec_from_file_location("ga_admin_chat_worker_under_test", WORKER_PATH)
chat_worker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(chat_worker)
# Protocol tests use a minimal fake Agent without a GenericAgent checkout.
chat_worker._install_worldline_hook = lambda: None


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


if __name__ == "__main__":
    unittest.main()
