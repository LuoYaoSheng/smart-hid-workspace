// 订单列表 + 退款（paid 订单可退）。
import { api } from '../api.js';
import { toast, statusBadge, fmtMoney, fmtTime } from '../../portal/ui.js';

export async function ordersView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { orders } = await api.orders();
  const list = orders || [];
  root.innerHTML = `
    <div class="page-header"><h1 class="page-title">订单（${list.length}）</h1></div>
    <div class="card">
      <table class="table">
        <thead><tr><th>订单号</th><th>用户</th><th>套餐</th><th>金额</th><th>状态</th><th>时间</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((o) => `
              <tr>
                <td class="mono">${o.order_id}</td>
                <td class="mono">${o.user_id}</td>
                <td>${o.plan_id}</td>
                <td>${fmtMoney(o.amount_cents, o.currency)}</td>
                <td>${statusBadge((o.status || '').toUpperCase())}</td>
                <td>${fmtTime(o.created_at)}</td>
                <td>${(o.status || '').toLowerCase() === 'paid'
                  ? `<button class="btn btn-outline btn-sm" data-refund="${o.order_id}">退款</button>`
                  : '<span class="muted tiny">—</span>'}</td>
              </tr>`).join('')
            : `<tr><td colspan="7" class="muted text-center">暂无订单</td></tr>`
        }</tbody>
      </table>
    </div>
  `;
  root.querySelectorAll('[data-refund]').forEach((btn) => {
    btn.onclick = async () => {
      const id = btn.getAttribute('data-refund');
      if (!confirm('确认退款此订单？')) return;
      btn.disabled = true; btn.textContent = '处理中…';
      try {
        await api.refundOrder(id);
        toast('已退款', 'success');
        render(root);
      } catch (e) {
        toast(e.message || '退款失败', 'error');
        btn.disabled = false; btn.textContent = '退款';
      }
    };
  });
}
