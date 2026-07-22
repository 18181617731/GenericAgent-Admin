import importlib.util
import sys
import tempfile
import types
import unittest
from pathlib import Path


sys.dont_write_bytecode = True
WORKER_PATH = Path(__file__).with_name("chat_worker.py")
SPEC = importlib.util.spec_from_file_location("ga_admin_chat_worker_ultraplan_tested", WORKER_PATH)
worker = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(worker)


class UltraPlanScriptTests(unittest.TestCase):
    def test_generated_script_uses_current_ga_ultraplan_api(self):
        with tempfile.TemporaryDirectory() as temp:
            run_dir = Path(temp) / "run"
            script = worker._build_ultraplan_script(temp, run_dir, "test objective", 0)

        self.assertIn("from assets.ga_ultraplan import phase, parallel", script)
        self.assertNotIn("import plan", script)
        self.assertNotIn("plan(RUN_DIR)", script)
        compile(script, "admin_chat_ultraplan.py", "exec")

    def test_generated_import_accepts_module_without_removed_plan_symbol(self):
        assets = types.ModuleType("assets")
        ultraplan = types.ModuleType("assets.ga_ultraplan")
        ultraplan.phase = object()
        ultraplan.parallel = object()
        previous_assets = sys.modules.get("assets")
        previous_ultraplan = sys.modules.get("assets.ga_ultraplan")
        sys.modules["assets"] = assets
        sys.modules["assets.ga_ultraplan"] = ultraplan
        try:
            exec("from assets.ga_ultraplan import phase, parallel", {})
        finally:
            if previous_assets is None:
                sys.modules.pop("assets", None)
            else:
                sys.modules["assets"] = previous_assets
            if previous_ultraplan is None:
                sys.modules.pop("assets.ga_ultraplan", None)
            else:
                sys.modules["assets.ga_ultraplan"] = previous_ultraplan


if __name__ == "__main__":
    unittest.main()
