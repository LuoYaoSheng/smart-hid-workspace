// Smart HID ControlHub — Web 管理界面逻辑
// 原生 JS，无依赖、无构建。API Key 存 localStorage，每次请求带 Bearer。
//
// 与后端契约（见 internal/api/handlers.go + internal/command/types.go）：
//   GET  /api/v1/health                       {status, protocol, device_cnt}
//   GET  /api/v1/devices                      {devices:[{device_id,boot_id,online,usb_hid_ready,firmware}], total}
//   POST /api/v1/devices/{id}/commands        SmartHidCommand → ack | 202
//   GET  /api/v1/commands/{request_id}        {request_id, status, code, execution_ms?}

(() => {
  'use strict';

  // ---------- 动作 schema：驱动类型/动作/payload 表单 ----------
  const ACTION_SCHEMA = {
    keyboard: {
      tap: [
        { name: 'key', label: 'Key', type: 'text', placeholder: 'ENTER', list: 'keys-list' },
        { name: 'hold_ms', label: 'Hold ms', type: 'number', value: 40 },
      ],
      hotkey: [
        { name: 'keys', label: 'Keys（逗号分隔）', type: 'text', placeholder: 'CTRL,C' },
        { name: 'hold_ms', label: 'Hold ms', type: 'number', value: 40 },
      ],
      key_down: [
        { name: 'key', label: 'Key', type: 'text', placeholder: 'A', list: 'keys-list' },
        { name: 'lease_ms', label: 'Lease ms（自动 key_up）', type: 'number', value: 2000 },
      ],
      key_up: [
        { name: 'key', label: 'Key', type: 'text', placeholder: 'A', list: 'keys-list' },
      ],
    },
    mouse: {
      move: [
        { name: 'dx', label: 'dX', type: 'number', value: 0 },
        { name: 'dy', label: 'dY', type: 'number', value: 0 },
      ],
      click: [
        { name: 'button', label: 'Button', type: 'select', options: ['left', 'right', 'middle'], value: 'left' },
        { name: 'count', label: 'Count', type: 'number', value: 1, min: 1 },
      ],
      button_down: [
        { name: 'button', label: 'Button', type: 'select', options: ['left', 'right', 'middle'], value: 'left' },
        { name: 'lease_ms', label: 'Lease ms（自动 up）', type: 'number', value: 2000 },
      ],
      button_up: [
        { name: 'button', label: 'Button', type: 'select', options: ['left', 'right', 'middle'], value: 'left' },
      ],
      wheel: [
        { name: 'delta', label: 'Delta（负=上，正=下）', type: 'number', value: -3 },
      ],
    },
    system: {
      release_all: [],
    },
  };

  const TYPE_LABELS = { keyboard: '键盘', mouse: '鼠标', system: '系统' };

  // ---------- 全局状态 ----------
  const state = {
    apiKey: localStorage.getItem('smarthid_apikey') || '',
    selectedDevice: null, // 选中的设备对象（用于发命令）
    healthTimer: null,
    deviceTimer: null,
  };

  // ---------- DOM ----------
  const $ = (id) => document.getElementById(id);
  const el = {
    apiKey: $('api-key'), authForm: $('auth-form'),
    healthDot: $('health-dot'), healthInfo: $('health-info'),
    devicesTable: $('devices-table').querySelector('tbody'),
    deviceCount: $('device-count'), refreshDevices: $('refresh-devices'),
    composer: $('composer'), composerDevice: $('composer-device'),
    cmdType: $('cmd-type'), cmdAction: $('cmd-action'), cmdTtl: $('cmd-ttl'),
    payloadFields: $('payload-fields'), sendCmd: $('send-cmd'),
    sendHint: $('send-hint'), cmdResult: $('cmd-result'),
    closeComposer: $('close-composer'),
    lookupForm: $('lookup-form'), lookupId: $('lookup-id'), lookupResult: $('lookup-result'),
    // CH-P2/P4：设置面板
    rotateKey: $('rotate-key'), rotateResult: $('rotate-result'),
    lanToggle: $('lan-toggle'), lanStatus: $('lan-status'),
    // CH-P5：配对面板
    pairCreate: $('pair-create'), pairResult: $('pair-result'),
    pairHint: $('pair-hint'), pairTimer: $('pair-timer'),
  };

  // ---------- API 调用 ----------
  async function api(method, path, body) {
    const opts = { method, headers: { Authorization: 'Bearer ' + state.apiKey } };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    let res, txt;
    try {
      res = await fetch('/api/v1' + path, opts);
      txt = await res.text();
    } catch (e) {
      return { ok: false, status: 0, error: '网络错误：' + e.message };
    }
    let json = null;
    if (txt) { try { json = JSON.parse(txt); } catch { /* 非 JSON，保留 txt */ } }
    return { ok: res.ok, status: res.status, json, text: txt };
  }

  // ---------- 工具 ----------
  const esc = (s) => String(s == null ? '' : s).replace(/[&<>"']/g, (c) => (
    { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  function genRequestId() {
    const rand = Math.random().toString(36).slice(2, 8);
    return 'web_' + Date.now() + '_' + rand;
  }

  function boolTag(b, okText, badText) {
    return b ? `<span class="tag ok">${okText}</span>` : `<span class="tag bad">${badText}</span>`;
  }

  // ---------- Health ----------
  async function pollHealth() {
    const r = await api('GET', '/health');
    if (!r.ok || !r.json) {
      el.healthDot.className = 'dot down';
      el.healthDot.title = 'ControlHub 不可达';
      el.healthInfo.textContent = r.status === 0 ? 'ControlHub 不可达' : ('HTTP ' + r.status);
      return;
    }
    el.healthDot.className = 'dot up';
    el.healthDot.title = 'ControlHub 健康';
    el.healthInfo.textContent = `protocol ${r.json.protocol} · ${r.json.device_cnt} 台设备`;
  }

  // ---------- 设备列表 ----------
  async function pollDevices() {
    if (!state.apiKey) {
      el.deviceCount.textContent = '';
      return;
    }
    const r = await api('GET', '/devices');
    if (!r.ok) {
      // 401 → Key 无效
      if (r.status === 401) {
        el.deviceCount.innerHTML = '<span class="tag bad">API Key 无效</span>';
      } else {
        el.deviceCount.textContent = '加载失败 (HTTP ' + r.status + ')';
      }
      return;
    }
    const devs = r.json.devices || [];
    el.deviceCount.textContent = devs.length + ' 台设备';
    if (devs.length === 0) {
      el.devicesTable.innerHTML = '<tr><td colspan="6" class="muted center">尚无设备注册（启动 mock-device 或真实 ESP32 后会自动出现）</td></tr>';
      return;
    }
    el.devicesTable.innerHTML = devs.map((d) => `
      <tr>
        <td><code>${esc(d.device_id)}</code></td>
        <td><code class="muted">${esc(d.boot_id)}</code></td>
        <td>${boolTag(d.online, '在线', '离线')}</td>
        <td>${boolTag(d.usb_hid_ready, '就绪', '未就绪')}</td>
        <td>${esc(d.firmware) || '<span class="muted">—</span>'}</td>
        <td><button class="small" data-device="${esc(d.device_id)}">发送命令</button></td>
      </tr>`).join('');

    // 绑定每行"发送命令"按钮
    el.devicesTable.querySelectorAll('button[data-device]').forEach((btn) => {
      btn.addEventListener('click', () => {
        const id = btn.getAttribute('data-device');
        const dev = devs.find((x) => x.device_id === id);
        openComposer(dev);
      });
    });
  }

  // ---------- 命令编辑器 ----------
  function populateTypes() {
    el.cmdType.innerHTML = Object.keys(ACTION_SCHEMA)
      .map((t) => `<option value="${t}">${TYPE_LABELS[t]}（${t}）</option>`).join('');
    populateActions();
  }

  function populateActions() {
    const type = el.cmdType.value;
    const actions = Object.keys(ACTION_SCHEMA[type]);
    el.cmdAction.innerHTML = actions.map((a) => `<option value="${a}">${a}</option>`).join('');
    renderPayloadFields();
  }

  function renderPayloadFields() {
    const type = el.cmdType.value;
    const action = el.cmdAction.value;
    const fields = ACTION_SCHEMA[type][action] || [];
    if (fields.length === 0) {
      el.payloadFields.innerHTML = '<span class="muted">该动作无 payload 字段</span>';
      return;
    }
    el.payloadFields.innerHTML = fields.map((f) => {
      const id = 'pf-' + f.name;
      if (f.type === 'select') {
        const opts = f.options.map((o) => `<option value="${o}" ${o === f.value ? 'selected' : ''}>${o}</option>`).join('');
        return `<label>${f.label} <select id="${id}">${opts}</select></label>`;
      }
      const attrs = [
        `type="${f.type}"`,
        f.placeholder ? `placeholder="${esc(f.placeholder)}"` : '',
        f.value !== undefined ? `value="${f.value}"` : '',
        f.min !== undefined ? `min="${f.min}"` : '',
        f.list ? `list="${f.list}"` : '',
      ].join(' ');
      return `<label>${f.label} <input id="${id}" ${attrs}></label>`;
    }).join('');
  }

  function openComposer(device) {
    state.selectedDevice = device;
    el.composer.hidden = false;
    el.composerDevice.innerHTML = `<code>${esc(device.device_id)}</code> · boot <code class="muted">${esc(device.boot_id)}</code> · ${
      device.online && device.usb_hid_ready
        ? '<span class="tag ok">可下发</span>'
        : '<span class="tag bad">未就绪（命令会被拒绝）</span>'}`;
    el.cmdResult.innerHTML = '';
    el.sendHint.textContent = '';
    el.composer.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
  }

  function closeComposer() {
    state.selectedDevice = null;
    el.composer.hidden = true;
  }

  async function sendCommand() {
    if (!state.selectedDevice) return;
    const dev = state.selectedDevice;
    const type = el.cmdType.value;
    const action = el.cmdAction.value;
    const ttl = parseInt(el.cmdTtl.value, 10) || 3000;

    // 收集 payload
    const payload = {};
    const fields = ACTION_SCHEMA[type][action] || [];
    for (const f of fields) {
      const node = $('pf-' + f.name);
      let v = node ? node.value : '';
      if (f.type === 'number') v = parseInt(v, 10) || 0;
      if (f.name === 'keys') v = String(v).split(',').map((s) => s.trim()).filter(Boolean);
      payload[f.name] = v;
    }
    if (type === 'system') {
      // system 无 payload；后端接受空对象
    }

    const cmd = {
      protocol: '1.0',
      request_id: genRequestId(),
      device_id: dev.device_id,
      target_boot_id: dev.boot_id,
      type, action, ttl_ms: ttl,
      payload,
    };

    el.sendCmd.disabled = true;
    el.sendHint.textContent = '发送中…（最长等待 ' + ttl + 'ms）';
    el.cmdResult.innerHTML = '';
    const r = await api('POST', `/devices/${encodeURIComponent(dev.device_id)}/commands`, cmd);
    el.sendCmd.disabled = false;
    el.sendHint.textContent = '';

    renderCmdResult(r, cmd.request_id);
  }

  function renderCmdResult(r, reqId) {
    const base = `<div class="result-line"><span class="muted">request_id</span> <code>${esc(reqId)}</code></div>`;
    let body;
    if (r.status === 0) {
      body = `<div class="result-box error">网络错误：${esc(r.error)}</div>`;
    } else if (r.status === 200 && r.json && r.json.status === 'executed') {
      body = `<div class="result-box success">✓ executed · code ${r.json.code}${r.json.execution_ms ? ' · ' + r.json.execution_ms + 'ms' : ''}</div>`;
    } else if (r.status === 200 && r.json && r.json.status === 'duplicate') {
      body = `<div class="result-box success">↻ duplicate（幂等，命令已执行过）</div>`;
    } else if (r.status === 202) {
      body = `<div class="result-box pending">⏳ accepted · 已发布但 TTL 内未收到终态 ACK</div>`;
    } else if (r.status === 422 && r.json) {
      body = `<div class="result-box error">✗ rejected · code ${r.json.code}</div>`;
    } else if (r.status === 504 && r.json) {
      body = `<div class="result-box error">⏱ expired · code ${r.json.code}</div>`;
    } else if (r.status === 400 && r.json && r.json.fields) {
      const fields = r.json.fields.map((f) => `<li><code>${esc(f.field)}</code> — ${esc(f.message)}</li>`).join('');
      body = `<div class="result-box error">校验失败<ul>${fields}</ul></div>`;
    } else {
      body = `<div class="result-box error">HTTP ${r.status}<pre>${esc(r.text || '')}</pre></div>`;
    }
    el.cmdResult.innerHTML = base + body;
  }

  // ---------- 命令查询 ----------
  async function lookupCommand() {
    const id = el.lookupId.value.trim();
    if (!id) return;
    el.lookupResult.innerHTML = '<span class="muted">查询中…</span>';
    const r = await api('GET', `/commands/${encodeURIComponent(id)}`);
    if (r.status === 404) {
      el.lookupResult.innerHTML = `<div class="result-box error">未找到 <code>${esc(id)}</code></div>`;
      return;
    }
    if (!r.ok) {
      el.lookupResult.innerHTML = `<div class="result-box error">HTTP ${r.status}</div>`;
      return;
    }
    const j = r.json;
    const statusTag = STATUS_LABEL(j.status);
    el.lookupResult.innerHTML = `
      <div class="result-box">
        <div><span class="muted">request_id</span> <code>${esc(j.request_id)}</code></div>
        <div><span class="muted">状态</span> ${statusTag}</div>
        <div><span class="muted">code</span> ${j.code}</div>
        ${j.execution_ms != null ? `<div><span class="muted">execution_ms</span> ${j.execution_ms}</div>` : ''}
      </div>`;
  }

  function STATUS_LABEL(s) {
    const map = {
      received: ['待处理', 'pending'], executing: ['执行中', 'pending'],
      executed: ['已执行', 'success'], rejected: ['已拒绝', 'error'],
      expired: ['已过期', 'error'], duplicate: ['重复（幂等）', 'success'],
    };
    const m = map[s] || [s, 'pending'];
    return `<span class="tag ${m[1]}">${m[0]}</span>`;
  }

  // ---------- 设置面板（CH-P2/P4） ----------
  async function fetchLAN() {
    if (!state.apiKey) return;
    const r = await api('GET', '/settings/lan-mode');
    if (!r.ok) return;
    el.lanToggle.checked = !!r.json.enabled;
    el.lanStatus.textContent = r.json.note || '';
  }

  async function toggleLAN() {
    const enabled = el.lanToggle.checked;
    el.lanStatus.textContent = '保存中…';
    const r = await api('POST', '/settings/lan-mode', { enabled });
    if (!r.ok) {
      el.lanToggle.checked = !enabled; // 回滚
      el.lanStatus.textContent = '失败（HTTP ' + r.status + '）';
      return;
    }
    el.lanStatus.textContent = r.json.note || '已保存';
  }

  async function rotateAPIKey() {
    if (!state.apiKey) { alert('请先输入当前 API Key'); return; }
    if (!confirm('确定重置 API Key？当前 key 会立即失效，所有使用旧 key 的客户端需更新。')) return;
    el.rotateResult.textContent = '旋转中…';
    const r = await api('POST', '/api-keys/rotate', {});
    if (!r.ok) {
      el.rotateResult.innerHTML = '<span class="tag error">失败 HTTP ' + r.status + '</span>';
      return;
    }
    const newKey = r.json.api_key;
    // 自动应用新 key 到本会话
    state.apiKey = newKey;
    localStorage.setItem('smarthid_apikey', newKey);
    el.apiKey.value = newKey;
    // 用 textContent 渲染（避免 XSS），新 key 只显示一次
    const tag = document.createElement('span');
    tag.className = 'tag success';
    tag.textContent = '新 Key 已自动应用：';
    const code = document.createElement('code');
    code.textContent = newKey;
    el.rotateResult.innerHTML = '';
    el.rotateResult.appendChild(tag);
    el.rotateResult.appendChild(code);
    // 重新拉设备（验证新 key 工作）
    pollHealth();
    pollDevices();
  }

  // ---------- 配对面板（CH-P5） ----------
  let pairPollTimer = null;

  async function createPairingSession() {
    if (!state.apiKey) { alert('请先输入 API Key'); return; }
    el.pairCreate.disabled = true;
    el.pairHint.textContent = '创建中…';
    const r = await api('POST', '/pairing/sessions');
    el.pairCreate.disabled = false;
    if (!r.ok) {
      el.pairHint.textContent = '创建失败（HTTP ' + r.status + '）';
      return;
    }
    const token = r.json.token;
    const expiresAt = r.json.expires_at;
    const qr = r.json.qr_payload;
    renderPairingResult(token, qr, expiresAt);
    // 轮询配对状态直到 success 或过期
    if (pairPollTimer) clearInterval(pairPollTimer);
    const poll = async () => {
      const st = await api('GET', '/pairing/sessions/' + token);
      if (!st.ok) return;
      const status = st.json.status;
      const statusTag = el.pairResult.querySelector('.pair-status');
      if (statusTag) statusTag.textContent = statusLabel(status);
      if (status === 'success') {
        clearInterval(pairPollTimer);
        pairPollTimer = null;
        el.pairHint.textContent = '配对成功！设备应已上线。';
        pollDevices(); // 刷新设备列表
      } else if (status === 'expired' || status === 'revoked') {
        clearInterval(pairPollTimer);
        pairPollTimer = null;
        el.pairHint.textContent = '会话已 ' + status + '，请重新创建。';
      }
      updatePairTimer(expiresAt);
    };
    pairPollTimer = setInterval(poll, 2000);
    poll();
  }

  function statusLabel(s) {
    return { pending: '等待设备配对…', success: '✓ 已配对', expired: '已过期', revoked: '已撤销' }[s] || s;
  }

  function renderPairingResult(token, qr, expiresAt) {
    el.pairResult.hidden = false;
    el.pairResult.innerHTML = ''; // clear
    const status = document.createElement('div');
    status.className = 'pair-status';
    status.textContent = '等待设备配对…';
    const tokenLabel = document.createElement('div');
    tokenLabel.className = 'pair-line';
    tokenLabel.innerHTML = '<span class="muted">Token:</span> ';
    const code = document.createElement('code');
    code.textContent = token;
    code.className = 'pair-token';
    tokenLabel.appendChild(code);
    const qrLabel = document.createElement('div');
    qrLabel.className = 'pair-line';
    qrLabel.innerHTML = '<span class="muted">QR / Deep-link:</span> ';
    const qrCode = document.createElement('code');
    qrCode.textContent = qr;
    qrCode.className = 'pair-token';
    qrLabel.appendChild(qrCode);
    el.pairResult.appendChild(status);
    el.pairResult.appendChild(tokenLabel);
    el.pairResult.appendChild(qrLabel);
    el.pairHint.textContent = '在 5 分钟内让设备扫描 BLE 配网；本面板会自动轮询。';
    updatePairTimer(expiresAt);
  }

  function updatePairTimer(expiresAt) {
    const remain = expiresAt - Math.floor(Date.now() / 1000);
    if (remain <= 0) {
      el.pairTimer.textContent = '已过期';
      return;
    }
    const mm = Math.floor(remain / 60);
    const ss = remain % 60;
    el.pairTimer.textContent = '剩余 ' + mm + ':' + (ss < 10 ? '0' + ss : ss);
  }

  // ---------- 事件绑定 ----------
  el.authForm.addEventListener('submit', (e) => {
    e.preventDefault();
    state.apiKey = el.apiKey.value.trim();
    localStorage.setItem('smarthid_apikey', state.apiKey);
    pollHealth();
    pollDevices();
    fetchLAN();
  });

  el.refreshDevices.addEventListener('click', pollDevices);
  el.cmdType.addEventListener('change', populateActions);
  el.cmdAction.addEventListener('change', renderPayloadFields);
  el.sendCmd.addEventListener('click', sendCommand);
  el.closeComposer.addEventListener('click', closeComposer);
  el.lookupForm.addEventListener('submit', (e) => { e.preventDefault(); lookupCommand(); });
  el.rotateKey.addEventListener('click', rotateAPIKey);
  el.lanToggle.addEventListener('change', toggleLAN);
  el.pairCreate.addEventListener('click', createPairingSession);

  // ---------- 启动 ----------
  function start() {
    el.apiKey.value = state.apiKey;
    populateTypes();
    pollHealth();
    pollDevices();
    fetchLAN();
    // 定时刷新（仅健康与设备；命令状态按需查询）
    state.healthTimer = setInterval(pollHealth, 5000);
    state.deviceTimer = setInterval(() => {
      if (state.apiKey && el.composer.hidden) pollDevices(); // 编辑器打开时暂停刷新，避免选中设备被替换
    }, 4000);
  }

  document.addEventListener('DOMContentLoaded', start);
})();
