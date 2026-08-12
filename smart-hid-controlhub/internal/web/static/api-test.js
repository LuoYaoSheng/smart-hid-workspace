// Smart HID ControlHub — API Playground
// 原生 JS、零依赖。给开发/测试者用：自由构造任意 endpoint 请求，看原始响应。
//
// API Key 与主控制台 (index.html) 共享 localStorage 'smarthid_apikey'。
// 同源请求（127.0.0.1:17890），无 CORS 问题。

(() => {
  'use strict';

  // ControlHub 全部 endpoint 预设（与 openapi.yaml 同步）。
  // path 里的 HID-AAAA1111 / TOKEN_PLACEHOLDER / req-test-1 等可直接在 UI 改。
  const ENDPOINTS = [
    { method: 'GET',    path: '/api/v1/health',
      desc: '健康检查（无需 API Key）', body: null },
    { method: 'GET',    path: '/api/v1/devices',
      desc: '列出所有已知设备', body: null },
    { method: 'GET',    path: '/api/v1/devices/HID-AAAA1111',
      desc: '查询单个设备详情（path 里改 device_id）', body: null },
    { method: 'POST',   path: '/api/v1/devices/HID-AAAA1111/commands',
      desc: '发送 HID 命令（target_boot_id 需匹配设备当前 boot_id）', body: {
        protocol: "1.0", request_id: "req-test-1", device_id: "HID-AAAA1111",
        target_boot_id: "BOOT-PLACEHOLDER", type: "keyboard", action: "tap",
        ttl_ms: 3000, payload: { key: "ENTER", hold_ms: 40 }
      }},
    { method: 'GET',    path: '/api/v1/commands/req-test-1',
      desc: '查询命令状态（path 里改 request_id）', body: null },
    { method: 'GET',    path: '/api/v1/api-keys',
      desc: '列出所有 API key（不含明文）', body: null },
    { method: 'POST',   path: '/api/v1/api-keys/rotate',
      desc: '轮换 API key（旧 key 立即失效，新 key 仅返回一次）', body: { label: "playground" } },
    { method: 'GET',    path: '/api/v1/settings/lan-mode',
      desc: '查询 LAN 模式开关', body: null },
    { method: 'POST',   path: '/api/v1/settings/lan-mode',
      desc: '切换 LAN 模式（持久化，重启 ControlHub 生效）', body: { enabled: true } },
    { method: 'POST',   path: '/api/v1/pairing/sessions',
      desc: '创建配对 session（返 token + QR payload）', body: {} },
    { method: 'GET',    path: '/api/v1/pairing/sessions/TOKEN_PLACEHOLDER',
      desc: '查询配对 session 状态（path 里填实际 token）', body: null },
    { method: 'GET',    path: '/api/v1/usage',
      desc: '查 Trial 用量（默认单设备自动选择）', body: null },
    { method: 'GET',    path: '/api/v1/usage/all',
      desc: '列出所有设备 Trial 用量', body: null },
  ];

  const state = {
    apiKey: localStorage.getItem('smarthid_apikey') || '',
  };

  const $ = (id) => document.getElementById(id);
  const el = {
    apiKey: $('api-key'), authForm: $('auth-form'),
    search: $('pg-search'), list: $('pg-list'),
    method: $('pg-method'), path: $('pg-path'), send: $('pg-send'),
    desc: $('pg-desc'), body: $('pg-body'),
    status: $('pg-status'), duration: $('pg-duration'),
    respBody: $('pg-resp-body'),
    copy: $('pg-copy'), clear: $('pg-clear'),
  };

  // ---------- endpoint 列表渲染 ----------
  function renderEndpoints(filter = '') {
    el.list.innerHTML = '';
    const f = filter.trim().toLowerCase();
    ENDPOINTS.forEach((ep, i) => {
      if (f && !ep.path.toLowerCase().includes(f) && !ep.method.toLowerCase().includes(f)) return;
      const li = document.createElement('li');
      li.className = 'pg-item method-' + ep.method.toLowerCase();
      const m = document.createElement('span');
      m.className = 'pg-method'; m.textContent = ep.method;
      const p = document.createElement('span');
      p.className = 'pg-path'; p.textContent = ep.path;
      li.appendChild(m); li.appendChild(p);
      li.addEventListener('click', () => selectEndpoint(i));
      el.list.appendChild(li);
    });
  }

  function selectEndpoint(i) {
    const ep = ENDPOINTS[i];
    el.method.value = ep.method;
    el.path.value = ep.path;
    el.desc.textContent = ep.desc || '';
    el.body.value = ep.body !== null ? JSON.stringify(ep.body, null, 2) : '';
  }

  // ---------- 发送请求 ----------
  async function sendRequest() {
    const method = el.method.value;
    let path = el.path.value.trim();
    if (!path) { alert('请填 path'); return; }
    if (!path.startsWith('/')) path = '/' + path;

    const opts = { method, headers: {} };
    if (state.apiKey) opts.headers['Authorization'] = 'Bearer ' + state.apiKey;
    if ((method === 'POST' || method === 'PUT' || method === 'DELETE') && el.body.value.trim()) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = el.body.value;
    }

    el.status.textContent = '发送中…';
    el.status.className = 'tag pending';
    el.duration.textContent = '';
    el.respBody.textContent = '';
    el.copy.hidden = true;
    el.send.disabled = true;

    const t0 = performance.now();
    try {
      const r = await fetch(path, opts);
      const dt = (performance.now() - t0).toFixed(0);
      const text = await r.text();
      el.duration.textContent = dt + ' ms';
      el.status.textContent = r.status + ' ' + statusText(r.status);
      el.status.className = 'tag ' + statusClass(r.status);

      let pretty = text;
      try {
        pretty = JSON.stringify(JSON.parse(text), null, 2);
      } catch (_) { /* 非 JSON，原样 */ }
      el.respBody.textContent = pretty || '(空响应体)';
      el.copy.hidden = false;
    } catch (e) {
      el.status.textContent = '网络错误';
      el.status.className = 'tag error';
      el.respBody.textContent = e.message + '\n\n（ControlHub 是否在运行？端口对不对？）';
    } finally {
      el.send.disabled = false;
    }
  }

  function statusText(code) {
    const map = {
      200: 'OK', 201: 'Created', 202: 'Accepted',
      400: 'Bad Request', 401: 'Unauthorized', 402: 'Payment Required',
      404: 'Not Found', 405: 'Method Not Allowed', 409: 'Conflict',
      422: 'Unprocessable Entity',
      500: 'Internal Server Error', 502: 'Bad Gateway', 504: 'Gateway Timeout',
    };
    return map[code] || '';
  }

  function statusClass(code) {
    if (code >= 200 && code < 300) return 'success';
    if (code >= 400 && code < 500) return 'error';
    if (code >= 500) return 'error';
    return 'pending';
  }

  // ---------- 事件绑定 ----------
  el.authForm.addEventListener('submit', (e) => {
    e.preventDefault();
    state.apiKey = el.apiKey.value.trim();
    localStorage.setItem('smarthid_apikey', state.apiKey);
    el.authForm.querySelector('button').textContent = '已保存';
    setTimeout(() => {
      el.authForm.querySelector('button').textContent = '保存';
    }, 1000);
  });
  el.search.addEventListener('input', () => renderEndpoints(el.search.value));
  el.send.addEventListener('click', sendRequest);
  el.copy.addEventListener('click', () => {
    navigator.clipboard.writeText(el.respBody.textContent).then(() => {
      el.copy.textContent = '已复制';
      setTimeout(() => { el.copy.textContent = '复制响应'; }, 1000);
    });
  });
  el.clear.addEventListener('click', () => {
    el.status.textContent = '未发送';
    el.status.className = 'tag pending';
    el.duration.textContent = '';
    el.respBody.textContent = '点击"发送"看响应';
    el.copy.hidden = true;
  });

  // Ctrl/Cmd + Enter 发送
  document.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault();
      sendRequest();
    }
  });

  // ---------- 初始化 ----------
  el.apiKey.value = state.apiKey;
  renderEndpoints();
  selectEndpoint(0);
})();
