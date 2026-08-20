// demo.js — 模拟键鼠演示台（OS-2）
//
// 全部命令经既有 POST /api/v1/devices/{id}/commands → MQTT → ESP32 → USB HID。
// 设计要点：
//   - 合流与限流：触控板 move 增量聚合、≤16 次/秒发送；在途上限 8，超出丢最旧（防设备队列 32 打满）
//   - 修饰键组合：可视化键盘锁定式；直通模式跟踪式（有修饰发 hotkey，无修饰发 key_down/key_up 对）
//   - lease 兜底：key_down/button_down 均带 lease_ms，断连/丢 key_up 时设备侧自动释放
(function () {
  'use strict';

  var $ = function (id) { return document.getElementById(id); };
  var el = {
    apiKey: $('api-key'), btnKey: $('btn-key'), deviceSel: $('device-sel'), devState: $('dev-state'),
    btnRelease: $('btn-release'),
    stSent: $('st-sent'), stOk: $('st-ok'), stBad: $('st-bad'), stRtt: $('st-rtt'), stLast: $('st-last'),
    mVk: $('m-vk'), mPt: $('m-pt'), viewVk: $('view-vk'), viewPt: $('view-pt'),
    fnToggle: $('fn-toggle'), fnRows: $('fn-rows'), vkbd: $('vkbd'),
    ptArea: $('pt-area'),
    pad: $('pad'), padDot: $('pad-dot'), btnSel: $('btn-sel'), btnClick: $('btn-click'),
    btnDbl: $('btn-dbl'), sens: $('sens'),
    blastText: $('blast-text'), btnBlast: $('btn-blast'), blastState: $('blast-state'),
  };

  var ws = null;
  var wsEvents = [];
  var wsFails = 0; // 连续失败计数（旧 key 场景停止无限重连，换 key 后重置）

  var state = {
    apiKey: localStorage.getItem('smarthid_apikey') || '',
    device: null,          // 选中的设备对象 {device_id, boot_id, online, usb_hid_ready}
    sent: 0, ok: 0, bad: 0,
    inFlight: 0, maxInFlight: 8,
    mods: {},              // 可视化键盘修饰键锁定 {CTRL:true,...}
    ptMods: {},            // 直通模式修饰键按下状态
    button: 'left',
    blastRunning: false,
  };

  // ---------- API ----------

  function api(method, path, body) {
    var opts = { method: method, headers: { Authorization: 'Bearer ' + state.apiKey } };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }
    return fetch('/api/v1' + path, opts).then(function (res) {
      return res.text().then(function (txt) {
        var json = null;
        if (txt) { try { json = JSON.parse(txt); } catch (e) { /* 非 JSON */ } }
        return { ok: res.ok, status: res.status, json: json };
      });
    }).catch(function (e) {
      return { ok: false, status: 0, error: String(e) };
    });
  }

  function genRequestId() {
    var rand = Math.random().toString(36).slice(2, 8);
    return 'demo_' + Date.now() + '_' + rand;
  }

  // 发一条 HID 命令（带统计/在途限流/键帽回联动效钩子）
  function send(type, action, payload, onAck) {
    if (!state.device) { setLast('未选择设备', true); return; }
    if (state.inFlight >= state.maxInFlight) { return; } // 限流：丢弃，不排队
    var cmd = {
      protocol: '1.0',
      request_id: genRequestId(),
      device_id: state.device.device_id,
      target_boot_id: state.device.boot_id,
      type: type, action: action,
      ttl_ms: 2000,
      payload: payload || {},
    };
    state.sent++;
    state.inFlight++;
    updateStats();
    var t0 = performance.now();
    api('POST', '/devices/' + encodeURIComponent(cmd.device_id) + '/commands', cmd)
      .then(function (r) {
        state.inFlight--;
        var rtt = Math.round(performance.now() - t0);
        if (r.status === 200 && r.json && (r.json.status === 'executed' || r.json.status === 'duplicate')) {
          state.ok++;
          setLast(cmd.action + ' ✓ ' + rtt + 'ms', false);
          if (onAck) onAck(true, r.json);
        } else if (r.status === 202) {
          state.ok++;
          setLast(cmd.action + ' 已发出(未回执) ' + rtt + 'ms', false);
        } else {
          state.bad++;
          var msg = r.json && (r.json.status || r.json.error) || ('HTTP ' + r.status);
          setLast(cmd.action + ' ✗ ' + msg, true);
          if (onAck) onAck(false, r.json);
        }
        updateStats(rtt);
      })
      .catch(function () { state.inFlight--; state.bad++; updateStats(); });
  }

  function updateStats(rtt) {
    el.stSent.textContent = state.sent;
    el.stOk.textContent = state.ok;
    el.stBad.textContent = state.bad;
    if (rtt) el.stRtt.textContent = rtt + ' ms';
  }
  function setLast(text, isBad) {
    el.stLast.textContent = text;
    el.stLast.className = isBad ? 'bad-text' : 'ok-text';
    el.stLast.style.fontSize = '13px';
  }

  // ---------- 连接区 ----------

  el.apiKey.value = state.apiKey;
  el.btnKey.addEventListener('click', function () {
    state.apiKey = el.apiKey.value.trim();
    localStorage.setItem('smarthid_apikey', state.apiKey);
    refreshDevices();
    if (ws) { try { ws.close(); } catch (e) {} }
    wsFails = 0; // 手动连接重置失败计数
    connectWS();
  });
  el.apiKey.addEventListener('keydown', function (e) { if (e.key === 'Enter') el.btnKey.click(); });

  function refreshDevices() {
    if (!state.apiKey) { el.devState.textContent = '请输入 API Key'; return; }
    el.devState.textContent = '加载设备…';
    api('GET', '/devices').then(function (r) {
      if (!r.ok) { el.devState.textContent = '❌ ' + (r.json && r.json.error || r.status); return; }
      var devs = (r.json && r.json.devices) || [];
      var prev = state.device && state.device.device_id;
      el.deviceSel.innerHTML = '<option value="">— 选择设备 —</option>' + devs.map(function (d) {
        var ready = d.online && d.usb_hid_ready;
        return '<option value="' + d.device_id + '"' + (d.device_id === prev ? ' selected' : '') + '>' +
          d.device_id + (ready ? ' ✓就绪' : '（未就绪）') + '</option>';
      }).join('');
      if (prev) { // 恢复选中
        for (var i = 0; i < devs.length; i++) {
          if (devs[i].device_id === prev) selectDevice(devs[i]);
        }
      }
      el.devState.textContent = devs.length ? '' : '暂无设备（先在控制台完成配对）';
    });
  }

  function selectDevice(d) {
    state.device = d;
    el.devState.textContent = d.online && d.usb_hid_ready
      ? '● 已连接 ' + d.device_id
      : '○ 设备未就绪，命令会被拒绝';
  }
  el.deviceSel.addEventListener('change', function () {
    var id = el.deviceSel.value;
    if (!id) { state.device = null; el.devState.textContent = ''; return; }
    api('GET', '/devices').then(function (r) {
      var devs = (r.json && r.json.devices) || [];
      for (var i = 0; i < devs.length; i++) {
        if (devs[i].device_id === id) { selectDevice(devs[i]); break; }
      }
    });
  });

  el.btnRelease.addEventListener('click', function () {
    send('system', 'release_all', {});
  });

  // ---------- 模式切换 ----------

  el.mVk.addEventListener('click', function () { switchMode('vk'); });
  el.mPt.addEventListener('click', function () { switchMode('pt'); });
  function switchMode(m) {
    el.mVk.className = m === 'vk' ? 'on' : '';
    el.mPt.className = m === 'pt' ? 'on' : '';
    el.viewVk.hidden = m !== 'vk';
    el.viewPt.hidden = m !== 'pt';
    if (m !== 'pt') clearPtMods();
  }

  // ---------- 可视化键盘 ----------

  // [label, hidName, extraClass]
  var ROWS = [
    [['1','DIGIT1'],['2','DIGIT2'],['3','DIGIT3'],['4','DIGIT4'],['5','DIGIT5'],['6','DIGIT6'],['7','DIGIT7'],['8','DIGIT8'],['9','DIGIT9'],['0','DIGIT0']],
    [['Q','Q'],['W','W'],['E','E'],['R','R'],['T','T'],['Y','Y'],['U','U'],['I','I'],['O','O'],['P','P']],
    [['A','A'],['S','S'],['D','D'],['F','F'],['G','G'],['H','H'],['J','J'],['K','K'],['L','L']],
    [['Z','Z'],['X','X'],['C','C'],['V','V'],['B','B'],['N','N'],['M','M']],
  ];
  var BOTTOM = [['SPACE','SPACE','key space'],['ENTER','ENTER','key wide'],['BACKSPACE','BACKSPACE','key wide'],['TAB','TAB'],['ESC','ESC']];
  var ARROWS = [['←','LEFT'],['↓','DOWN'],['↑','UP'],['→','RIGHT']];
  var MODS = ['CTRL', 'SHIFT', 'ALT', 'GUI'];
  var FNROW = [];
  for (var f = 1; f <= 12; f++) FNROW.push(['F' + f, 'F' + f]);

  function buildKbd() {
    // 功能键
    el.fnRows.innerHTML = '<div class="krow">' + FNROW.map(function (k) {
      return '<div class="key" data-key="' + k[1] + '">' + k[0] + '</div>';
    }).join('') + '</div>';
    // 主区
    var html = '';
    // 修饰键行
    html += '<div class="krow">' + MODS.map(function (m) {
      return '<div class="key mod" data-mod="' + m + '">' + m + '</div>';
    }).join('') + ARROWS.map(function (k) {
      return '<div class="key" data-key="' + k[1] + '">' + k[0] + '</div>';
    }).join('') + '</div>';
    ROWS.forEach(function (row) {
      html += '<div class="krow">' + row.map(function (k) {
        return '<div class="key" data-key="' + k[1] + '">' + k[0] + '</div>';
      }).join('') + '</div>';
    });
    html += '<div class="krow">' + BOTTOM.map(function (k) {
      return '<div class="' + k[2] + '" data-key="' + k[1] + '">' + k[0] + '</div>';
    }).join('') + '</div>';
    el.vkbd.innerHTML = html;

    // 事件委托
    el.vkbd.addEventListener('pointerdown', function (e) {
      var t = e.target.closest ? e.target.closest('.key') : null;
      if (!t) return;
      if (t.dataset.mod) {
        state.mods[t.dataset.mod] = !state.mods[t.dataset.mod];
        t.classList.toggle('lit', state.mods[t.dataset.mod]);
        return;
      }
      if (t.dataset.key) {
        t.classList.add('down');
        pressKey(t.dataset.key, t);
      }
    });
    el.vkbd.addEventListener('pointerup', function (e) {
      var t = e.target.closest ? e.target.closest('.key') : null;
      if (t) t.classList.remove('down');
    });
    el.vkbd.addEventListener('pointerleave', function (e) {
      if (e.target.classList) e.target.classList.remove('down');
    });
    el.fnRows.addEventListener('pointerdown', function (e) {
      var t = e.target.closest ? e.target.closest('.key') : null;
      if (t && t.dataset.key) pressKey(t.dataset.key, t);
    });
  }

  // 发送一个键：有锁定修饰 → hotkey；否则 tap
  function pressKey(key, keyEl) {
    var mods = activeMods(state.mods);
    var payload = mods.length ? { keys: mods.concat([key]), hold_ms: 40 } : { key: key, hold_ms: 40 };
    var action = mods.length ? 'hotkey' : 'tap';
    send('keyboard', action, payload, function (ok) {
      if (ok && keyEl) {
        keyEl.classList.add('hit');
        setTimeout(function () { keyEl.classList.remove('hit'); }, 220);
      }
    });
  }
  function activeMods(map) {
    return MODS.filter(function (m) { return map[m]; });
  }

  el.fnToggle.addEventListener('click', function () {
    var show = el.fnRows.classList.toggle('show');
    el.fnToggle.textContent = show ? '▾ 功能键 F1–F12' : '▸ 功能键 F1–F12';
  });

  // ---------- 实体键盘直通 ----------

  // KeyboardEvent.code → HID 键名（固件 hid_keymap 支持集）
  var CODE_MAP = {
    Escape: 'ESC', Enter: 'ENTER', NumpadEnter: 'ENTER', Backspace: 'BACKSPACE', Tab: 'TAB',
    Space: 'SPACE', CapsLock: 'CAPSLOCK',
    ArrowLeft: 'LEFT', ArrowRight: 'RIGHT', ArrowUp: 'UP', ArrowDown: 'DOWN',
    Insert: 'INSERT', Home: 'HOME', PageUp: 'PAGEUP', Delete: 'DELETE', End: 'END', PageDown: 'PAGEDOWN',
    ControlLeft: 'CTRL', ControlRight: 'CTRL',
    ShiftLeft: 'SHIFT', ShiftRight: 'SHIFT',
    AltLeft: 'ALT', AltRight: 'ALT',
    MetaLeft: 'GUI', MetaRight: 'GUI',
  };
  var CODE_MODS = { ControlLeft:1, ControlRight:1, ShiftLeft:1, ShiftRight:1, AltLeft:1, AltRight:1, MetaLeft:1, MetaRight:1 };

  function codeToHid(code) {
    if (CODE_MAP[code]) return CODE_MAP[code];
    var m;
    if ((m = /^Key([A-Z])$/.exec(code))) return m[1];
    if ((m = /^Digit([0-9])$/.exec(code))) return 'DIGIT' + m[1];
    if ((m = /^F([1-9]|1[0-2])$/.exec(code))) return 'F' + m[1];
    return null;
  }

  el.ptArea.addEventListener('keydown', function (e) {
    e.preventDefault();
    if (e.code === 'Escape') { el.ptArea.blur(); return; }
    var hid = codeToHid(e.code);
    if (!hid) { setLast('不支持：' + e.code, true); return; }
    if (CODE_MODS[e.code]) { state.ptMods[hid] = true; return; } // 修饰只跟踪
    if (e.repeat) return;
    var mods = activeMods(state.ptMods);
    if (mods.length) {
      send('keyboard', 'hotkey', { keys: mods.concat([hid]), hold_ms: 40 }); // hotkey 原子完成，无需 key_up
    } else {
      send('keyboard', 'key_down', { key: hid, lease_ms: 2000 });
    }
  });
  el.ptArea.addEventListener('keyup', function (e) {
    var hid = codeToHid(e.code);
    if (!hid) return;
    if (CODE_MODS[e.code]) { delete state.ptMods[hid]; return; }
    var mods = activeMods(state.ptMods);
    if (!mods.length) send('keyboard', 'key_up', { key: hid }); // 无修饰才需显式抬起
  });
  el.ptArea.addEventListener('blur', clearPtMods);
  function clearPtMods() { state.ptMods = {}; }

  // ---------- 触控板 ----------

  var MOVE_HZ = 16;                    // move 命令合流上限（次/秒）
  var MOVE_INTERVAL = 1000 / MOVE_HZ;
  var padAccX = 0, padAccY = 0, padTimer = null, padDown = false, lastPos = null;

  function sensFactor() { return parseInt(el.sens.value, 10) / 10; }

  el.pad.addEventListener('pointerdown', function (e) {
    e.preventDefault();
    el.pad.setPointerCapture(e.pointerId);
    padDown = true;
    lastPos = { x: e.clientX, y: e.clientY };
    el.pad.classList.add('active');
    el.padDot.hidden = false;
    moveDot(e);
  });
  el.pad.addEventListener('pointermove', function (e) {
    if (!padDown) return;
    moveDot(e);
    padAccX += (e.clientX - lastPos.x) * sensFactor();
    padAccY += (e.clientY - lastPos.y) * sensFactor();
    lastPos = { x: e.clientX, y: e.clientY };
    if (!padTimer) {
      padTimer = setTimeout(flushMove, MOVE_INTERVAL);
    }
  });
  function endPad(e) {
    if (!padDown) return;
    padDown = false;
    el.pad.classList.remove('active');
    el.padDot.hidden = true;
    if (e && e.type === 'pointerclick') return;
    clearTimeout(padTimer); padTimer = null;
    flushMove();
  }
  el.pad.addEventListener('pointerup', endPad);
  el.pad.addEventListener('pointercancel', endPad);

  function flushMove() {
    padTimer = null;
    if (Math.abs(padAccX) < 1 && Math.abs(padAccY) < 1) { padAccX = 0; padAccY = 0; return; }
    var dx = Math.round(padAccX), dy = Math.round(padAccY);
    padAccX -= dx; padAccY -= dy;
    send('mouse', 'move', { dx: dx, dy: dy }); // 大增量固件自动按 ±127 分包
  }
  function moveDot(e) {
    var r = el.pad.getBoundingClientRect();
    el.padDot.style.left = (e.clientX - r.left) + 'px';
    el.padDot.style.top = (e.clientY - r.top) + 'px';
  }

  // 滚轮 → 目标机滚轮（节流 80ms，聚合方向）
  var wheelTimer = null, wheelAcc = 0;
  el.pad.addEventListener('wheel', function (e) {
    e.preventDefault();
    wheelAcc += e.deltaY;
    if (!wheelTimer) {
      wheelTimer = setTimeout(function () {
        wheelTimer = null;
        var d = Math.round(wheelAcc / 100); // 归一化为步数
        wheelAcc = 0;
        if (d) send('mouse', 'wheel', { delta: d });
      }, 80);
    }
  }, { passive: false });

  // 按钮选择 + 单击/双击
  el.btnSel.addEventListener('click', function (e) {
    var b = e.target.dataset && e.target.dataset.b;
    if (!b) return;
    state.button = b;
    var btns = el.btnSel.querySelectorAll('button');
    for (var i = 0; i < btns.length; i++) btns[i].className = btns[i].dataset.b === b ? 'on' : '';
  });
  el.btnClick.addEventListener('click', function () {
    send('mouse', 'click', { button: state.button, count: 1 });
  });
  el.btnDbl.addEventListener('click', function () {
    send('mouse', 'click', { button: state.button, count: 2 });
  });

  // ---------- 文本连打 ----------

  el.btnBlast.addEventListener('click', function () {
    if (state.blastRunning) { state.blastRunning = false; el.btnBlast.textContent = '▶ 发送到目标电脑'; el.blastState.textContent = '已停止'; return; }
    var text = el.blastText.value;
    if (!text) { el.blastState.textContent = '请输入内容'; return; }
    var queue = [];
    var skipped = 0;
    for (var i = 0; i < text.length; i++) {
      var ch = text.charAt(i);
      var cmd = null;
      if (ch >= 'a' && ch <= 'z') cmd = { action: 'tap', payload: { key: ch.toUpperCase(), hold_ms: 40 } };
      else if (ch >= 'A' && ch <= 'Z') cmd = { action: 'hotkey', payload: { keys: ['SHIFT', ch], hold_ms: 40 } };
      else if (ch >= '0' && ch <= '9') cmd = { action: 'tap', payload: { key: 'DIGIT' + ch, hold_ms: 40 } };
      else if (ch === ' ') cmd = { action: 'tap', payload: { key: 'SPACE', hold_ms: 40 } };
      else if (ch === '\n') cmd = { action: 'tap', payload: { key: 'ENTER', hold_ms: 40 } };
      else { skipped++; continue; }
      queue.push(cmd);
    }
    if (!queue.length) { el.blastState.textContent = '没有可发送的字符（仅支持字母/数字/空格/换行）'; return; }
    state.blastRunning = true;
    el.btnBlast.textContent = '⏹ 停止';
    var total = queue.length, idx = 0;
    (function step() {
      if (!state.blastRunning) return;
      if (idx >= total) {
        state.blastRunning = false;
        el.btnBlast.textContent = '▶ 发送到目标电脑';
        el.blastState.textContent = '完成：' + total + ' 个字符' + (skipped ? '，跳过 ' + skipped + ' 个不支持的字符' : '');
        return;
      }
      var c = queue[idx++];
      el.blastState.textContent = '发送中 ' + idx + '/' + total;
      send('keyboard', c.action, c.payload);
      setTimeout(step, 80);
    })();
  });

  // ---------- WebSocket 实时事件通道（不可用时静默降级为纯 HTTP） ----------

  var wsDot = $('ws-dot');

  function connectWS() {
    if (!state.apiKey || ws) return;
    var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    try {
      ws = new WebSocket(proto + '//' + location.host + '/api/v1/realtime?key=' + encodeURIComponent(state.apiKey));
    } catch (e) { ws = null; return; }
    ws.onopen = function () {
      wsFails = 0;
      wsDot.className = 'ws-on';
      wsDot.textContent = '⚡ 实时通道 已连接';
      $('events-card').hidden = false;
    };
    ws.onmessage = function (ev) {
      var msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      handleEvent(msg);
    };
    ws.onclose = function () {
      ws = null;
      wsDot.className = 'ws-off';
      wsDot.textContent = '⚡ 实时通道（HTTP 模式）';
      wsFails++;
      if (wsFails >= 5) return; // 连续失败（如旧 key）不再重连；重新点「连接」会重置
      setTimeout(function () { if (state.apiKey) connectWS(); }, 3000);
    };
    ws.onerror = function () { try { ws.close(); } catch (e) {} };
  }

  function handleEvent(msg) {
    var d = msg.data || {};
    var line = '';
    if (msg.type === 'hello') line = 'hello · 服务端已连接';
    else if (msg.type === 'device') line = 'device · ' + d.device_id + (d.online ? ' 上线' : ' 离线') + (d.usb_hid_ready ? ' · HID 就绪' : '');
    else if (msg.type === 'ack') line = 'ack · ' + d.request_id + ' → ' + d.status + (d.execution_ms ? ' · ' + d.execution_ms + 'ms' : '');
    else line = msg.type;
    pushEvent(line);
    if (msg.type === 'device' && state.device && d.device_id === state.device.device_id) {
      el.devState.textContent = d.online && d.usb_hid_ready
        ? '● 已连接 ' + d.device_id
        : '○ 设备未就绪，命令会被拒绝';
    }
  }

  function pushEvent(line) {
    var t = new Date();
    var hh = ('0' + t.getHours()).slice(-2) + ':' + ('0' + t.getMinutes()).slice(-2) + ':' + ('0' + t.getSeconds()).slice(-2);
    wsEvents.unshift(hh + '  ' + line);
    if (wsEvents.length > 8) wsEvents.length = 8;
    $('events').innerHTML = wsEvents.map(function (e) { return '<div>' + e + '</div>'; }).join('');
  }

  // ---------- 启动 ----------

  buildKbd();
  if (state.apiKey) { refreshDevices(); connectWS(); }
  updateStats();
})();
