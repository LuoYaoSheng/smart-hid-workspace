// 套餐列表 + 上下架 + 新建/编辑（upsert）。
import { api } from '../api.js';
import { toast, fmtMoney, fmtTime } from '../../portal/ui.js';

export async function plansView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { plans } = await api.plans();
  const list = plans || [];
  root.innerHTML = `
    <div class="page-header"><h1 class="page-title">套餐（${list.length}）</h1></div>

    <div class="card">
      <h3>新建 / 编辑套餐</h3>
      <form id="plan-form" class="form" style="margin-top:14px;display:grid;grid-template-columns:repeat(auto-fill,minmax(200px,1fr));gap:12px">
        <label class="field"><span>plan_id</span><input name="plan_id" required placeholder="plan_xxx" style="font-family:monospace"></label>
        <label class="field"><span>名称</span><input name="name" required></label>
        <label class="field"><span>价格（分）</span><input name="price_cents" type="number" required placeholder="19900"></label>
        <label class="field"><span>币种</span><input name="currency" value="CNY"></label>
        <label class="field"><span>有效期（天）</span><input name="duration_days" type="number" required placeholder="365"></label>
        <label class="field"><span>特性（逗号分隔）</span><input name="features" value="hid_control"></label>
        <label class="field" style="grid-column:1/-1"><span>描述</span><input name="description"></label>
        <div style="grid-column:1/-1"><button type="submit" class="btn btn-primary">保存（upsert）</button></div>
      </form>
    </div>

    <div class="card" style="margin-top:20px">
      <table class="table">
        <thead><tr><th>plan_id</th><th>名称</th><th>价格</th><th>有效期</th><th>状态</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((p) => `
              <tr>
                <td class="mono">${p.plan_id}</td>
                <td>${p.name}</td>
                <td>${fmtMoney(p.price_cents, p.currency)}</td>
                <td>${p.duration_days} 天</td>
                <td>${p.active ? '<span class="badge badge-ok">上架</span>' : '<span class="badge badge-bad">下架</span>'}</td>
                <td class="row-actions">
                  ${p.active
                    ? `<button class="btn btn-outline btn-sm" data-id="${p.plan_id}" data-active="0">下架</button>`
                    : `<button class="btn btn-primary btn-sm" data-id="${p.plan_id}" data-active="1">上架</button>`}
                </td>
              </tr>`).join('')
            : `<tr><td colspan="6" class="muted text-center">暂无套餐</td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  // upsert
  root.querySelector('#plan-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const p = {
      plan_id: fd.get('plan_id').trim(),
      name: fd.get('name').trim(),
      price_cents: parseInt(fd.get('price_cents'), 10) || 0,
      currency: fd.get('currency').trim() || 'CNY',
      duration_days: parseInt(fd.get('duration_days'), 10) || 0,
      features: (fd.get('features') || '').split(',').map((s) => s.trim()).filter(Boolean),
      description: fd.get('description').trim(),
      active: true,
    };
    try {
      await api.upsertPlan(p);
      toast('套餐已保存', 'success');
      render(root);
    } catch (err) {
      toast(err.message || '保存失败', 'error');
    }
  });

  // 上下架
  root.querySelectorAll('[data-active]').forEach((btn) => {
    btn.onclick = async () => {
      const id = btn.getAttribute('data-id');
      const active = btn.getAttribute('data-active') === '1';
      try {
        await api.setPlanActive(id, active);
        toast(active ? '已上架' : '已下架', 'success');
        render(root);
      } catch (e) {
        toast(e.message || '操作失败', 'error');
      }
    };
  });
}
