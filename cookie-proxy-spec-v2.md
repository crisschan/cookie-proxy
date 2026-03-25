# Cookie Proxy — Spec v2

## 一句话描述

用户只需安装一个 Chrome 扩展，Claude Code 的 Skill 在首次运行时自动完成剩余所有初始化，之后 Agent 即可借用浏览器已有的登录态访问任意网站，或在已登录的页面上执行点击、填写表单、提交等交互操作。

---

## 用户体验目标

```
第一次使用：
  1. 安装 Chrome 扩展               ← 用户唯一需要做的事
  2. 在 Claude Code 里提问           ← 正常使用
  3. Skill 自动完成初始化            ← 用户无感知
  4. 浏览器弹出权限确认              ← 用户点允许
  5. 得到结果                        ← 完成

之后每次使用：
  1. 在 Claude Code 里提问
  2. 得到结果
```

---

## 整体架构

```
┌─────────────────────────────────────────────────────┐
│                   用户的电脑                         │
│                                                     │
│  Claude Code                                        │
│  └── Skill (Python)                                 │
│       ├── 1. 确保 proxy-server 在运行               │
│       ├── 2a. fetch_with_auth() → POST /fetch       │
│       └── 2b. interact_with_auth() → POST /action   │
│                         │                           │
│  proxy-server (Go 二进制)│                           │
│  └── 监听 localhost:7070 │                           │
│       ├── 收到请求        │                           │
│       └── WebSocket ─────┤                           │
│                         │                           │
│  Chrome Extension        │                           │
│  └── background SW       │                           │
│       ├── WebSocket ◄────┘                           │
│       ├── 弹出权限确认                               │
│       ├── fetch(url, {credentials:'include'})  ← /fetch 路径  │
│       └── scripting.executeScript(actions)    ← /action 路径 │
│                │                                    │
└────────────────┼────────────────────────────────────┘
                 │
         目标网站（带 Cookie / 已登录 Tab）
```

---

## 三个组件

### 组件一：Chrome Extension

**职责：** 唯一能接触 Cookie 的组件，负责权限确认、实际发请求，以及在已登录页面上执行 DOM 交互操作。

**工作流程（fetch 请求）：**
1. 启动时主动连接 `ws://localhost:7070/ws`
2. 收到 `type: fetch` 消息
3. 检查域名白名单
4. 不在白名单 → 用 `chrome.windows.create` 打开 `confirm.html` 确认窗口（注：MV3 Service Worker 无法用 `chrome.action.openPopup()` 弹出 Popup，必须创建独立窗口）
5. 用户允许 → 用 `fetch(url, {credentials: 'include'})` 发请求
6. 把响应通过 WebSocket 返回

**工作流程（interact 交互操作）：**
1. 收到 `type: interact` 消息，消息包含目标 `url` 和 `actions` 列表
2. 检查域名白名单（与 fetch 流程相同）
3. 用 `chrome.tabs.query` 找到已打开该域名的 tab；若不存在则用 `chrome.tabs.create` 打开目标 URL 并等待加载完成
4. 用 `chrome.scripting.executeScript` 向 tab 注入操作脚本，依次执行 actions
5. 执行完成后把结果（截图或最终页面 URL）通过 WebSocket 返回

**支持的 action 类型：**

| action type | 参数 | 说明 |
|---|---|---|
| `click` | `selector: string` | 点击匹配的元素 |
| `fill` | `selector: string, value: string` | 填写输入框 |
| `submit` | `selector: string` | 提交表单 |
| `wait` | `ms: number` | 等待指定毫秒 |
| `snapshot` | — | 返回当前页面 `document.title` 和 `location.href` |

**关键特性：**
- Cookie 永远不离开扩展，外部只能看到响应结果
- WebSocket 断开后自动重连（指数退避，最长 30 秒）
- proxy-server 未启动时，扩展图标变灰，静默等待
- **MV3 Service Worker 生命周期**：SW 在 ~30 秒无活动后会被 Chrome 终止，导致 WebSocket 断连。使用 `chrome.alarms`（每 25 秒触发一次）保持 SW 活跃并在断连时自动重连。

**manifest.json 权限：**
```json
{
  "manifest_version": 3,
  "permissions": ["storage", "alarms", "tabs", "scripting"],
  "host_permissions": [
    "ws://localhost:7070/*",
    "<all_urls>"
  ]
}
```

