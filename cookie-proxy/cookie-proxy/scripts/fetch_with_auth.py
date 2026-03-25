"""
fetch_with_auth — Claude Code Skill

Lets Claude Code borrow the user's browser login state to access any website.
Requires the Cookie Proxy Chrome extension to be installed.

Usage:
    result = fetch_with_auth(url, method="GET", headers={}, body=None)
"""
from __future__ import annotations

import httpx

from server_manager import ensure_running, PROXY_SERVER_URL


def fetch_with_auth(
    url: str,
    method: str = "GET",
    headers: dict | None = None,
    body: dict | str | None = None,
) -> str:
    """
    Fetch a URL using the browser's existing login cookies.

    The first time you access a domain, the browser will show a
    permission confirmation dialog. Subsequent calls to the same
    domain go through automatically.

    Returns the response body as a string, or an error message.
    """
    ensure_running()

    try:
        resp = httpx.post(
            f"{PROXY_SERVER_URL}/fetch",
            json={
                "url": url,
                "method": method,
                "headers": headers or {},
                "body": body,
            },
            timeout=40,  # extra time for user to confirm in browser
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
    if error:
        return f"Error ({error}): {result.get('message', '')}"

    return result.get("body", "")


if __name__ == "__main__":
    import sys
    target = sys.argv[1] if len(sys.argv) > 1 else "https://httpbin.org/get"
    print(fetch_with_auth(target))
