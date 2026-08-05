#!/usr/bin/env python3
"""
Cross-platform portable bundle builder for GenericAgent-Admin.

Packages GenericAgent source + Python runtime + venv + ga-admin binary
into a self-contained distribution.

Usage:
    python build_portable.py --platform windows --arch amd64 --out-dir ./portable-out
    python build_portable.py --platform linux --arch arm64 --out-dir ./portable-out --skip-build
"""
import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import urllib.request
import zipfile
from pathlib import Path

# Core dependencies installed into the venv
CORE_DEPS = [
    "anthropic==0.39.0",
    "httpx==0.27.2",
    "playwright==1.48.0",
    "python-dotenv==1.0.1",
    "tiktoken==0.8.0",
]

# GA source repo
GA_REPO_OWNER = "lsdefine"
GA_REPO_NAME = "GenericAgent"
DEFAULT_GA_REF = "main"

# Python version to bundle
PYTHON_VERSION = "3.12.7"


def say(msg: str):
    print(f"[portable] {msg}", flush=True)


def die(msg: str):
    print(f"[portable] ERROR: {msg}", file=sys.stderr, flush=True)
    sys.exit(1)


def run_cmd(args: list[str], cwd: Path | None = None, check: bool = True) -> subprocess.CompletedProcess:
    """Run a command and return CompletedProcess. Die on error if check=True."""
    say(f"Running: {' '.join(args)}")
    result = subprocess.run(args, cwd=cwd, capture_output=True, text=True)
    if check and result.returncode != 0:
        die(f"Command failed with exit code {result.returncode}:\n{result.stderr}")
    return result


def get_version_from_git(repo_root: Path) -> str:
    """Get version from git describe, fallback to 'dev'."""
    result = subprocess.run(
        ["git", "describe", "--tags", "--always"],
        cwd=repo_root,
        capture_output=True,
        text=True,
    )
    if result.returncode == 0 and result.stdout.strip():
        return result.stdout.strip()
    return "dev"


def download_ga_source(ref: str, dest: Path):
    """Download GA source archive from GitHub codeload."""
    url = f"https://codeload.github.com/{GA_REPO_OWNER}/{GA_REPO_NAME}/zip/refs/heads/{ref}"
    say(f"Downloading GA source from {url}")
    
    with tempfile.NamedTemporaryFile(suffix=".zip", delete=False) as tmp:
        tmp_path = Path(tmp.name)
    
    try:
        urllib.request.urlretrieve(url, tmp_path)
        say(f"Extracting to {dest}")
        
        with zipfile.ZipFile(tmp_path, "r") as zf:
            # Archive contains single top-level dir: GenericAgent-{ref}/
            members = zf.namelist()
            if not members:
                die("Empty archive")
            
            # Strip top-level directory during extraction
            top_dir = members[0].split("/")[0] + "/"
            for member in members:
                if not member.startswith(top_dir):
                    continue
                target_path = dest / member[len(top_dir):]
                
                if member.endswith("/"):
                    target_path.mkdir(parents=True, exist_ok=True)
                else:
                    target_path.parent.mkdir(parents=True, exist_ok=True)
                    with zf.open(member) as source, open(target_path, "wb") as target:
                        shutil.copyfileobj(source, target)
        
        # Prune demo assets
        demo_assets = dest / "assets" / "demo"
        if demo_assets.exists():
            shutil.rmtree(demo_assets)
            say(f"Pruned {demo_assets}")
        
        # Verify critical files
        if not (dest / "assets" / "tools_schema.json").exists():
            die("assets/tools_schema.json missing after prune - GA will not start")
        
    finally:
        tmp_path.unlink(missing_ok=True)


def install_python_runtime(stage: Path, python_version: str):
    """Install standalone Python using uv."""
    py_home = stage / "python"
    
    # Check if uv is available
    if shutil.which("uv") is None:
        die("uv not found. Install from https://github.com/astral-sh/uv")
    
    say(f"Installing Python {python_version} to {py_home}")
    py_stage = py_home.with_name(py_home.name + "_stage")
    
    run_cmd([
        "uv", "python", "install",
        python_version,
        "--install-dir", str(py_stage),
    ])
    
    # uv unpacks into <install-dir>/cpython-<ver>-<platform>/; flatten it
    py_exe_name = "python.exe" if platform.system() == "Windows" else "python"
    py_exe = py_stage / py_exe_name
    
    if py_exe.exists():
        py_stage.rename(py_home)
    else:
        # Find the actual nested directory
        nested = next(py_stage.iterdir(), None)
        if nested and nested.is_dir() and (nested / py_exe_name).exists():
            nested.rename(py_home)
            py_stage.rmdir()
        else:
            die(f"Could not find {py_exe_name} in uv Python install")
    
    say(f"Python runtime installed at {py_home}")


def create_venv_and_install_deps(ga_root: Path, python_exe: Path):
    """Create venv inside GA root and install core deps."""
    venv_path = ga_root / ".venv"
    say(f"Creating venv at {venv_path}")
    
    run_cmd([str(python_exe), "-m", "venv", str(venv_path)])
    
    # Determine venv python path
    if platform.system() == "Windows":
        venv_py = venv_path / "Scripts" / "python.exe"
    else:
        venv_py = venv_path / "bin" / "python"
    
    if not venv_py.exists():
        die(f"venv python not found at {venv_py}")
    
    say(f"Installing core deps: {', '.join(CORE_DEPS)}")
    run_cmd([str(venv_py), "-m", "pip", "install", "--upgrade", "pip", "--quiet", "--disable-pip-version-check"])
    run_cmd([str(venv_py), "-m", "pip", "install", "--quiet", "--disable-pip-version-check"] + CORE_DEPS)
    
    say("Core dependencies installed")


