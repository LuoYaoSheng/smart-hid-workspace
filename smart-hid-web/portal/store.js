// 极简响应式 state：token / user 持久化到 localStorage，支持订阅。
import { TOKEN_KEY, USER_KEY } from './config.js';

const listeners = new Set();

function load() {
  let user = null;
  try { user = JSON.parse(localStorage.getItem(USER_KEY) || 'null'); } catch { user = null; }
  return {
    token: localStorage.getItem(TOKEN_KEY),
    user,
  };
}

let state = load();

export function getState() { return state; }

export function setState(next) {
  state = { ...state, ...next };
  if (state.token) localStorage.setItem(TOKEN_KEY, state.token);
  else localStorage.removeItem(TOKEN_KEY);
  if (state.user) localStorage.setItem(USER_KEY, JSON.stringify(state.user));
  else localStorage.removeItem(USER_KEY);
  listeners.forEach((fn) => { try { fn(state); } catch {} });
}

export function subscribe(fn) {
  listeners.add(fn);
  return () => listeners.delete(fn);
}

export function isLoggedIn() { return !!state.token; }

export function logout() { setState({ token: null, user: null }); }
