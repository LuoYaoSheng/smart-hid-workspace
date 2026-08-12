// 账户：邮箱 + user_id + 登出 + 擦除本地凭据。
import { api } from '../api.js';
import { getState } from '../store.js';
import { logout } from '../store.js';
import { navigate } from '../router.js';
import { toast } from '../ui.js';

export async function accountView() {
  const root = document.createElement('div');
  const { user } = getState();

  root.innerHTML = `
    <div class="page-header">
      <h1 class="page-title">账户</h1>
    </div>
    <div class="card">
      <h3>当前登录</h3>
      <table class="table mt-16">
        <tbody>
          <tr><th>邮箱</th><td>${user ? user.email : '—'}</td></tr>
          <tr><th>账户 ID</th><td class="mono">${user ? user.user_id : '—'}</td></tr>
        </tbody>
      </table>
      <hr class="divider">
      <h3>本地数据</h3>
      <p class="muted tiny">Token 与账户信息仅存于本机 localStorage，登出即清除。License 文件一旦下载即为离线凭证，登出不影响已导入到 ControlHub 的授权。</p>
      <div class="row mt-16">
        <button class="btn btn-outline" id="acc-logout">登出当前账户</button>
      </div>
    </div>
  `;

  root.querySelector('#acc-logout').onclick = async () => {
    if (!confirm('确认登出？将清除本机保存的 Token。')) return;
    logout();
    toast('已登出', 'info');
    navigate('/login');
  };

  return root;
}
