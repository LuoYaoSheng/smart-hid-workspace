// 设备：注册（带 device_id 实时校验）+ 列表。
import { api } from '../api.js';
import { DEVICE_ID_RE } from '../config.js';
import { toast, fmtTime } from '../ui.js';

export async function devicesView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { devices } = await api.listDevices();
  const list = devices || [];

  root.innerHTML = `
    <div class="page-header">
      <div>
        <h1 class="page-title">设备管理</h1>
        <p class="page-sub">注册设备后才能把 License 激活到该设备。</p>
      </div>
    </div>

    <div class="card">
      <h3>注册新设备</h3>
      <form id="dev-form" class="form mt-16">
        <label class="field">
          <span>设备 ID</span>
          <input type="text" name="device_id" id="dev-id" required
                 placeholder="HID-ABCD1234" autocomplete="off" spellcheck="false"
                 style="font-family:ui-monospace,Menlo,monospace;text-transform:uppercase">
          <span class="field-hint" id="dev-hint">格式：HID- 后接 8 位大写字母或数字（如 HID-AAAA1111）</span>
        </label>
        <label class="field">
          <span>备注名（可选）</span>
          <input type="text" name="name" placeholder="如：办公电脑、客厅 HTPC">
        </label>
        <button type="submit" class="btn btn-primary" id="dev-submit">注册设备</button>
      </form>
    </div>

    <div class="card mt-24">
      <h3>已注册设备（${list.length}）</h3>
      <table class="table mt-16">
        <thead><tr><th>设备 ID</th><th>备注名</th><th>注册时间</th></tr></thead>
        <tbody>${
            list.length
            ? list.map((d) => `
              <tr>
                <td class="mono">${d.device_id}</td>
                <td>${d.display_name || '<span class="muted">—</span>'}</td>
                <td>${fmtTime(d.paired_at)}</td>
              </tr>`).join('')
            : `<tr><td colspan="3" class="muted text-center">还没有注册任何设备</td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  wireForm(root);
}

function wireForm(root) {
  const input = root.querySelector('#dev-id');
  const hint = root.querySelector('#dev-hint');
  const form = root.querySelector('#dev-form');
  const submit = root.querySelector('#dev-submit');

  // 实时校验
  input.addEventListener('input', () => {
    const v = input.value.toUpperCase();
    input.value = v;
    if (!v) { hint.className = 'field-hint'; input.classList.remove('invalid'); return; }
    if (DEVICE_ID_RE.test(v)) {
      hint.className = 'field-hint'; hint.style.color = '#10b981';
      hint.textContent = '✓ 格式正确';
      input.classList.remove('invalid');
    } else {
      hint.className = 'field-hint'; hint.style.color = '';
      hint.textContent = '格式：HID- 后接 8 位大写字母或数字';
      input.classList.add('invalid');
    }
  });

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const fd = new FormData(form);
    const device_id = (fd.get('device_id') || '').trim().toUpperCase();
    const name = (fd.get('name') || '').trim();
    if (!DEVICE_ID_RE.test(device_id)) {
      toast('设备 ID 格式不正确', 'error');
      return;
    }
    submit.disabled = true; submit.textContent = '注册中…';
    try {
      await api.registerDevice(device_id, name || undefined);
      toast('设备已注册', 'success');
      render(root); // 刷新列表
    } catch (err) {
      toast(err.message || '注册失败', 'error');
      submit.disabled = false; submit.textContent = '注册设备';
    }
  });
}
