import importlib.util
import json
import os
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock


SOURCE = Path(__file__).with_name("bootstrap.py")


def load_bootstrap(path: Path):
    spec = importlib.util.spec_from_file_location("portable_bootstrap_test", path)
    if spec is None or spec.loader is None:
        raise RuntimeError("could not load bootstrap.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class BootstrapConfigTest(unittest.TestCase):
    def assert_layout(self, nested: bool):
        with tempfile.TemporaryDirectory() as temp:
            bundle = Path(temp)
            ga_root = bundle / "GenericAgent"
            ga_root.mkdir()
            script = (ga_root if nested else bundle) / "bootstrap.py"
            script.write_bytes(SOURCE.read_bytes())

            module = load_bootstrap(script)
            Path(module.VENV_PY).parent.mkdir(parents=True)
            Path(module.VENV_PY).write_bytes(b"portable-test-python")
            Path(module.CONFIG).write_text(
                json.dumps({"ga_root": "", "python_path": "", "host": "127.0.0.1"}),
                encoding="utf-8",
            )

            self.assertEqual(Path(module.GA_ROOT), ga_root)
            self.assertEqual(Path(module.PY_HOME), script.parent / "python")
            self.assertTrue(module.fix_config())
            data = json.loads(Path(module.CONFIG).read_text(encoding="utf-8"))
            self.assertEqual(
                data["ga_root"].replace("\\", "/"),
                str(ga_root).replace("\\", "/"),
            )
            self.assertEqual(
                data["python_path"].replace("\\", "/"),
                str(Path(module.VENV_PY)).replace("\\", "/"),
            )
            self.assertEqual(Path(module.CONFIG), bundle / "config.local.json")
            self.assertFalse((ga_root / "config.local.json").exists())

    def test_writes_bundle_config_for_canonical_layout(self):
        self.assert_layout(nested=False)

    def test_required_imports_cover_frontend_service_chain(self):
        module = load_bootstrap(SOURCE)
        for name in ("rich", "qrcode", "websockets", "fastapi", "uvicorn", "psutil", "lark_oapi", "telegram", "botpy", "Crypto", "wecom_aibot_sdk", "dingtalk_stream"):
            self.assertIn(name, module.REQUIRED_IMPORTS)

    def test_verify_checks_module_specs_without_importing_sdk_packages(self):
        module = load_bootstrap(SOURCE)
        completed = SimpleNamespace(returncode=0, stdout="3.12.7\n", stderr="")
        with mock.patch.object(module, "usable", return_value=True), mock.patch(
            "subprocess.run", return_value=completed
        ) as run:
            self.assertTrue(module.verify())

        command = run.call_args.args[0]
        probe = command[2]
        self.assertIn("importlib.util.find_spec", probe)
        self.assertNotIn("import lark_oapi", probe)
        self.assertEqual(run.call_args.kwargs["timeout"], 10)
        self.assertIn("creationflags", run.call_args.kwargs)

    def test_writes_bundle_config_for_legacy_nested_layout(self):
        self.assert_layout(nested=True)


if __name__ == "__main__":
    unittest.main()
