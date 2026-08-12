// admin fetch 封装。path 传完整 /api/v1 相对路径（login 走 /auth/login，其余 /admin/*）。
// 401 → 登出；403 → 登出并提示非管理员（admin middleware 拒绝）。
import { API_BASE } from './config.js';
import { getState, logout } from './store.js';
import { toast } from '../portal/ui.js';

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message || code || `HTTP ${status}`);
    this.status = status; this.code = code;
  }
}

async function request(path, { method = 'GET', body, raw = false } = {}) {
  const { token } = getState();
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  let res;
  try {
    res = await fetch(API_BASE + path, {
      method, headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch {
    throw new ApiError(0, 'network', '网络错误：无法连接到服务端');
  }
  if (res.status === 401) { logout(); }
  if (res.status === 403) {
    toast('该账户不是管理员', 'error');
    logout();
  }
  if (!res.ok) {
    let code = 'error', message = '';
    try { const e = await res.json(); code = e.code || code; message = e.message || ''; } catch {}
    throw new ApiError(res.status, code, message);
  }
  if (raw) return res;
  const ct = res.headers.get('content-type') || '';
  if (ct.includes('application/json')) return res.json();
  return res.text();
}

export const api = {
  login: (email, password) => request('/auth/login', { method: 'POST', body: { email, password } }),

  stats: () => request('/admin/stats'),
  users: () => request('/admin/users'),
  orders: () => request('/admin/orders'),
  refundOrder: (id) => request('/admin/orders/' + encodeURIComponent(id) + '/refund', { method: 'POST' }),
  licenses: () => request('/admin/licenses'),
  setLicenseStatus: (id, action) =>
    request('/admin/licenses/' + encodeURIComponent(id) + '/' + action, { method: 'POST' }),
  plans: () => request('/admin/plans'),
  upsertPlan: (p) => request('/admin/plans', { method: 'POST', body: p }),
  setPlanActive: (id, active) =>
    request('/admin/plans/' + encodeURIComponent(id) + '/' + (active ? 'activate' : 'deactivate'), { method: 'POST' }),
  activationCodes: () => request('/admin/activation-codes'),
  createActivationCode: (user_id, device_id, plan_id) =>
    request('/admin/activation-codes', { method: 'POST', body: { user_id, device_id, plan_id } }),
  revokeActivationCode: (code) =>
    request('/admin/activation-codes/' + encodeURIComponent(code) + '/revoke', { method: 'POST' }),
};
