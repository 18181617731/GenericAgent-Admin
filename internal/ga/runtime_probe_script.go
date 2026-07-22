package ga

const runtimeProbeScript = `
import importlib
import json
import sys
import traceback

result = {
    "python_path": sys.executable,
    "python_version": sys.version.split()[0],
    "dependencies": {},
    "agentmain_ok": False,
    "agentmain_error": "",
    "ultraplan_ok": False,
    "ultraplan_missing": [],
    "ultraplan_error": "",
}

for module_name in ["requests", "bs4", "bottle", "simple_websocket_server", "aiohttp"]:
    try:
        importlib.import_module(module_name)
        result["dependencies"][module_name] = {"ok": True, "error": ""}
    except BaseException:
        result["dependencies"][module_name] = {"ok": False, "error": traceback.format_exc()}

try:
    importlib.import_module("agentmain")
    result["agentmain_ok"] = True
except BaseException:
    result["agentmain_error"] = traceback.format_exc()

try:
    ultraplan = importlib.import_module("assets.ga_ultraplan")
    result["ultraplan_missing"] = [name for name in ["phase", "parallel", "mapchain"] if not hasattr(ultraplan, name)]
    result["ultraplan_ok"] = not result["ultraplan_missing"]
except BaseException:
    result["ultraplan_error"] = traceback.format_exc()

print("GA_ADMIN_RUNTIME=" + json.dumps(result, ensure_ascii=False), flush=True)
`
