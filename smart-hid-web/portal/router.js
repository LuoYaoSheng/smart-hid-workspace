// hash 路由 + 守卫。路径形如 #/dashboard。
import { isLoggedIn } from './store.js';

const routes = new Map();      // path -> view factory
const PROTECTED = ['/dashboard', '/plans', '/devices', '/orders', '/licenses', '/account'];

export function register(path, view) { routes.set(path, view); }

export function currentPath() {
  const h = location.hash.replace(/^#/, '');
  if (!h) return '/login';
  return h;
}

export function navigate(path) { location.hash = '#' + path; }

function guard(path) {
  if (PROTECTED.includes(path) && !isLoggedIn()) return '/login';
  if ((path === '/login' || path === '/register') && isLoggedIn()) return '/dashboard';
  return path;
}

export function startRouter() {
  window.addEventListener('hashchange', dispatch);
  dispatch();
}

async function dispatch() {
  const raw = currentPath();
  const path = guard(raw);
  if (path !== raw) {
    // 守卫改了路径 —— 改 hash 会再次触发 dispatch，本次直接返回。
    location.hash = '#' + path;
    return;
  }
  const view = routes.get(path);
  if (!view) {
    location.hash = '#/login';
    return;
  }
  const main = document.getElementById('app');
  main.innerHTML = '';
  // 高亮当前导航
  document.querySelectorAll('#portal-nav a').forEach((a) => {
    a.classList.toggle('active', a.getAttribute('href') === '#' + path);
  });
  try {
    const el = await view(path);
    if (el) main.appendChild(el);
  } catch (e) {
    main.appendChild(renderErr(e));
  }
  window.scrollTo(0, 0);
}

function renderErr(e) {
  const div = document.createElement('div');
  div.className = 'empty-state';
  div.innerHTML = `<h3>出错了</h3><p class="muted">${(e && e.message) || e}</p>`;
  return div;
}