> `tabs` 用于查找已打开的目标 tab；`scripting` 用于向 tab 注入执行脚本。

---

### 组件二：proxy-server（Go 二进制）

**职责：** 作为 Claude Code 和扩展之间的桥梁，HTTP Server + WebSocket Server。

**对 Claude Code 侧暴露 HTTP 接口：**

```
POST localhost:7070/fetch     接收 fetch 请求，转发给扩展
POST localhost:7070/action    接收交互操作请求，转发给扩展
GET  localhost:7070/ping      健康检查
GET  localhost:7070/status    查看连接状态
```

**`/action` 接口 Request Body：**
```json
{
  "url": "https://example.com/page",
  "actions": [
    { "type": "fill",   "selector": "#username", "value": "alice" },
    { "type": "fill",   "selector": "#password", "value": "secret" },
    { "type": "click",  "selector": "button[type=submit]" },
    { "type": "wait",   "ms": 1000 },
    { "type": "snapshot" }
  ]
}
```

**`/action` 接口 Response Body：**
```json
{
  "title": "Dashboard — Example",
  "url": "https://example.com/dashboard",
  "error": ""
}
```

**对扩展侧暴露 WebSocket：**

```
ws://localhost:7070/ws       扩展连接的 WebSocket 端点
```

**请求流转（/fetch）：**
```
HTTP POST /fetch
    ↓
生成 request_id，封装 type:fetch 消息，放入等待队列
    ↓
通过 WebSocket 推送给扩展
    ↓
等待扩展返回（timeout: 35s）
    ↓
HTTP 响应返回给 Skill
```

**请求流转（/action）：**
```
HTTP POST /action
    ↓
SSRF 校验目标 URL
    ↓
生成 request_id，封装 type:interact 消息（含 actions 列表），放入等待队列
    ↓
通过 WebSocket 推送给扩展
    ↓
等待扩展返回（timeout: 60s，给用户操作留足时间）
    ↓
HTTP 响应返回给 Skill
```

**进程管理：**
- 绑定只在 `127.0.0.1`，不对外暴露
- 空闲 30 分钟无请求自动退出
- 单实例锁（pidfile），防止重复启动
- 启动耗时 < 100ms（Go 二进制冷启动快）

**二进制分发：**
- 编译为单文件，无任何运行时依赖
- 提供四个平台版本：`darwin-arm64` / `darwin-amd64` / `linux-amd64` / `win-amd64`
- 文件大小目标：< 5MB

---

### 组件三：Claude Code Skill（Python）

**职责：** 被 Claude Code 调用，管理 proxy-server 生命周期，向 proxy-server 发请求。

**核心逻辑：**

```python
PROXY_SERVER_VERSION = "1.0.0"
PROXY_SERVER_DIR = Path.home() / ".cookie-proxy"
PROXY_SERVER_BIN = PROXY_SERVER_DIR / "proxy-server"
PROXY_SERVER_URL = "http://127.0.0.1:7070"

def ensure_server_running():
    """确保 proxy-server 在运行，不在则启动"""

    # 1. 检查二进制是否存在
    if not PROXY_SERVER_BIN.exists():
        download_binary()  # 从 GitHub Release 下载对应平台版本

    # 2. 检查是否已在运行
    try:
        resp = httpx.get(f"{PROXY_SERVER_URL}/ping", timeout=1)
        data = resp.json()
        if data["version"] == PROXY_SERVER_VERSION:
            return  # 版本匹配，直接返回
        else:
            # 版本不匹配：终止旧进程，重新下载并启动新版本
            kill_old_server()   # 读取 pidfile 并 kill
            download_binary()   # 覆盖下载新版本二进制
    except:
        pass

    # 3. 启动进程
    subprocess.Popen(
        [PROXY_SERVER_BIN],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        start_new_session=True  # 脱离 Claude Code 进程树
    )

    # 4. 等待就绪（最多 3 秒）
    wait_for_ready(timeout=3)


def fetch_with_auth(url: str, method: str = "GET", body: dict = None) -> str:
    """
    借用浏览器登录态请求目标 URL。
    首次使用某个域名时，浏览器会弹出权限确认。
    """
    ensure_server_running()

    resp = httpx.post(
        f"{PROXY_SERVER_URL}/fetch",
        json={"url": url, "method": method, "body": body},
        timeout=35  # 留出扩展等待用户确认的时间
    )

    result = resp.json()

    if result.get("error") == "no_extension":
        return "请先安装 Cookie Proxy 扩展：https://xxx"
    if result.get("error") == "denied":
        return f"用户拒绝了对 {url} 的访问"
    if result.get("error") == "timeout":
        return "请求超时，请检查网络连接"

    return result["body"]


def interact_with_auth(url: str, actions: list) -> dict:
    """
    在浏览器已登录的页面上执行交互操作（点击、填写、提交等）。
    首次使用某个域名时，浏览器会弹出权限确认。

    actions 示例：
      [{"type": "fill", "selector": "#q", "value": "hello"},
       {"type": "click", "selector": "button[type=submit]"},
       {"type": "wait", "ms": 1000},
       {"type": "snapshot"}]

    返回：{"title": ..., "url": ...} 或错误描述字符串。
    """
    ensure_server_running()

    resp = httpx.post(
        f"{PROXY_SERVER_URL}/action",
        json={"url": url, "actions": actions},
        timeout=65  # 留出用户确认 + 操作执行的时间
    )

    result = resp.json()

    if result.get("error") == "no_extension":
        return "请先安装 Cookie Proxy 扩展"
    if result.get("error") == "denied":
        return f"用户拒绝了对 {url} 的访问"
    if result.get("error") == "timeout":
        return "操作超时"
    if result.get("error"):
        return f"操作失败：{result.get('error')}"

    return result
```

