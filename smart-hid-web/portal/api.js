// fetch 封装：token 注入、JSON 解析、错误归一化、401 触发登出。
import { API_BASE } from './config.js';
import { getState, logout } from './store.js';

export class ApiError extends Error {
  constructor(status, code, message) {
    super(message || code || `HTTP ${status}`);
    this.status = status;
    this.code = code;
  }
}

async function request(path, { method = 'GET', body, raw = false } = {}) {
  const { token } = getState();
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = 'Bearer ' + token;

  let res;
  try {
    res = await fetch(API_BASE + path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (e) {
    throw new ApiError(0, 'network', '网络错误：无法连接到服务端');
  }

  if (res.status === 401) {
    logout();
    if (location.hash !== '#/login' && location.hash !== '#/register') {
      location.hash = '#/login';
    }
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
  register: (email, password) => request('/auth/register', { method: 'POST', body: { email, password } }),
  login: (email, password) => request('/auth/login', { method: 'POST', body: { email, password } }),
  me: () => request('/users/me'),

  listPlans: () => request('/plans'),

  listDevices: () => request('/devices'),
  registerDevice: (device_id, name) => request('/devices', { method: 'POST', body: { device_id, name } }),

  listOrders: () => request('/orders'),
  createOrder: (plan_id) => request('/orders', { method: 'POST', body: { plan_id } }),
  payCallback: (orderID) => request('/orders/' + encodeURIComponent(orderID) + '/pay-callback', { method: 'POST' }),

  listLicenses: () => request('/licenses'),
  getLicense: (id) => request('/licenses/' + encodeURIComponent(id)),
  activateLicense: (id, device_id) =>
    request('/licenses/' + encodeURIComponent(id) + '/activate', { method: 'POST', body: { device_id } }),

  // 下载 .license —— 走 fetch（带 token）拿 blob，前端触发保存。
  async downloadLicense(id) {
    const res = await request('/licenses/' + encodeURIComponent(id) + '/download', { raw: true });
    const blob = await res.blob();
    const cd = res.headers.get('Content-Disposition') || '';
    const m = cd.match(/filename="([^"]+)"/);
    return { blob, filename: (m && m[1]) || id + '.license' };
  },
};
