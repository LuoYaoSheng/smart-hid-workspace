// Portal bootstrap：注册路由、渲染导航、启动路由分发。
import { startRouter, register, navigate } from './router.js';
import { subscribe, getState, logout } from './store.js';
import { api } from './api.js';

import { loginView, registerView } from './views/login.js';
import { dashboardView } from './views/dashboard.js';
import { plansView } from './views/plans.js';
import { devicesView } from './views/devices.js';
import { ordersView } from './views/orders.js';
import { licensesView } from './views/licenses.js';
import { accountView } from './views/account.js';

register('/login', loginView);
register('/register', registerView);
register('/dashboard', dashboardView);
register('/plans', plansView);
register('/devices', devicesView);
register('/orders', ordersView);
register('/licenses', licensesView);
register('/account', accountView);

const NAV_LINKS = [
  { path: '/dashboard', label: '概览' },
  { path: '/plans', label: '套餐' },
  { path: '/devices', label: '设备' },
  { path: '/orders', label: '订单' },
  { path: '/licenses', label: '授权' },
];

function renderNav() {
  const { user } = getState();
  const linksEl = document.getElementById('portal-nav');
  const ctaEl = document.getElementById('portal-nav-cta');
  if (!linksEl || !ctaEl) return;

  if (user) {
    linksEl.innerHTML = NAV_LINKS.map(
      (l) => `<li><a href="#${l.path}">${l.label}</a></li>`
    ).join('');
    ctaEl.innerHTML = `
      <span class="nav-user" title="${user.email}">${user.email}</span>
      <a class="btn btn-ghost btn-sm" href="#/account">账户</a>
      <button class="btn btn-outline btn-sm" id="logout-btn">登出</button>
    `;
    const lb = ctaEl.querySelector('#logout-btn');
    if (lb) lb.onclick = () => { logout(); navigate('/login'); };
  } else {
    linksEl.innerHTML = '';
    ctaEl.innerHTML = `
      <a class="btn btn-ghost btn-sm" href="#/login">登录</a>
      <a class="btn btn-primary btn-sm" href="#/register">注册</a>
    `;
  }
}

// 首次进入若已登录，静默刷新一下 /me 校验 token 仍然有效。
async function probeMe() {
  const { token } = getState();
  if (!token) return;
  try {
    const me = await api.me();
    // 后端如返回更新过的 user 字段可在此合并；当前仅校验存活。
    if (me && me.email) {
      // no-op
    }
  } catch {
    // 401 已在 api.js 处理登出 + 重定向
  }
}

renderNav();
subscribe(renderNav);
startRouter();  // router 内部已把 view() 结果 append 到 #app
probeMe();
