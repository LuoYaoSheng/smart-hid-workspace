// 订单：列表 + 模拟支付（V1）。
import { api } from '../api.js';
import { toast, statusBadge, fmtMoney, fmtTime } from '../ui.js';

export async function ordersView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { orders } = await api.listOrders();
  const list = orders || [];

  root.innerHTML = `
    <div class="page-header">
      <div>
        <h1 class="page-title">订单</h1>
        <p class="page-sub">V1 为模拟支付：点「模拟支付」即视为已付款并自动生成 License（UNUSED）。</p>
      </div>
      <a class="btn btn-primary btn-sm" href="#/plans">选购套餐</a>
    </div>

    <div class="card">
      <table class="table">
        <thead><tr><th>订单号</th><th>套餐</th><th>金额</th><th>状态</th><th>创建时间</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((o) => orderRow(o)).join('')
            : `<tr><td colspan="6" class="muted text-center">还没有订单，去 <a class="link" href="#/plans">选购套餐</a></td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  root.querySelectorAll('[data-pay]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const orderID = btn.getAttribute('data-pay');
      if (!confirm('确认模拟支付此订单？V1 不会产生真实扣款。')) return;
      btn.disabled = true; btn.textContent = '处理中…';
      try {
        const r = await api.payCallback(orderID);
        const licID = (r.license && r.license.license_id) || '';
        toast('支付成功，License 已生成' + (licID ? '：' + licID : ''), 'success');
        if (confirm('License 已生成，去激活？')) {
          location.hash = '#/licenses';
        } else {
          render(root);
        }
      } catch (e) {
        toast(e.message || '支付失败', 'error');
        btn.disabled = false; btn.textContent = '模拟支付（V1）';
      }
    });
  });
}

function orderRow(o) {
  const pending = (o.status || '').toLowerCase() === 'pending';
  return `
    <tr>
      <td class="mono">${o.order_id}</td>
      <td>${o.plan_id}</td>
      <td>${fmtMoney(o.amount_cents, o.currency)}</td>
      <td>${statusBadge((o.status || '').toUpperCase())}</td>
      <td>${fmtTime(o.created_at)}</td>
      <td class="row-actions">
        ${pending
          ? `<button class="btn btn-primary btn-sm" data-pay="${o.order_id}">模拟支付（V1）</button>`
          : `<a class="link" href="#/licenses">查看 License →</a>`}
      </td>
    </tr>
  `;
}
