// admin 登录：复用 /auth/login，登录后探测 /admin/stats 确认 admin 身份。
import { api } from '../api.js';
import { setState } from '../store.js';
import { navigate } from '../router.js';
import { toast } from '../../portal/ui.js';

export function loginView() {
  const wrap = document.createElement('div');
  wrap.className = 'auth-wrap';
  wrap.innerHTML = `
    <div class="auth-card">
      <h1>管理后台登录</h1>
      <p class="muted">仅管理员账户可登录。普通用户请前往 <a class="link" href="app.html">用户控制台</a>。</p>
      <form id="admin-login-form" class="form" style="margin-top:18px">
        <label class="field">
          <span>邮箱</span>
          <input type="email" name="email" required autocomplete="email" placeholder="admin@example.com">
        </label>
        <label class="field">
          <span>密码</span>
          <input type="password" name="password" required autocomplete="current-password">
        </label>
        <button type="submit" class="btn btn-primary btn-lg btn-block">登录</button>
      </form>
      <p class="muted tiny" style="margin-top:14px">第一个 admin 需先在用户控制台注册，再由 config.admin_email 提升权限。</p>
    </div>
  `;
  wrap.querySelector('#admin-login-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    const email = (fd.get('email') || '').trim();
    const password = fd.get('password') || '';
    const btn = e.target.querySelector('button[type=submit]');
    btn.disabled = true; btn.textContent = '登录中…';
    try {
      const r = await api.login(email, password);
      setState({ token: r.token, user: { user_id: r.user_id, email: r.email } });
      // 探测 admin 身份（非 admin 会被 api.js 的 403 处理登出）
      await api.stats();
      toast('管理员登录成功', 'success');
      navigate('/stats');
    } catch (err) {
      // 403 已由 api.js toast；其他错误这里提示
      if (err.status !== 403) toast(err.message || '登录失败', 'error');
      btn.disabled = false; btn.textContent = '登录';
    }
  });
  return wrap;
}
