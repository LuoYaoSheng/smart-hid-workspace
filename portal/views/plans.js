// 套餐：卡片网格 + 购买（建单后跳订单页）。
import { api } from '../api.js';
import { navigate } from '../router.js';
import { toast, fmtMoney } from '../ui.js';

export async function plansView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { plans } = await api.listPlans();
  const list = (plans || []).filter((p) => p.active);

  if (!list.length) {
    root.innerHTML = `<div class="empty-state"><h3>暂无可购套餐</h3><p class="muted">请稍后再来。</p></div>`;
    return;
  }

  root.innerHTML = `
    <div class="page-header">
      <div>
        <h1 class="page-title">选择套餐</h1>
        <p class="page-sub">购买后可在订单页完成支付（V1 为模拟支付）并激活到设备。</p>
      </div>
    </div>
    <div class="card-grid">
      ${list.map((p) => planCard(p)).join('')}
    </div>
  `;

  root.querySelectorAll('[data-buy]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const planID = btn.getAttribute('data-buy');
      btn.disabled = true; btn.textContent = '创建订单…';
      try {
        await api.createOrder(planID);
        toast('订单已创建，去完成支付', 'success');
        navigate('/orders');
      } catch (e) {
        toast(e.message || '下单失败', 'error');
        btn.disabled = false; btn.textContent = '购买';
      }
    });
  });
}

function planCard(p) {
  const period = p.duration_days >= 365 ? '年' : (p.duration_days >= 30 ? '月' : `${p.duration_days} 天`);
  return `
    <div class="card plan-card ${p.plan_id.includes('yearly') ? 'popular' : ''}">
      <h3>${p.name}</h3>
      <div class="plan-price">${fmtMoney(p.price_cents, p.currency)}<small> / ${period}</small></div>
      <p class="muted tiny">${p.description || ''}</p>
      <ul class="plan-features">
        ${(p.features || []).map((f) => `<li>${f}</li>`).join('')}
        <li>有效期 ${p.duration_days} 天</li>
      </ul>
      <button class="btn btn-primary btn-block" data-buy="${p.plan_id}">购买</button>
    </div>
  `;
}
