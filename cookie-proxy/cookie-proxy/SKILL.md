---
name: cookie-proxy
description: |
  Lets Claude Code access authenticated websites by borrowing the user's existing browser
  login state (cookies). Requires the Cookie Proxy Chrome extension to be installed.

  Use this skill when:
  - The user asks Claude to fetch, read, or scrape a page that requires login
    (e.g. GitHub, Notion, internal tools, dashboards, paywalled content)
  - A direct HTTP request would return a login redirect or 401/403
  - The user says phrases like "use my browser session", "I'm already logged in",
    "fetch with my cookies", or "access this with my account"
  - The user asks Claude to click buttons, fill forms, or submit data on a logged-in page
    (e.g. "submit my timesheet", "click the approve button", "fill in the form and send")

  Do NOT use for public URLs that need no authentication.
---

# Cookie Proxy Skill

## How it works

1. `server_manager.ensure_running()` starts the local `proxy-server` binary (port 7070) if not already running, downloading it first if needed.
2. **For fetch**: Claude sends `POST localhost:7070/fetch`; the extension calls `fetch(url, {credentials:'include'})` and returns the response body.
3. **For interact**: Claude sends `POST localhost:7070/action` with an `actions` list; the extension finds (or opens) the target tab and runs `chrome.scripting.executeScript` to perform DOM interactions.
4. On first access to a domain, the browser shows a permission confirmation window — the user must click **Allow**.
5. Results are returned as a string (fetch) or `{title, url}` dict (interact).

## Usage

### Fetch (read page content)

```python
import sys, pathlib
sys.path.insert(0, str(pathlib.Path(__file__).parent / "scripts"))
from fetch_with_auth import fetch_with_auth

body = fetch_with_auth(
    url="https://example.com/dashboard",
    method="GET",       # optional, default GET
    headers={},         # optional extra headers
    body=None,          # optional request body (dict or str)
)
print(body)
```

### Interact (click / fill / submit)

```python
import sys, pathlib
sys.path.insert(0, str(pathlib.Path(__file__).parent / "scripts"))
from interact_with_auth import interact_with_auth

result = interact_with_auth(
    url="https://example.com/form",
    actions=[
        {"type": "fill",   "selector": "#username", "value": "alice"},
        {"type": "fill",   "selector": "#password", "value": "secret"},
        {"type": "click",  "selector": "button[type=submit]"},
        {"type": "wait",   "ms": 1000},
        {"type": "snapshot"},
    ],
)
print(result)  # {"title": "...", "url": "..."}
```

Supported action types: `click`, `fill`, `submit`, `wait`, `snapshot`.

## Error strings returned (not raised)

| Return value contains | Meaning | Action |
|---|---|---|
| `Cannot connect to Cookie Proxy server` | proxy-server not running / Chrome not open | Ask user to open Chrome with the extension installed |
| `extension is not connected` | Extension installed but not connected | Ask user to open/reload Chrome |
| `Access denied` | User clicked Deny in the confirmation dialog | Inform user; do not retry automatically |
| `Request timed out` | User did not respond to the dialog | Ask user to try again |
| `Blocked: ... private/internal address` | SSRF block on private IPs | Cannot be bypassed by design |

## Files

- `scripts/fetch_with_auth.py` — fetch entry point, call `fetch_with_auth()`
- `scripts/interact_with_auth.py` — interact entry point, call `interact_with_auth()`
- `scripts/server_manager.py` — auto-starts/restarts proxy-server; exposes `ensure_running()` and `PROXY_SERVER_URL`
- `scripts/downloader.py` — downloads the correct platform binary from GitHub Releases on first run

## Prerequisites

- Cookie Proxy Chrome extension installed and Chrome is open
- `httpx` available in the Python environment (`pip install httpx`)
- Internet access on first run (to download the proxy-server binary)