---

## 接口协议

### HTTP 接口（Skill → proxy-server）

**POST /fetch**

Request:
```json
{
  "url": "https://api.github.com/notifications",
  "method": "GET",
  "headers": {"Accept": "application/json"},
  "body": null
}
```

Response（成功）:
```json
{
  "status": 200,
  "headers": {"content-type": "application/json"},
  "body": "{...}"
}
```

Response（失败）:
```json
{
  "error": "denied | timeout | no_extension | ssrf_blocked",
  "message": "human readable description"
}
```

**POST /action**

Request:
```json
{
  "url": "https://example.com/login",
  "actions": [
    { "type": "fill",   "selector": "#username", "value": "alice" },
    { "type": "fill",   "selector": "#password", "value": "secret" },
    { "type": "click",  "selector": "button[type=submit]" },
    { "type": "wait",   "ms": 1000 },
    { "type": "snapshot" }
  ]
}
```

Response（成功）:
```json
{
  "title": "Dashboard — Example",
  "url": "https://example.com/dashboard"
}
```

Response（失败）:
```json
{
  "error": "denied | timeout | no_extension | ssrf_blocked | action_failed",
  "message": "human readable description"
}
```

**GET /ping**
```json
{
  "status": "ok",
  "version": "1.0.0",
  "extension_connected": true
}
```

---

### WebSocket 协议（proxy-server ↔ Extension）

**proxy-server → Extension（fetch 请求）:**
```json
{
  "request_id": "uuid-v4",
  "type": "fetch",
  "url": "https://api.github.com/notifications",
  "method": "GET",
  "headers": {"Accept": "application/json"},
  "body": null
}
```

**proxy-server → Extension（interact 请求）:**
```json
{
  "request_id": "uuid-v4",
  "type": "interact",
  "url": "https://example.com/login",
  "actions": [
    { "type": "fill", "selector": "#username", "value": "alice" },
    { "type": "click", "selector": "button[type=submit]" },
    { "type": "snapshot" }
  ]
}
```

**Extension → proxy-server（fetch 返回结果）:**
```json
{
  "request_id": "uuid-v4",
  "status": 200,
  "headers": {"content-type": "application/json"},
  "body": "{...}"
}
```

**Extension → proxy-server（interact 返回结果）:**
```json
{
  "request_id": "uuid-v4",
  "title": "Dashboard — Example",
  "url": "https://example.com/dashboard"
}
```

**Extension → proxy-server（返回错误）:**
```json
{
  "request_id": "uuid-v4",
  "error": "denied"
}
```

---

## 权限确认机制

### 首次访问某个域名时

扩展弹出 Popup：

