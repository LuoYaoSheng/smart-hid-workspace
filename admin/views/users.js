// 用户列表（只读，V1 不做禁用）。
import { api } from '../api.js';
import { fmtTime } from '../../portal/ui.js';

export async function usersView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { users } = await api.users();
  const list = users || [];
  root.innerHTML = `
    <div class="page-header"><h1 class="page-title">用户（${list.length}）</h1></div>
    <div class="card">
      <table class="table">
        <thead><tr><th>User ID</th><th>邮箱</th><th>角色</th><th>注册时间</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((u) => `
              <tr>
                <td class="mono">${u.user_id}</td>
                <td>${u.email}</td>
                <td>${u.role === 'admin' ? '<span class="badge badge-ok">admin</span>' : '<span class="badge">user</span>'}</td>
                <td>${fmtTime(u.created_at)}</td>
              </tr>`).join('')
            : `<tr><td colspan="4" class="muted text-center">暂无用户</td></tr>`
        }</tbody>
      </table>
    </div>
  `;
}
