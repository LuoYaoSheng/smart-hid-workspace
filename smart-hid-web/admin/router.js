// admin hash 路由 + 守卫。
import { isLoggedIn } from './store.js';

const routes = new Map();
const PROTECTED = ['/stats', '/users', '/orders', '/licenses', '/plans', '/activation-codes'];

export function register(path, view) { routes.set(path, view); }
export function currentPath() {
  const h = location.hash.replace(/^#/, '');
  return h || '/login';
}
export function navigate(path) { location.hash = '#' + path; }

function guard(path) {
  if (PROTECTED.includes(path) && !isLoggedIn()) return '/login';
  if (path === '/login' && isLoggedIn()) return '/stats';
  return path;
}

export function startRouter() {
  window.addEventListener('hashchange', dispatch);
  dispatch();
}

async function dispatch() {
  const raw = currentPath();
  const path = guard(raw);
  if (path !== raw) { location.hash = '#' + path; return; }
  const view = routes.get(path);
  if (!view) { location.hash = '#/login'; return; }
  const main = document.getElementById('app');
  main.innerHTML = '';
  document.querySelectorAll('.admin-nav a').forEach((a) => {
    a.classList.toggle('active', a.getAttribute('href') === '#' + path);
  });
  try {
    const el = await view(path);
    if (el) main.appendChild(el);
  } catch (e) {
    main.innerHTML = `<div class="empty-state"><h3>出错了</h3><p class="muted">${(e && e.message) || e}</p></div>`;
  }
  window.scrollTo(0, 0);
}
