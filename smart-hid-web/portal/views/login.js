// 登录 / 注册 —— 两个视图共享同一表单骨架。
import { api } from '../api.js';
import { setState } from '../store.js';
import { navigate } from '../router.js';
import { toast } from '../ui.js';

function form(mode) {
  const isLogin = mode === 'login';
  const wrap = document.createElement('div');
  wrap.className = 'auth-wrap';
  wrap.innerHTML = `
    <div class="auth-card">
      <h1>${isLogin ? '登录到 Smart HID' : '创建 Smart HID 账户'}</h1>
      <p class="muted">
        ${isLogin ? '还没有账户？' : '已有账户？'}
        <a class="link" href="#${isLogin ? '/register' : '/login'}">${isLogin ? '去注册' : '去登录'}</a>
      </p>
      <form id="auth-form" class="form" autocomplete="on">
        <label class="field">
          <span>邮箱</span>
          <input type="email" name="email" required autocomplete="email" placeholder="you@example.com">
        </label>
        <label class="field">
          <span>密码${isLogin ? '' : '（至少 8 位）'}</span>
          <input type="password" name="password" required
                 autocomplete="${isLogin ? 'current-password' : 'new-password'}"
                 ${isLogin ? '' : 'minlength="8"'}>
        </label>
        <button type="submit" class="btn btn-primary btn-lg btn-block">${isLogin ? '登录' : '创建账户'}</button>
      </form>
      <p class="muted tiny">登录后可在控制台管理设备、套餐、License。Token 仅存本机 localStorage。</p>
    </div>
  `;
  const formEl = wrap.querySelector('#auth-form');
  formEl.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(formEl);
    const email = (fd.get('email') || '').trim();
    const password = fd.get('password') || '';
    const btn = formEl.querySelector('button[type=submit]');
    btn.disabled = true; btn.textContent = '处理中…';
    try {
      const r = isLogin
        ? await api.login(email, password)
        : await api.register(email, password);
      setState({ token: r.token, user: { user_id: r.user_id, email: r.email } });
      toast(isLogin ? '登录成功' : '注册成功，已自动登录', 'success');
      navigate('/dashboard');
    } catch (err) {
      toast(err.message || '操作失败', 'error');
      btn.disabled = false; btn.textContent = isLogin ? '登录' : '创建账户';
    }
  });
  return wrap;
}

export function loginView() { return form('login'); }
export function registerView() { return form('register'); }
