// License 列表 + 禁用/吊销/恢复。
import { api } from '../api.js';
import { toast, statusBadge, fmtTime } from '../../portal/ui.js';

export async function licensesView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { licenses } = await api.licenses();
  const list = licenses || [];
  root.innerHTML = `
    <div class="page-header">
      <h1 class="page-title">授权（${list.length}）</h1>
      <p class="page-sub">DISABLED 可恢复；REVOKED 不可逆。注意：已导入 ControlHub 的 license 离线仍有效（本地优先）。</p>
    </div>
    <div class="card">
      <table class="table">
        <thead><tr><th>License ID</th><th>用户</th><th>套餐</th><th>状态</th><th>设备</th><th>到期</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((l) => licenseRow(l)).join('')
            : `<tr><td colspan="7" class="muted text-center">暂无 License</td></tr>`
        }</tbody>
      </table>
    </div>
  `;
  root.querySelectorAll('[data-action]').forEach((btn) => {
    btn.onclick = async () => {
      const id = btn.getAttribute('data-id');
      const action = btn.getAttribute('data-action');
      const labels = { disable: '禁用', revoke: '吊销', 're-enable': '恢复' };
      if (!confirm(`确认${labels[action]}此 License？`)) return;
      btn.disabled = true;
      try {
        await api.setLicenseStatus(id, action);
        toast('已' + labels[action], 'success');
        render(root);
      } catch (e) {
        toast(e.message || '操作失败', 'error');
        btn.disabled = false;
      }
    };
  });
}

function licenseRow(l) {
  const st = (l.status || '').toUpperCase();
  let actions = '';
  if (st === 'ACTIVE') {
    actions = `<button class="btn btn-outline btn-sm" data-id="${l.license_id}" data-action="disable">禁用</button>`
            + `<button class="btn btn-outline btn-sm" data-id="${l.license_id}" data-action="revoke">吊销</button>`;
  } else if (st === 'DISABLED' || st === 'UNUSED' || st === 'EXPIRED') {
    actions = `<button class="btn btn-outline btn-sm" data-id="${l.license_id}" data-action="re-enable">恢复 ACTIVE</button>`;
    if (st !== 'EXPIRED') {
      actions += `<button class="btn btn-outline btn-sm" data-id="${l.license_id}" data-action="revoke">吊销</button>`;
    }
  } else { // REVOKED
    actions = '<span class="muted tiny">不可逆</span>';
  }
  return `
    <tr>
      <td class="mono">${l.license_id}</td>
      <td class="mono">${l.user_id}</td>
      <td>${l.plan_id}</td>
      <td>${statusBadge(st)}</td>
      <td class="mono">${l.device_id || '<span class="muted">—</span>'}</td>
      <td>${fmtTime(l.expires_at)}</td>
      <td class="row-actions">${actions}</td>
    </tr>
  `;
}
