"""Downloads the proxy-server binary for the current platform."""
import hashlib
import os
import platform
import stat
import sys
from pathlib import Path

import httpx

# Update these URLs when publishing a new release.
BASE_URL = "https://github.com/YOUR_ORG/cookie-proxy/releases/download/v1.0.0"

PLATFORM_MAP = {
    ("darwin", "arm64"):  "proxy-server-darwin-arm64",
    ("darwin", "x86_64"): "proxy-server-darwin-amd64",
    ("linux",  "x86_64"): "proxy-server-linux-amd64",
    ("win32",  "amd64"):  "proxy-server-win-amd64.exe",
    ("win32",  "x86_64"): "proxy-server-win-amd64.exe",
}


def _platform_key() -> tuple[str, str]:
    system = sys.platform          # 'darwin' | 'linux' | 'win32'
    machine = platform.machine().lower()  # 'arm64' | 'x86_64' | 'amd64'
    return (system, machine)


def download_binary(dest: Path):
    key = _platform_key()
    filename = PLATFORM_MAP.get(key)
    if not filename:
        raise RuntimeError(
            f"Unsupported platform: {key}. "
            "Please download proxy-server manually from the releases page."
        )

    url = f"{BASE_URL}/{filename}"
    checksum_url = f"{url}.sha256"

    print(f"Downloading proxy-server for {key[0]}-{key[1]}...")

    dest.parent.mkdir(parents=True, exist_ok=True)
    tmp = dest.with_suffix(".tmp")

    with httpx.stream("GET", url, follow_redirects=True, timeout=60) as r:
        r.raise_for_status()
        with open(tmp, "wb") as f:
            for chunk in r.iter_bytes(8192):
                f.write(chunk)

    # Verify checksum if available
    try:
        expected = httpx.get(checksum_url, timeout=10, follow_redirects=True).text.split()[0]
        actual = hashlib.sha256(tmp.read_bytes()).hexdigest()
        if actual != expected:
            tmp.unlink(missing_ok=True)
            raise RuntimeError(f"Checksum mismatch: expected {expected}, got {actual}")
    except httpx.HTTPError:
        pass  # checksum file not available, skip verification

    tmp.rename(dest)
    if sys.platform != "win32":
        dest.chmod(dest.stat().st_mode | stat.S_IXUSR | stat.S_IXGRP | stat.S_IXOTH)

    print(f"proxy-server installed at {dest}")
