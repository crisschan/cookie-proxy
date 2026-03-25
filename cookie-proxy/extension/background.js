'use strict';
const WS_URL = 'ws://localhost:7070/ws';
const RECONNECT_MAX = 30000;
let ws = null;
let reconnectDelay = 1000;
const pendingConfirms = new Map();

function connectWebSocket() {
  if (ws && (ws.readyState === WebSocket.CONNECTING || ws.readyState === WebSocket.OPEN)) return;
  ws = new WebSocket(WS_URL);
  ws.addEventListener('open', () => { reconnectDelay = 1000; setIcon(true); });
  ws.addEventListener('message', (event) => {
    try { const msg = JSON.parse(event.data); handleRequest(msg).catch(err => sendWS({request_id: msg.request_id, error: 'internal_error'})); }
    catch (e) { console.error('[CookieProxy] parse error', e); }
  });
  ws.addEventListener('close', () => {
    setIcon(false);
    setTimeout(() => { reconnectDelay = Math.min(reconnectDelay * 2, RECONNECT_MAX); connectWebSocket(); }, reconnectDelay);
  });
  ws.addEventListener('error', () => {});
}

async function handleRequest(msg) {
  if (msg.type === 'interact') { await handleInteract(msg); return; }
  await handleFetch(msg);
}

async function handleFetch(msg) {
  const { request_id, url, method = 'GET', headers = {}, body = null } = msg;
  if (!url || !request_id) return;
  let domain;
  try {
    const parts = new URL(url).hostname.split('.');
    domain = parts.length >= 2 ? parts.slice(-2).join('.') : parts[0];
  } catch { sendWS({request_id, error: 'invalid_url'}); return; }

  const { whitelist = [] } = await chrome.storage.local.get('whitelist');
  let decision;
  if (whitelist.includes(domain)) {
    decision = 'allow_once';
  } else {
    decision = await askUser(request_id, domain, method, url);
  }
  if (decision === 'allow_domain') {
    await chrome.storage.local.set({ whitelist: [...whitelist.filter(d => d !== domain), domain] });
  } else if (decision === 'denied') {
    sendWS({request_id, error: 'denied'}); return;
  }

  try {
    const init = { method, credentials: 'include', headers };
    if (body !== null && method !== 'GET' && method !== 'HEAD')
      init.body = typeof body === 'string' ? body : JSON.stringify(body);
    const res = await fetch(url, init);
    const rh = {}; res.headers.forEach((v,k) => { rh[k]=v; });
    const rb = await res.text();
    if (rb.length > 10*1024*1024) { sendWS({request_id, error: 'response_too_large'}); return; }
    sendWS({request_id, status: res.status, headers: rh, body: rb});
  } catch (e) { sendWS({request_id, error: 'fetch_failed', message: e.message}); }
}

