'use strict';
async function render() {
  const dot = document.getElementById('dot');
  const st  = document.getElementById('st');
  try {
    const res  = await fetch('http://localhost:7070/ping', { signal: AbortSignal.timeout(1500) });
    const data = await res.json();
    dot.className = 'dot ' + (data.extension_connected ? 'ok' : 'err');
    st.textContent = data.extension_connected ? '已连接' : '扩展未就绪';
  } catch {
    dot.className = 'dot err';
    st.textContent = '代理未运行';
  }
  const { whitelist = [] } = await chrome.storage.local.get('whitelist');
  const c = document.getElementById('wl');
  if (!whitelist.length) { c.innerHTML = '<div class="empty">暂无信任域名</div>'; return; }
  c.innerHTML = '';
  for (const d of whitelist) {
    const el = document.createElement('div');
    el.className = 'item';
    el.innerHTML = '<span>&#10003; '+d+'</span><button class="del" data-d="'+d+'">删除</button>';
    c.appendChild(el);
  }
  c.querySelectorAll('.del').forEach(b => b.addEventListener('click', async () => {
    const { whitelist: wl = [] } = await chrome.storage.local.get('whitelist');
    await chrome.storage.local.set({ whitelist: wl.filter(x => x !== b.dataset.d) });
    render();
  }));
}
render();
