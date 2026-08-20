// 需求与反馈列表 + 状态分诊（FB-1）。
// 匿名提交的内容经此处渲染——esc() 转义是硬要求（本系统首个公开写、后台渲染的输入）。
import { api } from '../api.js';
import { toast, fmtTime } from '../../portal/ui.js';

const STATUS = {
  new:     { label: '待处理', badge: 'badge-warn' },
  planned: { label: '已计划', badge: 'badge-info' },
  shipped: { label: '已发布', badge: 'badge-ok' },
  rejected:{ label: '已拒绝', badge: 'badge-bad' },
};
const CATEGORY = { feature: '功能需求', bug: 'Bug', other: '其他' };
const FILTERS = [
  { value: '',        label: '全部' },
  { value: 'new',     label: '待处理' },
  { value: 'planned', label: '已计划' },
  { value: 'shipped', label: '已发布' },
  { value: 'rejected',label: '已拒绝' },
];

// esc HTML 转义：title/body/contact 均为匿名用户输入，插入模板前必须过这里。
function esc(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

// 当前状态筛选（视图模块级保持，切页重置）
let filter = '';

export async function feedbackView() {
  const root = document.createElement('div');
  root.innerHTML = '<div class="loading-state">加载中…</div>';
  render(root).catch((e) => {
    root.innerHTML = `<div class="empty-state"><h3>加载失败</h3><p class="muted">${esc(e.message)}</p></div>`;
  });
  return root;
}

async function render(root) {
  const { items } = await api.feedbackList(filter);
  const list = items || [];
  root.innerHTML = `
    <div class="page-header">
      <h1 class="page-title">反馈${filter ? '' : '（全部）'}（${list.length}）</h1>
      <p class="page-sub">匿名提交的需求与 Bug。标记「已计划 / 已发布」后会出现在公开路线图（docs/roadmap）；拒绝时建议写明原因。</p>
    </div>
    <div style="display:flex;gap:8px;flex-wrap:wrap;margin-bottom:16px">
      ${FILTERS.map((f) => `<button class="btn btn-sm ${filter === f.value ? 'btn-primary' : 'btn-outline'}" data-filter="${f.value}">${f.label}</button>`).join('')}
    </div>
    <div class="card">
      <table class="table">
        <thead><tr><th>ID</th><th>类目</th><th>标题</th><th>状态</th><th>提交时间</th><th>操作</th></tr></thead>
        <tbody>${
          list.length
            ? list.map((f) => fbRows(f)).join('')
            : `<tr><td colspan="6" class="muted text-center">暂无反馈</td></tr>`
        }</tbody>
      </table>
    </div>
  `;

  // 筛选切换
  root.querySelectorAll('[data-filter]').forEach((btn) => {
    btn.onclick = () => { filter = btn.getAttribute('data-filter'); render(root); };
  });

  // 详情展开 / 收起
  root.querySelectorAll('[data-toggle]').forEach((btn) => {
    btn.onclick = () => {
      const tr = document.getElementById('detail-' + btn.getAttribute('data-toggle'));
      if (tr) tr.hidden = !tr.hidden;
    };
  });

  // 状态流转（备注随行提交）
  root.querySelectorAll('[data-status]').forEach((btn) => {
    btn.onclick = async () => {
      const id = btn.getAttribute('data-id');
      const status = btn.getAttribute('data-status');
      const note = noteOf(id);
      const labels = { new: '待处理', planned: '已计划', shipped: '已发布', rejected: '已拒绝' };
      if (!confirm(`确认标记为「${labels[status]}」？`)) return;
      btn.disabled = true;
      try {
        await api.setFeedbackStatus(id, status, note);
        toast('已标记为' + labels[status], 'success');
        render(root);
      } catch (e) {
        toast(e.message || '操作失败', 'error');
        btn.disabled = false;
      }
    };
  });

  // 仅保存备注（状态不变）
  root.querySelectorAll('[data-savenote]').forEach((btn) => {
    btn.onclick = async () => {
      const id = btn.getAttribute('data-id');
      const cur = btn.getAttribute('data-cur');
      btn.disabled = true;
      try {
        await api.setFeedbackStatus(id, cur, noteOf(id));
        toast('备注已保存', 'success');
      } catch (e) {
        toast(e.message || '保存失败', 'error');
      }
      btn.disabled = false;
    };
  });
}

function noteOf(id) {
  const ta = document.getElementById('note-' + id);
  return ta ? ta.value.trim() : '';
}

// fbRows 主行 + 隐藏详情行。
function fbRows(f) {
  const st = STATUS[f.status] || { label: f.status, badge: 'badge-warn' };
  const cat = CATEGORY[f.category] || f.category;
  const id = esc(f.feedback_id);
  const others = Object.keys(STATUS).filter((k) => k !== f.status);
  const actionLabels = { new: '重开待处理', planned: '标记已计划', shipped: '标记已发布', rejected: '拒绝' };
  return `
    <tr>
      <td class="mono tiny">${id.slice(0, 11)}…</td>
      <td>${esc(cat)}</td>
      <td style="max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap" title="${esc(f.title)}">${esc(f.title)}</td>
      <td><span class="badge ${st.badge}">${st.label}</span></td>
      <td>${fmtTime(f.created_at)}</td>
      <td><button class="btn btn-outline btn-sm" data-toggle="${id}">详情</button></td>
    </tr>
    <tr id="detail-${id}" hidden>
      <td colspan="6" style="background:var(--bg-soft,#f6f7f9);padding:16px">
        <div style="white-space:pre-wrap;font-size:13.5px;line-height:1.6;margin-bottom:10px">${esc(f.body)}</div>
        <p class="tiny muted" style="margin:0 0 12px">
          编号 <span class="mono">${id}</span>
          ${f.contact ? ' · 联系方式 <span class="mono">' + esc(f.contact) + '</span>' : ''}
          · IP <span class="mono">${esc(f.client_ip || '—')}</span>
          · UA <span class="mono">${esc(f.user_agent || '—')}</span>
        </p>
        <textarea id="note-${id}" rows="2" placeholder="admin 备注（planned/shipped 时对外可见，≤500 字）"
          style="width:100%;padding:10px;border:1px solid var(--line);border-radius:var(--radius-sm);font:inherit;box-sizing:border-box">${esc(f.admin_note || '')}</textarea>
        <div class="row-actions" style="margin-top:10px">
          ${others.map((s) => `<button class="btn btn-outline btn-sm" data-id="${id}" data-status="${s}">${actionLabels[s]}</button>`).join('')}
          <button class="btn btn-sm" data-id="${id}" data-cur="${esc(f.status)}" data-savenote="1">保存备注</button>
        </div>
      </td>
    </tr>
  `;
}
