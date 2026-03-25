'use strict';
const p = new URLSearchParams(location.search);
const requestId = p.get('request_id') || '';
document.getElementById('site').textContent = '\uD83C\uDF10 ' + (p.get('domain') || '?');
document.getElementById('path').textContent = (p.get('method') || 'GET') + ' ' + (p.get('path') || '/');
function respond(decision) {
  chrome.runtime.sendMessage({ type: 'confirm_result', request_id: requestId, decision });
  window.close();
}
document.getElementById('ba').addEventListener('click', () => respond('allow_domain'));
document.getElementById('bb').addEventListener('click', () => respond('allow_once'));
document.getElementById('bc').addEventListener('click', () => respond('denied'));
