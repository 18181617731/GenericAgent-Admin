"""Portable bundle self-healing bootstrap.

Runs with the BUNDLED BASE interpreter (python\\python.exe), never with the
venv interpreter -- the whole point is that the venv may be broken until this
script repairs it.

What it does, all idempotent:
  1. rewrite GenericAgent/.venv/pyvenv.cfg so home/executable/command point at
     the bundled python of the CURRENT extraction path (a moved venv otherwise
     dies with rc=103 "No Python at '<old path>'").
  2. fill config.local.json ga_root / python_path when they are empty or point
     at a path that no longer exists, leaving every other user setting alone.

Exit code is always 0 unless the bundle is structurally broken, so a launcher
can call it unconditionally on every start.
"""

from __future__ import annotations

import json
import os
import sys

ROOT = os.path.dirname(os.path.abspath(__file__))
# Canonical bundles place bootstrap.py beside GenericAgent/.  A short-lived
# cross-platform builder placed it inside GenericAgent/ instead, so retain
# compatibility with those artifacts while preferring the published layout.
if os.path.isdir(os.path.join(ROOT, "GenericAgent")):
    BUNDLE_ROOT = ROOT
    GA_ROOT = os.path.join(ROOT, "GenericAgent")
elif os.path.basename(ROOT).lower() == "genericagent":
    BUNDLE_ROOT = os.path.dirname(ROOT)
    GA_ROOT = ROOT
else:
    BUNDLE_ROOT = ROOT
    GA_ROOT = os.path.join(ROOT, "GenericAgent")
PY_HOME = os.path.join(ROOT, "python")
VENV_DIR = os.path.join(GA_ROOT, ".venv")
CONFIG = os.path.join(BUNDLE_ROOT, "config.local.json")

IS_WIN = os.name == "nt"
BASE_PY = os.path.join(PY_HOME, "python.exe" if IS_WIN else os.path.join("bin", "python3"))
VENV_PY = (
    os.path.join(VENV_DIR, "Scripts", "python.exe")
    if IS_WIN
    else os.path.join(VENV_DIR, "bin", "python")
)
REQUIRED_IMPORTS = (
    "requests", "bs4", "bottle", "aiohttp", "rich", "qrcode",
    "websockets", "fastapi", "uvicorn", "psutil", "lark_oapi",
    "telegram", "botpy", "Crypto", "wecom_aibot_sdk", "dingtalk_stream",
)


def log(msg: str) -> None:
    print("[bootstrap] " + msg, file=sys.stderr, flush=True)


def fix_pyvenv_cfg() -> bool:
    """Repoint a relocated venv at the bundled interpreter. True if changed."""
    cfg_path = os.path.join(VENV_DIR, "pyvenv.cfg")
    if not os.path.isfile(cfg_path):
        log("no pyvenv.cfg at " + cfg_path + " (skipped)")
        return False

    with open(cfg_path, "r", encoding="utf-8") as fh:
        lines = fh.read().splitlines()

    wanted = {
        "home": PY_HOME,
        "executable": BASE_PY,
        "command": BASE_PY + " -m venv " + VENV_DIR,
    }

    out = []
    seen = set()
    changed = False
    for line in lines:
        if "=" not in line:
            out.append(line)
            continue
        key = line.split("=", 1)[0].strip()
        if key in wanted:
            seen.add(key)
            new_line = key + " = " + wanted[key]
            if new_line != line:
                changed = True
            out.append(new_line)
        else:
            out.append(line)

    for key, val in wanted.items():
        if key not in seen:
            out.append(key + " = " + val)
            changed = True

    if changed:
        with open(cfg_path, "w", encoding="utf-8") as fh:
            fh.write("\n".join(out) + "\n")
        log("pyvenv.cfg repointed to " + PY_HOME)
    else:
        log("pyvenv.cfg already correct")
    return changed


def usable(path: str) -> bool:
    return bool(path) and os.path.isfile(path)


def fix_config() -> bool:
    """Fill ga_root / python_path when unset or stale. True if written."""
    cfg = {}
    if os.path.isfile(CONFIG):
        try:
            with open(CONFIG, "r", encoding="utf-8") as fh:
                cfg = json.load(fh) or {}
        except (OSError, ValueError) as exc:
            log("config.local.json unreadable (" + str(exc) + "), regenerating")
            cfg = {}
    if not isinstance(cfg, dict):
        cfg = {}

    changed = False

    # The config file lives INSIDE the bundle, so it must always describe this
    # bundle. Never keep an inherited path just because it still resolves on
    # this machine -- after a copy the old bundle usually still exists and would
    # silently keep winning (the build machine's path leaking into the copy).
    want_root = GA_ROOT.replace("\\", "/")
    if os.path.isdir(GA_ROOT):
        if str(cfg.get("ga_root") or "").strip().replace("\\", "/") != want_root:
            cfg["ga_root"] = want_root
            changed = True
            log("ga_root -> " + GA_ROOT)
    else:
        log("WARNING bundled GenericAgent missing: " + GA_ROOT)

    want_py = VENV_PY.replace("\\", "/")
    if usable(VENV_PY):
        if str(cfg.get("python_path") or "").strip().replace("\\", "/") != want_py:
            cfg["python_path"] = want_py
            changed = True
            log("python_path -> " + VENV_PY)
    else:
        log("WARNING venv interpreter missing: " + VENV_PY)

    if changed:
        tmp = CONFIG + ".tmp"
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh, ensure_ascii=False, indent=2)
            fh.write("\n")
        os.replace(tmp, CONFIG)
        log("config.local.json updated")
    else:
        log("config.local.json already points at this bundle")
    return changed


def verify() -> bool:
    """Import-check the venv interpreter so failures surface here, not later."""
    if not usable(VENV_PY):
        return False
    import subprocess

    probe = (
        "import sys;"
        "import " + ", ".join(REQUIRED_IMPORTS) + ";"
        "print(sys.version.split()[0])"
    )
    try:
        res = subprocess.run(
            [VENV_PY, "-c", probe],
            capture_output=True,
            text=True,
            timeout=90,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        log("WARNING venv probe failed to launch: " + str(exc))
        return False
    if res.returncode == 0:
        log("venv ok, python " + res.stdout.strip())
        return True
    log("WARNING venv probe rc=" + str(res.returncode) + " " + (res.stderr or "").strip()[:400])
    return False


def main() -> int:
    log("root " + ROOT)
    if not os.path.isfile(BASE_PY):
        log("FATAL bundled interpreter missing: " + BASE_PY)
        return 2
    fix_pyvenv_cfg()
    fix_config()
    if not verify():
        log("FATAL bundled venv verification failed")
        return 3
    return 0


if __name__ == "__main__":
    sys.exit(main())
