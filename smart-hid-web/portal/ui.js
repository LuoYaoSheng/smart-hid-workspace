// 小工具：Toast 提示 + DOM 便捷构造 + 状态徽章。

let toastTimer;
export function toast(msg, type = 'info') {
  const el = document.getElementById('toast');
  if (!el) return;
  el.textContent = msg;
  el.className = 'toast toast-' + type;
  el.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.hidden = true; }, 3200);
}

export function h(tag, attrs = {}, children = []) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === 'class') el.className = v;
    else if (k === 'html') el.innerHTML = v;
    else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
    else if (v !== null && v !== undefined && v !== false) el.setAttribute(k, v);
  }
  for (const c of [].concat(children)) {
    if (c == null || c === false) continue;
    el.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
  }
  return el;
}

// License / Order 状态 → 徽章 class + 文案
const STATUS_LABEL = {
  UNUSED: '未激活', ACTIVE: '生效中', EXPIRED: '已过期', DISABLED: '已停用', REVOKED: '已吊销',
  PENDING: '待支付', PAID: '已支付',
};

export function statusBadge(status) {
  const cls = status === 'ACTIVE' || status === 'PAID' ? 'badge-ok'
    : status === 'EXPIRED' || status === 'REVOKED' ? 'badge-bad'
    : 'badge-warn';
  const label = STATUS_LABEL[status] || status;
  return `<span class="badge ${cls}">${label}</span>`;
}

export function fmtTime(unix) {
  if (!unix) return '—';
  const d = new Date(unix * 1000);
  return d.toLocaleString('zh-CN', { hour12: false });
}

export function fmtMoney(cents, currency = 'CNY') {
  if (cents == null) return '—';
  const sym = currency === 'CNY' ? '¥' : (currency === 'USD' ? '$' : '');
  return sym + (cents / 100).toFixed(2);
}
