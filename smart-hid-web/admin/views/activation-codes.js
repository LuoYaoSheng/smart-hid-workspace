// 激活码：生成（user+device+plan）+ 列表 + 作废。
// 注意：消费端（ControlHub 输入码换 license）待 ControlHub 后续支持。
import { api } from '../api.js';
import { toast, statusBadge, fmtTime } from '../../portal/ui.js';

export async function activationCodesView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const [codesRes, usersRes] = await Promise.all([
    api.activationCodes(),
    api.users().catch(() => ({ users: [] })),
  ]);
  const list = codesRes.codes || [];
  const users = usersRes.users || [];

  root.innerHTML = `
    <div class="page-header">
      <h1 class="page-title">激活码（${list.length}）</h1>
      <p class="page-sub">生成后预建一张 UNUSED License 并绑定。消费端（ControlHub 输入码换 License）待后续支持。</p>
    </div>

    <div class="card">
      <h3>生成激活码</h3>
      <form id="code-form" class="form" style="margin-top:14px;display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:12px">
        <label class="field">
          <span>用户</span>
          <select name="user_id" id="code-user" required style="width:100%;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);font:inherit">
            ${users.map((u) => `<option value="${u.user_id}">${u.email}</option>`).join('')}
          </select>
        </label>
        <label class="field"><span>设备 ID</span><input name="device_id" required placeholder="HID-AAAAAAAA" style="font-family:monospace"></label>
        <label class="field"><span>套餐</span><input name="plan_id" required placeholder="plan_basic_yearly" style="font-family:monospace"></label>
        <div style="grid-column:1/-1"><button type="submit" class="btn btn-primary">生成</button></div>
      </form>
    </div>

    <div class="card" style="margin-top:20px">
      <table class="table">
        <thead><tr><th>激活码</th><th>License</th><th>用户</th><th>设备</th><th>状态</th><th>到期</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((c) => codeRow(c)).join('')
            : `<tr><td colspan="7" class="muted text-center">暂无激活码</td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  root.querySelector('#code-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const btn = e.target.querySelector('button[type=submit]');
    btn.disabled = true; btn.textContent = '生成中…';
    try {
      const c = await api.createActivationCode(fd.get('user_id'), fd.get('device_id'), fd.get('plan_id'));
      toast('已生成：' + c.code, 'success');
      render(root);
    } catch (err) {
      toast(err.message || '生成失败', 'error');
      btn.disabled = false; btn.textContent = '生成';
    }
  });

  root.querySelectorAll('[data-revoke]').forEach((btn) => {
    btn.onclick = async () => {
      const code = btn.getAttribute('data-revoke');
      if (!confirm('确认作废此激活码？')) return;
      try {
        await api.revokeActivationCode(code);
        toast('已作废', 'success');
        render(root);
      } catch (e) {
        toast(e.message || '作废失败', 'error');
      }
    };
  });
}

function codeRow(c) {
  const used = !!c.used_at;
  return `
    <tr>
      <td class="mono"><strong>${c.code}</strong></td>
      <td class="mono">${c.license_id}</td>
      <td class="mono">${c.user_id}</td>
      <td class="mono">${c.device_id}</td>
      <td>${used ? statusBadge('PAID').replace('已支付','已使用') : '<span class="badge badge-warn">未使用</span>'}</td>
      <td>${fmtTime(c.expires_at)}</td>
      <td>${used ? '<span class="muted tiny">—</span>' : `<button class="btn btn-outline btn-sm" data-revoke="${c.code}">作废</button>`}</td>
    </tr>
  `;
}
