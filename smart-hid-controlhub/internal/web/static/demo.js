// demo.js — 模拟键鼠演示台（OS-2 建立；OS-3 加实时通道；OS-4 加多设备/广播）
//
// 全部命令经既有 POST /api/v1/devices/{id}/commands → MQTT → ESP32 → USB HID。
// 设计要点：
//   - 多设备：芯片条 1-click 切换主控；🎯 广播模式把每次操作发给所有就绪设备（各独立 request_id）
//   - 合流与限流：触控板 move 增量聚合、≤16 次/秒发送；在途上限 8×目标数，超出丢弃
//   - 修饰键组合：可视化键盘锁定式；直通模式跟踪式（有修饰发 hotkey，无修饰发 key_down/key_up 对）
//   - lease 兜底：key_down/button_down 均带 lease_ms，断连/丢 key_up 时设备侧自动释放
(function () {
  'use strict';

  var $ = function (id) { return document.getElementById(id); };
  var el = {
    apiKey: $('api-key'), btnKey: $('btn-key'), deviceChips: $('device-chips'), devState: $('dev-state'),
    btnBroadcast: $('btn-broadcast'), btnRelease: $('btn-release'),
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
    devices: [],           // 全部已配对设备 [{device_id,boot_id,online,usb_hid_ready,firmware}]
    device: null,          // 主控设备（芯片条选中）
    broadcast: false,      // 🎯 广播模式：命令发往所有就绪设备
    sent: 0, ok: 0, bad: 0,
    inFlight: 0,
    mods: {},              // 可视化键盘修饰键锁定 {CTRL:true,...}
    ptMods: {},            // 直通模式修饰键按下状态
    button: localStorage.getItem('smarthid_demo_button') || 'left',
    blastRunning: false,
  };
  var MAX_INFLIGHT_PER_TARGET = 8;

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

  // ---------- 目标解析 / 命令分发 ----------

  // 本次操作的目标设备列表：广播=所有就绪设备；否则=[主控]
  function targets() {
    if (state.broadcast) {
      return state.devices.filter(function (d) { return d.online && d.usb_hid_ready; });
    }
    return state.device ? [state.device] : [];
  }

  // 发一条 HID 命令到指定设备（统计/在途限流/键帽回联动效钩子）
  function sendTo(dev, type, action, payload, onAck) {
    if (!dev) { setLast('未选择设备', true); return; }
    var cap = Math.max(MAX_INFLIGHT_PER_TARGET, targets().length * MAX_INFLIGHT_PER_TARGET);
    if (state.inFlight >= cap) { return; } // 限流：丢弃，不排队
    var cmd = {
      protocol: '1.0',
      request_id: genRequestId(),
      device_id: dev.device_id,
      target_boot_id: dev.boot_id,
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

  // 分发到当前目标集（广播=N 台各一条；单发=主控一条）。onAck 仅在首个成功时触发（键帽闪亮）。
  function dispatch(type, action, payload, onAck) {
    var ts = targets();
    if (!ts.length) { setLast(state.broadcast ? '无就绪设备可广播' : '未选择设备', true); return; }
    var fired = false;
    ts.forEach(function (d) {
      sendTo(d, type, action, payload, function (ok, json) {
        if (ok && !fired) { fired = true; if (onAck) onAck(ok, json); }
      });
    });
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

  // ---------- 连接区 / 设备芯片条 ----------

  el.apiKey.value = state.apiKey;
  el.btnKey.addEventListener('click', function () {
    state.apiKey = el.apiKey.value.trim();
    localStorage.setItem('smarthid_apikey', state.apiKey);
    refreshDevices();
    // 主动断开旧连接：先摘 onclose（避免触发重连计时）再同步置空（close 是异步的，
    // 否则 connectWS 的 if (ws) return 会把新连接拦掉）
    if (ws) { try { ws.onclose = null; ws.close(); } catch (e) {} ws = null; }
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
      state.devices = devs;
      if (state.device) { // 保留主控选中（刷新 boot_id/状态）
        for (var i = 0; i < devs.length; i++) {
          if (devs[i].device_id === state.device.device_id) state.device = devs[i];
        }
      }
      if (!state.device) { // 默认选第一台就绪设备
        for (var j = 0; j < devs.length; j++) {
          if (devs[j].online && devs[j].usb_hid_ready) { state.device = devs[j]; break; }
        }
      }
      renderChips();
      updateDevStateText();
      updateBroadcastBtn();
      el.devState.textContent = devs.length ? '' : '暂无设备（先在控制台完成配对）';
      if (devs.length) updateDevStateText(); // 有设备时 devState 显示主控/广播态而非空
    });
  }

  function renderChips() {
    if (!state.devices.length) { el.deviceChips.innerHTML = '<span class="muted">暂无设备</span>'; return; }
    el.deviceChips.innerHTML = state.devices.map(function (d) {
      var ready = d.online && d.usb_hid_ready;
      var active = state.device && d.device_id === state.device.device_id;
      return '<button class="chip' + (active ? ' active' : '') + (ready ? '' : ' dim') + '" data-id="' + d.device_id + '"' +
        (ready ? '' : ' disabled') + ' title="' + d.device_id + (ready ? '' : '（未就绪）') + '">' +
        '<span class="chip-dot' + (ready ? ' on' : '') + '"></span>' + d.device_id + '</button>';
    }).join('');
  }

  el.deviceChips.addEventListener('click', function (e) {
    var chip = e.target.closest ? e.target.closest('.chip') : null;
    if (!chip || chip.disabled) return;
    var id = chip.dataset.id;
    state.devices.forEach(function (d) {
      if (d.device_id === id && d.online && d.usb_hid_ready) {
        state.device = d;
        renderChips();
        updateDevStateText();
      }
    });
  });

  function updateDevStateText() {
    if (state.broadcast) {
      var n = targets().length;
      el.devState.textContent = n ? '🎯 广播 → ' + n + ' 台设备' : '🎯 广播（无就绪设备）';
    } else if (state.device) {
      el.devState.textContent = state.device.online && state.device.usb_hid_ready
        ? '● 已连接 ' + state.device.device_id
        : '○ 设备未就绪，命令会被拒绝';
    } else {
      el.devState.textContent = '';
    }
  }

  // 广播开关
  el.btnBroadcast.addEventListener('click', function () {
    state.broadcast = !state.broadcast;
    updateBroadcastBtn();
    updateDevStateText();
  });
  function updateBroadcastBtn() {
    var n = targets().length;
    el.btnBroadcast.textContent = state.broadcast ? '🎯 广播：开' + (n ? '（' + n + ' 台）' : '') : '🎯 广播：关';
    el.btnBroadcast.className = state.broadcast ? 'on' : '';
  }

  el.btnRelease.addEventListener('click', function () {
    dispatch('system', 'release_all', {});
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
    el.fnRows.innerHTML = '<div class="krow">' + FNROW.map(function (k) {
      return '<div class="key" data-key="' + k[1] + '">' + k[0] + '</div>';
    }).join('') + '</div>';
    var html = '';
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

  function pressKey(key, keyEl) {
    var mods = activeMods(state.mods);
    var payload = mods.length ? { keys: mods.concat([key]), hold_ms: 40 } : { key: key, hold_ms: 40 };
    var action = mods.length ? 'hotkey' : 'tap';
    dispatch('keyboard', action, payload, function (ok) {
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
    if (CODE_MODS[e.code]) { state.ptMods[hid] = true; return; }
    if (e.repeat) return;
    var mods = activeMods(state.ptMods);
    if (mods.length) {
      dispatch('keyboard', 'hotkey', { keys: mods.concat([hid]), hold_ms: 40 });
    } else {
      dispatch('keyboard', 'key_down', { key: hid, lease_ms: 2000 });
    }
  });
  el.ptArea.addEventListener('keyup', function (e) {
    var hid = codeToHid(e.code);
    if (!hid) return;
    if (CODE_MODS[e.code]) { delete state.ptMods[hid]; return; }
    var mods = activeMods(state.ptMods);
    if (!mods.length) dispatch('keyboard', 'key_up', { key: hid });
  });
  el.ptArea.addEventListener('blur', clearPtMods);
  function clearPtMods() { state.ptMods = {}; }

  // ---------- 触控板 ----------

  var MOVE_HZ = 16;
  var MOVE_INTERVAL = 1000 / MOVE_HZ;
  var padAccX = 0, padAccY = 0, padTimer = null, padDown = false, lastPos = null;

  function sensFactor() { return parseInt(el.sens.value, 10) / 10; }

  el.sens.value = localStorage.getItem('smarthid_demo_sens') || el.sens.value;
  el.sens.addEventListener('change', function () {
    localStorage.setItem('smarthid_demo_sens', el.sens.value);
  });

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
  function endPad() {
    if (!padDown) return;
    padDown = false;
    el.pad.classList.remove('active');
    el.padDot.hidden = true;
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
    dispatch('mouse', 'move', { dx: dx, dy: dy }); // 大增量固件自动按 ±127 分包
  }
  function moveDot(e) {
    var r = el.pad.getBoundingClientRect();
    el.padDot.style.left = (e.clientX - r.left) + 'px';
    el.padDot.style.top = (e.clientY - r.top) + 'px';
  }

  var wheelTimer = null, wheelAcc = 0;
  el.pad.addEventListener('wheel', function (e) {
    e.preventDefault();
    wheelAcc += e.deltaY;
    if (!wheelTimer) {
      wheelTimer = setTimeout(function () {
        wheelTimer = null;
        var d = Math.round(wheelAcc / 100);
        wheelAcc = 0;
        if (d) dispatch('mouse', 'wheel', { delta: d });
      }, 80);
    }
  }, { passive: false });

  el.btnSel.addEventListener('click', function (e) {
    var b = e.target.dataset && e.target.dataset.b;
    if (!b) return;
    state.button = b;
    localStorage.setItem('smarthid_demo_button', b);
    var btns = el.btnSel.querySelectorAll('button');
    for (var i = 0; i < btns.length; i++) btns[i].className = btns[i].dataset.b === b ? 'on' : '';
  });
  // 初始化记忆的按钮选择
  (function () {
    var btns = el.btnSel.querySelectorAll('button');
    for (var i = 0; i < btns.length; i++) btns[i].className = btns[i].dataset.b === state.button ? 'on' : '';
  })();
  el.btnClick.addEventListener('click', function () {
    dispatch('mouse', 'click', { button: state.button, count: 1 });
  });
  el.btnDbl.addEventListener('click', function () {
    dispatch('mouse', 'click', { button: state.button, count: 2 });
  });

  // ---------- 文本连打 ----------

  el.btnBlast.addEventListener('click', function () {
    if (state.blastRunning) { state.blastRunning = false; el.btnBlast.textContent = '▶ 发送到目标电脑'; el.blastState.textContent = '已停止'; return; }
    var text = el.blastText.value;
    if (!text) { el.blastState.textContent = '请输入内容'; return; }
    if (!targets().length) { el.blastState.textContent = state.broadcast ? '无就绪设备可广播' : '请先选择设备'; return; }
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
    var total = queue.length, idx = 0, perTick = targets().length;
    (function step() {
      if (!state.blastRunning) return;
      if (idx >= total) {
        state.blastRunning = false;
        el.btnBlast.textContent = '▶ 发送到目标电脑';
        el.blastState.textContent = '完成：' + total + ' 个字符' + (perTick > 1 ? ' × ' + perTick + ' 台' : '') + (skipped ? '，跳过 ' + skipped + ' 个不支持的字符' : '');
        return;
      }
      var c = queue[idx++];
      el.blastState.textContent = '发送中 ' + idx + '/' + total + (perTick > 1 ? ' ×' + perTick + '台' : '');
      dispatch('keyboard', c.action, c.payload);
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
    else if (msg.type === 'device') {
      line = 'device · ' + d.device_id + (d.online ? ' 上线' : ' 离线') + (d.usb_hid_ready ? ' · HID 就绪' : '');
      upsertDevice(d); // 芯片条实时更新
    }
    else if (msg.type === 'ack') line = 'ack · ' + d.device_id + ' · ' + d.request_id + ' → ' + d.status + (d.execution_ms ? ' · ' + d.execution_ms + 'ms' : '');
    else line = msg.type;
    pushEvent(line);
  }

  // WS device 事件 → 更新本地设备列表与芯片条（含主控设备状态文案）
  function upsertDevice(d) {
    var found = false;
    state.devices.forEach(function (dev, i) {
      if (dev.device_id === d.device_id) { state.devices[i] = d; found = true; }
    });
    if (!found) state.devices.push(d);
    if (state.device && d.device_id === state.device.device_id) state.device = d;
    renderChips();
    updateDevStateText();
    updateBroadcastBtn();
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
  updateBroadcastBtn();
  if (state.apiKey) { refreshDevices(); connectWS(); }
  updateStats();
})();
