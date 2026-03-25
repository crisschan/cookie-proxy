"""Manages the proxy-server process lifecycle."""
import os
import signal
import subprocess
import sys
import time
from pathlib import Path

import httpx

from downloader import download_binary

PROXY_SERVER_VERSION = "1.0.0"
PROXY_SERVER_DIR = Path.home() / ".cookie-proxy"
PROXY_SERVER_URL = "http://127.0.0.1:7070"
PIDFILE = PROXY_SERVER_DIR / "proxy-server.pid"


def _bin_path() -> Path:
    suffix = ".exe" if sys.platform == "win32" else ""
    return PROXY_SERVER_DIR / f"proxy-server{suffix}"


def _ping() -> dict | None:
    try:
        resp = httpx.get(f"{PROXY_SERVER_URL}/ping", timeout=1.5)
        return resp.json()
    except Exception:
        return None


def _kill_old_server():
    if not PIDFILE.exists():
        return
    try:
        pid = int(PIDFILE.read_text().strip())
        os.kill(pid, signal.SIGTERM)
        time.sleep(0.5)
    except Exception:
        pass
    finally:
        PIDFILE.unlink(missing_ok=True)


def _start_server():
    PROXY_SERVER_DIR.mkdir(parents=True, exist_ok=True)
    bin_path = _bin_path()
    if not bin_path.exists():
        download_binary(bin_path)

    proc = subprocess.Popen(
        [str(bin_path)],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    PIDFILE.write_text(str(proc.pid))

    # Wait up to 4s for server to be ready
    for _ in range(8):
        time.sleep(0.5)
        data = _ping()
        if data and data.get("status") == "ok":
            return
    raise RuntimeError("proxy-server failed to start within 4 seconds")


def ensure_running():
    """Ensure proxy-server is running with the correct version."""
    data = _ping()
    if data:
        if data.get("version") == PROXY_SERVER_VERSION:
            return  # already running, correct version
        # Version mismatch: kill old, re-download, restart
        _kill_old_server()
        bin_path = _bin_path()
        if bin_path.exists():
            bin_path.unlink()  # force re-download
    _start_server()