async function handleInteract(msg) {
  const { request_id, url, actions = [] } = msg;
  if (!url || !request_id) return;
  let domain;
  try {
    const parts = new URL(url).hostname.split('.');
    domain = parts.length >= 2 ? parts.slice(-2).join('.') : parts[0];
  } catch { sendWS({request_id, error: 'invalid_url'}); return; }

  const { whitelist = [] } = await chrome.storage.local.get('whitelist');
  let decision;
  if (whitelist.includes(domain)) {
    decision = 'allow_once';
  } else {
    decision = await askUser(request_id, domain, 'INTERACT', url);
  }
  if (decision === 'allow_domain') {
    await chrome.storage.local.set({ whitelist: [...whitelist.filter(d => d !== domain), domain] });
  } else if (decision === 'denied') {
    sendWS({request_id, error: 'denied'}); return;
  }

  try {
    // Find existing tab for this domain or open a new one, then navigate to target URL
    const tabs = await chrome.tabs.query({ url: `*://*.${domain}/*` });
    let tab;
    const waitForLoad = (tabId) => new Promise((resolve, reject) => {
      const timeout = setTimeout(() => reject(new Error('tab load timeout')), 15000);
      chrome.tabs.onUpdated.addListener(function listener(id, info) {
        if (id === tabId && info.status === 'complete') {
          clearTimeout(timeout);
          chrome.tabs.onUpdated.removeListener(listener);
          resolve();
        }
      });
    });
    if (tabs.length > 0) {
      tab = tabs[0];
      // Navigate to the correct URL if not already there
      if (tab.url !== url) {
        await chrome.tabs.update(tab.id, { url });
        await waitForLoad(tab.id);
      }
    } else {
      tab = await chrome.tabs.create({ url, active: false });
      await waitForLoad(tab.id);
    }

    // Inject and execute actions in the tab
    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: async (actions) => {
        for (const action of actions) {
          if (action.type === 'click') {
            const el = document.querySelector(action.selector);
            if (!el) return { error: 'element_not_found', message: `No element: ${action.selector}` };
            el.click();
          } else if (action.type === 'fill') {
            const el = document.querySelector(action.selector);
            if (!el) return { error: 'element_not_found', message: `No element: ${action.selector}` };
            el.focus();
            el.value = action.value;
            el.dispatchEvent(new Event('input', { bubbles: true }));
            el.dispatchEvent(new Event('change', { bubbles: true }));
          } else if (action.type === 'submit') {
            const el = document.querySelector(action.selector);
            if (!el) return { error: 'element_not_found', message: `No element: ${action.selector}` };
            el.closest('form') ? el.closest('form').submit() : el.click();
          } else if (action.type === 'wait') {
            await new Promise(r => setTimeout(r, action.ms || 500));
          } else if (action.type === 'get_inputs') {
            const els = Array.from(document.querySelectorAll('input, textarea, select, button[type=submit]'));
            const info = els.map(el => ({ tag: el.tagName, id: el.id, name: el.name, type: el.type, placeholder: el.placeholder, label: el.getAttribute('aria-label') || '', cls: el.className.substring(0, 80) }));
            return { title: document.title, page_url: location.href, result: info };
          } else if (action.type === 'query') {
            const els = Array.from(document.querySelectorAll(action.selector || '*'));
            const info = els.slice(0, 50).map(el => ({ tag: el.tagName, id: el.id, cls: el.className.substring(0, 80), text: el.textContent.trim().substring(0, 100), role: el.getAttribute('role') || '', label: el.getAttribute('aria-label') || '', checked: el.checked, value: el.value || '' }));
            return { title: document.title, page_url: location.href, result: info };
          } else if (action.type === 'snapshot') {
            return { title: document.title, page_url: location.href };
          }
        }
        return { title: document.title, page_url: location.href };
      },
      args: [actions],
    });

    const result = results[0]?.result || {};
    if (result.error) {
      sendWS({request_id, error: result.error, message: result.message});
    } else {
      const wsMsg = {request_id, title: result.title, page_url: result.page_url};
      if (result.result !== undefined) wsMsg.result = result.result;
      sendWS(wsMsg);
    }
  } catch (e) { sendWS({request_id, error: 'action_failed', message: e.message}); }
}

function askUser(request_id, domain, method, url) {
  return new Promise((resolve) => {
    pendingConfirms.set(request_id, resolve);
    let path = '/'; try { path = new URL(url).pathname; } catch {}
    const p = new URLSearchParams({request_id, domain, method, path});
    chrome.windows.create({ url: chrome.runtime.getURL('confirm.html?'+p), type:'popup', width:440, height:260, focused:true });
    setTimeout(() => { if (pendingConfirms.has(request_id)) { pendingConfirms.delete(request_id); resolve('denied'); } }, 60000);
  });
}

chrome.runtime.onMessage.addListener((message, sender) => {
  if (message.type === 'confirm_result') {
    const resolve = pendingConfirms.get(message.request_id);
    if (resolve) { pendingConfirms.delete(message.request_id); resolve(message.decision); }
    if (sender.tab && sender.tab.windowId) chrome.windows.remove(sender.tab.windowId);
  }
});

function sendWS(obj) { if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj)); }
function setIcon(ok) {
  chrome.action.setIcon({ path: {
    16:  ok ? 'icons/16.png'  : 'icons/16-gray.png',
    48:  ok ? 'icons/48.png'  : 'icons/48-gray.png',
    128: ok ? 'icons/128.png' : 'icons/128-gray.png'
  }}).catch(()=>{});
}

chrome.alarms.create('keepalive', { periodInMinutes: 0.4 });
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === 'keepalive' && (!ws || ws.readyState >= 2)) connectWebSocket();
});

connectWebSocket();
