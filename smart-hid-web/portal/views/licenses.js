// License：列表 + 激活（选设备）+ 下载 .license。
import { api } from '../api.js';
import { toast, statusBadge, fmtTime, saveBlob } from '../ui.js';

export async function licensesView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${e.message}</p></div>`;
  });
  return root;
}

async function render(root) {
  const [licRes, devRes] = await Promise.all([
    api.listLicenses(),
    api.listDevices().catch(() => ({ devices: [] })),
  ]);
  const list = licRes.licenses || [];
  const devices = devRes.devices || [];

  root.innerHTML = `
    <div class="page-header">
      <div>
        <h1 class="page-title">授权（License）</h1>
        <p class="page-sub">UNUSED 可激活到设备；ACTIVE 可下载 .license 文件交给 ControlHub 离线导入。</p>
      </div>
    </div>
    <div class="card">
      <table class="table">
        <thead><tr><th>License ID</th><th>套餐</th><th>状态</th><th>设备</th><th>到期</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((l) => licRow(l, devices)).join('')
            : `<tr><td colspan="6" class="muted text-center">还没有 License。先 <a class="link" href="#/plans">购买套餐</a> 并完成支付。</td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  wireActions(root, devices, render);
}

function licRow(l, devices) {
  const status = (l.status || '').toUpperCase();
  let actions = '';
  if (status === 'UNUSED') {
    if (!devices.length) {
      actions = '<span class="muted tiny">需先注册设备</span>';
    } else {
      actions = `<button class="btn btn-primary btn-sm" data-activate="${l.license_id}">激活</button>`;
    }
  } else if (status === 'ACTIVE') {
    actions = `<button class="btn btn-primary btn-sm" data-download="${l.license_id}">下载 .license</button>`;
  }
  return `
    <tr>
      <td class="mono">${l.license_id}</td>
      <td>${l.plan_id}</td>
      <td>${statusBadge(status)}</td>
      <td class="mono">${l.device_id || '<span class="muted">—</span>'}</td>
      <td>${fmtTime(l.expires_at)}</td>
      <td class="row-actions">${actions}</td>
    </tr>
  `;
}

function wireActions(root, devices, refresh) {
  // 激活：弹一个简易设备选择
  root.querySelectorAll('[data-activate]').forEach((btn) => {
    btn.addEventListener('click', () => {
      const licID = btn.getAttribute('data-activate');
      promptActivate(licID, devices, async (deviceID) => {
        btn.disabled = true; btn.textContent = '激活中…';
        try {
          await api.activateLicense(licID, deviceID);
          toast('License 已激活到 ' + deviceID, 'success');
          refresh(root);
        } catch (e) {
          toast(e.message || '激活失败', 'error');
          btn.disabled = false; btn.textContent = '激活';
        }
      });
    });
  });

  // 下载
  root.querySelectorAll('[data-download]').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const licID = btn.getAttribute('data-download');
      btn.disabled = true; btn.textContent = '下载中…';
      try {
        const { blob, filename } = await api.downloadLicense(licID);
        saveBlob(blob, filename);
        toast('已下载 ' + filename, 'success');
      } catch (e) {
        toast(e.message || '下载失败', 'error');
      }
      btn.disabled = false; btn.textContent = '下载 .license';
    });
  });
}

function promptActivate(licID, devices, onConfirm) {
  // 关闭已存在的对话框
  document.getElementById('activate-modal')?.remove();

  const modal = document.createElement('div');
  modal.id = 'activate-modal';
  modal.className = 'modal-backdrop';
  modal.innerHTML = `
    <div class="modal">
      <h3>激活 License</h3>
      <p class="muted tiny">选择要绑定的设备（激活后不可更改）：</p>
      <select id="activate-device" class="field" style="width:100%;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);font:inherit">
        ${devices.map((d) => `<option value="${d.device_id}">${d.device_id}${d.display_name ? ' — ' + d.display_name : ''}</option>`).join('')}
      </select>
      <div class="row" style="justify-content:flex-end;margin-top:18px">
        <button class="btn btn-ghost btn-sm" id="activate-cancel">取消</button>
        <button class="btn btn-primary btn-sm" id="activate-confirm">确认激活</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);
  modal.querySelector('#activate-cancel').onclick = () => modal.remove();
  modal.querySelector('#activate-confirm').onclick = () => {
    const deviceID = modal.querySelector('#activate-device').value;
    modal.remove();
    onConfirm(deviceID);
  };
  modal.onclick = (e) => { if (e.target === modal) modal.remove(); };
}