```
┌─────────────────────────────────────────┐
│  🔐 Cookie Proxy                        │
│                                         │
│  Claude Code 想借用你在以下网站         │
│  的登录状态发起请求：                   │
│                                         │
│  🌐 github.com                          │
│     GET /notifications                  │
│                                         │
│  [允许此域名] [仅允许一次] [拒绝]       │
└─────────────────────────────────────────┘
```

### 白名单行为

| 选择 | 行为 |
|---|---|
| 允许此域名 | 存入 `chrome.storage.local`，后续不再询问 |
| 仅允许一次 | 本次通过，下次再问 |
| 拒绝 | 返回 `error: denied` |

---

## 安全设计

### Cookie 不离开扩展
扩展用 `fetch(url, {credentials: 'include'})` 发请求，Cookie 由浏览器自动附加，proxy-server 和 Skill 只能看到响应体，永远看不到 Cookie 原始值。

### 防 SSRF（由 proxy-server 负责）
proxy-server 在转发给扩展前校验目标 URL，拒绝内网地址：

```
拒绝：127.x.x.x / 192.168.x.x / 10.x.x.x / 169.254.x.x / localhost / *.local
```

### 同域限制（由扩展负责）
扩展收到请求后，提取目标 URL 的 eTLD+1，与白名单比对，防止子域名绕过：

```
白名单：github.com
允许：api.github.com ✅  gist.github.com ✅
拒绝：evil.com ❌
```

> 职责分工：proxy-server 只做 SSRF 防护（防止访问内网）；白名单和域名匹配由扩展内部执行（因为白名单存储在 `chrome.storage.local`，proxy-server 无法访问）。

### 仅本地通信
proxy-server 只绑定 `127.0.0.1`，不绑定 `0.0.0.0`。

### 响应大小限制
单次响应最大 10MB，超出截断并提示。

---

## 扩展 Popup UI

```
┌─────────────────────────────────┐
│  🔐 Cookie Proxy          ● 已连接 │
│                                 │
│  已信任的域名：                  │
│  ✓ github.com            [删除] │
│  ✓ notion.so             [删除] │
│                                 │
│  今日请求：12 次                 │
│  [清空白名单]                   │
└─────────────────────────────────┘
```

未连接状态（proxy-server 未运行）：

```
┌─────────────────────────────────┐
│  🔐 Cookie Proxy          ○ 未连接 │
│                                 │
│  等待 Claude Code 发起请求...   │
│  proxy-server 将自动启动         │
└─────────────────────────────────┘
```

---

## 文件结构

```
cookie-proxy/
├── extension/                   # Chrome 扩展
│   ├── manifest.json
│   ├── background.js            # Service Worker：WebSocket + alarms keepalive + 权限确认 + fetch
│   ├── popup.html               # 扩展图标点击后的 Popup：连接状态 + 白名单管理
│   ├── popup.js
│   ├── confirm.html             # 权限确认窗口（由 chrome.windows.create 打开）
│   ├── confirm.js
│   └── icons/
│
├── proxy-server/                # Go 二进制
│   ├── main.go
│   ├── server.go                # HTTP Server
│   ├── ws.go                    # WebSocket 管理
│   ├── security.go              # SSRF 防护、同域校验
│   └── Makefile                 # 交叉编译脚本
│
└── skill/                       # Claude Code Skill
    ├── fetch_with_auth.py       # fetch 主入口
    ├── interact_with_auth.py    # interact 主入口（新增）
    ├── server_manager.py        # proxy-server 生命周期管理
    └── downloader.py            # 二进制下载逻辑
```

---

## 不做的事

| 限制 | 原因 |
|---|---|
| 不暴露 Cookie 原始值 | Cookie 是最敏感的凭证，泄露等于账号失控 |
| 不支持 WebSocket 代理 | service worker 生命周期不可靠，范围外的复杂度 |
| 不记录请求内容日志 | 请求内容属于用户隐私 |
| 不支持文件上传下载 | 二进制流处理复杂，10MB 限制已覆盖常见场景 |
| interact 不返回截图 | 截图涉及二进制传输，复杂度高；用 snapshot 返回 title+url 已满足大多数验证需求 |
| interact 不支持跨 tab 跳转监听 | 新窗口/tab 由 scripting 注入代码无法感知，范围外 |
| 不提供云端同步 | 白名单是本地数据，上云引入不必要风险 |
| 不支持请求内网地址 | 防止 SSRF 攻击 |
