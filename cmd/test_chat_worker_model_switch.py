import json
import os
import queue
import subprocess
import sys
import tempfile
import textwrap
import threading
import unittest
from pathlib import Path


WORKER = Path(__file__).with_name('chat_worker.py')


class ChatWorkerModelSwitchTest(unittest.TestCase):
    def test_switches_the_real_backend_between_requests(self):
        with self.worker() as process:
            first = self.request(process, {'prompt': 'first', 'llm_no': 0})
            second = self.request(process, {'prompt': 'second', 'llm_no': 1})

        self.assertEqual(first[0], {'type': 'model', 'model_id': 'model-zero', 'llm_no': 0})
        self.assertEqual(first[-1]['message']['content'], 'used model-zero')
        self.assertEqual(second[0], {'type': 'model', 'model_id': 'model-one', 'llm_no': 1})
        self.assertEqual(second[-1]['message']['content'], 'used model-one')
        self.assertEqual(second[-1]['message']['llm_no'], 1)

    def test_does_not_use_the_old_backend_when_switch_fails(self):
        with self.worker(fail_switch=True) as process:
            events = self.request(process, {'prompt': 'must not run', 'llm_no': 1})

        self.assertEqual([event['type'] for event in events], ['error'])
        self.assertIn('model switch rejected by fake GA', events[0]['message']['content'])
        self.assertNotIn('used model-zero', events[0]['message']['content'])
        self.assertEqual(events[0]['message']['llm_no'], 0)

    def worker(self, fail_switch=False):
        return WorkerProcess(fail_switch)

    def request(self, process, payload):
        process.stdin.write(json.dumps(payload) + '\n')
        process.stdin.flush()
        events = []
        while True:
            lines = queue.Queue()
            threading.Thread(target=lambda: lines.put(process.stdout.readline()), daemon=True).start()
            try:
                line = lines.get(timeout=10)
            except queue.Empty:
                process.kill()
                _, stderr = process.communicate(timeout=5)
                self.fail('worker protocol timed out:\n' + stderr)
            self.assertTrue(line, 'worker exited without protocol output')
            event = json.loads(line)
            events.append(event)
            if event.get('type') in ('done', 'error'):
                return events


class WorkerProcess:
    def __init__(self, fail_switch):
        self.fail_switch = fail_switch
        self.temp = None
        self.process = None

    def __enter__(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        (root / 'agentmain.py').write_text(FAKE_AGENT, encoding='utf-8')
        env = os.environ.copy()
        env['GA_ROOT'] = str(root)
        if self.fail_switch:
            env['FAIL_MODEL_SWITCH'] = '1'
        self.process = subprocess.Popen(
            [sys.executable, str(WORKER)],
            cwd=root,
            env=env,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding='utf-8',
        )
        return self.process

    def __exit__(self, exc_type, exc_value, traceback):
        if self.process:
            self.process.kill()
            self.process.wait(timeout=5)
            for stream in (self.process.stdin, self.process.stdout, self.process.stderr):
                stream.close()
        if self.temp:
            self.temp.cleanup()


FAKE_AGENT = textwrap.dedent(r'''
    import os
    import queue
    import time

    class Backend:
        def __init__(self, model):
            self.model = model
            self.history = []

    class Client:
        def __init__(self, model):
            self.backend = Backend(model)
            self.last_tools = ''

    class GeneraticAgent:
        def __init__(self):
            self.llmclients = [Client('model-zero'), Client('model-one')]
            self.llm_no = 0
            self.llmclient = self.llmclients[0]
            self.history = []
            self.handler = None
            self.script_dir = os.getcwd()
            self.verbose = False
            self.inc_out = True

        def next_llm(self, index):
            if os.environ.get('FAIL_MODEL_SWITCH') == '1' and index == 1:
                raise RuntimeError('model switch rejected by fake GA')
            self.llm_no = index
            self.llmclient = self.llmclients[index]

        def run(self):
            while True:
                time.sleep(0.1)

        def put_task(self, prompt, source='admin_chat'):
            output = queue.Queue()
            output.put({'done': 'used ' + self.llmclient.backend.model})
            return output
''')


if __name__ == '__main__':
    unittest.main()
