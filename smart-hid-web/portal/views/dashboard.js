// 概览：用户卡片 + 统计 + 活跃授权 + 快捷入口。
import { api } from '../api.js';
import { getState } from '../store.js';
import { navigate } from '../router.js';
import { toast, statusBadge, fmtTime } from '../ui.js';

export async function dashboardView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { user } = getState();
  const [devs, lics, orders] = await Promise.all([
    api.listDevices().catch(() => ({ devices: [] })),
    api.listLicenses().catch(() => ({ licenses: [] })),
    api.listOrders().catch(() => ({ orders: [] })),
  ]);

  const deviceList = devs.devices || [];
  const licList = lics.licenses || [];
  const orderList = orders.orders || [];
  const activeLics = licList.filter((l) => l.status === 'ACTIVE');

  const activeLicHTML = activeLics.length
    ? activeLics.map((l) => `
        <tr>
          <td class="mono">${l.device_id || '—'}</td>
          <td>${l.plan_id}</td>
          <td>${statusBadge(l.status)}</td>
          <td>${fmtTime(l.expires_at)}</td>
          <td><a class="link" href="#/licenses">查看 →</a></td>
        </tr>`).join('')
    : `<tr><td colspan="5" class="muted text-center">暂无生效中的授权</td></tr>`;

  root.innerHTML = `
    <div class="page-header">
      <div>
        <h1 class="page-title">欢迎回来</h1>
        <p class="page-sub">${user ? user.email : ''}</p>
      </div>
      <div class="row">
        <a class="btn btn-primary btn-sm" href="#/plans">选购套餐</a>
        <a class="btn btn-ghost btn-sm" href="#/devices">注册设备</a>
      </div>
    </div>

    <div class="card-grid mt-24">
      <div class="card">
        <h3>设备</h3>
        <div class="plan-price">${deviceList.length}<small> 台</small></div>
        <p class="muted tiny mt-16"><a class="link" href="#/devices">管理设备 →</a></p>
      </div>
      <div class="card">
        <h3>生效授权</h3>
        <div class="plan-price">${activeLics.length}<small> 个</small></div>
        <p class="muted tiny mt-16"><a class="link" href="#/licenses">查看授权 →</a></p>
      </div>
      <div class="card">
        <h3>订单</h3>
        <div class="plan-price">${orderList.length}<small> 笔</small></div>
        <p class="muted tiny mt-16"><a class="link" href="#/orders">订单记录 →</a></p>
      </div>
    </div>

    <div class="card mt-24">
      <div class="row between">
        <h3 style="margin:0">生效中的授权</h3>
        <a class="link" href="#/licenses">全部授权 →</a>
      </div>
      <table class="table mt-16">
        <thead><tr><th>设备</th><th>套餐</th><th>状态</th><th>到期</th><th></th></tr></thead>
        <tbody>${activeLicHTML}</tbody>
      </table>
    </div>
  `;
}
