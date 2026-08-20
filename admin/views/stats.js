// 概览：统计卡片网格。
import { api } from '../api.js';
import { fmtMoney } from '../../portal/ui.js';

export async function statsView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const st = await api.stats();
  const cards = [
    { label: '用户总数', value: st.users_total },
    { label: '订单总数', value: st.orders_total },
    { label: '已支付订单', value: st.orders_paid },
    { label: '累计金额', value: fmtMoney(st.revenue_cents) },
    { label: 'License 总数', value: st.licenses_total },
    { label: '生效 License', value: st.licenses_active },
    { label: '未用激活码', value: st.codes_unused },
  ];
  root.innerHTML = `
    <div class="page-header"><h1 class="page-title">概览</h1></div>
    <div class="stat-grid">
      ${cards.map((c) => `
        <div class="stat-card">
          <div class="label">${c.label}</div>
          <div class="value">${c.value}</div>
        </div>`).join('')}
    </div>
  `;
}
