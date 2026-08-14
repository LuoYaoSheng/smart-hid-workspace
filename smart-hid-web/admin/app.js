// admin bootstrap：注册路由、渲染侧边栏、启动分发。
import { startRouter, register, navigate } from './router.js';
import { subscribe, getState, logout } from './store.js';

import { loginView } from './views/login.js';
import { statsView } from './views/stats.js';
import { usersView } from './views/users.js';
import { ordersView } from './views/orders.js';
import { licensesView } from './views/licenses.js';
import { plansView } from './views/plans.js';
import { activationCodesView } from './views/activation-codes.js';
import { feedbackView } from './views/feedback.js';

register('/login', loginView);
register('/stats', statsView);
register('/users', usersView);
register('/orders', ordersView);
register('/licenses', licensesView);
register('/plans', plansView);
register('/activation-codes', activationCodesView);
register('/feedback', feedbackView);

const NAV = [
  { path: '/stats', label: '概览', icon: '📊' },
  { path: '/users', label: '用户', icon: '👤' },
  { path: '/orders', label: '订单', icon: '🧾' },
  { path: '/licenses', label: '授权', icon: '🔑' },
  { path: '/plans', label: '套餐', icon: '📦' },
  { path: '/activation-codes', label: '激活码', icon: '🎟️' },
  { path: '/feedback', label: '反馈', icon: '💬' },
];

function renderSidebar() {
  const { user } = getState();
  const sidebar = document.getElementById('admin-sidebar');
  if (!sidebar) return;
  if (user) {
    sidebar.innerHTML = `
      <div class="admin-brand"><span class="mark">⚡</span> Smart HID <span class="tag">Admin</span></div>
      <nav class="admin-nav">
        ${NAV.map((n) => `<a href="#${n.path}"><span class="ico">${n.icon}</span>${n.label}</a>`).join('')}
      </nav>
      <div class="admin-foot">
        <div class="admin-user" title="${user.email}">${user.email}</div>
        <button class="btn btn-outline btn-sm btn-block" id="admin-logout">登出</button>
        <a class="admin-back" href="index.html">← 返回落地页</a>
      </div>
    `;
    const lb = sidebar.querySelector('#admin-logout');
    if (lb) lb.onclick = () => { logout(); navigate('/login'); };
  } else {
    sidebar.innerHTML = `
      <div class="admin-brand"><span class="mark">⚡</span> Smart HID <span class="tag">Admin</span></div>
      <div class="admin-foot"><a class="admin-back" href="index.html">← 返回落地页</a></div>
    `;
  }
}

renderSidebar();
subscribe(renderSidebar);
startRouter();
