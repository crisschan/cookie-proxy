"""
interact_with_auth — Claude Code Skill

Lets Claude Code perform DOM interactions (click, fill, submit) on pages
where the user is already logged in via the browser.
Requires the Cookie Proxy Chrome extension to be installed.

Usage:
    result = interact_with_auth(url, actions)

    actions = [
        {"type": "fill",   "selector": "#username", "value": "alice"},
        {"type": "fill",   "selector": "#password", "value": "secret"},
        {"type": "click",  "selector": "button[type=submit]"},
        {"type": "wait",   "ms": 1000},
        {"type": "snapshot"},
    ]
"""
from __future__ import annotations

import httpx

from server_manager import ensure_running, PROXY_SERVER_URL


def interact_with_auth(
    url: str,
    actions: list[dict],
) -> dict | str:
    """
    Perform DOM interactions on a page using the browser's existing login state.

    The first time you access a domain, the browser will show a
    permission confirmation dialog. Subsequent calls to the same
    domain go through automatically.

    Supported action types:
      {"type": "click",  "selector": "<css>"}              — click an element
      {"type": "fill",   "selector": "<css>", "value": ""} — fill an input
      {"type": "submit", "selector": "<css>"}              — submit the closest form
      {"type": "wait",   "ms": 500}                        — wait N milliseconds
      {"type": "snapshot"}                                 — return title + current URL

    Returns a dict {"title": ..., "url": ...} on success, or an error string.
    """
    ensure_running()

    try:
        resp = httpx.post(
            f"{PROXY_SERVER_URL}/action",
            json={
                "url": url,
                "actions": actions,
            },
            timeout=65,  # extra time for user confirmation + action execution
        )
        result = resp.json()
    except httpx.ConnectError:
        return (
            "Cannot connect to Cookie Proxy server. "
            "Please ensure the Chrome extension is installed and the browser is open."
        )
    except Exception as e:
        return f"Request error: {e}"

    error = result.get("error")
    if error == "no_extension":
        return (
            "Cookie Proxy Chrome extension is not connected. "
            "Install it from: https://github.com/YOUR_ORG/cookie-proxy"
        )
    if error == "denied":
        return f"Access denied: the user rejected the request to {url}"
    if error == "timeout":
        return "Request timed out — the user may not have responded to the permission dialog."
    if error == "ssrf_blocked":
        return f"Blocked: {url} points to a private/internal address."
    if error == "element_not_found":
        return f"Action failed: {result.get('message', 'element not found')}"
    if error == "action_failed":
        return f"Action failed: {result.get('message', '')}"
    if error:
        return f"Error ({error}): {result.get('message', '')}"

    out = {"title": result.get("title", ""), "url": result.get("url", "")}
    if "result" in result:
        out["result"] = result["result"]
    return out


if __name__ == "__main__":
    import sys
    import json
    target = sys.argv[1] if len(sys.argv) > 1 else "https://httpbin.org/get"
    result = interact_with_auth(target, [{"type": "snapshot"}])
    print(json.dumps(result, ensure_ascii=False, indent=2))