def write_config(stage: Path):
    """Write empty config.local.json (bootstrap will populate ga_root/python_path)."""
    config = {
        "ga_root": "",
        "python_path": "",
        "host": "127.0.0.1",
        "port": 8787,
        "vite_host": "127.0.0.1",
        "vite_port": 5173,
        "vite_allowed_hosts": [],
        "backend_proxy_host": "127.0.0.1",
        "log_tail_lines": 200,
        "buffer_lines": 1000,
    }
    
    config_path = stage / "config.local.json"
    say(f"Writing {config_path}")
    
    # Write UTF-8 without BOM
    with open(config_path, "w", encoding="utf-8") as f:
        json.dump(config, f, indent=2)
        f.write("\n")


def copy_binary(dist_dir: Path, stage: Path, target_platform: str):
    """Copy ga-admin binary from dist/ to stage root."""
    if target_platform == "windows":
        bin_name = "ga-admin.exe"
    else:
        bin_name = "ga-admin"
    
    src = dist_dir / bin_name
    if not src.exists():
        die(f"Binary not found: {src}")
    
    dest = stage / bin_name
    shutil.copy2(src, dest)
    
    # Set executable on Unix
    if target_platform in ["linux", "darwin"]:
        dest.chmod(0o755)
    
    say(f"Copied {bin_name} to {dest}")


def run_bootstrap_check(stage: Path, target_platform: str):
    """Run bootstrap.py once to verify the bundle."""
    ga_root = stage / "GenericAgent"
    bootstrap_py = ga_root / "bootstrap.py"
    
    if not bootstrap_py.exists():
        die(f"bootstrap.py not found at {bootstrap_py}")
    
    # Determine venv python
    venv_path = ga_root / ".venv"
    if target_platform == "windows":
        venv_py = venv_path / "Scripts" / "python.exe"
    else:
        venv_py = venv_path / "bin" / "python"
    
    say("Running bootstrap self-check")
    run_cmd([str(venv_py), str(bootstrap_py)])
    say("Bootstrap check passed")


def write_source_info(stage: Path, repo_owner: str, repo_name: str, ref: str, source_method: str):
    """Write PORTABLE_SOURCE.txt with provenance info."""
    info_path = stage / "PORTABLE_SOURCE.txt"
    
    # Use Python's datetime for cross-platform timestamp
    from datetime import datetime, timezone
    timestamp = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    
    content = f"""GenericAgent-Admin Portable Bundle
Repository: https://github.com/{repo_owner}/{repo_name}
Reference: {ref}
Source: {source_method}
Fetched: {timestamp}
"""
    
    # Write UTF-8 without BOM
    with open(info_path, "w", encoding="utf-8", newline="\n") as f:
        f.write(content)
    
    say(f"Wrote {info_path}")


def main():
    parser = argparse.ArgumentParser(description="Build portable GenericAgent-Admin bundle")
    parser.add_argument("--platform", required=True, choices=["windows", "linux", "darwin"], help="Target platform")
    parser.add_argument("--arch", required=True, choices=["amd64", "arm64"], help="Target architecture")
    parser.add_argument("--out-dir", type=Path, required=True, help="Output directory for bundle")
    parser.add_argument("--skip-build", action="store_true", help="Skip Go build, use existing dist/ binary")
    parser.add_argument("--ga-ref", default=DEFAULT_GA_REF, help=f"GA repo branch/tag (default: {DEFAULT_GA_REF})")
    parser.add_argument("--python-version", default=PYTHON_VERSION, help=f"Python version (default: {PYTHON_VERSION})")
    
    args = parser.parse_args()
    
    # Determine repo root and version
    repo_root = Path(__file__).resolve().parent.parent.parent
    version = get_version_from_git(repo_root)
    say(f"Version: {version}")
    
    # Prepare stage directory
    bundle_name = f"ga-admin-portable-{args.platform}-{args.arch}-{version}"
    args.out_dir.mkdir(parents=True, exist_ok=True)
    stage = args.out_dir / bundle_name
    
    if stage.exists():
        say(f"Removing existing {stage}")
        shutil.rmtree(stage)
    
    stage.mkdir(parents=True)
    say(f"Stage directory: {stage}")
    
    # 1. Download GA source
    ga_root = stage / "GenericAgent"
    download_ga_source(args.ga_ref, ga_root)
    
    # 2. Install Python runtime
    install_python_runtime(stage, args.python_version)
    
    # 3. Create venv and install deps
    python_exe = stage / "python" / ("python.exe" if args.platform == "windows" else "python")
    create_venv_and_install_deps(ga_root, python_exe)
    
    # 4. Write config
    write_config(stage)
    
    # 5. Copy binary
    dist_dir = repo_root / "dist"
    copy_binary(dist_dir, stage, args.platform)
    
    # 6. Bootstrap check
    run_bootstrap_check(stage, args.platform)
    
    # 7. Write provenance
    write_source_info(stage, GA_REPO_OWNER, GA_REPO_NAME, args.ga_ref, "codeload archive")
    
    say(f"✓ Portable bundle ready at {stage}")
    say(f"  Bundle name: {bundle_name}")
    say(f"  Total size: {sum(f.stat().st_size for f in stage.rglob('*') if f.is_file()) / 1024 / 1024:.1f} MB")


if __name__ == "__main__":
    main()
